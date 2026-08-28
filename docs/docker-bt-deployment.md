# 宝塔 Docker 部署指南

> 适用：习惯用宝塔面板管理的服务器。用宝塔的 Docker 编排功能拉起整套服务（risk-engine + PostgreSQL + Redis），日常启停在面板里点按钮。
> 纯命令行部署请看 [docker-deployment.md](docker-deployment.md)。**两套方案只能选一套**（容器名相同会冲突），但底层文件完全一致，随时可切换。

## 1. 与标准 Docker 部署的差异

| 差异点 | 标准版（docker-compose.yml） | 宝塔版（docker-compose.bt.yml） |
|--------|------------------------------|--------------------------------|
| compose 存放位置 | 仓库 `docker/` 目录 | 宝塔编排目录（面板自己管理） |
| 挂载路径 | 相对路径 `../binfiles` | **绝对路径** `/opt/efilter/...`（宝塔目录下相对路径会失效） |
| 镜像构建 | `compose up --build` 一步完成 | **先 SSH 预构建镜像**，编排里只有 `image:` |
| 敏感变量 | `${}` 从 `.env` 插值 | `env_file:` 绝对路径注入（宝塔目录下没有 .env 可插值） |
| 日常管理 | SSH 命令 | 宝塔 Docker 编排界面（启动/停止/重建/日志） |

专用编排文件在仓库里：`docker/docker-compose.bt.yml`。

## 2. 前置：宝塔安装 Docker

1. 宝塔面板 → **软件商店** → 搜索 **Docker** → 安装（新版面板为左侧 **Docker** 菜单自带的安装引导）。
2. 安装完成后 SSH 确认：

```bash
docker version && docker compose version
```

3. 服务器规格建议与标准版相同：内存 ≥ 2GB，磁盘空闲 ≥ 10GB。

## 3. 首次部署（SSH 操作部分）

宝塔 UI 不负责构建，以下都在 SSH 里完成：

```bash
# 1. 拉代码（默认 /opt/efilter；换目录需同步改 docker-compose.bt.yml 里所有绝对路径）
git clone https://github.com/JR-coderli/efilter.git /opt/efilter
cd /opt/efilter

# 2. 根目录 .env（IP 库下载地址）
cp .env.example .env && vim .env

# 3. docker/.env（数据库密码 / API Key）
cp docker/.env.example docker/.env && vim docker/.env

# 4. 首次下载 IP 数据库
bash tools/update-ipdb/update-ipdb.sh

# 5. MaxMind 首次下载（可选，需 GeoIP.local.conf）
# GEOIP_CONF=/opt/efilter/GeoIP.local.conf bash tools/update-ipdb/update-maxmind.sh

# 6. 预构建镜像（只需在部署和代码升级时执行）
docker build -f /opt/efilter/docker/Dockerfile -t efilter/risk-engine:latest /opt/efilter
```

## 4. 宝塔面板拉起编排

1. 宝塔面板 → 左侧 **Docker** → **编排**（旧版在 Docker 管理器里）→ **添加编排**。
2. 名称填 `efilter`，来源选 **粘贴 / 自定义**，内容粘贴服务器上的 `docker/docker-compose.bt.yml`：

```bash
cat /opt/efilter/docker/docker-compose.bt.yml
```

3. 提交并启动。宝塔会依次拉起 redis → postgres → app（app 等前两者 healthy 才启动）。
4. 编排列表里三个容器都显示运行中 / 健康即成功。

> ⚠ 粘贴前确认 `docker/.env` 已存在（第 3 节步骤 3），否则 `env_file` 找不到文件，编排直接报错——这是刻意的快速失败，防止用默认密码跑起来。

## 5. 首次建索引（SSH，一次性）

等 app 启动完成自动迁移（约 10–20 秒）后执行：

```bash
docker exec -i efilter-postgres psql -v ON_ERROR_STOP=1 -U postgres -d risk_engine <<'SQL'
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_access_logs_domain_trgm ON access_logs USING gin (domain gin_trgm_ops);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_access_logs_page_path_trgm ON access_logs USING gin (page_path gin_trgm_ops);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_access_logs_country_ip2location_trgm ON access_logs USING gin (country_ip2location gin_trgm_ops);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_access_logs_country_maxmind_trgm ON access_logs USING gin (country_maxmind gin_trgm_ops);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_access_logs_max_city ON access_logs(max_city);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_access_logs_max_asn ON access_logs(max_asn);
SQL
```

