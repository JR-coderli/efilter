# 广告流量风控 API 系统（risk-engine）

基于开发文档 `backend/docs/risk-engine-development.md.md` 实现的本地 Go 服务。

## 已完成功能

- [x] Go 1.25 本地环境（无需系统安装）
- [x] Gin HTTP 服务
- [x] PostgreSQL 本地数据库 + GORM 模型/迁移（同时保留 MySQL / SQLite 支持）
- [x] Redis 缓存与限流
- [x] IP2Location / IP2Proxy BIN 本地查询（IPv4）
- [x] IP2Proxy IPv6 CSV 内存加载查询（IPv6 Public Proxy fallback）
- [x] 风险评分引擎（VPN / Proxy / Datacenter / Tor / 黑名单 / 白名单 / 动态规则）
- [x] API Key 认证 + 固定窗口限流（当前 QPS 上限 2000）
- [x] 统一访问日志（`logs/app.log`）
- [x] `POST /api/v1/check` 接口（完整风控详情）
- [x] `POST /api/v1/results` 接口（**配合 PHP 落地页过滤，返回布尔结果**）
- [x] 访问记录写入 PostgreSQL（保留 24 小时自动清理）
- [x] 前端风控面板 `/dashboard/`（展示记录、放行/拦截统计、自动刷新）
- [x] `GET /api/v1/logs` 接口（供面板查询访问记录）

## 目录

```
risk-engine/
├── cmd/server/main.go          # 服务入口
├── internal/
│   ├── api/                    # Handler + Router
│   ├── config/                 # 配置读取
│   ├── database/               # GORM、Redis、IPDB
│   ├── logger/                 # Zap 日志
│   ├── middleware/             # RequestID、APIKey、RateLimit、AccessLog
│   ├── models/                 # 数据模型
│   └── service/                # 风控评分服务
├── configs/config.yaml         # 配置文件
├── logs/app.log                # 运行日志
└── risk-engine.exe             # 编译产物

frontend/
└── index.html                  # 风控记录面板
```

## 快速启动

```bash
cd "G:/【CPL】/CODE/efilter/backend/risk-engine"
export PATH="/g/【CPL】/CODE/efilter/tools/go/bin:$PATH"

go build -o risk-engine.exe ./cmd/server/main.go
./risk-engine.exe
```

服务默认监听 `http://127.0.0.1:8080`。

> **注意：** 需要先启动 PostgreSQL 和 Redis。当前 Redis 因 6379 端口被占用，使用 **6380 端口**，见 `configs/config.yaml`。

## 依赖环境

- **PostgreSQL**：当前使用 `localhost:5432`，数据库 `risk_engine`，用户 `postgres` 密码 `admin123`
  - Windows 启动示例（使用已安装服务）：
    ```bash
    net start postgresql-x64-18
    ```
  - 若 `postgresql.conf` 存在 UTF-8 BOM 导致启动失败，可用 Python 去 BOM：
    ```bash
    python -c "import codecs; s=open('C:/Program Files/PostgreSQL/18/data/postgresql.conf','rb').read(); open('C:/Program Files/PostgreSQL/18/data/postgresql.conf','wb').write(s.lstrip(codecs.BOM_UTF8))"
    ```
  - 启动后确保已创建数据库 `risk_engine`：
    ```bash
    createdb -U postgres risk_engine
    ```
- **Redis**：当前运行在 `127.0.0.1:6380`（Laragon Redis 改端口启动）
  ```bash
  /c/laragon/bin/redis/redis-x64-5.0.14.1/redis-server.exe --port 6380 --daemonize yes
  ```
- **数据库**：默认使用 PostgreSQL（`risk_engine`，`localhost:5432`，用户 `postgres`）
- **IP 库**（相对路径，以 `backend/risk-engine` 工作目录为基准）：
  - IP2Location：`../../binfiles/IP2LOCATION-LITE-DB1.IPV6.BIN/IP2LOCATION-LITE-DB1.IPV6.BIN`（**支持 IPv4 + IPv6**）
  - IP2Proxy：`../../binfiles/IP2PROXY-LITE-PX2.BIN/IP2PROXY-LITE-PX2.BIN`（IPv4）
  - IP2Proxy IPv6 CSV：`../../binfiles/IP2PROXY-LITE-PX2.IPV6.CSV/IP2PROXY-LITE-PX2.IPV6.CSV`（IPv6 Public Proxy）
  - 注意：文件名带 `.IPV6` 的版本才同时支持 IPv4 与 IPv6；不带 `.IPV6` 的 DB 文件通常为 IPv4 only。
  - IP2Proxy LITE 的 IPv6 数据只有 CSV，启动时加载到内存，约 130MB；LITE 版仅含 `PUB` 类型。

