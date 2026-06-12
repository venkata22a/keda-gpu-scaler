//go:build mock

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

// This file replaces nvml.go when built with -tags mock.
// It provides a zero-dependency NewCollector() backed by MockCollector so
// that gpu-metrics and keda-gpu-scaler can be built and run locally without
// NVIDIA drivers or CGO.
//
// Build:  CGO_ENABLED=0 go build -tags mock ./...
// Test:   go test -tags mock ./...

package gpu

import "go.uber.org/zap"

// NewCollector returns a MockCollector pre-loaded with DefaultMockDevices().
// No NVML initialisation is performed; no NVIDIA driver is required.
func NewCollector(logger *zap.Logger) (*MockCollector, error) {
	logger.Info("Mock GPU collector initialised — using synthetic data (no NVIDIA driver required)")
	return NewMockCollector(DefaultMockDevices()), nil
}
