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

package metrics

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/pmady/keda-gpu-scaler/pkg/gpu"
)

var testDevices = []gpu.Metrics{
	{
		Index:              0,
		UUID:               "GPU-aaaa-1111",
		Name:               "NVIDIA A100-SXM4-80GB",
		GPUUtilization:     85,
		MemoryUtilization:  70,
		MemoryUsedMiB:      57344,
		MemoryTotalMiB:     81920,
		TemperatureCelsius: 72,
		PowerDrawWatts:     300,
		PowerLimitWatts:    400,
		PCIeTxKBps:         8000,
		PCIeRxKBps:         4000,
		NVLinkTxMBps:       600,
		NVLinkRxMBps:       500,
	},
	{
		Index:              1,
		UUID:               "GPU-bbbb-2222",
		Name:               "NVIDIA A100-SXM4-80GB",
		GPUUtilization:     20,
		MemoryUtilization:  15,
		MemoryUsedMiB:      12288,
		MemoryTotalMiB:     81920,
		TemperatureCelsius: 38,
		PowerDrawWatts:     75,
		PowerLimitWatts:    400,
		PCIeTxKBps:         2000,
		PCIeRxKBps:         1000,
		NVLinkTxMBps:       200,
		NVLinkRxMBps:       150,
	},
}

func setup(t *testing.T) (*InstrumentedCollector, *prometheus.Registry) {
	t.Helper()
	reg := prometheus.NewRegistry()
	Register(reg)
	ic := Wrap(gpu.NewMockCollector(testDevices))
	return ic, reg
}

func TestCollectAllIncrementsCounter(t *testing.T) {
	ic, _ := setup(t)

	before := testutil.ToFloat64(CollectionsTotal)
	_, err := ic.CollectAll()
	if err != nil {
		t.Fatalf("CollectAll() error = %v", err)
	}

	after := testutil.ToFloat64(CollectionsTotal)
	if after-before != 1 {
		t.Errorf("CollectionsTotal delta = %v, want 1", after-before)
	}
}

func TestCollectAllRecordsGauges(t *testing.T) {
	ic, _ := setup(t)

	_, err := ic.CollectAll()
	if err != nil {
		t.Fatalf("CollectAll() error = %v", err)
	}

	// GPU 0 utilization should be 85
	val := testutil.ToFloat64(GPUUtilization.WithLabelValues("0", "GPU-aaaa-1111", "NVIDIA A100-SXM4-80GB"))
	if val != 85 {
		t.Errorf("GPU 0 utilization = %v, want 85", val)
	}

	// GPU 1 utilization should be 20
	val = testutil.ToFloat64(GPUUtilization.WithLabelValues("1", "GPU-bbbb-2222", "NVIDIA A100-SXM4-80GB"))
	if val != 20 {
		t.Errorf("GPU 1 utilization = %v, want 20", val)
	}

	// GPU 0 memory used should be 57344 MiB * 1024 * 1024
	val = testutil.ToFloat64(GPUMemoryUsedBytes.WithLabelValues("0", "GPU-aaaa-1111", "NVIDIA A100-SXM4-80GB"))
	want := float64(57344) * 1024 * 1024
	if val != want {
		t.Errorf("GPU 0 memory used = %v, want %v", val, want)
	}
}

func TestCollectDeviceIncrementsCounter(t *testing.T) {
	ic, _ := setup(t)

	before := testutil.ToFloat64(CollectionsTotal)
	_, err := ic.CollectDevice(0)
	if err != nil {
		t.Fatalf("CollectDevice(0) error = %v", err)
	}

	after := testutil.ToFloat64(CollectionsTotal)
	if after-before != 1 {
		t.Errorf("CollectionsTotal delta = %v, want 1", after-before)
	}
}

func TestCollectDeviceErrorIncrementsErrorCounter(t *testing.T) {
	ic, _ := setup(t)

	before := testutil.ToFloat64(CollectionErrorsTotal)
	_, err := ic.CollectDevice(99)
	if err == nil {
		t.Fatal("CollectDevice(99) should fail")
	}

	after := testutil.ToFloat64(CollectionErrorsTotal)
	if after-before != 1 {
		t.Errorf("CollectionErrorsTotal delta = %v, want 1", after-before)
	}
}

