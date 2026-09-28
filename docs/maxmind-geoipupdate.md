# MaxMind GeoIP 数据库更新方案（geoipupdate）

> 面向技术开发 / 运维。采用 MaxMind 官方推荐方式：`geoipupdate` 自动更新 GeoLite2 二进制库（`.mmdb`）。  
> 参考：[Updating Databases](https://dev.maxmind.com/geoip/updating-databases/) · [geoipupdate GitHub](https://github.com/maxmind/geoipupdate)

---

## 一、方案结论

| 项 | 说明 |
|----|------|
| 推荐方式 | 安装 `geoipupdate` ≥ 4.x，定时执行更新 |
| 不推荐 | 手工 curl 下载（仅当无法安装工具，或需要 CSV 时再考虑） |
| 当前订购库 | `GeoLite2-ASN`、`GeoLite2-City`、`GeoLite2-Country` |
| 配置文件 | 仓库根目录 `GeoIP.local.conf`（含密钥，**勿提交到公开仓库**） |
| 与现有 IP2Location | 可并存；本方案只负责 MaxMind `.mmdb` 的获取与更新 |

CSV 格式库 **不支持** `geoipupdate`，必须走官方「直接下载」流程。本项目当前 Edition 均为二进制，走 `geoipupdate` 即可。

---

## 二、配置说明

本地已准备好 `GeoIP.local.conf`，关键字段：

```bash
# GeoIP.conf — geoipupdate >= 3.1.1
AccountID YOUR_ACCOUNT_ID
LicenseKey YOUR_LICENSE_KEY
EditionIDs GeoLite2-ASN GeoLite2-City GeoLite2-Country
```

| 字段 | 含义 |
|------|------|
| `AccountID` | MaxMind 账号 ID |
| `LicenseKey` | 账号 License Key（密钥） |
| `EditionIDs` | 需要下载/更新的数据库产品 ID |

生产环境建议：

1. 将 `GeoIP.local.conf` 复制为服务器上的 `GeoIP.conf`，权限收紧（如 `chmod 600`）。
2. **不要**把真实 `LicenseKey` 写进 Git；仓库内只保留示例或本地私有文件。
3. 可选在配置中增加 `DatabaseDirectory`，显式指定 `.mmdb` 落盘目录。

账号后台可下载预填配置：  
https://www.maxmind.com/en/accounts/current/license-key/GeoIP.conf

---

## 三、安装 geoipupdate

版本要求：**≥ 4.x**（强制 TLS 1.2+）。发布包：  
https://github.com/maxmind/geoipupdate/releases

### CentOS / RHEL（rpm）

```bash
# 将版本号、架构换成实际文件名
sudo rpm -Uvhi geoipupdate_X.Y.Z_linux_amd64.rpm
# 默认二进制：/usr/bin/geoipupdate
# 默认配置：/etc/GeoIP.conf
```

### Linux tarball

```bash
# 解压后
sudo cp geoipupdate_X.Y.Z_linux_amd64/geoipupdate /usr/local/bin/
# 默认配置：/usr/local/etc/GeoIP.conf
```

### 从源码 / Go 安装

```bash
go install github.com/maxmind/geoipupdate/v8/cmd/geoipupdate@latest
# 安装到 $GOPATH/bin/geoipupdate
```

### Docker（可选）

官方镜像：https://hub.docker.com/r/maxmindinc/geoipupdate

---

## 四、部署与首次验证

### 1. 放置配置

```bash
# rpm 安装示例
sudo cp /path/to/GeoIP.local.conf /etc/GeoIP.conf
sudo chmod 600 /etc/GeoIP.conf

# 或指定非默认路径，运行时用 -f
sudo mkdir -p /opt/efilter/configs
sudo cp /path/to/GeoIP.local.conf /opt/efilter/configs/GeoIP.conf
sudo chmod 600 /opt/efilter/configs/GeoIP.conf
```

### 2. 指定数据库目录（建议）

与项目其他 IP 库统一管理，例如：

```bash
sudo mkdir -p /opt/efilter/binfiles/maxmind
```

### 3. 手动跑一次（带详细日志）

```bash
# 使用默认配置路径
sudo geoipupdate -v

# 或显式指定配置与目录
sudo geoipupdate -v \
  -f /opt/efilter/configs/GeoIP.conf \
  -d /opt/efilter/binfiles/maxmind
```

成功后目录中应出现类似文件：

- `GeoLite2-Country.mmdb`
- `GeoLite2-City.mmdb`
- `GeoLite2-ASN.mmdb`

### 4. 网络与防火墙

| 要求 | 说明 |
|------|------|
| DNS | 可解析 MaxMind / R2 域名 |
| HTTPS 443 | 出站必须放行 |
| 跟随重定向 | HTTP 客户端需跟随 3xx（`geoipupdate` 已支持） |
| R2 域名 | 2024 起下载会跳转到 Cloudflare R2，需放行：`mm-prod-geoip-databases.a2649acb697e2c09b632799562c076f2.r2.cloudflarestorage.com` |

代理或防火墙若拦截 R2 主机名，更新会失败。详见官方说明：  
https://dev.maxmind.com/geoip/updating-databases/

---

## 五、定时自动更新

建议每周至少 1～2 次（官方示例为每周两次）。库有日下载限额，避免过于频繁。

```bash
sudo crontab -e
```

示例（每周一、四凌晨 3 点）：

```cron
0 3 * * 1,4 /usr/bin/geoipupdate -f /opt/efilter/configs/GeoIP.conf -d /opt/efilter/binfiles/maxmind >> /opt/efilter/logs/geoipupdate.log 2>&1
```

若业务进程启动时加载 `.mmdb` 到内存，更新后需重启对应服务才能读到新库；若运行时按路径打开文件，按你们实现决定是否热加载。

---

## 六、排错清单

| 现象 | 处理 |
|------|------|
| 认证失败 | 检查 `AccountID` / `LicenseKey` 是否有效、订阅是否过期 |
| 连接被拒 / 超时 | 检查出站 443、是否拦 R2 域名、是否需代理 |
| 旧版 geoipupdate 报错 | 升级到 4.x 或更高 |
| 需要逐步日志 | 加 `-v`：`geoipupdate -v` |
| 下载次数过多 | 注意官方日下载限额与失败重试导致的限流 |

订阅与账号：  
https://www.maxmind.com/en/accounts/current/people/current

---

## 七、开发对接要点（risk-engine）

本方案只覆盖「如何稳定拿到最新 `.mmdb`」。接入查询层时注意：

1. **读本地文件**：与现有 IP2Location BIN 思路一致，直接读磁盘 `.mmdb`，不要整库灌 PostgreSQL/Redis。
2. **Go 客户端**：常用 `github.com/oschwald/geoip2-golang`（或 MaxMind 官方 Go 库）打开 `.mmdb`。
3. **路径配置**：在 `configs/config.yaml` 中增加 MaxMind 路径项，与 `binfiles/maxmind/` 对齐。
4. **更新后加载**：二选一——定时更新后 `systemctl restart efilter`，或实现文件变更后热重载。
5. **与 IP2Location 关系**：可先并行验证（同一 IP 对比国家/ASN），再决定是否替换或互补。

当前项目主路径仍是 IP2Location / IP2Proxy；MaxMind 为可选增强数据源，按业务需要接入。

---

## 八、落地 Checklist

- [ ] 安装 `geoipupdate` ≥ 4.x，`geoipupdate -V` 可输出版本
- [ ] 生产机放置 `GeoIP.conf`（权限 600），不含在 Git 公开内容中
- [ ] 创建数据库目录（如 `/opt/efilter/binfiles/maxmind`）
- [ ] `geoipupdate -v -f ... -d ...` 首次成功，三个 `.mmdb` 存在
- [ ] 确认出站可访问 MaxMind 与 R2 域名
- [ ] 配置 crontab 定期更新，日志可查
- [ ]（若已接入代码）更新后重启或热加载生效
- [ ] License 续费提醒纳入运维日历

---

## 九、不走本方案时的备选（仅记录）

无法安装 `geoipupdate`、或必须用 CSV 时，用账号后台 Permalink + Basic Auth 直接下载，并建议用 HEAD 检查 `Last-Modified` 再决定是否下载（HEAD 不计日配额）。详见：  
https://dev.maxmind.com/geoip/updating-databases/#directly-downloading-databases

---

## 最后更新

- 2026-08-17：新增本文档；确认采用 `geoipupdate` 更新 GeoLite2-ASN / City / Country。
