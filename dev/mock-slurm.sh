#!/usr/bin/env bash
# dev/mock-slurm.sh
# Simulates a SLURM GPU job environment and runs gpu-metrics with synthetic data.
# This script is gitignored — safe to modify locally without affecting the repo.
#
# Uses --env=slurm (unified flag). The legacy --slurm flag still works but is deprecated.
#
# Usage:
#   make mock-slurm                  # build + run (default scenarios)
#   bash dev/mock-slurm.sh           # run directly (requires mock-build done first)
#   bash dev/mock-slurm.sh --format json  # pass extra flags to gpu-metrics

set -euo pipefail

BINARY="./bin/gpu-metrics"
if [[ ! -x "$BINARY" ]]; then
  echo "ERROR: $BINARY not found. Run 'make mock-build' first." >&2
  exit 1
fi

echo "════════════════════════════════════════════════════════════"
echo " SLURM mock — single-node job, 2 GPUs (table)"
echo "════════════════════════════════════════════════════════════"
SLURM_JOB_ID="12345" \
SLURM_JOB_NAME="vllm-inference" \
SLURM_JOB_PARTITION="gpu-a100" \
SLURM_NODELIST="gpu-node-01" \
SLURM_NODENAME="gpu-node-01" \
SLURM_JOB_NUM_NODES="1" \
SLURM_NTASKS="1" \
SLURM_PROCID="0" \
SLURM_LOCALID="0" \
SLURM_STEP_GPUS="0,1" \
  "$BINARY" --env slurm "$@"

echo ""
echo "════════════════════════════════════════════════════════════"
echo " SLURM mock — multi-node DGX job, all 4 GPUs (JSON)"
echo "════════════════════════════════════════════════════════════"
SLURM_JOB_ID="99999" \
SLURM_JOB_NAME="megatron-lm-training" \
SLURM_JOB_PARTITION="dgx-a100" \
SLURM_NODELIST="dgx[001-004]" \
SLURM_NODENAME="dgx002" \
SLURM_JOB_NUM_NODES="4" \
SLURM_NTASKS="32" \
SLURM_PROCID="8" \
SLURM_LOCALID="0" \
SLURM_STEP_GPUS="0,1,2,3" \
  "$BINARY" --env slurm --format json "$@"

echo ""
echo "════════════════════════════════════════════════════════════"
echo " SLURM mock — CUDA_VISIBLE_DEVICES fallback (CSV)"
echo "════════════════════════════════════════════════════════════"
SLURM_JOB_ID="77777" \
SLURM_JOB_NAME="batch-inference" \
SLURM_NODENAME="gpu-node-03" \
CUDA_VISIBLE_DEVICES="2,3" \
  "$BINARY" --env slurm --format csv "$@"
