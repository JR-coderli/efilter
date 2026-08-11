# efilter 部署踩坑总结

> 记录 efilter 从本地开发到 CentOS/OpenCloudOS + 宝塔面板部署过程中遇到的问题、原因和解决方案，供后续参考。

---

## 1. Git 行尾问题导致 shell 脚本解析失败

**现象：** 执行 `tools/update-ipdb/update-ipdb.sh` 时报错：

```text
update-ipdb.sh: line 152: unexpected EOF while looking for matching `'"''
```

**原因：** Windows 上编辑后 CRLF 行尾（`\r\n`）上传到 Linux，bash 把 `\r` 当作命令的一部分。

**解决：**
- 仓库根目录添加 `.gitattributes`，强制 `*.sh` 使用 LF：

```gitattributes
*.sh text eol=lf
```

- 已污染的文件重新 checkout：

```bash
git rm --cached -r .
git reset --hard
```

---

## 2. `.env` 文件中 URL 包含 `&` 导致 source 时解析错乱

**现象：** 执行 `source .env` 后环境变量值被截断，curl 报 `URL rejected: Malformed input to a URL function`，下载 zip 失败。

**原因：** `.env` 里的 URL 形如：

```text
IP2LOCATION_URL=https://.../download?token=xxx&file=DB1LITEBINIPV6
```

`&` 在 shell 中被解释为后台运行符，导致整行被拆成多个后台命令。

**解决：** `.env` 中所有 URL 值用双引号包裹：

```text
IP2LOCATION_URL="https://.../download?token=xxx&file=DB1LITEBINIPV6"
IP2PROXY_URL="https://.../download?token=xxx&file=PX2LITEBIN"
IP2PROXY_IPv6_URL="https://.../download?token=xxx&file=PX2LITECSVIPV6"
```

---

## 3. IP2Location 下载次数限制

**现象：** 下载返回 HTML，内容包含：

```text
THIS FILE CAN ONLY BE DOWNLOADED 5 TIMES WITHIN 24 HOURS
```

zip 无法解压。

**原因：** IP2Location LITE 版免费 token 每 24 小时只能下载 5 次。

**解决：**
- 谨慎测试下载脚本，避免频繁手动触发。
- 失败时保留已有 BIN/CSV 文件，服务仍可正常启动。
- 考虑准备多个 token，或在测试时使用本地已有文件。

---

## 4. IP 数据库文件名/路径反复出错

**现象：** 服务启动报 `open ip2location db failed: no such file`，或下载后文件名变成 `IP2LOCATION.BIN`、`IP2LOCATION-LITE-DB1.IPV6.BIN.BIN`。

**原因：**
- 脚本早期用 `basename` 取目录名作为目标文件名，导致重复追加扩展名。
- zip 内实际文件名与目标配置不一致。

**解决：**
- 脚本里使用固定目标目录名和固定目标文件名，例如：

```text
IP2LOCATION-LITE-DB1.IPV6.BIN/IP2LOCATION-LITE-DB1.IPV6.BIN
IP2PROXY-LITE-PX2.BIN/IP2PROXY-LITE-PX2.BIN
IP2PROXY-LITE-PX2.IPV6.CSV/IP2PROXY-LITE-PX2.IPV6.CSV
```

- 配置文件 `configs/config.yaml` 与脚本目标路径保持一致。

---

## 5. Redis 端口不一致

**现象：** 服务启动报 Redis 连接失败。

**原因：** 本地开发用 6380，服务器上用 6379，配置未同步。

**解决：** 部署前确认 `configs/config.yaml` 中的 `redis.addr` 与服务器实际 Redis 端口一致。

---

## 6. PostgreSQL 冲突：系统服务 vs 宝塔 PostgreSQL

**现象：** 部署脚本安装的系统 postgresql 无法启动，因为宝塔已安装 PostgreSQL 并占用 5432。

**原因：** 宝塔面板自带 PostgreSQL，与 yum/dnf 安装的系统服务冲突。

**解决：**
- 停用系统 postgresql：

```bash
systemctl stop postgresql
systemctl disable postgresql
```

- 使用宝塔 PostgreSQL，DSN 指向 `localhost:5432/risk_engine`。

---

## 7. systemd 服务未创建

**现象：** `systemctl status efilter` 报 `Unit efilter.service could not be found.`

**原因：** 部署脚本中断，未执行到创建 systemd 服务的步骤。

**解决：** 手动创建 `/etc/systemd/system/efilter.service`：

```ini
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
```

然后：

```bash
systemctl daemon-reload
systemctl enable efilter
systemctl start efilter
```

---

## 8. Nginx 配置：根目录选择

**问题：** 宝塔站点根目录应该指向 `/opt/efilter` 还是 `/opt/efilter/backend/risk-engine`？

**结论：** 指向 `/opt/efilter`（项目根目录）。原因：
- 前端静态文件在 `/opt/efilter/frontend/index.html`
- Go 后端工作目录是 `/opt/efilter/backend/risk-engine`
- 无论用方案 A（全部反代）还是方案 B（Nginx 直接服务静态文件），根目录选项目根目录都更合理。

---

## 9. 访问日志压库风险

**现象：** 每条请求都异步单条写入 PostgreSQL，高 QPS 下数据库压力大。

**解决：**
- 移除黑白名单查询（用户暂不需要）。
- 访问日志改为 1 分钟批量写入。
- 保留 24 小时清理任务。

---

## 10. `/favicon.ico` 产生多余访问日志

**现象：** dashboard 面板出现 `/favicon.ico` 记录。

**原因：** 浏览器自动请求 favicon，`NoRoute` 返回 index.html 并被记录。

**解决：** 在 router 中显式处理：

```go
r.GET("/favicon.ico", func(c *gin.Context) { c.Status(http.StatusNoContent) })
```

---

## 后续建议

1. **token 管理：** IP2Location token 不要 hardcode 在 `.env` 里上传到仓库，生产环境用更安全的方式注入。
2. **日志策略：** 如果 QPS 很高，可考虑只记录拦截请求，或进一步延长批量时间。
3. **监控告警：** 为 `efilter.service`、Redis、PostgreSQL 添加 systemd 失败告警。
4. **HTTPS：** 通过宝塔面板申请 SSL 证书，强制 HTTPS 访问。
5. **备份：** 定期备份 PostgreSQL 中的规则、API Key 等配置数据（IP 库用脚本重新下载即可）。

---

最后更新：2026-08-11
