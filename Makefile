# GPU 健康度平台 Makefile
.PHONY: deps up seed down server scorer simulator sim-init web build-backend help

help:
	@echo "常用命令："
	@echo "  make up          # podman 拉起 MySQL+Redis"
	@echo "  make seed        # 灌种子数据(需先 server 建表)"
	@echo "  make server      # 启动 API 服务(首次自动建表)"
	@echo "  make sim-init    # 仿真：只初始化拓扑"
	@echo "  make simulator   # 仿真：每分钟造数据"
	@echo "  make scorer      # 评分：每分钟评分"
	@echo "  make web         # 启动前端"
	@echo "  make down        # 停止并清理 podman 容器"

# ---- Podman 依赖 ----
up:
	@bash deployments/podman-up.sh

seed:
	@bash deployments/podman-seed.sh

down:
	@podman compose -f deployments/podman-compose.yml down -v 2>/dev/null || \
	 podman-compose -f deployments/podman-compose.yml down -v

# ---- 后端服务（宿主机直接跑，开发最方便）----
server:
	@go run ./cmd/server

scorer:
	@go run ./cmd/scorer

simulator:
	@go run ./cmd/simulator

sim-init:
	@go run ./cmd/simulator -init

# ---- 前端 ----
web:
	@cd web && pnpm dev

# ---- 容器化构建（可选）----
build-backend:
	@podman build -t gpuhealth-backend -f deployments/Containerfile.backend .
