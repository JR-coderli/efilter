# efilter CentOS Stream 9 手动部署自查清单

> 用于在新服务器上手动安装环境后逐项检查，确认无误后再运行部署脚本或启动服务。

---

## 一、基础系统环境

| 检查项 | 预期结果 | 检查命令 | 备注 |
|--------|----------|----------|------|
| 系统版本 | CentOS Stream 9 x86_64 | `cat /etc/os-release` | |
| 防火墙 | 80/443 已开放，其他按需 | `sudo firewall-cmd --list-ports` | 宝塔面板会自动处理 |
| SELinux | 不阻止服务运行 | `getenforce` | 建议 `Permissive` 或已配置规则 |
| 时间同步 | 时间正确 | `timedatectl status` | |
| 磁盘空间 | 根分区至少 10GB 可用 | `df -h /` | IP 数据库约 500MB，日志会增长 |
| 内存 | 建议 ≥ 2GB | `free -h` | CSV 加载约 200MB |

---

## 二、宝塔面板及 Nginx

| 检查项 | 预期结果 | 检查命令 | 备注 |
|--------|----------|----------|------|
| 宝塔面板已安装 | 可登录面板 | `bt default` | 记录面板地址/账号/密码 |
| Nginx 已安装 | 运行中 | `systemctl status nginx` 或 `/www/server/nginx/sbin/nginx -v` | 宝塔面板内安装 |
| Nginx 配置可测试通过 | `syntax is ok` | `/www/server/nginx/sbin/nginx -t` | |

---

## 三、Go 环境

| 检查项 | 预期结果 | 检查命令 | 备注 |
|--------|----------|----------|------|
| Go 已安装 | go1.25.0 或更高 | `export PATH="/usr/local/go/bin:$PATH" && go version` | 如未安装，执行下方安装命令 |

**安装命令：**

```bash
GO_VERSION=1.25.0
wget -q "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" -O /tmp/go.tar.gz
rm -rf /usr/local/go
tar -C /usr/local -xzf /tmp/go.tar.gz
rm -f /tmp/go.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' > /etc/profile.d/go.sh
echo 'export GOPROXY=https://goproxy.cn,direct' >> /etc/profile.d/go.sh
source /etc/profile.d/go.sh
go version
```

---

## 四、PostgreSQL

| 检查项 | 预期结果 | 检查命令 | 备注 |
|--------|----------|----------|------|
| PostgreSQL 已运行 | active | `systemctl status pgsql` 或 `systemctl status postgresql-*` | 宝塔面板内安装 |
| 端口 5432 监听 | `localhost:5432` | `ss -tlnp | grep 5432` | |
| 数据库 risk_engine 已创建 | 存在 | `su - postgres -c "psql -l"` 或宝塔内查看 | |
| 用户 postgres 密码正确 | 可用密码登录 | `PGPASSWORD=你的密码 psql -U postgres -d risk_engine -h localhost -c "SELECT 1;"` | 建议先设成 admin123，后续再改 |

**宝塔安装路径参考：** `/www/server/pgsql`

---

## 五、Redis

| 检查项 | 预期结果 | 检查命令 | 备注 |
|--------|----------|----------|------|
| Redis 已运行 | active | `systemctl status redis` | 宝塔面板内安装 |
| 端口 6379 监听 | `127.0.0.1:6379` | `ss -tlnp | grep 6379` | 本项目生产用 6379 |
| 无密码或密码已配置 | 可连接 | `redis-cli ping` | |

---

## 六、项目代码

| 检查项 | 预期结果 | 检查命令 | 备注 |
|--------|----------|----------|------|
| 代码已 clone 到 /opt/efilter | 存在 | `ls /opt/efilter` | |
| 在 main 分支 | 最新 | `cd /opt/efilter && git branch && git log --oneline -3` | |
| `.env` 已配置 | 包含真实 token | `cat /opt/efilter/.env` | URL 必须加双引号 |
| `configs/config.yaml` 已调整 | 与服务器环境一致 | `cat /opt/efilter/backend/risk-engine/configs/config.yaml` | 重点看 redis、database、ipdb 路径 |

---

## 七、IP 数据库文件

| 检查项 | 预期结果 | 检查命令 | 备注 |
|--------|----------|----------|------|
| IP2Location BIN 存在 | 文件非空 | `ls -lh /opt/efilter/binfiles/IP2LOCATION-LITE-DB1.IPV6.BIN/` | |
| IP2Proxy BIN 存在 | 文件非空 | `ls -lh /opt/efilter/binfiles/IP2PROXY-LITE-PX2.BIN/` | |
| IP2Proxy IPv6 CSV 存在 | 文件非空 | `ls -lh /opt/efilter/binfiles/IP2PROXY-LITE-PX2.IPV6.CSV/` | |

**如不存在，运行：**

