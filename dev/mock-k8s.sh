#!/usr/bin/env bash
# dev/mock-k8s.sh
# Simulates a Kubernetes pod environment and runs gpu-metrics with synthetic data.
# This script is gitignored — safe to modify locally without affecting the repo.
#
# Mimics env vars that a real K8s pod receives via:
#   - KUBERNETES_SERVICE_HOST  (always set by the kubelet)
#   - MY_NODE_NAME / MY_POD_NAME / MY_POD_NAMESPACE  (Downward API fieldRef)
#   - JOB_COMPLETION_INDEX   (set for indexed batch Jobs)
#   - NVIDIA_VISIBLE_DEVICES  (set by the NVIDIA device plugin)
#
# Usage:
#   make mock-k8s                    # build + run (default scenarios)
#   bash dev/mock-k8s.sh             # run directly (requires mock-build done first)
#   bash dev/mock-k8s.sh --format json  # pass extra flags to gpu-metrics

set -euo pipefail

BINARY="./bin/gpu-metrics"
if [[ ! -x "$BINARY" ]]; then
  echo "ERROR: $BINARY not found. Run 'make mock-build' first." >&2
  exit 1
fi

echo "════════════════════════════════════════════════════════════"
echo " K8s mock — DaemonSet pod, 1 GPU (table)"
echo " Equivalent to: running gpu-metrics inside a DaemonSet pod"
echo "════════════════════════════════════════════════════════════"
KUBERNETES_SERVICE_HOST="10.96.0.1" \
MY_NODE_NAME="gpu-node-01" \
MY_POD_NAME="gpu-metrics-daemonset-xk9pb" \
MY_POD_NAMESPACE="monitoring" \
NVIDIA_VISIBLE_DEVICES="0" \
  "$BINARY" --env k8s "$@"

echo ""
echo "════════════════════════════════════════════════════════════"
echo " K8s mock — indexed batch Job, 4 GPUs (JSON)"
echo " Equivalent to: a batch.kubernetes.io Job with completions=4"
echo "════════════════════════════════════════════════════════════"
KUBERNETES_SERVICE_HOST="10.96.0.1" \
MY_NODE_NAME="gpu-node-02" \
MY_POD_NAME="llm-training-job-2" \
MY_POD_NAMESPACE="ai-workloads" \
JOB_COMPLETION_INDEX="2" \
NVIDIA_VISIBLE_DEVICES="0,1,2,3" \
  "$BINARY" --env k8s --format json "$@"

echo ""
echo "════════════════════════════════════════════════════════════"
echo " K8s mock — auto-detect (KUBERNETES_SERVICE_HOST present)"
echo " Uses --env=auto to show detection works without explicit flag"
echo "════════════════════════════════════════════════════════════"
KUBERNETES_SERVICE_HOST="10.96.0.1" \
MY_NODE_NAME="gpu-node-03" \
MY_POD_NAME="vllm-inference-0" \
MY_POD_NAMESPACE="inference" \
NVIDIA_VISIBLE_DEVICES="0,1" \
  "$BINARY" --env auto --format csv "$@"
