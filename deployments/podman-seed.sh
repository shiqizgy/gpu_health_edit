#!/usr/bin/env bash
# 单独灌种子数据。先决条件：表已由 server 的 AutoMigrate 建好。
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
podman exec -i gpuhealth-mysql mysql -uroot -prootpass gpu_health < "$ROOT_DIR/scripts/seed.sql"
echo "种子数据灌入完成"
