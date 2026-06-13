/*
Copyright 2026 The keda-gpu-scaler Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package env provides unified environment detection and metadata for the
// gpu-metrics CLI. It normalises the differences between Kubernetes, SLURM,
// Flux, and standalone execution into a single Context struct so that JSON
// output is schema-compatible across all environments — enabling side-by-side
// GPU performance comparisons between on-prem HPC clusters and cloud.
package env

import (
	"os"
	"strconv"
	"strings"
)

// Type identifies the orchestration environment in which gpu-metrics is running.
type Type string

const (
	// TypeKubernetes indicates the process is running inside a Kubernetes pod.
	TypeKubernetes Type = "k8s"
	// TypeSLURM indicates the process is running inside a SLURM job allocation.
	TypeSLURM Type = "slurm"
	// TypeFlux indicates the process is running inside a Flux job.
	TypeFlux Type = "flux"
	// TypeStandalone indicates no recognised orchestrator was detected.
	TypeStandalone Type = "standalone"
)

// Context holds normalised environment metadata regardless of the underlying
// orchestrator. All fields map to a single unified JSON schema so that outputs
// from different environments can be compared directly.
type Context struct {
	// Orchestrator is the detected (or forced) environment type.
	Orchestrator Type `json:"orchestrator"`
	// Node is the hostname or node name where this process is running.
	Node string `json:"node,omitempty"`
	// JobID is a scheduler-assigned job identifier, or the pod name in K8s.
	JobID string `json:"job_id,omitempty"`
	// TaskRank is the global task/process rank within the job (0-based).
	TaskRank int `json:"task_rank"`
	// LocalRank is the per-node rank of this task (0-based).
	LocalRank int `json:"local_rank"`
	// GPUs is a comma-separated list of GPU device indices assigned to this task.
	GPUs string `json:"gpus,omitempty"`
	// Namespace is only set for Kubernetes environments (pod namespace).
	Namespace string `json:"namespace,omitempty"`
	// Partition is only set for SLURM environments (queue/partition name).
	Partition string `json:"partition,omitempty"`
}

// Detect auto-detects the current environment by inspecting well-known
// environment variables. Detection order: SLURM → Flux → Kubernetes →
// standalone.
func Detect() Type {
	if _, ok := os.LookupEnv("SLURM_JOB_ID"); ok {
		return TypeSLURM
	}
	if _, ok := os.LookupEnv("FLUX_JOB_ID"); ok {
		return TypeFlux
	}
	if _, ok := os.LookupEnv("KUBERNETES_SERVICE_HOST"); ok {
		return TypeKubernetes
	}
	return TypeStandalone
}

// FromEnv builds a Context by detecting the environment and reading the
// appropriate env vars. Pass envType = TypeStandalone to skip detection and
// return a minimal standalone context.
func FromEnv(envType Type) Context {
	switch envType {
	case TypeSLURM:
		return fromSLURM()
	case TypeFlux:
		return fromFlux()
	case TypeKubernetes:
		return fromKubernetes()
	default:
		return standalone()
	}
}

// --- SLURM -------------------------------------------------------------------

func fromSLURM() Context {
	return Context{
		Orchestrator: TypeSLURM,
		Node:         coalesce(os.Getenv("SLURM_NODENAME"), hostname()),
		JobID:        os.Getenv("SLURM_JOB_ID"),
		TaskRank:     envInt("SLURM_PROCID"),
		LocalRank:    envInt("SLURM_LOCALID"),
		GPUs:         slurmGPUs(),
		Partition:    os.Getenv("SLURM_JOB_PARTITION"),
	}
}

// slurmGPUs resolves the GPU assignment from the SLURM env var priority chain.
func slurmGPUs() string {
	for _, key := range []string{
		"SLURM_STEP_GPUS",
		"SLURM_JOB_GPUS",
		"GPU_DEVICE_ORDINAL",
		"CUDA_VISIBLE_DEVICES",
	} {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return ""
}

// --- Flux --------------------------------------------------------------------

func fromFlux() Context {
	return Context{
		Orchestrator: TypeFlux,
		Node:         coalesce(hostname()),
		JobID:        os.Getenv("FLUX_JOB_ID"),
		TaskRank:     envInt("FLUX_TASK_RANK"),
		LocalRank:    envInt("FLUX_TASK_LOCAL_ID"),
		GPUs:         coalesce(os.Getenv("CUDA_VISIBLE_DEVICES")),
	}
}

// --- Kubernetes --------------------------------------------------------------

// fromKubernetes reads metadata that Kubernetes injects via the Downward API.
// Users must configure their pod spec to expose:
//
//	env:
//	  - name: MY_NODE_NAME
//	    valueFrom: {fieldRef: {fieldPath: spec.nodeName}}
//	  - name: MY_POD_NAME
//	    valueFrom: {fieldRef: {fieldPath: metadata.name}}
//	  - name: MY_POD_NAMESPACE
//	    valueFrom: {fieldRef: {fieldPath: metadata.namespace}}
//	  - name: JOB_COMPLETION_INDEX   # for indexed Jobs
//	    valueFrom: {fieldRef: {fieldPath: metadata.annotations['batch.kubernetes.io/job-completion-index']}}
func fromKubernetes() Context {
	return Context{
		Orchestrator: TypeKubernetes,
		Node:         coalesce(os.Getenv("MY_NODE_NAME"), hostname()),
		JobID:        coalesce(os.Getenv("MY_POD_NAME"), os.Getenv("HOSTNAME")),
		TaskRank:     envInt("JOB_COMPLETION_INDEX"),
		LocalRank:    0,
		GPUs:         coalesce(os.Getenv("NVIDIA_VISIBLE_DEVICES"), os.Getenv("CUDA_VISIBLE_DEVICES")),
		Namespace:    os.Getenv("MY_POD_NAMESPACE"),
	}
}

// --- standalone --------------------------------------------------------------

func standalone() Context {
	return Context{
		Orchestrator: TypeStandalone,
		Node:         hostname(),
	}
}

// --- helpers -----------------------------------------------------------------

func envInt(key string) int {
	v, _ := strconv.Atoi(os.Getenv(key))
	return v
}

func hostname() string {
	h, _ := os.Hostname()
	return h
}

// coalesce returns the first non-empty string from the provided values.
func coalesce(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// VisibleDevices parses the GPUs field into a slice of integer device indices.
// Non-numeric entries (e.g. MIG UUIDs, "NoDevFiles") are silently skipped.
func (c Context) VisibleDevices() []int {
	if c.GPUs == "" {
		return nil
	}
	parts := strings.Split(c.GPUs, ",")
	devs := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if idx, err := strconv.Atoi(p); err == nil {
			devs = append(devs, idx)
		}
	}
	return devs
}

// Header returns column names for CSV/table output.
func (c Context) Header() []string {
	base := []string{"Orchestrator", "Node", "JobID", "TaskRank", "LocalRank", "GPUs"}
	if c.Orchestrator == TypeSLURM {
		return append(base, "Partition")
	}
	if c.Orchestrator == TypeKubernetes {
		return append(base, "Namespace")
	}
	return base
}

// Row returns values matching Header().
func (c Context) Row() []string {
	base := []string{
		string(c.Orchestrator),
		c.Node,
		c.JobID,
		strconv.Itoa(c.TaskRank),
		strconv.Itoa(c.LocalRank),
		c.GPUs,
	}
	if c.Orchestrator == TypeSLURM {
		return append(base, c.Partition)
	}
	if c.Orchestrator == TypeKubernetes {
		return append(base, c.Namespace)
	}
	return base
}
