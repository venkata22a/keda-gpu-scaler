# Cross-Environment GPU Metrics Comparison

`gpu-metrics` uses a single binary and a unified JSON schema across all supported environments — Kubernetes, SLURM, Flux, and standalone — so you can compare GPU performance between on-prem HPC clusters and cloud without changing your tooling or post-processing scripts.

---

## The `--env` flag

```
gpu-metrics --env <value>
```

| Value | Meaning |
|-------|---------|
| `auto` *(default)* | Auto-detect: checks `SLURM_JOB_ID` → `FLUX_JOB_ID` → `KUBERNETES_SERVICE_HOST` → falls back to `standalone` |
| `slurm` | Force SLURM context (reads `SLURM_*` env vars) |
| `flux` | Force Flux context (reads `FLUX_*` + `CUDA_VISIBLE_DEVICES`) |
| `k8s` | Force Kubernetes context (reads Downward API env vars) |
| `standalone` | No scheduler context |

> **Note:** The legacy `--slurm` and `--flux` flags still work but are deprecated. Migrate to `--env=slurm` / `--env=flux`.

---

## Unified JSON schema

Every `gpu-metrics --format json` invocation emits the same top-level schema, with `schema_version` for forward-compatibility:

```json
{
  "schema_version": "v1",
  "collected_at": "<RFC3339 timestamp>",
  "environment": {
    "orchestrator": "<k8s | slurm | flux | standalone>",
    "node": "<node/hostname>",
    "job_id": "<job or pod identifier>",
    "task_rank": 0,
    "local_rank": 0,
    "gpus": "<comma-separated device indices>"
  },
  "devices": [ ... ]
}
```

Orchestrator-specific fields (`partition`, `namespace`) are included only when non-empty. The `devices` array is identical in every environment.

---

## Side-by-side examples

The examples below show the same workload — an LLM inference server consuming GPU 0 at ~85% utilization — as it would look in each environment.

### SLURM

```bash
# Inside a SLURM job
SLURM_JOB_ID=12345 SLURM_NODENAME=gpu-node-01 \
  SLURM_STEP_GPUS=0 gpu-metrics --format json
```

```json
{
  "schema_version": "v1",
  "collected_at": "2026-06-12T14:00:00Z",
  "environment": {
    "orchestrator": "slurm",
    "node": "gpu-node-01",
    "job_id": "12345",
    "task_rank": 0,
    "local_rank": 0,
    "gpus": "0",
    "partition": "gpu-a100"
  },
  "devices": [
    {
      "Index": 0,
      "Name": "NVIDIA A100-SXM4-80GB",
      "GPUUtilization": 85,
      "MemoryUsedMiB": 58368,
      "MemoryTotalMiB": 81920,
      "TemperatureCelsius": 74,
      "PowerDrawWatts": 342
    }
  ]
}
```

### Flux

```bash
# Inside a Flux job
FLUX_JOB_ID=f23r45t CUDA_VISIBLE_DEVICES=0 \
  gpu-metrics --format json
```

```json
{
  "schema_version": "v1",
  "collected_at": "2026-06-12T14:00:00Z",
  "environment": {
    "orchestrator": "flux",
    "node": "gpu-node-01",
    "job_id": "f23r45t",
    "task_rank": 0,
    "local_rank": 0,
    "gpus": "0"
  },
  "devices": [
    {
      "Index": 0,
      "Name": "NVIDIA A100-SXM4-80GB",
      "GPUUtilization": 85,
      "MemoryUsedMiB": 58368,
      "MemoryTotalMiB": 81920,
      "TemperatureCelsius": 74,
      "PowerDrawWatts": 342
    }
  ]
}
```

### Kubernetes

```bash
# Inside a K8s pod (Downward API env vars set in pod spec)
KUBERNETES_SERVICE_HOST=10.96.0.1 \
  MY_NODE_NAME=gpu-node-01 \
  MY_POD_NAME=vllm-inference-0 \
  MY_POD_NAMESPACE=inference \
  NVIDIA_VISIBLE_DEVICES=0 \
  gpu-metrics --format json
```

```json
{
  "schema_version": "v1",
  "collected_at": "2026-06-12T14:00:00Z",
  "environment": {
    "orchestrator": "k8s",
    "node": "gpu-node-01",
    "job_id": "vllm-inference-0",
    "task_rank": 0,
    "local_rank": 0,
    "gpus": "0",
    "namespace": "inference"
  },
  "devices": [
    {
      "Index": 0,
      "Name": "NVIDIA A100-SXM4-80GB",
      "GPUUtilization": 85,
      "MemoryUsedMiB": 58368,
      "MemoryTotalMiB": 81920,
      "TemperatureCelsius": 74,
      "PowerDrawWatts": 342
    }
  ]
}
```

---

## Comparing across environments with `jq`

Because the schema is uniform, standard `jq` pipelines work identically on output from all environments:

```bash
# Collect snapshots on-prem (SLURM) and in cloud (K8s)
gpu-metrics --format json > slurm_snapshot.json
gpu-metrics --format json > k8s_snapshot.json

# Compare GPU utilization for device 0 in both environments
jq -r '[.environment.orchestrator, (.devices[0].GPUUtilization | tostring) + "%"] | join("\t")' \
  slurm_snapshot.json k8s_snapshot.json
# slurm  85%
# k8s    72%

# Pull all device metrics from either file with the same query
jq '.devices[] | {gpu: .Index, util: .GPUUtilization, mem_pct: (.MemoryUsedMiB / .MemoryTotalMiB * 100 | round)}' \
  slurm_snapshot.json
```

---

## Kubernetes pod spec — Downward API setup

To get node, pod, and namespace metadata in K8s mode, configure the Downward API in your pod spec. For indexed Jobs also expose `JOB_COMPLETION_INDEX`:

```yaml
env:
  - name: MY_NODE_NAME
    valueFrom:
      fieldRef:
        fieldPath: spec.nodeName
  - name: MY_POD_NAME
    valueFrom:
      fieldRef:
        fieldPath: metadata.name
  - name: MY_POD_NAMESPACE
    valueFrom:
      fieldRef:
        fieldPath: metadata.namespace
  # For indexed batch Jobs only:
  - name: JOB_COMPLETION_INDEX
    valueFrom:
      fieldRef:
        fieldPath: metadata.annotations['batch.kubernetes.io/job-completion-index']
```

---

## Auto-detection precedence

When `--env=auto` (the default), the binary checks environment variables in this order:

1. `SLURM_JOB_ID` present → `slurm`
2. `FLUX_JOB_ID` present → `flux`
3. `KUBERNETES_SERVICE_HOST` present → `k8s`
4. None of the above → `standalone`

This means running under `sbatch` or `flux run` will auto-select the correct mode without any flag.

---

## Running the mock scripts

```bash
make mock-build         # CGO-free binary using synthetic GPU data

make mock-slurm         # Simulate a SLURM job
make mock-flux          # Simulate a Flux job
make mock-k8s           # Simulate a Kubernetes pod
make mock-keda          # Start the mock KEDA gRPC server
```
