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

package env_test

import (
	"strconv"
	"testing"

	"github.com/pmady/keda-gpu-scaler/pkg/env"
)

// setEnv sets a map of environment variables and returns a cleanup function
// that restores the original values.
func setEnv(t *testing.T, vars map[string]string) {
	t.Helper()
	for k, v := range vars {
		t.Setenv(k, v)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Detect
// ─────────────────────────────────────────────────────────────────────────────

func TestDetect_SLURM(t *testing.T) {
	setEnv(t, map[string]string{"SLURM_JOB_ID": "99999"})
	if got := env.Detect(); got != env.TypeSLURM {
		t.Fatalf("want %s, got %s", env.TypeSLURM, got)
	}
}

func TestDetect_Flux(t *testing.T) {
	// SLURM absent, Flux present
	setEnv(t, map[string]string{"FLUX_JOB_ID": "abc123"})
	if got := env.Detect(); got != env.TypeFlux {
		t.Fatalf("want %s, got %s", env.TypeFlux, got)
	}
}

func TestDetect_Kubernetes(t *testing.T) {
	// SLURM and Flux absent, K8s present
	setEnv(t, map[string]string{"KUBERNETES_SERVICE_HOST": "10.96.0.1"})
	if got := env.Detect(); got != env.TypeKubernetes {
		t.Fatalf("want %s, got %s", env.TypeKubernetes, got)
	}
}

func TestDetect_Standalone(t *testing.T) {
	// No orchestrator env vars set
	if got := env.Detect(); got != env.TypeStandalone {
		t.Fatalf("want %s, got %s", env.TypeStandalone, got)
	}
}

// SLURM takes precedence over Flux and K8s when all vars are set.
func TestDetect_Precedence_SLURMWins(t *testing.T) {
	setEnv(t, map[string]string{
		"SLURM_JOB_ID":          "1",
		"FLUX_JOB_ID":           "f1",
		"KUBERNETES_SERVICE_HOST": "10.0.0.1",
	})
	if got := env.Detect(); got != env.TypeSLURM {
		t.Fatalf("want %s, got %s", env.TypeSLURM, got)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// FromEnv — SLURM
// ─────────────────────────────────────────────────────────────────────────────

func TestFromEnv_SLURM(t *testing.T) {
	setEnv(t, map[string]string{
		"SLURM_JOB_ID":        "12345",
		"SLURM_NODENAME":      "gpu-node-01",
		"SLURM_PROCID":        "2",
		"SLURM_LOCALID":       "0",
		"SLURM_STEP_GPUS":     "0,1",
		"SLURM_JOB_PARTITION": "gpu-a100",
	})
	ctx := env.FromEnv(env.TypeSLURM)
	if ctx.Orchestrator != env.TypeSLURM {
		t.Errorf("orchestrator: want slurm, got %s", ctx.Orchestrator)
	}
	if ctx.JobID != "12345" {
		t.Errorf("job_id: want 12345, got %s", ctx.JobID)
	}
	if ctx.Node != "gpu-node-01" {
		t.Errorf("node: want gpu-node-01, got %s", ctx.Node)
	}
	if ctx.TaskRank != 2 {
		t.Errorf("task_rank: want 2, got %d", ctx.TaskRank)
	}
	if ctx.GPUs != "0,1" {
		t.Errorf("gpus: want 0,1, got %s", ctx.GPUs)
	}
	if ctx.Partition != "gpu-a100" {
		t.Errorf("partition: want gpu-a100, got %s", ctx.Partition)
	}
}

// SLURM GPU priority: SLURM_STEP_GPUS > SLURM_JOB_GPUS > GPU_DEVICE_ORDINAL > CUDA_VISIBLE_DEVICES
func TestFromEnv_SLURM_GPUPriorityChain(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantGPU string
	}{
		{
			name:    "SLURM_STEP_GPUS wins",
			env:     map[string]string{"SLURM_JOB_ID": "1", "SLURM_STEP_GPUS": "0,1", "SLURM_JOB_GPUS": "2", "CUDA_VISIBLE_DEVICES": "3"},
			wantGPU: "0,1",
		},
		{
			name:    "SLURM_JOB_GPUS second",
			env:     map[string]string{"SLURM_JOB_ID": "1", "SLURM_JOB_GPUS": "2,3", "CUDA_VISIBLE_DEVICES": "0"},
			wantGPU: "2,3",
		},
		{
			name:    "CUDA_VISIBLE_DEVICES fallback",
			env:     map[string]string{"SLURM_JOB_ID": "1", "CUDA_VISIBLE_DEVICES": "1,2"},
			wantGPU: "1,2",
		},
		{
			name:    "no GPU vars",
			env:     map[string]string{"SLURM_JOB_ID": "1"},
			wantGPU: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setEnv(t, tc.env)
			ctx := env.FromEnv(env.TypeSLURM)
			if ctx.GPUs != tc.wantGPU {
				t.Errorf("want %q, got %q", tc.wantGPU, ctx.GPUs)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// FromEnv — Flux
// ─────────────────────────────────────────────────────────────────────────────

func TestFromEnv_Flux(t *testing.T) {
	setEnv(t, map[string]string{
		"FLUX_JOB_ID":          "f99abc",
		"FLUX_TASK_RANK":       "3",
		"FLUX_TASK_LOCAL_ID":   "1",
		"CUDA_VISIBLE_DEVICES": "2,3",
	})
	ctx := env.FromEnv(env.TypeFlux)
	if ctx.Orchestrator != env.TypeFlux {
		t.Errorf("orchestrator: want flux, got %s", ctx.Orchestrator)
	}
	if ctx.JobID != "f99abc" {
		t.Errorf("job_id: want f99abc, got %s", ctx.JobID)
	}
	if ctx.TaskRank != 3 {
		t.Errorf("task_rank: want 3, got %d", ctx.TaskRank)
	}
	if ctx.LocalRank != 1 {
		t.Errorf("local_rank: want 1, got %d", ctx.LocalRank)
	}
	if ctx.GPUs != "2,3" {
		t.Errorf("gpus: want 2,3, got %s", ctx.GPUs)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// FromEnv — Kubernetes
// ─────────────────────────────────────────────────────────────────────────────

func TestFromEnv_Kubernetes(t *testing.T) {
	setEnv(t, map[string]string{
		"KUBERNETES_SERVICE_HOST": "10.96.0.1",
		"MY_NODE_NAME":            "gpu-node-02",
		"MY_POD_NAME":             "vllm-inference-0",
		"MY_POD_NAMESPACE":        "inference",
		"JOB_COMPLETION_INDEX":    "4",
		"NVIDIA_VISIBLE_DEVICES":  "0,1",
	})
	ctx := env.FromEnv(env.TypeKubernetes)
	if ctx.Orchestrator != env.TypeKubernetes {
		t.Errorf("orchestrator: want k8s, got %s", ctx.Orchestrator)
	}
	if ctx.Node != "gpu-node-02" {
		t.Errorf("node: want gpu-node-02, got %s", ctx.Node)
	}
	if ctx.JobID != "vllm-inference-0" {
		t.Errorf("job_id: want vllm-inference-0, got %s", ctx.JobID)
	}
	if ctx.TaskRank != 4 {
		t.Errorf("task_rank: want 4, got %d", ctx.TaskRank)
	}
	if ctx.Namespace != "inference" {
		t.Errorf("namespace: want inference, got %s", ctx.Namespace)
	}
	if ctx.GPUs != "0,1" {
		t.Errorf("gpus: want 0,1, got %s", ctx.GPUs)
	}
}

// NVIDIA_VISIBLE_DEVICES takes precedence over CUDA_VISIBLE_DEVICES in K8s.
func TestFromEnv_Kubernetes_GPUPriority(t *testing.T) {
	setEnv(t, map[string]string{
		"KUBERNETES_SERVICE_HOST": "10.96.0.1",
		"NVIDIA_VISIBLE_DEVICES":  "0",
		"CUDA_VISIBLE_DEVICES":    "1,2",
	})
	ctx := env.FromEnv(env.TypeKubernetes)
	if ctx.GPUs != "0" {
		t.Errorf("want NVIDIA_VISIBLE_DEVICES=0, got %s", ctx.GPUs)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// FromEnv — standalone
// ─────────────────────────────────────────────────────────────────────────────

func TestFromEnv_Standalone(t *testing.T) {
	ctx := env.FromEnv(env.TypeStandalone)
	if ctx.Orchestrator != env.TypeStandalone {
		t.Errorf("orchestrator: want standalone, got %s", ctx.Orchestrator)
	}
	if ctx.JobID != "" {
		t.Errorf("job_id should be empty for standalone, got %s", ctx.JobID)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// VisibleDevices
// ─────────────────────────────────────────────────────────────────────────────

func TestVisibleDevices(t *testing.T) {
	tests := []struct {
		gpus string
		want []int
	}{
		{"0,1,2,3", []int{0, 1, 2, 3}},
		{"0", []int{0}},
		{"", nil},
		{"0,,2", []int{0, 2}},          // empty element skipped
		{"0,MIG-abc,2", []int{0, 2}},   // non-numeric skipped
		{" 0 , 1 ", []int{0, 1}},       // whitespace trimmed
	}
	for _, tc := range tests {
		ctx := env.Context{GPUs: tc.gpus}
		got := ctx.VisibleDevices()
		if len(got) != len(tc.want) {
			t.Errorf("gpus=%q: want %v, got %v", tc.gpus, tc.want, got)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("gpus=%q [%d]: want %d, got %d", tc.gpus, i, tc.want[i], got[i])
			}
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Header / Row
// ─────────────────────────────────────────────────────────────────────────────

func TestHeaderRow_SLURM(t *testing.T) {
	ctx := env.Context{
		Orchestrator: env.TypeSLURM,
		Node:         "gpu-node-01",
		JobID:        "12345",
		TaskRank:     2,
		LocalRank:    0,
		GPUs:         "0,1",
		Partition:    "gpu-a100",
	}
	hdr := ctx.Header()
	row := ctx.Row()
	if len(hdr) != len(row) {
		t.Fatalf("header len %d != row len %d", len(hdr), len(row))
	}
	m := make(map[string]string)
	for i, h := range hdr {
		m[h] = row[i]
	}
	if m["Orchestrator"] != "slurm" {
		t.Errorf("Orchestrator: want slurm, got %s", m["Orchestrator"])
	}
	if m["Partition"] != "gpu-a100" {
		t.Errorf("Partition: want gpu-a100, got %s", m["Partition"])
	}
	if m["TaskRank"] != strconv.Itoa(2) {
		t.Errorf("TaskRank: want 2, got %s", m["TaskRank"])
	}
}

func TestHeaderRow_Kubernetes(t *testing.T) {
	ctx := env.Context{
		Orchestrator: env.TypeKubernetes,
		Node:         "gpu-node-01",
		JobID:        "vllm-0",
		Namespace:    "inference",
	}
	hdr := ctx.Header()
	row := ctx.Row()
	m := make(map[string]string)
	for i, h := range hdr {
		m[h] = row[i]
	}
	if m["Namespace"] != "inference" {
		t.Errorf("Namespace: want inference, got %s", m["Namespace"])
	}
}

func TestHeaderRow_Flux(t *testing.T) {
	ctx := env.Context{
		Orchestrator: env.TypeFlux,
		Node:         "gpu-node-01",
		JobID:        "f23r45t",
		TaskRank:     1,
		LocalRank:    0,
		GPUs:         "0",
	}
	hdr := ctx.Header()
	row := ctx.Row()
	if len(hdr) != len(row) {
		t.Fatalf("header len %d != row len %d", len(hdr), len(row))
	}
	// Flux has no Partition or Namespace column
	m := make(map[string]string)
	for i, h := range hdr {
		m[h] = row[i]
	}
	if _, ok := m["Partition"]; ok {
		t.Error("Flux rows should not have a Partition column")
	}
	if _, ok := m["Namespace"]; ok {
		t.Error("Flux rows should not have a Namespace column")
	}
}
