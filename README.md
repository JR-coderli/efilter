# efilter

广告流量风控 API 系统，用于检测 IP 风险（VPN / Proxy / 数据中心 / Tor）、国家地区判断，并配合 PHP 落地页进行流量过滤。

GitHub：https://github.com/JR-coderli/efilter

## 主要功能

- `POST /api/v1/check`：完整风控详情接口
- `POST /api/v1/results`：PHP 落地页布尔过滤接口
- `GET /api/v1/logs`：访问记录查询接口
- `/dashboard/`：前端风控记录面板
- 访问日志写入 PostgreSQL，保留 24 小时自动清理
- API Key 认证 + QPS 限流

## 技术栈

- Go + Gin
- PostgreSQL（GORM）
- Redis
- IP2Location / IP2Proxy BIN 本地查询
- IP2Proxy IPv6 CSV 内存加载查询
- Nginx 反向代理

## 目录结构

```
efilter/
├── backend/
│   ├── docs/                   # 开发文档、PHP 配合示例
│   └── risk-engine/            # Go 服务
├── binfiles/                   # IP 数据库 BIN 文件（不上传 Git）
├── frontend/
│   └── index.html              # 风控记录面板
├── tools/
│   ├── update-ipdb/            # IP 数据库自动更新脚本
│   └── deploy/                 # CentOS 一键部署脚本
├── .env                        # 本地环境变量（gitignored）
├── .env.example                # 环境变量模板
├── .gitignore                  # Git 忽略规则
├── README.md                   # 本文件
└── CLAUDE.md                   # 项目交接文档
```

## 快速启动（本地）

前置条件：

- PostgreSQL 已启动，数据库 `risk_engine` 已创建
- Redis 已启动
- IP 数据库 BIN 文件已放到 `binfiles/` 目录

```bash
cd backend/risk-engine
go build -o risk-engine ./cmd/server/main.go
./risk-engine
```

默认监听 `http://127.0.0.1:8080`。

本地 Windows 可使用项目内 Go：

```bash
export PATH="/g/【CPL】/CODE/efilter/tools/go/bin:$PATH"
go version  # go1.25.0
```

## 生产部署（CentOS）

```bash
# 方式 1：clone 后执行
git clone https://github.com/JR-coderli/efilter.git /opt/efilter
cd /opt/efilter
sudo bash tools/deploy/deploy.sh

# 方式 2：直接远程执行
sudo bash -c "$(curl -fsSL https://raw.githubusercontent.com/JR-coderli/efilter/main/tools/deploy/deploy.sh)"
```

脚本会自动安装 Go、PostgreSQL、Redis、Nginx，拉取代码、编译、初始化数据库、下载 IP 数据库并配置 systemd。

**部署后必做：**

1. 修改默认 PostgreSQL 密码（当前 `admin123`）。
2. 修改 `backend/risk-engine/configs/config.yaml` 中的数据库 DSN。
3. 修改默认 API Key：`risk-engine-dev-key-2026`。
4. 替换 `.env` 中的 `YOUR_TOKEN` 为真实 IP2Location 下载 token。
5. 配置防火墙，只开放 80/443。
6. 建议配置 HTTPS。
7. 设置 crontab 定时更新 IP 数据库：

```bash
sudo crontab -e
# 添加：
0 3 * * * /opt/efilter/tools/update-ipdb/update-ipdb.sh >> /opt/efilter/logs/ipdb-update.log 2>&1 && /usr/bin/systemctl restart efilter
```

## 默认 API Key

```text
risk-engine-dev-key-2026
```

## 相关文档

- [CLAUDE.md](CLAUDE.md) — 项目记忆、关键决策、运行环境、部署后管理
- [backend/risk-engine/README.md](backend/risk-engine/README.md) — 后端服务详细说明
- [backend/docs/risk-engine-development.md.md](backend/docs/risk-engine-development.md.md) — 原始开发 PRD
- [backend/docs/php配合.md](backend/docs/php配合.md) — PHP 落地页调用示例
- [tools/deploy/README.md](tools/deploy/README.md) — 部署脚本详细说明
- [tools/update-ipdb/README.md](tools/update-ipdb/README.md) — IP 数据库自动更新说明