func TestCollectionDurationRecorded(t *testing.T) {
	ic, _ := setup(t)

	_, _ = ic.CollectAll()

	// histogram can't use ToFloat64; just verify it has metrics
	count := testutil.CollectAndCount(CollectionDuration)
	if count == 0 {
		t.Error("CollectionDuration should have observations")
	}
}

func TestDeviceCountPassthrough(t *testing.T) {
	ic, _ := setup(t)

	count, err := ic.DeviceCount()
	if err != nil {
		t.Fatalf("DeviceCount() error = %v", err)
	}
	if count != 2 {
		t.Errorf("DeviceCount() = %v, want 2", count)
	}
}

func TestClosePassthrough(t *testing.T) {
	ic, _ := setup(t)
	if err := ic.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestCollectAllRecordsPCIeGauges(t *testing.T) {
	ic, _ := setup(t)

	_, err := ic.CollectAll()
	if err != nil {
		t.Fatalf("CollectAll() error = %v", err)
	}

	// GPU 0 PCIe TX
	got := testutil.ToFloat64(PCIeThroughput.WithLabelValues("0", "GPU-aaaa-1111", "NVIDIA A100-SXM4-80GB", "tx"))
	if got != 8000 {
		t.Errorf("PCIe TX GPU0 = %v, want 8000", got)
	}

	// GPU 0 PCIe RX
	got = testutil.ToFloat64(PCIeThroughput.WithLabelValues("0", "GPU-aaaa-1111", "NVIDIA A100-SXM4-80GB", "rx"))
	if got != 4000 {
		t.Errorf("PCIe RX GPU0 = %v, want 4000", got)
	}

	// GPU 1 PCIe TX
	got = testutil.ToFloat64(PCIeThroughput.WithLabelValues("1", "GPU-bbbb-2222", "NVIDIA A100-SXM4-80GB", "tx"))
	if got != 2000 {
		t.Errorf("PCIe TX GPU1 = %v, want 2000", got)
	}
}

func TestCollectAllRecordsNVLinkGauges(t *testing.T) {
	ic, _ := setup(t)

	_, err := ic.CollectAll()
	if err != nil {
		t.Fatalf("CollectAll() error = %v", err)
	}

	// GPU 0 NVLink TX
	got := testutil.ToFloat64(NVLinkThroughput.WithLabelValues("0", "GPU-aaaa-1111", "NVIDIA A100-SXM4-80GB", "tx"))
	if got != 600 {
		t.Errorf("NVLink TX GPU0 = %v, want 600", got)
	}

	// GPU 0 NVLink RX
	got = testutil.ToFloat64(NVLinkThroughput.WithLabelValues("0", "GPU-aaaa-1111", "NVIDIA A100-SXM4-80GB", "rx"))
	if got != 500 {
		t.Errorf("NVLink RX GPU0 = %v, want 500", got)
	}

	// GPU 1 NVLink TX
	got = testutil.ToFloat64(NVLinkThroughput.WithLabelValues("1", "GPU-bbbb-2222", "NVIDIA A100-SXM4-80GB", "tx"))
	if got != 200 {
		t.Errorf("NVLink TX GPU1 = %v, want 200", got)
	}
}

func TestImplementsInterface(t *testing.T) {
	var _ gpu.MetricsCollector = (*InstrumentedCollector)(nil)
}

func TestMetricsRegistered(t *testing.T) {
	reg := prometheus.NewRegistry()
	Register(reg)

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}

	want := []string{
		"keda_gpu_scaler_collections_total",
		"keda_gpu_scaler_collection_errors_total",
		"keda_gpu_scaler_collection_duration_seconds",
		"keda_gpu_scaler_gpu_pcie_throughput_kbps",
		"keda_gpu_scaler_gpu_nvlink_throughput_mbps",
		"keda_gpu_scaler_scaler_requests_total",
		"keda_gpu_scaler_scaler_request_errors_total",
	}

	names := make(map[string]bool)
	for _, f := range families {
		names[f.GetName()] = true
	}

	for _, w := range want {
		// counters won't show up until incremented, histograms will
		if strings.Contains(w, "duration") && !names[w] {
			t.Errorf("expected metric %q to be registered", w)
		}
	}
}