（宝塔编排的项目名与仓库 compose 不同，`create-indexes.sh` 里的 `docker compose exec` 定位不到，这里直接 `docker exec` 按容器名操作。）

## 6. 验证

```bash
docker ps --filter name=efilter        # 三个容器 Up（app 带 healthy）
curl http://127.0.0.1:8080/health
curl -s -X POST http://127.0.0.1:8080/api/v1/results \
  -H "X-API-Key: 你的APIKEY" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "ip=8.8.8.8&country=US"
```

面板地址：`http://服务器IP:8080/`（建议只在内网/反代后使用，见第 9 节）。

## 7. 每日更新：宝塔计划任务

宝塔面板 → **计划任务** → **添加计划任务**，建两个 **Shell 脚本** 任务：

**任务 1：IP2Location / IP2Proxy 更新（每天 3:00）**

```bash
cd /opt/efilter && bash tools/update-ipdb/update-ipdb.sh >> logs/ipdb-update.log 2>&1 && docker restart efilter-app
```

**任务 2：MaxMind 更新（每天 3:10）**

```bash
cd /opt/efilter && GEOIP_CONF=/opt/efilter/GeoIP.local.conf bash tools/update-ipdb/update-maxmind.sh >> logs/geoipupdate.log 2>&1 && docker restart efilter-app
```

> IP 库是挂载文件，`docker restart` 即可生效，无需重建镜像。

## 8. 代码升级

```bash
cd /opt/efilter && git pull
docker build -f /opt/efilter/docker/Dockerfile -t efilter/risk-engine:latest /opt/efilter
docker rm -f efilter-app
```

然后到宝塔 **Docker → 编排 → efilter** 点 **启动**（会按新镜像重建 app 容器；postgres/redis 不动）。

数据库结构由应用启动时 GORM 自动迁移；新增索引按需重跑第 5 节命令（幂等）。

## 9. 宝塔反向代理 + HTTPS（推荐）

1. **网站** → **添加站点**：域名填你的域名，纯静态即可（不放文件）。
2. 站点 **设置** → **反向代理** → **添加反向代理**：
   - 目标 URL：`http://127.0.0.1:8080`
   - 发送域名：`$host`
3. HTTPS：站点设置 → **SSL** → Let's Encrypt 申请（或宝塔 SSL），开启**强制 HTTPS**。
4. 防火墙：宝塔**安全**页 + 云厂商安全组只放行 `80/443`，**不要对外放行 8080**（可保留本机访问用于验证）。

## 10. 故障排查（宝塔相关）

| 现象 | 排查 |
|------|------|
| 编排启动报 env file not found | `docker/.env` 未创建，回看第 3 节步骤 3 |
| 编排启动报容器名冲突 | 本机已用标准版 compose（或旧容器残留）拉起过：`docker rm -f efilter-app efilter-postgres efilter-redis` 后重试；两套编排只能存在一套 |
| 挂载报 /opt/efilter/binfiles 不存在 | IP 库没下载（第 3 节步骤 4），或安装目录不是 /opt/efilter —— 后者需同步改 docker-compose.bt.yml 全部绝对路径 |
| postgres 密码改了不生效 | `POSTGRES_PASSWORD` 仅首次初始化数据卷时生效；要换密码需宝塔编排**停止 → SSH 删除 pgdata 卷（数据会清空）→ 重新启动** |
| app 反复重启 | 编排界面点 app **日志**，或 `docker logs efilter-app`；最常见是 binfiles 目录名与 config 不符 |
| 改了 docker/.env 不生效 | 宝塔编排界面**停止再启动**（restart 不会重读 env_file） |
| 升级后行为没变化 | 只 build 了镜像没重建容器：按第 8 节 `docker rm -f efilter-app` 后在宝塔点启动 |
| 时区不对 | 确认 `docker/.env` 里 `TZ=Asia/Shanghai`，编排停止→启动 |
