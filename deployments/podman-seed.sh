#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
for f in accel_metric_scoring.sql 003_score_scope.sql seed_strategy.sql seed_fault_pool.sql; do
  echo "灌入 $f ..."
  podman exec -i gpuhealth-mysql mysql -uroot -prootpass --default-character-set=utf8mb4 gpu_health \
    < "$ROOT_DIR/scripts/$f"
done
echo "种子数据灌入完成"
