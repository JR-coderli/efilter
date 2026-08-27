# Docker 部署实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 按已批准的设计（`docs/superpowers/specs/2026-08-27-docker-deployment-design.md`）创建 docker/ 目录下的全部部署文件与使用教程，实现全栈容器化（app + PostgreSQL + Redis），零 Go 代码改动。

**Architecture:** 多阶段 Dockerfile（golang:1.25-alpine 编译 → alpine:3.20 运行），镜像内复刻仓库目录结构（`/app/backend/risk-engine` 工作目录 + `/app/frontend` + `/app/binfiles` 挂载点）使现有相对路径原样生效。docker-compose 起 3 个服务，敏感信息经 `RISK_` 前缀环境变量从 `docker/.env` 注入。IP 库由宿主机 cron 更新后重启 app 容器。

**Tech Stack:** Docker multi-stage build, docker compose v2, golang:1.25-alpine, alpine:3.20, postgres:18-alpine, redis:7-alpine, viper env override（已存在，无需改代码）

## Global Constraints

- **零 Go 代码改动**：所有路径兼容通过镜像内目录复刻实现，`WORKDIR /app/backend/risk-engine`。
- 镜像内不含任何真实密码/API Key：`config.docker.yaml` 只含容器内主机名（`postgres`/`redis`）与默认占位值；真实值通过 `RISK_DATABASE_DSN`、`RISK_APP_API_KEY` 环境变量注入。
- `binfiles/` 以 read-only 挂载，**不进镜像**。
- 构建上下文为仓库根（compose 中 `context: ..`），`dockerfile: docker/Dockerfile`。
- postgres/redis 不映射宿主机端口，仅 app 暴露（默认 8080，`.env` 的 `APP_PORT` 可改）。
- 所有索引/扩展 SQL 幂等（`IF NOT EXISTS`）。
- `docker/.env` 必须 gitignored。
- compose 文件内的相对路径（`../binfiles`）相对 compose 文件所在目录解析，任意 cwd 执行 `docker compose -f docker/docker-compose.yml ...` 均正确。
- 现有 systemd/宝塔部署不受影响，不动 `tools/deploy/`。

---

## Task 1: 根目录 .dockerignore

**Files:**
- Create: `.dockerignore`

**Interfaces:**
- Produces: 构建上下文瘦身（排除 `tools/go/` 约 1GB+、binfiles、数据、日志、二进制）

- [ ] **Step 1: 创建 .dockerignore**

```
# VCS
.git
.gitignore
.gitattributes

# Secrets（镜像与构建上下文都不允许含真实密钥）
.env
.env.local
.env.*.local
docker/.env
GeoIP.local.conf

# IP 数据库（运行时挂载，不进镜像）
binfiles/

# 运行时数据与日志
data/
logs/
tmp/
backend/risk-engine/logs/

# 本地工具链与压缩包（tools/go 约 1GB+）
tools/go/
tools/go.zip
tools/mariadb.zip

# 预编译二进制
backend/risk-engine/risk-engine
backend/risk-engine/risk-engine.exe

# 文档（构建不需要）
docs/
backend/docs/
CLAUDE.md
README.md

# 杂项
*.zip
*.tar.gz
*.exe
*.dll
*.db
*.sqlite*
vendor/
```

- [ ] **Step 2: Commit**

```bash
git add .dockerignore
git commit -m "chore(docker): add .dockerignore to slim build context"
```

---

## Task 2: docker/config.docker.yaml

**Files:**
- Create: `docker/config.docker.yaml`

**Interfaces:**
- Consumes: 现有 `backend/risk-engine/configs/config.yaml` 的结构（viper 按 `./configs/config.yaml` 读取）
- Produces: 烘入镜像的容器专用配置（DSN host=postgres、Redis addr=redis:6379）

- [ ] **Step 1: 创建配置文件**

与现有 `backend/risk-engine/configs/config.yaml` 唯一区别：`database.dsn` 的 `host=postgres`、`redis.addr: "redis:6379"`。密码字段保留默认占位值（真实值由环境变量覆盖）。

