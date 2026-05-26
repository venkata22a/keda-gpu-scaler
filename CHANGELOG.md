# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- Optional Prometheus metrics endpoint (`--metrics-port=9090`, set to 0 to disable)
- Per-GPU Prometheus gauges: utilization, memory, temperature, power draw
- Scaler operational metrics: collection counters, duration histogram, gRPC request counters
- `InstrumentedCollector` wrapper for transparent metrics collection
- `/healthz` HTTP health check endpoint (when metrics enabled)
- Helm values: `metrics.enabled` and `metrics.port`
- Unit tests for `pkg/metrics` package

## [v0.2.0] - 2026-05-25

### Added

- GPU collector package tests (`pkg/gpu/collector_test.go`) — MockCollector interface compliance, boundary conditions, empty device handling

### Changed

- Dependabot updates: grpc 1.81.1, zap 1.28.0, golangci-lint-action v9, actions/checkout v6, actions/setup-go v6, docker/login-action v4, docker/build-push-action v7

[v0.2.0]: https://github.com/pmady/keda-gpu-scaler/compare/v0.1.0...v0.2.0

## [v0.1.0] - 2026-05-19

### Added

- KEDA External Scaler gRPC server implementing `externalscaler.ExternalScalerServer`
- Direct NVML GPU metrics collection via `go-nvml` C-bindings
- 6 GPU metrics: utilization, memory utilization, memory used (MiB and %), temperature, power draw
- Pre-built scaling profiles: `vllm-inference`, `triton-inference`, `training`, `batch`
- Multi-GPU aggregation: `max`, `min`, `avg`, `sum`
- Scale-to-zero support via KEDA activation thresholds
- Per-GPU index targeting (`gpuIndex` parameter)
- Mock GPU collector for development and testing without hardware
- DaemonSet deployment manifests and Helm chart
- Unit tests for profiles, metric aggregation, and gRPC server
- E2E tests for full gRPC scaling path (no GPU required)
- CI pipeline: build, unit tests, e2e tests, lint, Helm lint, Docker build + push
- OpenSSF Best Practices badge

[v0.1.0]: https://github.com/pmady/keda-gpu-scaler/releases/tag/v0.1.0
