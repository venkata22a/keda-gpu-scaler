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

package profiles

// Profile defines a pre-built scaling configuration for a specific workload type.
type Profile struct {
	Name               string
	MetricName         string
	Description        string
	TargetValue        float64
	ActivationValue    float64
	MetricType         MetricType
	CooldownSeconds    int
	ScaleUpStabilize   int
	ScaleDownStabilize int
}

// MetricType represents the GPU metric to use for scaling decisions.
type MetricType string

const (
	MetricGPUUtilization    MetricType = "gpu_utilization"
	MetricMemoryUtilization MetricType = "memory_utilization"
	MetricMemoryUsedMiB     MetricType = "memory_used_mib"
	MetricMemoryUsedPercent MetricType = "memory_used_percent"
	MetricTemperature       MetricType = "temperature"
	MetricPowerDraw         MetricType = "power_draw"
	// PCIe throughput — useful for CPU↔GPU data transfer bottlenecks.
	MetricPCIeTxKBps MetricType = "pcie_tx_kbps"
	MetricPCIeRxKBps MetricType = "pcie_rx_kbps"
	// NVLink throughput — useful for GPU↔GPU communication bottlenecks
	// in data-parallel training on NVSwitch/DGX systems.
	MetricNVLinkTxMBps MetricType = "nvlink_tx_mbps"
	MetricNVLinkRxMBps MetricType = "nvlink_rx_mbps"
)

// Built-in profiles for common AI/ML workloads.
var builtinProfiles = map[string]Profile{
	"distributed-training": {
		Name:               "distributed-training",
		MetricName:         "keda_gpu_distributed_training",
		Description:        "Data-parallel training on NVLink systems — scale on inter-GPU communication bandwidth",
		TargetValue:        800, // MB/s — tune per hardware (A100 NVLink peak ~600 GB/s aggregate)
		ActivationValue:    100,
		MetricType:         MetricNVLinkTxMBps,
		CooldownSeconds:    300,
		ScaleUpStabilize:   60,
		ScaleDownStabilize: 300,
	},
	"vllm-inference": {
		Name:               "vllm-inference",
		MetricName:         "keda_gpu_vllm_inference",
		Description:        "vLLM / LLM serving — memory-based, supports scale-to-zero",
		TargetValue:        80,
		ActivationValue:    5,
		MetricType:         MetricMemoryUsedPercent,
		CooldownSeconds:    60,
		ScaleUpStabilize:   15,
		ScaleDownStabilize: 120,
	},
	"triton-inference": {
		Name:               "triton-inference",
		MetricName:         "keda_gpu_triton_inference",
		Description:        "Triton Inference Server — GPU compute utilization",
		TargetValue:        75,
		ActivationValue:    10,
		MetricType:         MetricGPUUtilization,
		CooldownSeconds:    30,
		ScaleUpStabilize:   10,
		ScaleDownStabilize: 90,
	},
	"training": {
		Name:               "training",
		MetricName:         "keda_gpu_training",
		Description:        "Training jobs — high GPU util target, no scale-to-zero",
		TargetValue:        90,
		ActivationValue:    0,
		MetricType:         MetricGPUUtilization,
		CooldownSeconds:    300,
		ScaleUpStabilize:   60,
		ScaleDownStabilize: 300,
	},
	"batch": {
		Name:               "batch",
		MetricName:         "keda_gpu_batch",
		Description:        "Batch inference — aggressive scale-down and scale-to-zero",
		TargetValue:        70,
		ActivationValue:    1,
		MetricType:         MetricMemoryUsedPercent,
		CooldownSeconds:    30,
		ScaleUpStabilize:   5,
		ScaleDownStabilize: 60,
	},
}

// Get returns a profile by name. Returns the profile and true if found,
// or a zero Profile and false if not.
func Get(name string) (Profile, bool) {
	p, ok := builtinProfiles[name]
	return p, ok
}

// List returns all available profile names.
func List() []string {
	names := make([]string, 0, len(builtinProfiles))
	for name := range builtinProfiles {
		names = append(names, name)
	}
	return names
}
