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

package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/pmady/keda-gpu-scaler/pkg/env"
	"github.com/pmady/keda-gpu-scaler/pkg/gpu"
	"go.uber.org/zap"
)

// schemaVersion is bumped when the JSON output schema changes in a
// backward-incompatible way. Consumers should gate on this field.
const schemaVersion = "v1"

var (
	format   = flag.String("format", "table", "Output format: table, json, csv")
	interval = flag.Duration("interval", 0, "Collection interval (0 = one-shot)")
	device   = flag.Int("device", -1, "GPU device index (-1 = all)")
	quiet    = flag.Bool("quiet", false, "Suppress log output")

	// --env is the unified environment flag (replaces --slurm and --flux).
	// Accepted values: auto, k8s, slurm, flux, standalone.
	envFlag = flag.String("env", "auto", "Environment: auto, k8s, slurm, flux, standalone")

	// Legacy flags kept for backward compatibility; they are aliases for --env.
	// Deprecated: use --env instead.
	slurmMode = flag.String("slurm", "", "Deprecated: use --env=slurm. SLURM mode: auto, on, off")
	fluxMode  = flag.String("flux", "", "Deprecated: use --env=flux. Flux mode: auto, on, off")
)

func main() {
	flag.Parse()

	logger := zap.NewNop()
	if !*quiet {
		l, _ := zap.NewProduction()
		logger = l
	}
	defer func() { _ = logger.Sync() }()

	envCtx := resolveEnv(logger)

	collector, err := gpu.NewCollector(logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "nvml init failed: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = collector.Close() }()

	if !*quiet {
		logger.Info("environment detected",
			zap.String("orchestrator", string(envCtx.Orchestrator)),
			zap.String("node", envCtx.Node),
			zap.String("job_id", envCtx.JobID),
			zap.Int("task_rank", envCtx.TaskRank),
			zap.String("gpus", envCtx.GPUs),
		)
	}

	// one-shot
	if *interval <= 0 {
		metrics, err := collect(collector, envCtx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "collection failed: %v\n", err)
			os.Exit(1)
		}
		output(metrics, *format, envCtx)
		return
	}

	// continuous mode
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(*interval)
	defer ticker.Stop()

	for {
		metrics, err := collect(collector, envCtx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "collection failed: %v\n", err)
		} else {
			output(metrics, *format, envCtx)
		}

		select {
		case <-sigCh:
			return
		case <-ticker.C:
		}
	}
}

// resolveEnv determines the environment context, handling the --env flag and
// the legacy --slurm/--flux aliases with a clear precedence:
//  1. Explicit --env (not "auto") beats everything.
//  2. Legacy --slurm on  → env=slurm; --flux on → env=flux.
//  3. --env=auto (default) → auto-detect via env vars.
func resolveEnv(logger *zap.Logger) env.Context {
	// Legacy flag shims
	if *slurmMode == "on" && *envFlag == "auto" {
		if !*quiet {
			logger.Warn("--slurm is deprecated, use --env=slurm")
		}
		return env.FromEnv(env.TypeSLURM)
	}
	if *fluxMode == "on" && *envFlag == "auto" {
		if !*quiet {
			logger.Warn("--flux is deprecated, use --env=flux")
		}
		return env.FromEnv(env.TypeFlux)
	}
	if *slurmMode == "off" && *envFlag == "auto" {
		return env.FromEnv(env.TypeStandalone)
	}
	if *fluxMode == "off" && *envFlag == "auto" {
		return env.FromEnv(env.TypeStandalone)
	}

	switch *envFlag {
	case "k8s", "kubernetes":
		return env.FromEnv(env.TypeKubernetes)
	case "slurm":
		return env.FromEnv(env.TypeSLURM)
	case "flux":
		return env.FromEnv(env.TypeFlux)
	case "standalone":
		return env.FromEnv(env.TypeStandalone)
	default: // "auto"
		detected := env.Detect()
		return env.FromEnv(detected)
	}
}

// collect gathers metrics for the GPUs that belong to this task.
// Priority: --device flag > scheduler-assigned GPUs > all GPUs.
func collect(c gpu.MetricsCollector, envCtx env.Context) ([]gpu.Metrics, error) {
	if *device >= 0 {
		m, err := c.CollectDevice(*device)
		if err != nil {
			return nil, err
		}
		return []gpu.Metrics{m}, nil
	}

	if devs := envCtx.VisibleDevices(); len(devs) > 0 {
		return collectDevices(c, devs)
	}

	return c.CollectAll()
}

func collectDevices(c gpu.MetricsCollector, devs []int) ([]gpu.Metrics, error) {
	out := make([]gpu.Metrics, 0, len(devs))
	for _, idx := range devs {
		m, err := c.CollectDevice(idx)
		if err != nil {
			return nil, fmt.Errorf("gpu %d: %w", idx, err)
		}
		out = append(out, m)
	}
	return out, nil
}