```yaml
app:
  name: "risk-engine"
  mode: "release"
  port: 8080
  # 默认占位；生产通过 RISK_APP_API_KEY 环境变量覆盖
  api_key: "risk-engine-dev-key-2026"

database:
  driver: "postgres"
  # host=postgres 为 compose 服务名；密码占位，生产通过 RISK_DATABASE_DSN 覆盖
  dsn: "host=postgres user=postgres password=admin123 dbname=risk_engine port=5432 sslmode=disable TimeZone=Asia/Shanghai"
  max_open_conns: 100
  max_idle_conns: 20

redis:
  addr: "redis:6379"
  password: ""
  db: 5
  pool_size: 20

ipdb:
  # 相对路径以 /app/backend/risk-engine 为工作目录，镜像内复刻仓库结构
  ip2location: "../../binfiles/IP2LOCATION-LITE-DB1.IPV6.BIN/IP2LOCATION-LITE-DB1.IPV6.BIN"
  ip2proxy: "../../binfiles/IP2PROXY-LITE-PX2.BIN/IP2PROXY-LITE-PX2.BIN"
  ip2proxy_ipv6_csv: "../../binfiles/IP2PROXY-LITE-PX2.IPV6.CSV/IP2PROXY-LITE-PX2.IPV6.CSV"
  maxmind:
    country: "../../binfiles/maxmind/GeoLite2-Country.mmdb"
    city: "../../binfiles/maxmind/GeoLite2-City.mmdb"
    asn: "../../binfiles/maxmind/GeoLite2-ASN.mmdb"

rate_limit:
  window: 1
  max_requests: 2000

log:
  level: "info"
  path: "logs/app.log"
```

- [ ] **Step 2: Commit**

```bash
git add docker/config.docker.yaml
git commit -m "feat(docker): add container-specific config with service hostnames"
```

---

## Task 3: docker/Dockerfile

**Files:**
- Create: `docker/Dockerfile`

**Interfaces:**
- Consumes: `backend/risk-engine/go.mod`、`go.sum`、源码；`frontend/`；`docker/config.docker.yaml`
- Produces: 镜像 `/app` 目录树（backend/risk-engine + frontend + binfiles 挂载点），最终镜像约 60–70MB

- [ ] **Step 1: 创建 Dockerfile**

```dockerfile
# ---------- Build stage ----------
FROM golang:1.25-alpine AS builder

# 依赖单独成层，源码变动不触发 go mod download 重跑
WORKDIR /src/backend/risk-engine

COPY backend/risk-engine/go.mod backend/risk-engine/go.sum ./
RUN go mod download

COPY backend/risk-engine/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/risk-engine ./cmd/server

# ---------- Runtime stage ----------
FROM alpine:3.20

RUN apk add --no-cache tzdata ca-certificates \
    && adduser -D -u 10001 appuser

# 复刻仓库结构，使 ../../binfiles、../../frontend、./configs 相对路径原样生效
WORKDIR /app/backend/risk-engine

COPY --from=builder /out/risk-engine ./risk-engine
COPY docker/config.docker.yaml ./configs/config.yaml
COPY frontend/ /app/frontend

RUN mkdir -p /app/binfiles ./logs \
    && chown -R appuser:appuser /app

USER appuser

EXPOSE 8080

ENTRYPOINT ["./risk-engine"]
```

- [ ] **Step 2: Commit**

```bash
git add docker/Dockerfile
git commit -m "feat(docker): multi-stage Dockerfile replicating repo layout"
```

---

## Task 4: postgres init SQL

**Files:**
- Create: `docker/postgres/init/01-extensions.sql`

**Interfaces:**
- Consumes: postgres 镜像 `/docker-entrypoint-initdb.d/` 机制（仅首次初始化数据卷时执行）
- Produces: `pg_trgm` 扩展（后续 GIN 索引依赖）

- [ ] **Step 1: 创建 init SQL**

```sql
-- 官方 postgres 镜像自带 contrib 模块，pg_trgm 可直接创建。
-- 仅在数据卷首次初始化时执行；老数据卷由 docker/create-indexes.sh 幂等补建。
CREATE EXTENSION IF NOT EXISTS pg_trgm;
```

