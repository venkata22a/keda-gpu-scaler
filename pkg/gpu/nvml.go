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

package gpu

import (
	"fmt"
	"sync"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"go.uber.org/zap"
)

// maxNVLinks is the maximum number of NVLink connections per GPU (H100 upper bound).
const maxNVLinks = 18

// Metrics holds a snapshot of GPU metrics for a single device.
type Metrics struct {
	Index              int
	UUID               string
	Name               string
	GPUUtilization     uint32 // percentage 0-100
	MemoryUtilization  uint32 // percentage 0-100
	MemoryUsedMiB      uint64
	MemoryTotalMiB     uint64
	TemperatureCelsius uint32
	PowerDrawWatts     uint32
	PowerLimitWatts    uint32
	// PCIe throughput — sampled by NVML over a ~20ms window.
	PCIeTxKBps uint32
	PCIeRxKBps uint32
	// NVLink throughput — aggregate across all active links on this device.
	NVLinkTxMBps uint64
	NVLinkRxMBps uint64
}

// Collector wraps NVML to collect GPU metrics.
type Collector struct {
	logger *zap.Logger
	mu     sync.Mutex
}

// NewCollector creates a new GPU metrics collector.
func NewCollector(logger *zap.Logger) (*Collector, error) {
	ret := nvml.Init()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("failed to initialize NVML: %v", nvml.ErrorString(ret))
	}
	logger.Info("NVML initialized successfully")
	return &Collector{logger: logger}, nil
}

// Close shuts down the NVML library.
func (c *Collector) Close() error {
	ret := nvml.Shutdown()
	if ret != nvml.SUCCESS {
		return fmt.Errorf("failed to shutdown NVML: %v", nvml.ErrorString(ret))
	}
	return nil
}

// DeviceCount returns the number of GPU devices on this node.
func (c *Collector) DeviceCount() (int, error) {
	count, ret := nvml.DeviceGetCount()
	if ret != nvml.SUCCESS {
		return 0, fmt.Errorf("failed to get device count: %v", nvml.ErrorString(ret))
	}
	return count, nil
}

// CollectAll gathers metrics from all GPU devices on this node.
func (c *Collector) CollectAll() ([]Metrics, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	count, err := c.DeviceCount()
	if err != nil {
		return nil, err
	}

	metrics := make([]Metrics, 0, count)
	for i := 0; i < count; i++ {
		m, err := c.collectDevice(i)
		if err != nil {
			c.logger.Warn("failed to collect metrics for device", zap.Int("index", i), zap.Error(err))
			continue
		}
		metrics = append(metrics, m)
	}
	return metrics, nil
}

// CollectDevice gathers metrics from a specific GPU device by index.
func (c *Collector) CollectDevice(index int) (Metrics, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.collectDevice(index)
}

func (c *Collector) collectDevice(index int) (Metrics, error) {
	device, ret := nvml.DeviceGetHandleByIndex(index)
	if ret != nvml.SUCCESS {
		return Metrics{}, fmt.Errorf("failed to get device handle for index %d: %v", index, nvml.ErrorString(ret))
	}

	m := Metrics{Index: index}

	// UUID
	uuid, ret := device.GetUUID()
	if ret == nvml.SUCCESS {
		m.UUID = uuid
	}

	// Name
	name, ret := device.GetName()
	if ret == nvml.SUCCESS {
		m.Name = name
	}

	// Utilization rates
	utilization, ret := device.GetUtilizationRates()
	if ret == nvml.SUCCESS {
		m.GPUUtilization = utilization.Gpu
		m.MemoryUtilization = utilization.Memory
	}

	// Memory info
	memInfo, ret := device.GetMemoryInfo()
	if ret == nvml.SUCCESS {
		m.MemoryUsedMiB = memInfo.Used / (1024 * 1024)
		m.MemoryTotalMiB = memInfo.Total / (1024 * 1024)
	}

	// Temperature
	temp, ret := device.GetTemperature(nvml.TEMPERATURE_GPU)
	if ret == nvml.SUCCESS {
		m.TemperatureCelsius = temp
	}

	// Power
	power, ret := device.GetPowerUsage()
	if ret == nvml.SUCCESS {
		m.PowerDrawWatts = power / 1000 // milliwatts to watts
	}
	powerLimit, ret := device.GetPowerManagementLimit()
	if ret == nvml.SUCCESS {
		m.PowerLimitWatts = powerLimit / 1000
	}

	// PCIe throughput — KB/s over the last ~20ms NVML sampling window.
	if tx, ret := device.GetPcieThroughput(nvml.PCIE_UTIL_TX_BYTES); ret == nvml.SUCCESS {
		m.PCIeTxKBps = tx
	}
	if rx, ret := device.GetPcieThroughput(nvml.PCIE_UTIL_RX_BYTES); ret == nvml.SUCCESS {
		m.PCIeRxKBps = rx
	}

	// NVLink throughput — iterate all possible links and aggregate active ones.
	// On hardware without NVLink (e.g. T4, A10), every link returns an error
	// and is skipped, leaving NVLinkTxMBps/NVLinkRxMBps as 0.
	// A warning is logged so operators know NVLink is unavailable rather than
	// silently getting 0s that could trigger unexpected scale-to-zero.
	var nvlinkTxKBps, nvlinkRxKBps uint64
	activeLinks := 0
	for link := 0; link < maxNVLinks; link++ {
		tx, rx, ret := nvml.DeviceGetNvLinkUtilizationCounter(device, link, 0)
		if ret != nvml.SUCCESS {
			continue
		}
		nvlinkTxKBps += tx
		nvlinkRxKBps += rx
		activeLinks++
	}
	if activeLinks == 0 {
		c.logger.Debug("no NVLink connections found on device — NVLink metrics will be 0 (normal for non-NVLink hardware like T4/A10)",
			zap.Int("gpuIndex", index),
			zap.String("gpu", m.Name),
		)
	}
	m.NVLinkTxMBps = nvlinkTxKBps / 1024
	m.NVLinkRxMBps = nvlinkRxKBps / 1024

	return m, nil
}