func output(metrics []gpu.Metrics, format string, envCtx env.Context) {
	switch format {
	case "json":
		outputJSON(metrics, envCtx)
	case "csv":
		outputCSV(metrics, envCtx)
	default:
		outputTable(metrics, envCtx)
	}
}

// JSONOutput is the unified schema emitted by --format json across all
// environments. The schema_version field lets consumers detect breaking
// changes. The environment block is identical regardless of orchestrator —
// only the fields relevant to that orchestrator are populated.
type JSONOutput struct {
	SchemaVersion string        `json:"schema_version"`
	CollectedAt   string        `json:"collected_at"`
	Environment   env.Context   `json:"environment"`
	Devices       []gpu.Metrics `json:"devices"`
}

func outputJSON(metrics []gpu.Metrics, envCtx env.Context) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(JSONOutput{
		SchemaVersion: schemaVersion,
		CollectedAt:   time.Now().UTC().Format(time.RFC3339),
		Environment:   envCtx,
		Devices:       metrics,
	})
}

func outputCSV(metrics []gpu.Metrics, envCtx env.Context) {
	w := csv.NewWriter(os.Stdout)
	hdr := append(envCtx.Header(), csvHeader()...)
	_ = w.Write(hdr)
	for _, m := range metrics {
		row := append(envCtx.Row(), csvRow(m)...)
		_ = w.Write(row)
	}
	w.Flush()
}

func outputTable(metrics []gpu.Metrics, envCtx env.Context) {
	switch envCtx.Orchestrator {
	case env.TypeSLURM:
		fmt.Printf("SLURM  job=%s  node=%s  rank=%d  gpus=[%s]  partition=%s\n\n",
			envCtx.JobID, envCtx.Node, envCtx.TaskRank, envCtx.GPUs, envCtx.Partition)
	case env.TypeFlux:
		fmt.Printf("Flux   job=%s  node=%s  rank=%d  local=%d  gpus=[%s]\n\n",
			envCtx.JobID, envCtx.Node, envCtx.TaskRank, envCtx.LocalRank, envCtx.GPUs)
	case env.TypeKubernetes:
		fmt.Printf("K8s    pod=%s  node=%s  ns=%s  gpus=[%s]\n\n",
			envCtx.JobID, envCtx.Node, envCtx.Namespace, envCtx.GPUs)
	default:
		fmt.Printf("Standalone  node=%s\n\n", envCtx.Node)
	}

	fmt.Printf("%-5s %-20s %6s %6s %10s %10s %6s %6s %10s %10s %10s %10s\n",
		"GPU", "Name", "Util%", "Mem%", "MemUsed", "MemTotal", "Temp", "Power",
		"PCIeTx", "PCIeRx", "NVLTx", "NVLRx")
	fmt.Println("---   ----                 -----  -----  ---------  ---------  -----  -----  ---------  ---------  ---------  ---------")
	for _, m := range metrics {
		fmt.Printf("%-5d %-20s %5d%% %5d%% %7dMiB %7dMiB %4d°C %4dW %7dKB/s %7dKB/s %7dMB/s %7dMB/s\n",
			m.Index, truncate(m.Name, 20),
			m.GPUUtilization, m.MemoryUtilization,
			m.MemoryUsedMiB, m.MemoryTotalMiB,
			m.TemperatureCelsius, m.PowerDrawWatts,
			m.PCIeTxKBps, m.PCIeRxKBps,
			m.NVLinkTxMBps, m.NVLinkRxMBps)
	}
}

func csvHeader() []string {
	return []string{
		"index", "uuid", "name",
		"gpu_util_pct", "mem_util_pct", "mem_used_mib", "mem_total_mib",
		"temp_c", "power_w", "power_limit_w",
		"pcie_tx_kbps", "pcie_rx_kbps",
		"nvlink_tx_mbps", "nvlink_rx_mbps",
	}
}

func csvRow(m gpu.Metrics) []string {
	return []string{
		strconv.Itoa(m.Index), m.UUID, m.Name,
		strconv.FormatUint(uint64(m.GPUUtilization), 10),
		strconv.FormatUint(uint64(m.MemoryUtilization), 10),
		strconv.FormatUint(m.MemoryUsedMiB, 10),
		strconv.FormatUint(m.MemoryTotalMiB, 10),
		strconv.FormatUint(uint64(m.TemperatureCelsius), 10),
		strconv.FormatUint(uint64(m.PowerDrawWatts), 10),
		strconv.FormatUint(uint64(m.PowerLimitWatts), 10),
		strconv.FormatUint(uint64(m.PCIeTxKBps), 10),
		strconv.FormatUint(uint64(m.PCIeRxKBps), 10),
		strconv.FormatUint(m.NVLinkTxMBps, 10),
		strconv.FormatUint(m.NVLinkRxMBps, 10),
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
