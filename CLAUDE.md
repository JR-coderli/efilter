# efilter 项目记忆 / 交接文档

> 本文件记录 risk-engine 项目的关键决策、运行环境和重要实现逻辑，方便后续继续开发或交接。

## 项目目标

构建本地部署的广告流量风控 API 系统，用于：

- IP 信息查询（IPv4 / IPv6）
- VPN / Proxy / 数据中心 / Tor 检测
- 国家地区判断
- 广告流量质量过滤（配合 PHP 落地页）

## 关键文件位置

| 文件 | 说明 |
|------|------|
| `backend/docs/risk-engine-development.md.md` | 原始开发 PRD |
| `backend/docs/php配合.md` | PHP 落地页调用示例 |
| `backend/risk-engine/` | Go 服务代码 |
| `backend/risk-engine/configs/config.yaml` | 服务配置 |
| `backend/risk-engine/logs/app.log` | 运行日志 |
| `.env` | 本地环境变量配置（gitignored） |
| `.env.example` | 环境变量模板 |
| `frontend/index.html` | 风控记录面板 |
| `tools/go/` | 本地 Go 环境 |
| `tools/update-ipdb/` | IP 数据库自动更新脚本 |
| `tools/deploy/` | CentOS 一键部署脚本 |

## 本地运行依赖

### Go 环境

无需系统安装 Go，使用项目内版本：

```bash
export PATH="/g/【CPL】/CODE/efilter/tools/go/bin:$PATH"
go version  # go1.25.0
```

### Redis

本机 6379 端口被占用，当前 Redis 运行在 **6380** 端口：

```bash
/c/laragon/bin/redis/redis-x64-5.0.14.1/redis-server.exe --port 6380 --daemonize yes
```

对应配置：

```yaml
# backend/risk-engine/configs/config.yaml
redis:
  addr: "127.0.0.1:6380"
```

### PostgreSQL

本地使用 PostgreSQL 18（Windows 安装版）：

```bash
export PATH="/c/Program Files/PostgreSQL/18/bin:$PATH"
export PGPASSWORD=admin123

# 启动服务
pg_ctl -D "C:/Program Files/PostgreSQL/18/data" start

# 连接
psql -U postgres -d risk_engine
```

注意：安装后 `postgresql.conf` 文件可能带 UTF-8 BOM 头，导致 PostgreSQL 启动失败。需要去掉 BOM 后再启动。

对应配置：

```yaml
# backend/risk-engine/configs/config.yaml
database:
  driver: "postgres"
  dsn: "host=localhost user=postgres password=admin123 dbname=risk_engine port=5432 sslmode=disable TimeZone=Asia/Shanghai"
```

如需切换回 SQLite/MySQL，可修改 `internal/database/db.go` 和 `configs/config.yaml`。

### Redis

| 类型 | 路径 | 用途 |
|------|------|------|
| IP2Location | `binfiles/IP2LOCATION-LITE-DB1.IPV6.BIN/IP2LOCATION-LITE-DB1.IPV6.BIN` | 国家/地区查询，支持 IPv4 + IPv6 |
| IP2Proxy | `binfiles/IP2PROXY-LITE-PX2.BIN/IP2PROXY-LITE-PX2.BIN` | 代理/VPN/数据中心检测 |

**注意：** 文件名带 `.IPV6` 的版本才同时支持 IPv4 与 IPv6；不带 `.IPV6` 的 DB 文件（如 `IP2LOCATION-LITE-DB3.BIN`）通常为 IPv4 only，查询 IPv6 会报错。

## 接口说明

### 1. POST /api/v1/check

完整风控详情接口，返回 IP、国家、ISP、ASN、代理类型、风险评分、建议动作等。

支持 IPv4 和 IPv6。

### 2. POST /api/v1/results

为 PHP 落地页提供的布尔过滤接口。

请求：`x-www-form-urlencoded`，字段 `ip`（必填）、`country`（可选）

响应：`{"result": true/false}`

判断逻辑：

1. 白名单 → true
2. 黑名单 → false
3. Proxy / VPN / Tor / 数据中心 → false
4. 查不到国家 → true（默认放行）
5. `country` 为 `ALL` 或空 → true
6. IP 国家在目标列表中 → true
7. 其他 → false

## 默认 API Key

```text
risk-engine-dev-key-2026
```

传递方式：`X-API-Key` 请求头、URL 参数 `api_key`、或 `Authorization: Bearer <key>`。

## 风险评分规则

| 条件 | 加分 | 说明 |
|------|------|------|
| VPN | +30 | |
| Proxy | +40 | |
| Datacenter / Hosting | +30 | |
| Tor | +80 | |
| 黑名单 | +100 | 直接 block |

分数区间：

- 0–29：safe
- 30–69：review
- 70–100：block

## 限流

每个 API Key 固定窗口限流。当前配置为 **QPS 2000**：窗口 1 秒，每个 API Key 每窗口最多 2000 次请求（生产实际 QPS 约 200，留有约 10 倍余量）。

## 重要实现细节