- [ ] **Step 2: Commit**

```bash
git add docker/postgres/init/01-extensions.sql
git commit -m "feat(docker): create pg_trgm extension on first db init"
```

---

## Task 5: docker/.env.example 与 .gitignore

**Files:**
- Create: `docker/.env.example`
- Modify: `.gitignore`（追加 `docker/.env`）

**Interfaces:**
- Produces: compose 变量模板（`docker compose --env-file` 默认读取同目录 `.env`）

- [ ] **Step 1: 创建 .env.example**

```bash
# ===== efilter Docker 部署环境变量 =====
# 复制为 docker/.env 后修改（docker/.env 已 gitignored）

# PostgreSQL root 密码（postgres 容器初始化 + DSN 共用）
POSTGRES_PASSWORD=admin123

# 完整数据库 DSN（覆盖镜像内 config.docker.yaml 的占位值）
# host=postgres 固定指向 compose 内的 postgres 服务，勿改
RISK_DATABASE_DSN=host=postgres user=postgres password=admin123 dbname=risk_engine port=5432 sslmode=disable TimeZone=Asia/Shanghai

# API Key（覆盖默认 risk-engine-dev-key-2026）
RISK_APP_API_KEY=risk-engine-dev-key-2026

# 宿主机暴露端口（容器内固定 8080）
APP_PORT=8080

# 时区
TZ=Asia/Shanghai
```

- [ ] **Step 2: .gitignore 追加一行**

在 `.gitignore` 的 env/secrets 区块（`.env` 相关行附近）追加：

```gitignore
docker/.env
```

- [ ] **Step 3: Commit**

```bash
git add docker/.env.example .gitignore
git commit -m "feat(docker): add compose env template and ignore docker/.env"
```

---

## Task 6: docker/docker-compose.yml

**Files:**
- Create: `docker/docker-compose.yml`

**Interfaces:**
- Consumes: Task 3 镜像、Task 4 init SQL、Task 5 env 变量
- Produces: 三服务栈（app/postgres/redis），网络 `efilter-net`，卷 `pgdata`、`app-logs`

- [ ] **Step 1: 创建 docker-compose.yml**

```yaml
name: efilter

services:
  app:
    build:
      context: ..
      dockerfile: docker/Dockerfile
    image: efilter/risk-engine:latest
    container_name: efilter-app
    restart: unless-stopped
    environment:
      TZ: ${TZ:-Asia/Shanghai}
      RISK_APP_API_KEY: ${RISK_APP_API_KEY:-risk-engine-dev-key-2026}
      RISK_DATABASE_DSN: ${RISK_DATABASE_DSN:-host=postgres user=postgres password=admin123 dbname=risk_engine port=5432 sslmode=disable TimeZone=Asia/Shanghai}
    ports:
      - "${APP_PORT:-8080}:8080"
    volumes:
      # IP 数据库宿主机挂载，read-only；更新流程：宿主机脚本替换文件后 restart app
      - ../binfiles:/app/binfiles:ro
      # 文件日志持久化（stdout 也可用 docker logs 查看）
      - app-logs:/app/backend/risk-engine/logs
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
    healthcheck:
      test: ["CMD-SHELL", "wget -qO- http://127.0.0.1:8080/health || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 15s
    networks:
      - efilter-net

  postgres:
    image: postgres:18-alpine
    container_name: efilter-postgres
    restart: unless-stopped
    environment:
      TZ: ${TZ:-Asia/Shanghai}
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:-admin123}
      POSTGRES_DB: risk_engine
    volumes:
      - pgdata:/var/lib/postgresql/data
      # 仅首次初始化数据卷时执行（建 pg_trgm 扩展）
      - ./postgres/init:/docker-entrypoint-initdb.d:ro
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres -d risk_engine"]
      interval: 10s
      timeout: 5s
      retries: 5
    networks:
      - efilter-net

  redis:
    image: redis:7-alpine
    container_name: efilter-redis
    restart: unless-stopped
    # 仅用于限流计数，纯内存、不持久化
    command: ["redis-server", "--save", "", "--appendonly", "no"]
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 10s
      timeout: 5s
      retries: 5
    networks:
      - efilter-net

volumes:
  pgdata:
  app-logs:

networks:
  efilter-net:
    driver: bridge
```

