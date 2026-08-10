# efilter

广告流量风控 API 系统，用于检测 IP 风险（VPN / Proxy / 数据中心 / Tor）、国家地区判断，并配合 PHP 落地页进行流量过滤。

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

## 目录结构

```
efilter/
├── backend/
│   ├── docs/                   # 开发文档、PHP 配合示例
│   └── risk-engine/            # Go 服务
├── binfiles/                   # IP 数据库 BIN 文件
├── frontend/
│   └── index.html              # 风控记录面板
├── tools/
│   ├── update-ipdb/            # IP 数据库自动更新脚本
│   └── deploy/                 # CentOS 一键部署脚本
├── .env.example                # 环境变量模板
└── CLAUDE.md                   # 项目交接文档
```

## 快速启动（本地）

```bash
cd backend/risk-engine
go build -o risk-engine ./cmd/server/main.go
./risk-engine
```

默认监听 `http://127.0.0.1:8080`。

## 生产部署（CentOS）

```bash
sudo bash tools/deploy/deploy.sh
```

脚本会自动安装 Go、PostgreSQL、Redis、Nginx，拉取代码、编译、初始化数据库、下载 IP 数据库并配置 systemd。

## 默认 API Key

```text
risk-engine-dev-key-2026
```

## 相关文档

- [CLAUDE.md](CLAUDE.md) — 项目记忆、关键决策、运行环境
- [backend/risk-engine/README.md](backend/risk-engine/README.md) — 后端服务详细说明
- [backend/docs/risk-engine-development.md.md](backend/docs/risk-engine-development.md.md) — 原始开发 PRD
- [backend/docs/php配合.md](backend/docs/php配合.md) — PHP 落地页调用示例