- `internal/database/ipdb.go`：统一封装 IP2Location 和 IP2Proxy 查询，自动清理 `"UNAVAILABLE"`、`"NOT SUPPORTED"`、`"N/A"` 等占位字符串。
- `internal/service/risk.go`：`Filter()` 用于 `/api/v1/results`，`Check()` 用于 `/api/v1/check`。
- 数据库模型：`users`、`api_keys`、`risk_rules`、`ip_blacklists`、`ip_whitelists`。
- 启动时会自动迁移并写入默认用户、API Key 和风险规则。
- **IP 数据源使用本地 BIN 文件直接读取**，不进入 PostgreSQL。原因见下方「生产环境数据源方案」。
- **Redis 当前主要用于 API Key 限流**。由于广告流量 IP 重复率极低，缓存 IP 查询结果命中率不高，收益有限；且切换 IP 库时易造成缓存脏读，后续建议关闭 IP 结果缓存以简化链路。

## 生产环境数据源方案

**推荐：直接读取本地 BIN 文件 + 定时自动更新。**

不建议把 IP2Location / IP2Proxy 数据导入 PostgreSQL 或 Redis 作为查询源。

| 方案 | 查询延迟 | 稳定性 | 运维复杂度 | 建议 |
|------|---------|--------|------------|------|
| 直接读 BIN | 0.01~0.1ms | 高（无网络依赖） | 低 | ✅ 推荐 |
| 入库 PostgreSQL | 1~10ms | 中（受 DB/连接池影响） | 高（数据量大、导入重） | ❌ 不推荐 |
| 入 Redis | 0.2~1ms | 中（受网络 RTT 影响） | 中（数据量大、key 爆炸） | ❌ 不推荐 |

原因：

- BIN 文件本身是为高速 IP 查询优化的压缩索引结构，比 SQL/CIDR 查询快 1~2 个数量级。
- 生产 QPS 200，目标 1000~10000，单机 BIN 完全能支撑。
- 广告流量 IP 重复率低，Redis 缓存命中率有限，反而多一次网络往返。

**BIN 自动更新机制建议：**

已实现脚本：`tools/update-ipdb/update-ipdb.sh`

1. 每天凌晨低峰期从 IP2Location/IP2Proxy 官方下载最新 **zip**。
2. 解压 zip 到临时目录。
3. 查找 `.BIN` 文件，复制到目标目录的临时文件，再用 `mv` 原子替换正式文件。
4. 清理临时目录。
5. 更新完成后重启 `risk-engine` 服务加载新 BIN。
6. 多机部署时，可每台机器独立跑更新脚本，或从对象存储/NFS 同步。

下载地址记录在 `.env`（本地）和 `.env.example`（模板）：

```text
IP2LOCATION_URL=https://www.ip2location.com/download?token=...&file=DB1LITEBINIPV6
IP2PROXY_URL=https://www.ip2location.com/download?token=...&file=PX2LITEBIN
```

> Linux 不能直接读取 zip 内的 BIN，必须先解压成文件再供服务读取。

## 访问记录与前端面板

**访问记录模型：** `models.AccessLog`

每次 API 请求（`/api/v1/check`、`/api/v1/results`）都会异步写入 PostgreSQL：

- `request_id`、`client_ip`、`method`、`path`
- `country`、`risk_score`、`action`、`rule_hit`
- `request_body`、`response_body`
- `status_code`、`response_time_ms`、`created_at`

**数据保留策略：** 24 小时。服务启动后会在后台启动一个每小时运行一次的清理任务，删除 `created_at < now() - 24h` 的记录。

**前端面板：**

- URL：`/` 或 `/dashboard/`
- 实时展示最近访问记录、放行/拦截统计、最近 1 小时请求数
- 支持按 IP、接口路径筛选
- 每 30 秒自动刷新
- 数据来源：`GET /api/v1/logs?limit=100&offset=0&ip=&path=`

## 生产部署

已提供 CentOS 一键部署脚本：

```bash
sudo bash tools/deploy/deploy.sh
```

脚本会自动完成：安装 Go/PostgreSQL/Redis/Nginx、拉取代码、编译服务、初始化数据库、下载 IP 数据库、配置 systemd + Nginx、启动服务。

生产部署后：

- 面板：`http://服务器IP/`
- API：`http://服务器IP/api/v1/`

## 后续扩展方向

- 后台管理接口（规则/黑白名单/API Key CRUD）
- Docker / Systemd / Nginx 部署
- Device Fingerprint
- Click Tracking + ClickHouse 分析
- 更细粒度的 results 策略（如单独允许 residential proxy）
- 关闭或可选化 Redis IP 查询结果缓存，简化请求链路

## 最后更新

- 2026-08-10：新增前端访问记录面板 `/dashboard/`，访问日志写入 PostgreSQL 并保留 24 小时；新增 `GET /api/v1/logs`。
- 2026-08-10：新增 `.gitignore`、`tools/deploy/deploy.sh` CentOS 一键部署脚本；`.env.production.example` 重命名为 `.env`（本地），并新增 `.env.example` 模板。
- 2026-08-10：新增 `tools/update-ipdb/update-ipdb.sh` 自动更新脚本，支持下载 zip、解压、原子替换 BIN；下载地址记录到 `.env` / `.env.example`。
- 2026-08-10：补充生产环境 IP 数据源方案说明：推荐直接读 BIN + 定时更新，不入库；Redis 主要用于限流，IP 结果缓存收益有限。
- 2026-08-10：本地数据库从 SQLite 切换到 PostgreSQL，新增 `.env.production.example` 环境变量示例。
- 2026-08-10：新增 `/api/v1/results` 接口，接入 IPv6 数据源，Redis 改用 6380 端口。
