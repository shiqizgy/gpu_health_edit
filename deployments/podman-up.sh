#!/usr/bin/env bash
# ============================================================================
# Podman 一键拉起后端依赖（MySQL + Redis）并灌入种子数据。
# 用法：bash deployments/podman-up.sh
# ============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

echo ">>> 1. 启动 MySQL + Redis 容器"
# 兼容 podman compose（新）和 podman-compose（旧）两种命令
if podman compose version >/dev/null 2>&1; then
  COMPOSE="podman compose"
elif command -v podman-compose >/dev/null 2>&1; then
  COMPOSE="podman-compose"
else
  echo "未找到 podman compose 或 podman-compose，请先安装" >&2
  exit 1
fi
$COMPOSE -f "$SCRIPT_DIR/podman-compose.yml" up -d

echo ">>> 2. 等待 MySQL 就绪 ..."
for i in $(seq 1 40); do
  if podman exec gpuhealth-mysql mysqladmin ping -h localhost -uroot -prootpass >/dev/null 2>&1; then
    echo "    MySQL 已就绪"
    break
  fi
  sleep 2
  if [ "$i" -eq 40 ]; then echo "MySQL 启动超时" >&2; exit 1; fi
done

echo ">>> 3. 由 server 自动建表（AutoMigrate），此处先跳过建表，直接尝试灌种子"
echo "    注意：种子数据依赖表已存在。请先启动一次 server（会自动建表），再运行本脚本的种子步骤，"
echo "    或取消下面注释让脚本等待你手动建表。"

echo ">>> 4. 灌入种子数据（指标定义 + 默认策略 + 故障知识）"
# 若表尚未建立，这一步会报错；正常流程是先 go run ./cmd/server 建表，再执行 seed。
podman exec -i gpuhealth-mysql mysql -uroot -prootpass gpu_health < "$ROOT_DIR/scripts/seed.sql" \
  && echo "    种子数据灌入成功" \
  || echo "    种子灌入失败：请确认已先启动过 server 完成建表，再重跑：bash deployments/podman-seed.sh"

echo ">>> 完成。后续步骤："
echo "    1) go run ./cmd/server      # 启动 API（首次会自动建表）"
echo "    2) go run ./cmd/simulator   # 启动仿真（建拓扑 + 每分钟造数据）"
echo "    3) go run ./cmd/scorer      # 启动评分（每分钟评分）"
echo "    4) cd web && pnpm install && pnpm dev"