```bash
cd /opt/efilter
sudo bash tools/update-ipdb/update-ipdb.sh
```

> 注意：IP2Location 免费 token 每 24 小时限下载 5 次，谨慎执行。

---

## 八、编译服务

| 检查项 | 预期结果 | 检查命令 | 备注 |
|--------|----------|----------|------|
| 可成功编译 | 生成 risk-engine 二进制 | `cd /opt/efilter/backend/risk-engine && export PATH="/usr/local/go/bin:$PATH" && go build -o risk-engine ./cmd/server/main.go` | |

---

## 九、systemd 服务

| 检查项 | 预期结果 | 检查命令 | 备注 |
|--------|----------|----------|------|
| efilter.service 已创建 | 存在 | `cat /etc/systemd/system/efilter.service` | 参考下方配置 |
| 服务可启动 | active (running) | `sudo systemctl restart efilter && sleep 2 && sudo systemctl status efilter` | |
| 开机自启 | enabled | `sudo systemctl is-enabled efilter` | |

**服务配置参考：**

cat > /etc/systemd/system/efilter.service <<'EOF'
[Unit]
Description=efilter risk engine service
After=network.target postgresql.service redis.service

[Service]
Type=simple
User=root
WorkingDirectory=/opt/efilter/backend/risk-engine
ExecStart=/opt/efilter/backend/risk-engine/risk-engine
Restart=always
RestartSec=5
Environment="GIN_MODE=release"

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable efilter
systemctl start efilter
sleep 2
systemctl status efilter --no-pager
---

## 十、Nginx 反向代理（宝塔）

| 检查项 | 预期结果 | 检查命令 | 备注 |
|--------|----------|----------|------|
| 站点已添加 | 宝塔面板内可见 | 登录宝塔 → 网站 | 根目录 `/opt/efilter`，PHP 选纯静态 |
| 配置文件已应用 | 无 404/502 | `curl -s http://127.0.0.1/health` 通过 Nginx 返回 | |
| 外网可访问 | 返回 JSON | `curl -s http://你的服务器IP/health` | |

**配置文件来源：** `/opt/efilter/tools/deploy/nginx-bt.conf`

---

## 十一、API 功能验证

| 检查项 | 预期结果 | 检查命令 | 备注 |
|--------|----------|----------|------|
| health 正常 | `{"code":0,...}` | `curl -s http://127.0.0.1:8080/health` | |
| results 接口正常 | `{"result":true/false}` | `curl -s -X POST http://127.0.0.1:8080/api/v1/results -H "X-API-Key: risk-engine-dev-key-2026" -d "ip=8.8.8.8" -d "country=US"` | |
| check 接口正常 | 返回详细 JSON | `curl -s -X POST http://127.0.0.1:8080/api/v1/check -H "X-API-Key: risk-engine-dev-key-2026" -H "Content-Type: application/json" -d '{"ip":"8.8.8.8"}'` | |
| IPv6 CSV 生效 | 命中代理返回 is_proxy=true | `curl -s -X POST http://127.0.0.1:8080/api/v1/check -H "X-API-Key: risk-engine-dev-key-2026" -H "Content-Type: application/json" -d '{"ip":"::ffff:1.0.19.240"}'` | |

---

## 十二、定时任务

| 检查项 | 预期结果 | 检查命令 | 备注 |
|--------|----------|----------|------|
| crontab 已设置 | 存在 | `sudo crontab -l` | |

**应包含：**

```cron
0 2 * * * /opt/efilter/tools/update-ipdb/update-ipdb.sh >> /opt/efilter/logs/ipdb-update.log 2>&1 && /usr/bin/systemctl restart efilter
```

---

## 十三、安全加固（可选但建议）

| 检查项 | 预期结果 | 检查命令/操作 | 备注 |
|--------|----------|---------------|------|
| 修改 PostgreSQL 默认密码 | 非 admin123 | 宝塔面板内修改 | 同步改 config.yaml |
| 修改默认 API Key | 非 risk-engine-dev-key-2026 | 改数据库 api_keys 表 | 同步改前端和调用方 |
| 配置 HTTPS | 443 可用 | 宝塔面板申请 SSL | |
| 限制 SSH 端口 | 非 22 或已配密钥 | `ss -tlnp | grep sshd` | |

---

## 反馈模板

填好以下内容后发给部署助手：

```text
服务器 IP：
宝塔面板版本：
Nginx 版本：
PostgreSQL 版本及路径：
Redis 版本及端口：
Go 版本：

各检查项结果（通过/未通过）：
- 基础系统环境：
- 宝塔面板及 Nginx：
- Go 环境：
- PostgreSQL：
- Redis：
- 项目代码：
- IP 数据库文件：
- 编译服务：
- systemd 服务：
- Nginx 反向代理：
- API 功能验证：
- 定时任务：

未通过项及报错信息：
```

---

最后更新：2026-08-11
