# KEDA GPU Scaler

**Scale Kubernetes GPU workloads from real hardware metrics. No DCGM. No PromQL. Optional Prometheus metrics built in.**

[![CI](https://github.com/pmady/keda-gpu-scaler/actions/workflows/ci.yaml/badge.svg)](https://github.com/pmady/keda-gpu-scaler/actions/workflows/ci.yaml)
[![Go Report Card](https://goreportcard.com/badge/github.com/pmady/keda-gpu-scaler)](https://goreportcard.com/report/github.com/pmady/keda-gpu-scaler)
[![GitHub Stars](https://img.shields.io/github/stars/pmady/keda-gpu-scaler?style=for-the-badge&logo=github)](https://github.com/pmady/keda-gpu-scaler/stargazers)
[![GitHub Forks](https://img.shields.io/github/forks/pmady/keda-gpu-scaler?style=for-the-badge&logo=github)](https://github.com/pmady/keda-gpu-scaler/network/members)
[![Contributors](https://img.shields.io/github/contributors/pmady/keda-gpu-scaler?style=for-the-badge&logo=github)](https://github.com/pmady/keda-gpu-scaler/graphs/contributors)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![OpenSSF Best Practices](https://www.bestpractices.dev/projects/12912/badge)](https://www.bestpractices.dev/en/projects/12912)
[![Documentation](https://readthedocs.org/projects/keda-gpu-scaler/badge/?version=latest)](https://keda-gpu-scaler.readthedocs.io/en/latest/)
![KEDA: v2.10+](https://img.shields.io/badge/KEDA-v2.10%2B-orange)
![Kubernetes: v1.24+](https://img.shields.io/badge/Kubernetes-v1.24%2B-blue)

A [KEDA External Scaler](https://keda.sh/docs/latest/concepts/external-scalers/) that reads NVIDIA GPU metrics directly from NVML C-bindings and autoscales your vLLM, Triton, and custom inference deployments — including scale-to-zero.

### What it does in 30 seconds

```
GPU Node                          KEDA Operator
┌─────────────────────┐           ┌──────────────────┐
│ keda-gpu-scaler     │──gRPC───> │ External Scaler  │
│ (DaemonSet)         │           │ trigger          │
│                     │           └────────┬─────────┘
│ NVML: 92% GPU util  │                    │
│ NVML: 14.2GB VRAM   │           Scale vllm-deployment
│ :9090/metrics (opt) │           from 3 → 8 replicas
└─────────────────────┘
```

---

## Why This Exists

Scaling AI inference on Kubernetes using CPU/Memory HPA is broken. Your GPU nodes sit at 10% CPU while the GPUs are 100% saturated with 200+ pending requests in the vLLM queue.

The standard workaround — dcgm-exporter + Prometheus + KEDA Prometheus scaler — works but adds significant operational overhead:

```
BEFORE: GPU Pod → dcgm-exporter → Prometheus → PromQL → KEDA → HPA
        (5 components, 15-30s scrape delay, PromQL queries break on upgrades)

AFTER:  GPU Pod → keda-gpu-scaler (NVML) → KEDA → HPA
        (2 components, sub-second metrics, zero configuration)
```

**keda-gpu-scaler eliminates the entire metrics pipeline** — it reads GPU state directly from the hardware on each node and serves it to KEDA over gRPC.

### Why Not a Native KEDA Scaler?

Embedding GPU support directly inside KEDA core is architecturally impossible for three reasons:

1. **CGO Constraint**: NVIDIA's Go bindings ([`go-nvml`](https://github.com/NVIDIA/go-nvml)) require `CGO_ENABLED=1`. KEDA builds with `CGO_ENABLED=0`.
2. **Node-Level Hardware Access**: The KEDA operator runs as a central pod. NVML requires local GPU device access via `libnvidia-ml.so`, which only a **DaemonSet on GPU nodes** can provide.
3. **Independent Release Cycle**: Ship GPU scaling improvements without waiting for KEDA release cycles.

This design is documented in [KEDA issue #7538](https://github.com/kedacore/keda/issues/7538).

---

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│  GPU Node (DaemonSet)                                    │
│                                                          │
│   ┌───────────────────┐       ┌────────────────────────┐ │
│   │  keda-gpu-scaler  │◄─────►│ NVIDIA GPU (NVML)      │ │
│   │  gRPC :6000       │       │ libnvidia-ml.so        │ │
│   │                   │       │ A100 / H100 / L40S ... │ │
│   └─────────▲─────────┘       └────────────────────────┘ │
│             │                                            │
└─────────────┼────────────────────────────────────────────┘
              │ gRPC (ExternalScaler protocol)
┌─────────────┼────────────────────────────────────────────┐
│  KEDA       │                                            │
│   ┌─────────▼──────────┐      ┌────────────────────────┐ │
│   │  External Scaler   │─────►│  HPA (scale up/down)   │ │
│   │  trigger           │      │  your-vllm-deployment  │ │
│   └────────────────────┘      └────────────────────────┘ │
└──────────────────────────────────────────────────────────┘
```

1. **DaemonSet** — Runs on nodes labeled with `nvidia.com/gpu.present: "true"`.
2. **NVML Bindings** — Directly reads Streaming Multiprocessor (SM) utilization and Frame Buffer Memory via `go-nvml` C-bindings.
3. **gRPC Interface** — Implements `externalscaler.ExternalScalerServer` (`IsActive`, `StreamIsActive`, `GetMetricSpec`, `GetMetrics`) to natively integrate with the central KEDA operator.
4. **ScaledObject Trigger** — Kubernetes deployments scale up/down (including to zero) based on GPU thresholds defined in the ScaledObject.

---

## GPU Metrics

| Metric | Description | Unit |
|--------|-------------|------|
| `gpu_utilization` | GPU compute (SM) utilization | % (0-100) |
| `memory_utilization` | GPU memory controller utilization | % (0-100) |
| `memory_used_mib` | GPU VRAM used | MiB |
| `memory_used_percent` | GPU VRAM used as percentage of total | % (0-100) |
| `temperature` | GPU die temperature | Celsius |
| `power_draw` | GPU power consumption | Watts |

---

## Pre-built Scaling Profiles

Instead of configuring raw metric thresholds, use a profile optimized for your workload:

| Profile | Primary Metric | Target | Activation | Use Case |
|---------|---------------|--------|------------|----------|
| `vllm-inference` | Memory % | 80 | 5 | vLLM / LLM serving with scale-to-zero |
| `triton-inference` | GPU Util | 75 | 10 | NVIDIA Triton Inference Server |
| `training` | GPU Util | 90 | 0 | Training jobs (no scale-to-zero) |
| `batch` | Memory % | 70 | 1 | Batch inference with aggressive scale-down |

---

## Prerequisites

- A Kubernetes cluster (e.g., **OKE**, GKE, EKS, AKS) with **NVIDIA GPU worker nodes**
- [KEDA v2.10+](https://keda.sh/docs/latest/deploy/) installed in the cluster
- NVIDIA GPU drivers and [Device Plugin](https://github.com/NVIDIA/k8s-device-plugin) installed

---

## Quick Start

### 1. Deploy the Scaler

Deploy the DaemonSet and gRPC service into your cluster. (Ensure KEDA is already installed.)

```bash
kubectl apply -f deploy/manifests.yaml
```

This deploys a DaemonSet that runs on every GPU node in your cluster, plus a ClusterIP Service for KEDA to discover it.

Or use Helm:

```bash
helm install keda-gpu-scaler deploy/helm/keda-gpu-scaler \
  --namespace keda \
  --set nodeSelector."nvidia\.com/gpu\.present"=true
```

### 2. Attach to your AI Workload

Create a ScaledObject pointing to the external scaler service:

```yaml
apiVersion: keda.sh/v1alpha1
kind: ScaledObject
metadata:
  name: vllm-inference-scaler
  namespace: ai-workloads
spec:
  scaleTargetRef:
    name: vllm-deepseek-deployment
  minReplicaCount: 1
  maxReplicaCount: 50
  triggers:
    - type: external
      metadata:
        scalerAddress: "keda-gpu-scaler.keda.svc.cluster.local:6000"
        targetGpuUtilization: "80"
```

Or use a pre-built profile:

```yaml
triggers:
  - type: external
    metadata:
      scalerAddress: "keda-gpu-scaler.keda.svc.cluster.local:6000"
      profile: "vllm-inference"
```

### 3. Custom Configuration

Override any profile default or use raw GPU metrics directly:

```yaml
triggers:
  - type: external
    metadata:
      scalerAddress: "keda-gpu-scaler.keda.svc.cluster.local:6000"
      metricType: "gpu_utilization"
      targetValue: "85"
      activationThreshold: "10"
      gpuIndex: "0"              # specific GPU index, or omit for all
      aggregation: "max"         # max, min, avg, sum across GPUs
```

See `deploy/examples/` for ready-to-use ScaledObject manifests.

---

## Configuration Reference

| Parameter | Description | Default |
|-----------|-------------|---------|
| `profile` | Pre-built scaling profile name | (none) |
| `metricType` | GPU metric to scale on | `gpu_utilization` |
| `targetValue` | Target metric value for scaling | `80` |
| `targetGpuUtilization` | Shorthand for GPU utilization target | (none) |
| `targetMemoryUtilization` | Shorthand for VRAM utilization target | (none) |
| `activationThreshold` | Value below which scale-to-zero activates | `0` |
| `gpuIndex` | Specific GPU index to monitor | `-1` (all GPUs) |
| `aggregation` | Multi-GPU aggregation: `max`, `min`, `avg`, `sum` | `max` |
| `pollIntervalSeconds` | Metric polling interval | `10` |

---

## Prometheus Metrics (Optional)

The scaler exposes an optional Prometheus-compatible `/metrics` endpoint for monitoring the scaler itself and GPU fleet health. **This is independent of the KEDA scaling path** — scaling works identically with or without it.

### Enable/Disable

```bash
# Enabled by default on port 9090
--metrics-port=9090

# Disable entirely (zero overhead)
--metrics-port=0
```

Helm:
```yaml
metrics:
  enabled: true   # set to false to disable
  port: 9090
```

### Exposed Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `keda_gpu_scaler_gpu_utilization_percent` | Gauge | GPU compute utilization (per GPU) |
| `keda_gpu_scaler_gpu_memory_used_bytes` | Gauge | GPU memory in use (per GPU) |
| `keda_gpu_scaler_gpu_memory_total_bytes` | Gauge | Total GPU memory (per GPU) |
| `keda_gpu_scaler_gpu_temperature_celsius` | Gauge | GPU temperature (per GPU) |
| `keda_gpu_scaler_gpu_power_draw_watts` | Gauge | GPU power draw (per GPU) |
| `keda_gpu_scaler_collections_total` | Counter | Total NVML collection calls |
| `keda_gpu_scaler_collection_errors_total` | Counter | Failed NVML collection calls |
| `keda_gpu_scaler_collection_duration_seconds` | Histogram | NVML collection latency |
| `keda_gpu_scaler_scaler_requests_total` | Counter | gRPC requests by method |
| `keda_gpu_scaler_scaler_request_errors_total` | Counter | gRPC errors by method |

All per-GPU metrics are labeled with `gpu_index`, `gpu_uuid`, and `gpu_name`.

## Kubernetes Probes

The scaler exposes liveness and readiness endpoints on a dedicated probe port:

- `/healthz` returns `200` while the process is alive.
- `/readyz` returns `200` after NVML initializes and the first metrics collection succeeds.

```bash
--probe-port=8081
```

Helm:
```yaml
probes:
  enabled: true
  port: 8081
```

---

## Build it Yourself

This project requires `CGO_ENABLED=1` to compile the NVIDIA C-bindings.

```bash
# Build binary (requires CGO for NVML)
make build

# Run unit tests
make test

# Run linter
make lint

# Generate protobuf Go code
make proto

# Build and push a release image
make docker-release VERSION=v0.1.0

# Deploy to cluster
make deploy
```

Or build the Docker image directly:

```bash
docker build -t your-registry/keda-gpu-scaler:v0.1.0 .
docker push your-registry/keda-gpu-scaler:v0.1.0
```

---

## How It Compares

| | keda-gpu-scaler | dcgm-exporter + Prometheus | Custom Metrics API |
|---|---|---|---|
| **Components** | 1 DaemonSet (+ optional /metrics) | dcgm-exporter + Prometheus + adapter | Custom metrics server |
| **Metric latency** | Sub-second (direct NVML) | 15-30s (scrape interval) | Depends on implementation |
| **Scale-to-zero** | Yes (KEDA native) | Yes (with KEDA Prometheus scaler) | Manual |
| **Configuration** | 3-line ScaledObject | PromQL query per metric | Custom code |
| **GPU metrics** | 6 hardware metrics | 50+ DCGM metrics | Whatever you build |
| **Dependencies** | KEDA, NVIDIA drivers | KEDA, Prometheus, dcgm-exporter | Varies |
| **Failure domain** | Node-local | Centralized Prometheus | Varies |

---

## Documentation

- **[Design Document](docs/DESIGN.md)** — Architecture decisions, gRPC interface, scaling profiles, testing strategy
- **[Migration Guide](docs/MIGRATION.md)** — Replace dcgm-exporter + Prometheus with keda-gpu-scaler
- **[FAQ](docs/FAQ.md)** — Common questions about GPU scaling, MIG, multi-GPU, scale-to-zero
- **[Changelog](CHANGELOG.md)** — Release history

---

## Featured In

- **[GPU Autoscaling on Kubernetes with KEDA — Building an External Scaler](https://www.cncf.io/blog/2026/05/27/gpu-autoscaling-on-kubernetes-with-keda-building-an-external-scaler/)** — CNCF Blog (May 2026)
- **[Abstracting AI Infrastructure: Native GPU Scaling for Internal Developer Platforms](https://platformengineering.com/contributed-content/abstracting-ai-infrastructure-native-gpu-scaling-for-internal-developer-platforms/)** — Platform Engineering (May 2026)
- **[The Financial Trap of Autonomous Networks: Scaling Agentic AI in the Telecom Core](https://techblog.comsoc.org/2026/03/30/the-financial-trap-of-autonomous-networks-scaling-agentic-ai-in-the-telecom-core/)** — IEEE ComSoc Technology Blog (March 2026)

---

## Adopters

Using keda-gpu-scaler? Add your organization to [ADOPTERS.md](ADOPTERS.md).

---

## Roadmap

- [ ] AMD ROCm support via `rocm-smi` bindings
- [ ] Multi-Instance GPU (MIG) per-instance metrics
- [ ] PCIe bandwidth and NVLink utilization metrics
- [ ] Inference-framework-aware scaling (vLLM queue depth via engine API)
- [ ] Grafana dashboard for GPU fleet visibility (Prometheus metrics endpoint now available)
- [ ] OCI/OKE optimized deployment guide

---

## Contributing

Contributions welcome — GPU autoscaling use cases, vendor support (AMD ROCm, Intel), or docs improvements. See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

Apache License 2.0. See [LICENSE](LICENSE) for details.
