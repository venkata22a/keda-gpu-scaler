#!/usr/bin/env bash
# dev/mock-flux.sh
# Simulates a Flux GPU job environment and runs gpu-metrics with synthetic data.
# This script is gitignored — safe to modify locally without affecting the repo.
#
# Uses --env=flux (unified flag). The legacy --flux flag still works but is deprecated.
#
# Usage:
#   make mock-flux                   # build + run (default scenarios)
#   bash dev/mock-flux.sh            # run directly (requires mock-build done first)
#   bash dev/mock-flux.sh --quiet    # pass extra flags to gpu-metrics

set -euo pipefail

BINARY="./bin/gpu-metrics"
if [[ ! -x "$BINARY" ]]; then
  echo "ERROR: $BINARY not found. Run 'make mock-build' first." >&2
  exit 1
fi

echo "════════════════════════════════════════════════════════════"
echo " Flux mock — single-node job, 1 GPU (table)"
echo " Equivalent to: flux run -N1 -g1 gpu-metrics"
echo "════════════════════════════════════════════════════════════"
FLUX_JOB_ID="f23r45t" \
FLUX_TASK_RANK="0" \
FLUX_TASK_LOCAL_ID="0" \
FLUX_JOB_SIZE="1" \
FLUX_JOB_NNODES="1" \
FLUX_URI="local:///run/flux/local" \
CUDA_VISIBLE_DEVICES="0" \
  "$BINARY" --env flux "$@"

echo ""
echo "════════════════════════════════════════════════════════════"
echo " Flux mock — single-node job, 2 GPUs (JSON)"
echo " Equivalent to: flux run -N1 -g2 gpu-metrics --format json"
echo "════════════════════════════════════════════════════════════"
FLUX_JOB_ID="g99hx2z" \
FLUX_TASK_RANK="0" \
FLUX_TASK_LOCAL_ID="0" \
FLUX_JOB_SIZE="1" \
FLUX_JOB_NNODES="1" \
CUDA_VISIBLE_DEVICES="0,1" \
  "$BINARY" --env flux --format json "$@"

echo ""
echo "════════════════════════════════════════════════════════════"
echo " Flux mock — multi-node MPI job, task rank 4 (CSV)"
echo " Equivalent to: flux run -N4 -g2 --tasks-per-node=2 ..."
echo "════════════════════════════════════════════════════════════"
FLUX_JOB_ID="mpirun42" \
FLUX_TASK_RANK="4" \
FLUX_TASK_LOCAL_ID="0" \
FLUX_JOB_SIZE="8" \
FLUX_JOB_NNODES="4" \
CUDA_VISIBLE_DEVICES="2,3" \
  "$BINARY" --env flux --format csv "$@"