## 默认 API Key

```text
risk-engine-dev-key-2026
```

可在请求头或 URL 中传递：

- `X-API-Key: risk-engine-dev-key-2026`
- `?api_key=risk-engine-dev-key-2026`

## 接口示例

### 健康检查

```bash
curl http://127.0.0.1:8080/health
```

### 风险检测（完整详情）

```bash
curl -X POST http://127.0.0.1:8080/api/v1/check \
  -H "Content-Type: application/json" \
  -H "X-API-Key: risk-engine-dev-key-2026" \
  -d '{"ip":"8.8.8.8"}'
```

IPv6 示例：

```bash
curl -X POST http://127.0.0.1:8080/api/v1/check \
  -H "Content-Type: application/json" \
  -H "X-API-Key: risk-engine-dev-key-2026" \
  -d '{"ip":"2001:4860:4860::8888"}'
```

响应示例：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "ip": "8.8.8.8",
    "country": "US",
    "city": "",
    "isp": "",
    "asn": "",
    "is_proxy": false,
    "is_vpn": false,
    "is_datacenter": false,
    "risk_score": 0,
    "action": "safe",
    "rule_hit": "",
    "request_id": "..."
  }
}
```

### PHP 落地页过滤（布尔结果）

配合 `backend/docs/php配合.md` 使用，返回 `{"result": true/false}`。

```bash
curl -X POST http://127.0.0.1:8080/api/v1/results \
  -H "X-API-Key: risk-engine-dev-key-2026" \
  -d "ip=8.8.8.8&country=US,AU,CA,GB,DE"
```

响应：

```json
{"result": true}
```

#### /api/v1/results 判断逻辑

按以下优先级返回 `result`：

1. IP 在白名单 → `true`
2. IP 在黑名单 → `false`
3. IP 是 Proxy / VPN / Tor / 数据中心 → `false`
4. IP 库查不到国家 → `true`（默认放行）
5. `country` 为 `ALL` 或空 → `true`
6. IP 国家在目标列表中 → `true`
7. 否则 → `false`

`country` 支持多国家逗号分隔，例如 `US,AU,CA`。

### 访问记录面板

打开浏览器访问：

```
http://127.0.0.1:8080/dashboard/
```

面板展示：

- 总请求数、放行数、拦截/审查数、最近 1 小时请求数
- 最近访问记录列表（时间、IP、接口、国家、风险分、动作、命中规则、耗时、状态、响应）
- 支持按 IP、接口路径筛选
- 每 30 秒自动刷新

数据通过 `GET /api/v1/logs` 读取，访问日志写入 PostgreSQL 并保留 24 小时。

## 切换数据库

项目 `internal/database/db.go` 已同时支持 PostgreSQL、MySQL、SQLite。

- **PostgreSQL**（当前默认）：
  ```yaml
  database:
    driver: "postgres"
    dsn: "host=localhost user=postgres password=admin123 dbname=risk_engine port=5432 sslmode=disable TimeZone=Asia/Shanghai"
  ```

- **MySQL**：
  ```yaml
  database:
    driver: "mysql"
    dsn: "root:@tcp(127.0.0.1:3306)/risk_engine?charset=utf8mb4&parseTime=True&loc=Local"
  ```

- **SQLite**：
  ```yaml
  database:
    driver: "sqlite"
    dsn: "file:G:/【CPL】/CODE/efilter/data/risk_engine.db?_pragma=foreign_keys(1)"
  ```

## 后续可扩展

- 后台管理接口（规则/黑白名单/API Key CRUD）
- Docker / Systemd / Nginx 部署
- Device Fingerprint、Click Tracking、ClickHouse 分析
- 根据业务需要，扩展 /api/v1/results 的代理类型判断（如单独允许 residential proxy）
