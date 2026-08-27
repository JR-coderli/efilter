# Docker 部署指南

> 适用：新服务器 / 测试机的标准化部署。一套 `docker compose` 起全部服务（risk-engine + PostgreSQL + Redis），宿主机只需安装 Docker。
> 与宝塔 + systemd 方案并存互不影响；已在跑 systemd 部署的机器无需迁移。

## 1. 架构说明

```
宿主机 /opt/efilter（git clone）
├── binfiles/  ←── 挂载 (ro) ───┐
│   ├── IP2LOCATION-LITE-DB1.IPV6.BIN/
│   ├── IP2PROXY-LITE-PX2.BIN/
│   ├── IP2PROXY-LITE-PX2.IPV6.CSV/
│   └── maxmind/*.mmdb           │
│      ↑ 宿主机 cron 更新（update-ipdb.sh / update-maxmind.sh）
│        更新后 docker compose restart app
│
└── docker/
    ├── Dockerfile            多阶段构建（golang:1.25-alpine → alpine:3.20，约 60–70MB）
    ├── docker-compose.yml    app + postgres + redis 三服务
    ├── .env                  密码 / API Key / 端口（gitignored，从 .env.example 复制）
    ├── config.docker.yaml    容器内配置（DSN host=postgres、Redis redis:6379）
    ├── create-indexes.sh     首次启动后建查询索引
    └── postgres/init/        首次建库时自动创建 pg_trgm 扩展

Docker 内部网络 efilter-net：
  app(:8080) ──→ postgres:5432（数据卷 pgdata 持久化）
             ──→ redis:6379（纯内存，仅限流）
宿主机仅暴露 app 端口（默认 8080）；postgres / redis 不对外。
```

要点：

- **IP 数据库不进镜像**：`binfiles/` 以只读方式挂载进容器。更新 = 宿主机脚本替换文件 + 重启 app 容器。
- **零代码改动**：镜像内复刻仓库目录结构（`/app/backend/risk-engine` + `/app/frontend` + `/app/binfiles`），所有相对路径与源码部署完全一致。
- **敏感信息不进镜像**：密码、API Key 通过环境变量（`RISK_DATABASE_DSN`、`RISK_APP_API_KEY`）从 `docker/.env` 注入，`.env` 已 gitignored。

## 2. 前置条件

### 2.1 安装 Docker（CentOS / OpenCloudOS）

```bash
# 安装 docker-ce 与 compose 插件
sudo dnf install -y dnf-utils
sudo dnf config-manager --add-repo https://download.docker.com/linux/centos/docker-ce.repo
sudo dnf install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin

# 启动并设置开机自启
sudo systemctl enable --now docker
docker version && docker compose version
```

### 2.2 服务器规格建议

| 资源 | 建议 | 说明 |
|------|------|------|
| 内存 | ≥ 2GB | IP2Proxy IPv6 CSV 内存约 130MB + MaxMind mmap 约 80MB + PostgreSQL + 系统 |
| 磁盘 | ≥ 10GB 空闲 | binfiles 约 600MB + PostgreSQL 数据 + 镜像 |

### 2.3 IP2Location 下载 token

`.env`（仓库根）中需配置 `IP2LOCATION_URL` / `IP2PROXY_URL` 等下载地址，参考 `.env.example`。

## 3. 首次部署

```bash
# 1. 拉代码
git clone https://github.com/JR-coderli/efilter.git /opt/efilter
cd /opt/efilter

# 2. 配置根目录 .env（IP 库下载地址用）
cp .env.example .env && vim .env

# 3. 配置 docker/.env（数据库密码 / API Key / 端口）
cp docker/.env.example docker/.env && vim docker/.env

# 4. 首次下载 IP 数据库（宿主机执行，需上面 .env 的 token）
bash tools/update-ipdb/update-ipdb.sh

# 5. MaxMind 首次下载（需要 GeoIP.local.conf，见第 5 节）
# GEOIP_CONF=/opt/efilter/GeoIP.local.conf bash tools/update-ipdb/update-maxmind.sh

# 6. 构建并启动全部服务
docker compose -f docker/docker-compose.yml up -d --build

# 7. 等 app 完成首次自动迁移（约 10–20 秒），创建查询索引
bash docker/create-indexes.sh
```

## 4. 验证