- [ ] **Step 2: Commit**

```bash
git add docker/docker-compose.yml
git commit -m "feat(docker): add full-stack compose (app + postgres + redis)"
```

---

## Task 7: docker/create-indexes.sh

**Files:**
- Create: `docker/create-indexes.sh`

**Interfaces:**
- Consumes: 运行中的 postgres 容器（`docker compose exec`）；GORM 首次自动迁移已建表
- Produces: 幂等索引脚本（pg_trgm 扩展 + GIN/B-tree 索引；GORM 已建的 B-tree 索引同名会被 IF NOT EXISTS 跳过）

- [ ] **Step 1: 创建脚本**

```bash
#!/usr/bin/env bash
# 在 postgres 容器内幂等创建 dashboard 查询索引。
# 用法：应用容器首次启动完成自动迁移后执行一次
#   bash docker/create-indexes.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${SCRIPT_DIR}"

# psql 逐条执行（每条独立 autocommit），CREATE INDEX CONCURRENTLY 不能在事务块内运行
docker compose exec -T postgres psql -v ON_ERROR_STOP=1 -U postgres -d risk_engine <<'SQL'
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_access_logs_domain_trgm ON access_logs USING gin (domain gin_trgm_ops);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_access_logs_page_path_trgm ON access_logs USING gin (page_path gin_trgm_ops);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_access_logs_country_ip2location_trgm ON access_logs USING gin (country_ip2location gin_trgm_ops);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_access_logs_country_maxmind_trgm ON access_logs USING gin (country_maxmind gin_trgm_ops);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_access_logs_max_city ON access_logs(max_city);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_access_logs_max_asn ON access_logs(max_asn);
SQL

echo "Indexes created (existing ones skipped)."
```

- [ ] **Step 2: 赋予执行权限**

```bash
git update-index --chmod=+x docker/create-indexes.sh
```

- [ ] **Step 3: Commit**

```bash
git add docker/create-indexes.sh
git commit -m "feat(docker): add idempotent index creation script"
```

---

## Task 8: docs/docker-deployment.md 使用教程

**Files:**
- Create: `docs/docker-deployment.md`

**Interfaces:**
- Consumes: Task 1–7 全部产物
- Produces: 面向新服务器/测试机的完整部署教程（中文）

- [ ] **Step 1: 编写教程**

内容结构（完整 Markdown 见实现）：

1. **架构说明**：三容器 + binfiles 宿主机挂载图示，与 systemd 方案的关系（并存）
2. **前置条件**：CentOS/OpenCloudOS 安装 Docker + compose 插件命令（`dnf install -y docker-ce docker-compose-plugin` + systemd enable），服务器规格建议（IP2Proxy CSV 约 130MB 内存 + maxmind mmap）
3. **首次部署**：
   ```bash
   git clone https://github.com/JR-coderli/efilter.git /opt/efilter && cd /opt/efilter
   cp docker/.env.example docker/.env && vim docker/.env   # 改密码/API Key
   bash tools/update-ipdb/update-ipdb.sh                    # 首次下载 IP 库（需 .env 里的 token）
   docker compose -f docker/docker-compose.yml up -d --build
   bash docker/create-indexes.sh                            # 首次建索引
   ```
4. **验证**：`curl http://127.0.0.1:8080/health`、results 接口 curl 示例、面板 `http://IP:8080/`、`docker compose ps` 健康状态说明
5. **MaxMind 首次数据**：GeoIP.local.conf 放仓库根，`GEOIP_CONF=/opt/efilter/GeoIP.local.conf bash tools/update-ipdb/update-maxmind.sh`（说明脚本默认路径与仓库模板位置不一致，必须显式传 GEOIP_CONF）
6. **每日更新 crontab**（IP2Location 3:00、MaxMind 3:10，均追加 `docker compose -f docker/docker-compose.yml restart app`）
7. **升级**：`git pull && docker compose -f docker/docker-compose.yml up -d --build`
8. **常用命令**：logs -f app、ps、restart、down（保留卷）/ down -v（连数据一起删，危险提示）、进 postgres 容器 psql
9. **Nginx 反代**（可选）：宝塔或独立 nginx 指向 `127.0.0.1:8080` 的 server 块示例
10. **故障排查**：app 起不来查 logs（常见：binfiles 未下载/目录名不符）、503 database unavailable 含义（新版本语义：服务仍可用，仅日志面板不可用）、postgres 数据卷权限、时区

