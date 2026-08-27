# Docker 部署设计

> 日期：2026-08-27
> 状态：已获用户确认
> 目标：为 efilter risk-engine 提供标准化 Docker 部署方案，新服务器只需安装 Docker 即可跑起整套系统（应用 + PostgreSQL + Redis）。

## 需求决策（已确认）

| 决策点 | 选择 |
|--------|------|
| 部署范围 | 全栈容器（app + postgres + redis） |
| IP 库处理 | binfiles/ 宿主机挂载（read-only） |
| IP 库更新 | 宿主机 cron + 现有 update-ipdb.sh / update-maxmind.sh，更新后重启 app 容器 |
| 镜像构建 | 服务器本地 `docker compose build`，不推镜像仓库 |
| Go 代码改动 | 零改动 |

## 架构

```
宿主机 /opt/efilter（git clone）
├── binfiles/  ←─── 挂载 (ro) ───┐
│   ├── IP2LOCATION-LITE-DB1.IPV6.BIN/
│   ├── IP2PROXY-LITE-PX2.BIN/
│   ├── IP2PROXY-LITE-PX2.IPV6.CSV/
│   └── maxmind/*.mmdb           │
│      ↑ 宿主机 cron 更新 + docker compose restart app
│
├── docker/
│   ├── Dockerfile
│   ├── docker-compose.yml
│   ├── .env.example
│   ├── config.docker.yaml
│   └── create-indexes.sh
├── .dockerignore（仓库根）
└── docs/docker-deployment.md（使用教程）

Docker 内部网络 efilter-net：
  app(:8080) ──→ postgres:5432（named volume: pgdata）
             ──→ redis:6379（纯内存）
宿主机仅暴露 app 端口（默认 8080）；postgres/redis 不映射宿主机端口。
```

## 容器内目录布局（零代码改动的关键）

镜像内复刻仓库结构，使现有相对路径逻辑原样生效：

```
/app/
├── backend/risk-engine/
│   ├── risk-engine          # 二进制
│   ├── configs/config.yaml  # 由 config.docker.yaml 烘入
│   └── logs/                # 文件日志（stdout 同步输出，docker logs 可查）
├── frontend/index.html      # ../../frontend 相对引用照常
└── binfiles/                # 运行时挂载点
```

`WORKDIR /app/backend/risk-engine`；`../../binfiles`、`../../frontend`、`./configs`、`logs/app.log` 全部不变。

## 文件清单与职责

| 文件 | 说明 |
|------|------|
| `docker/Dockerfile` | 多阶段：`golang:1.25-alpine` 编译（`CGO_ENABLED=0`，`go mod download` 单独成层缓存依赖）→ `alpine:3.20` 运行（tzdata + ca-certificates）。最终镜像约 60–70MB。构建上下文为仓库根：`docker compose` 中 `build: {context: .., dockerfile: docker/Dockerfile}` |
| `docker/docker-compose.yml` | 3 个服务，`restart: unless-stopped`；postgres/redis 有 healthcheck；app `depends_on: condition: service_healthy`；挂载 `../binfiles:/app/binfiles:ro`；TZ=Asia/Shanghai |
| `docker/config.docker.yaml` | 与现有 config.yaml 唯一区别：DSN `host=postgres`、Redis `addr: "redis:6379"`；IP 库与前端相对路径不变 |
| `docker/.env.example` | `POSTGRES_PASSWORD`、`RISK_DATABASE_DSN`、`RISK_APP_API_KEY`、宿主机端口 `APP_PORT`、`TZ`。`docker/.env` 加入 .gitignore |
| `docker/create-indexes.sh` | 首次启动后手动执行一次：进 postgres 容器建 `pg_trgm` 扩展 + GIN/B-tree 索引（GORM 自动迁移不覆盖 GIN） |
| 根目录 `.dockerignore` | 排除 `tools/`（本地 Go 工具链约 1GB+）、`binfiles/`、`data/`、`logs/`、`.git`、`*.zip`、`*.exe` 等，防止 build context 过大 |
| `docs/docker-deployment.md` | 使用教程（安装 Docker → 首次部署 → 建索引 → cron 更新 → 升级 → 常用命令 → 故障排查） |
| `docker/postgres/init/01-extensions.sql` | `CREATE EXTENSION IF NOT EXISTS pg_trgm;`，挂载到 postgres 容器 `/docker-entrypoint-initdb.d/`，首次初始化数据卷时自动执行 |

## 敏感信息

利用 viper 已有的环境变量覆盖（`RISK_` 前缀 + `.` → `_`）：

- `RISK_DATABASE_DSN` 覆盖 `database.dsn`（注入真实密码）
- `RISK_APP_API_KEY` 覆盖 `app.api_key`

密码只存于 `docker/.env`（gitignored）；烘入镜像的 config.docker.yaml 仅含容器内主机名（`postgres`/`redis`），无真实密码。

## pg_trgm 说明

官方 `postgres:18-alpine` 镜像自带 contrib 模块，`pg_trgm` 可直接创建（与宝塔版受限不同）。扩展由 init SQL 首次建库时创建；`create-indexes.sh` 中的 `CREATE EXTENSION IF NOT EXISTS` 与之幂等兼容（数据卷已初始化的老环境也能补建），索引在该脚本中于应用首次完成自动迁移后执行。

## 运维流程

```bash
# 首次部署
git clone https://github.com/JR-coderli/efilter.git /opt/efilter && cd /opt/efilter
cp docker/.env.example docker/.env && vim docker/.env
bash tools/update-ipdb/update-ipdb.sh
docker compose -f docker/docker-compose.yml up -d --build
bash docker/create-indexes.sh

# IP 库每日更新（crontab）
0 3 * * * cd /opt/efilter && bash tools/update-ipdb/update-ipdb.sh >> logs/ipdb-update.log 2>&1 && docker compose -f docker/docker-compose.yml restart app
10 3 * * * cd /opt/efilter && GEOIP_CONF=/opt/efilter/GeoIP.local.conf bash tools/update-ipdb/update-maxmind.sh >> logs/geoipupdate.log 2>&1 && docker compose -f docker/docker-compose.yml restart app

# 代码升级
cd /opt/efilter && git pull && docker compose -f docker/docker-compose.yml up -d --build

# 日志 / 状态
docker compose -f docker/docker-compose.yml logs -f app
docker compose -f docker/docker-compose.yml ps
```

注意：`update-maxmind.sh` 默认 `GEOIP_CONF=${PROJECT_ROOT}/configs/GeoIP.conf` 与仓库实际模板位置（根目录）不符，cron 中显式传 `GEOIP_CONF` 环境变量规避（不做代码改动）。

## 明确不做（YAGNI）

- 不做 CI 推镜像仓库（单机规模，本地构建）
- 不做 geoipupdate sidecar 容器（宿主机 cron 统一处理两类库）
- 不做 app-only compose profile（宝塔环境继续用 systemd 方案，并存不冲突）
- 不改任何 Go 代码

## 与现有 systemd 部署的关系

Docker 方案面向新服务器/测试机标准化部署；现有宝塔 + systemd 生产环境不受影响，两套方案并存，由服务器侧自行选择。
