# IP 数据库自动更新工具

用于定时下载并原子替换 IP2Location / IP2Proxy 的 BIN 数据库文件，以及 IP2Proxy 的 IPv6 CSV 数据库文件。

## 目录结构

```
tools/update-ipdb/
├── update-ipdb.sh    # Linux/macOS 更新脚本
└── README.md         # 本文件
```

## 使用方式

### 1. 配置下载地址

编辑 `.env.example` 中的 token 和下载 URL，或设置环境变量：

```bash
export IP2LOCATION_URL="https://www.ip2location.com/download?token=YOUR_TOKEN&file=DB1LITEBINIPV6"
export IP2PROXY_URL="https://www.ip2location.com/download?token=YOUR_TOKEN&file=PX2LITEBIN"
export IP2PROXY_IPV6_URL="https://www.ip2location.com/download?token=YOUR_TOKEN&file=PX2LITECSVIPV6"
```

> 注意：URL 中的 `&` 在命令行传递时需要加反斜杠转义，或在 `.env` 文件中使用引号包裹。

### 2. 执行更新

```bash
cd /path/to/efilter
bash tools/update-ipdb/update-ipdb.sh
```

### 3. 定时任务（推荐每天凌晨执行）

```bash
# crontab -e
0 3 * * * /path/to/efilter/tools/update-ipdb/update-ipdb.sh >> /path/to/efilter/logs/ipdb-update.log 2>&1
```

## 更新流程

1. 下载 zip 文件到临时目录
2. 解压 zip
3. 查找 `.BIN` / `.CSV` 文件
4. 复制到目标目录的临时文件
5. `mv` 原子替换正式文件
6. 清理临时目录

> **注意：** IP2Proxy IPv6 CSV 与 BIN 格式不同，服务启动时会将其加载到内存，因此更新 CSV 后必须重启服务。

## 重启服务

更新完成后，需要重启 `risk-engine` 以加载新的 BIN/CSV 文件：

```bash
systemctl restart efilter
# 或
kill -HUP <pid>
# 或
/path/to/efilter/backend/risk-engine/risk-engine
```

当前服务读取路径（相对路径，以 `backend/risk-engine` 为工作目录）：

```yaml
ipdb:
  ip2location: "../../binfiles/IP2LOCATION-LITE-DB1.IPV6.BIN/IP2LOCATION-LITE-DB1.IPV6.BIN"
  ip2proxy: "../../binfiles/IP2PROXY-LITE-PX2.BIN/IP2PROXY-LITE-PX2.BIN"
  ip2proxy_ipv6_csv: "../../binfiles/IP2PROXY-LITE-PX2.IPV6.CSV/IP2PROXY-LITE-PX2.IPV6.CSV"
```

## 原子替换说明

Linux 下 `mv` 同一文件系统内是原子操作，服务在读取旧文件时不会被半写文件影响。更新脚本使用 `cp` 到临时文件再 `mv` 的方式实现原子替换。