- [ ] **Step 2: Commit**

```bash
git add docs/docker-deployment.md
git commit -m "docs: add docker deployment tutorial"
```

---

## Task 9: 语法校验与收尾

**Files:**
- 无新文件；校验 Task 1–8 产物

**Interfaces:**
- Consumes: 全部 docker/ 文件

- [ ] **Step 1: compose 语法校验（若本机有 Docker）**

```bash
docker compose -f docker/docker-compose.yml config --quiet && echo "compose syntax OK"
```

若本机无 Docker（Windows 开发机可能没有），跳过并在测试机执行同样命令。

- [ ] **Step 2: Dockerfile 关键点人工复核**

- [ ] 构建上下文排除生效：`.dockerignore` 含 `tools/go/`、`binfiles/`
- [ ] `COPY backend/risk-engine/go.mod backend/risk-engine/go.sum ./` 两个文件都存在于仓库
- [ ] `WORKDIR /app/backend/risk-engine` + `COPY frontend/ /app/frontend` 路径对应 `../../frontend`
- [ ] `docker/config.docker.yaml` 存在且 `host=postgres`、`redis:6379`

- [ ] **Step 3: 测试机验证清单（交给用户在 OpenCloudOS 测试机执行）**

```bash
cd /opt/efilter && git pull
docker compose -f docker/docker-compose.yml config --quiet
docker compose -f docker/docker-compose.yml up -d --build
docker compose -f docker/docker-compose.yml ps
curl http://127.0.0.1:8080/health
curl -s -X POST http://127.0.0.1:8080/api/v1/results -H "X-API-Key: <key>" \
  -H "Content-Type: application/x-www-form-urlencoded" -d "ip=8.8.8.8&country=US"
bash docker/create-indexes.sh
```

- [ ] **Step 4: 汇总推送**

```bash
git push origin main
```

---

## Self-Review

1. **Spec coverage:**
   - Dockerfile ✅ Task 3；compose ✅ Task 6；config.docker.yaml ✅ Task 2
   - .env.example + gitignore ✅ Task 5；.dockerignore ✅ Task 1
   - init SQL（pg_trgm）✅ Task 4；create-indexes.sh ✅ Task 7
   - 使用教程 ✅ Task 8；校验 ✅ Task 9
   - 设计文档的「运维流程」全部落入教程 Task 8 ✅

2. **Placeholder scan:** 无 TBD/TODO；每个文件给出完整内容。Task 7 heredoc 内 echo 行已在正文显式澄清最终写法。

3. **Type consistency:**
   - compose 引用变量 `POSTGRES_PASSWORD`/`RISK_DATABASE_DSN`/`RISK_APP_API_KEY`/`APP_PORT`/`TZ` 与 `.env.example` 键一致 ✅
   - `RISK_` 前缀与 viper `SetEnvPrefix("RISK")` + `.` → `_` 替换一致（`RISK_DATABASE_DSN` → `database.dsn`）✅
   - 镜像路径 `/app/backend/risk-engine` + `../../binfiles`、`../../frontend` 与 Go 代码 `getFrontendDir`/config 相对路径一致 ✅
   - `docker compose exec -T postgres` 服务名与 compose `services.postgres` 一致 ✅

4. **Gap 说明:**
   - 本地 Windows 开发机可能无 Docker，build 验证移至测试机（Task 9 Step 3 明确）。
   - `postgres:18-alpine` 镜像 tag 以测试机 `docker pull` 实际结果为准；如不存在回退 `postgres:17-alpine` 并同步改 DSN 无需（DSN 兼容）。