```bash
# 容器状态（三个都应为 Up / healthy）
docker compose -f docker/docker-compose.yml ps

# 健康检查
curl http://127.0.0.1:8080/health
# {"code":0,"message":"ok","data":{"status":"up"}}

# results 接口（PHP 落地页用的布尔过滤）
curl -s -X POST http://127.0.0.1:8080/api/v1/results \
  -H "X-API-Key: 你的APIKEY" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "ip=8.8.8.8&country=US"

# 面板
浏览器打开 http://服务器IP:8080/
```

## 5. MaxMind 数据配置

1. 参考 `GeoIP.conf.example`，把真实 `AccountID` / `LicenseKey` 写入仓库根的 `GeoIP.local.conf`（已 gitignored）。
2. **注意**：`update-maxmind.sh` 默认读取 `${PROJECT_ROOT}/configs/GeoIP.conf`，与仓库实际位置不一致，必须显式传 `GEOIP_CONF`：

```bash
GEOIP_CONF=/opt/efilter/GeoIP.local.conf bash tools/update-ipdb/update-maxmind.sh
```

3. 服务器需安装 `geoipupdate` 二进制（宿主机执行，不在容器内）。

## 6. 每日自动更新（crontab）

```bash
sudo crontab -e
```

添加：

```cron
# IP2Location / IP2Proxy 更新（3:00）+ 重启 app 容器
0 3 * * * cd /opt/efilter && bash tools/update-ipdb/update-ipdb.sh >> logs/ipdb-update.log 2>&1 && docker compose -f docker/docker-compose.yml restart app
# MaxMind 更新（3:10）+ 重启 app 容器
10 3 * * * cd /opt/efilter && GEOIP_CONF=/opt/efilter/GeoIP.local.conf bash tools/update-ipdb/update-maxmind.sh >> logs/geoipupdate.log 2>&1 && docker compose -f docker/docker-compose.yml restart app
```

> compose 文件内的相对挂载路径（`../binfiles`）以 compose 文件所在目录为基准，任意工作目录执行都正确。

## 7. 代码升级

```bash
cd /opt/efilter
git pull
docker compose -f docker/docker-compose.yml up -d --build
```

数据库结构由应用启动时 GORM 自动迁移；新增的 GIN 索引按需重跑 `bash docker/create-indexes.sh`（幂等）。

## 8. 常用命令

```bash
DC="docker compose -f docker/docker-compose.yml"

$DC ps                  # 状态
$DC logs -f app         # 跟踪应用日志（stdout）
$DC restart app         # 重启应用（IP 库更新后用）
$DC stop                # 停止全部（保留数据）
$DC down                # 停止并删除容器（数据卷 pgdata 保留）
$DC down -v             # ⚠️ 连数据卷一起删，数据库记录会丢失
$DC exec postgres psql -U postgres -d risk_engine   # 进数据库
$DC exec redis redis-cli                            # 进 redis
```

文件日志同时持久化在 `app-logs` 卷中：`docker volume inspect efilter_app-logs` 可查宿主机路径。

## 9. Nginx 反向代理（可选）

如需 80/443 端口或 HTTPS，在宿主机 nginx（宝塔或独立安装均可）加 server 块：

```nginx
server {
    listen 80;
    server_name your.domain.com;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
}
```

## 10. 故障排查

| 现象 | 排查 |
|------|------|
| app 容器反复重启 | `docker compose -f docker/docker-compose.yml logs app`；最常见是 `binfiles/` 未下载或目录名不符（脚本下载后的目录名需与 `docker/config.docker.yaml` 中路径一致） |
| 面板显示 `database unavailable`（503） | 应用以"无数据库"模式运行：results/check 接口仍可用，仅访问日志与面板不可用。检查 postgres 容器是否 healthy、`RISK_DATABASE_DSN` 密码是否与 `POSTGRES_PASSWORD` 一致 |
| postgres 首次起不来 | `logs postgres`；确认 `pgdata` 卷没有残留旧数据（`down -v` 后重试） |
| 时区不对 | 确认 `docker/.env` 的 `TZ=Asia/Shanghai`，重启栈 |
| 改了 `.env` 不生效 | `docker compose -f docker/docker-compose.yml up -d` 重建容器（restart 不会重读 env） |
| 索引脚本报表不存在 | app 首次启动自动迁移尚未完成，等 10–20 秒重试 |
