# 一键部署说明

## 准备工作

1. 将本项目 push 到 GitHub（或你的 Git 仓库）。
2. 确保 `.env.production.example` 中的下载 token 已替换为真实 token。
3. 准备一台 CentOS 7/8/Stream 服务器。

## 快速部署

在服务器上执行：

```bash
sudo bash -c "$(curl -fsSL https://raw.githubusercontent.com/JR-coderli/efilter/main/tools/deploy/deploy.sh)"
```

或者先 clone 再执行：

```bash
git clone https://github.com/JR-coderli/efilter.git /opt/efilter
cd /opt/efilter
sudo bash tools/deploy/deploy.sh
```

## 部署内容

脚本会自动完成：

- 安装依赖：Go、PostgreSQL、Redis、Nginx、unzip、curl
- 初始化 PostgreSQL 数据库 `risk_engine`
- 启动 Redis
- 从 GitHub 拉取/克隆代码
- 编译 `risk-engine`
- 下载/更新 IP 数据库（IP2Location + IP2Proxy）
- 创建 systemd 服务 `efilter.service`
- 配置 Nginx 反向代理
- 启动服务

## 部署后管理

```bash
# 查看服务状态
sudo systemctl status efilter

# 重启服务
sudo systemctl restart efilter

# 查看日志
sudo journalctl -u efilter -f

# 手动更新 IP 数据库
sudo bash /opt/efilter/tools/update-ipdb/update-ipdb.sh
sudo systemctl restart efilter

# 设置定时自动更新 IP 数据库
sudo crontab -e
# 添加：
0 3 * * * /opt/efilter/tools/update-ipdb/update-ipdb.sh >> /opt/efilter/logs/ipdb-update.log 2>&1 && /usr/bin/systemctl restart efilter
```

## 配置说明

生产环境配置：

- 服务端口：`8080`（Nginx 反代到 80）
- PostgreSQL：`localhost:5432/risk_engine`
- Redis：`localhost:6379`
- 工作目录：`/opt/efilter/backend/risk-engine`

如需修改数据库密码、Redis 端口等，请编辑：

- `backend/risk-engine/configs/config.yaml`
- `.env.production.example`
- `tools/deploy/deploy.sh` 顶部的配置项

## 安全建议

- 修改默认 PostgreSQL 密码
- 修改默认 API Key
- 配置 HTTPS（使用 certbot 或自签证书）
- 限制 `X-API-Key` 的泄露
