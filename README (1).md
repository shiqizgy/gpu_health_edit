# GPU 健康度监测平台

一个面向 GPU 集群的健康度监测平台。后端 Go(Gin + GORM)，数据库 MySQL，实时指标仿真走 Redis，部署用 Podman。前端 Vue 3 + Naive UI。

## 目录结构

```
gpu-health-platform/
├── cmd/                      三个独立可执行程序
│   ├── server/               Gin API 服务(首次启动自动建表)
│   ├── scorer/               评分服务(每分钟从 Redis 拉指标算分)
│   └── simulator/            仿真服务(每分钟造 2000 卡指标写 Redis)
├── internal/
│   ├── config/               YAML 配置加载
│   ├── model/                GORM 数据模型(8 张表)
│   ├── repository/           数据访问层(每表一个 repo)
│   ├── service/              业务逻辑(策略编译/评分/仿真)
│   ├── scoring/              评分引擎(曲线 + 两层加权 + 一票否决)
│   ├── redisclient/          Redis 封装(仿真→评分的实时通道)
│   ├── handler/              HTTP 处理器(按页面组织)
│   └── router/               路由装配
├── pkg/                      通用工具(日志/响应)
├── configs/config.yaml       配置文件
├── scripts/seed.sql          种子数据(25 指标 + 默认策略 + 故障知识)
├── deployments/              Podman 部署文件
└── web/                      Vue 3 前端(7 个页面)
```

## 架构与数据流

```
仿真服务(2000卡/分钟)  ──写──►  Redis(每卡最新全量指标)
                                      │
                                      ▼ 读
                              评分服务(每分钟)
                                      │ 按策略算分
                                      ▼
                              MySQL(单卡快照 + 集群汇总预聚合)
                                      │
                                      ▼
                              API 服务(Gin) ──► 前端(Vue)
```

关键设计：
- **指标定义与权重分离**：`metric_definition` 存"指标是什么"，`strategy_metric_rule` 存"某策略下该指标的权重和曲线"。同一指标在不同任务(策略)下可有不同权重。
- **策略组**：维度权重 + 各指标权重/曲线打包成策略，算分时选策略。改策略后 `version+1`，评分服务 5 秒内热加载。
- **预聚合**：评分服务每轮把集群汇总写入 `cluster_health_summary`，集群表格页只查这张表(几十行)，万卡场景毫秒响应。
- **Redis 做实时通道**：每卡最新指标用 String 存 JSON(`gpu:metrics:{uuid}`)，覆盖写，带 TTL。选 Redis 而非 Kafka 是因为本场景是 KV 覆盖写，本地负载小。

## 评分算法

四个维度(权重)：hardware 0.45 / stability 0.25 / performance 0.20 / environment 0.10。

```
单指标分 = 曲线(指标值)         # piecewise / log / xid_table / veto / none
维度分   = Σ(单指标分×权重) / Σ(权重)
总分     = Σ(维度分×维度权重)
一票否决 = 先算完总分，若触发(DBE/不可纠正remap/remap失败/致命XID)则 min(总分,29) → failed
```

等级：≥90 healthy / ≥75 sub_healthy / ≥60 warning / ≥30 critical / <30 failed。

## 快速启动

### 前置
- Go 1.22+
- Node 18+ 与 pnpm
- Podman + podman-compose(或 podman 内置 compose)

### 步骤

```bash
# 1. 用 Podman 拉起 MySQL + Redis
make up
# 等价于 bash deployments/podman-up.sh

# 2. 启动 API 服务(首次会自动建表 AutoMigrate)
make server          # go run ./cmd/server

# 3. 表建好后，灌种子数据(25 指标 + 默认策略 + 故障知识)
make seed            # bash deployments/podman-seed.sh

# 4. 启动仿真服务(建集群/节点/GPU 拓扑 + 每分钟造数据)
make simulator       # go run ./cmd/simulator

# 5. 启动评分服务(每分钟评分)
make scorer          # go run ./cmd/scorer

# 6. 启动前端
make web             # cd web && pnpm install && pnpm dev
# 打开 http://localhost:5173
```

> 启动顺序很重要：**server 先建表 → seed 灌数据 → simulator 造数据 → scorer 评分**。
> 仿真服务首次启动会自动创建 2000 卡的拓扑(集群/节点/GPU)并写入数据库。

## 验证

```bash
# Redis 是否有指标
podman exec gpuhealth-redis redis-cli KEYS 'gpu:metrics:*' | head

# MySQL 指标/策略是否就绪
podman exec gpuhealth-mysql mysql -uroot -prootpass gpu_health \
  -e "SELECT COUNT(*) FROM metric_definition; SELECT COUNT(*) FROM strategy_metric_rule;"

# API 是否工作
curl -s http://127.0.0.1:8080/api/v1/dashboard/overview
curl -s http://127.0.0.1:8080/api/v1/health/clusters
```

## 演示流程

1. **健康大盘**：看 GPU 总数、平均分、等级分布、风险最高的卡
2. **指标系统**：浏览 25 个指标，可增删改查
3. **健康评估 → 集群拓扑**：展开集群-节点-GPU 三级树
4. **健康评估 → 健康值**：看集群表格 → 点集群看单卡评分；切到「评分策略」改权重
5. **故障注入(演示)**：给某张卡注入 XID 致命错误
6. 等约 1-2 分钟(仿真生成 + 评分各一轮)，回到「健康值」看该卡掉到 failed
7. **GPU 故障 → 故障知识图谱**：增删改查故障知识

## 调试

- **只跑一次**：`go run ./cmd/scorer -once` / `go run ./cmd/simulator -once`
- **只初始化拓扑**：`go run ./cmd/simulator -init`
- **改评分间隔**：`configs/config.yaml` 的 `scorer.cron` / `simulator.cron`
- **改仿真规模/异常率**：`configs/config.yaml` 的 `simulator.gpu_count` / `anomaly_rate`
- **GORM SQL 日志**：`config.yaml` 把 `server.mode` 设为 `debug`

## 未实现(按规划预留)

- **故障预测**：计划接外部 API
- **故障根因分析**：先做基于规则的分析，计划接外部 API 做成 AI 助手

## 关停

```bash
make down    # 停止并清理 podman 容器和数据卷
```
