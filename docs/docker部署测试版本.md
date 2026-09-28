# Docker 部署测试版本（先行验证流程）

> 适用：生产已用 Docker 部署（宝塔编排或 docker compose），想先在服务器上验证新代码，确认没问题后再切到正式版本。
> 核心思路：**测试容器与生产容器并行**，测试镜像打 `test` 标签、用独立端口（8081），不碰生产任何东西；验证通过后再把 `test` 转正为 `latest` 重建生产容器。

## 流程总览

```text
git pull → 构建 test 镜像 → 起测试容器(8081) → curl 验证接口
    ├─ 通过 → test 改标 latest → 重建生产容器 → 清理测试容器
    └─ 不通过 → 删测试容器/镜像 → 生产毫无影响
```

## 1. 拉代码并构建测试镜像

```bash
cd /opt/efilter && git pull
docker build -f docker/Dockerfile -t efilter/risk-engine:test .
```

> 镜像名 `efilter/risk-engine:test`，与生产的 `latest` 完全隔离，互不影响。

## 2. 启动测试容器

先找到编排创建的网络（容器名固定为 `efilter-*`，网络一般是 `efilter_efilter-net`）：

```bash
docker network ls | grep efilter
```

启动测试容器（宿主机 8081 → 容器 8080，避开生产的 8080）：

```bash
docker run -d --name efilter-app-test \
  --network efilter_efilter-net \
  --env-file /opt/efilter/docker/.env \
  -e TZ=Asia/Shanghai \
  -p 8081:8080 \
  -v /opt/efilter/binfiles:/app/binfiles:ro \
  --restart unless-stopped \
  efilter/risk-engine:test
```

说明：

- `--env-file` 复用生产配置（API Key、数据库 DSN 指向 `postgres` 容器，同网络内可达）；
- binfiles 只读挂载，**不会**碰 IP 库文件；
- 测试容器与生产共用 PostgreSQL / Redis——测试请求会写入生产库的 `access_logs`（面板里能看到，无害），介意可在验证后不清理该数据；
- 若新版本改了数据库模型，启动时 AutoMigrate 会对共享库执行迁移。**只测试加列/加索引类的变更是安全的**；涉及改列名/删列的版本，建议先读一遍迁移影响再测。

## 3. 验证接口

```bash
# 本机验证
curl http://127.0.0.1:8081/health
curl -s -X POST http://127.0.0.1:8081/api/v1/results \
  -H "X-API-Key: 你的key" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "ip=8.8.8.8&country=US"
curl -s -X POST http://127.0.0.1:8081/api/v1/check \
  -H "X-API-Key: 你的key" -H "Content-Type: application/json" \
  -d '{"ip":"8.8.8.8"}'
```

需要外网 IP 访问测试时（如从办公室打流量）：

```bash
# 宝塔「安全」放行 8081（或云厂商安全组），测完记得回收
```

同时盯一下日志和资源：

```bash
docker logs --tail 50 -f efilter-app-test
docker stats --no-stream efilter-app-test
```

## 4A. 验证通过 → 转正为生产版本

```bash
# 1. 停掉并删除测试容器
docker rm -f efilter-app-test

# 2. test 镜像改标为 latest（无需重新构建）
docker tag efilter/risk-engine:test efilter/risk-engine:latest

# 3. 重建生产容器
docker rm -f efilter-app
# 宝塔编排：Docker → 编排 → efilter → 启动（按新 latest 重建）
# 标准 compose：docker compose -f docker/docker-compose.yml up -d
```

验证生产恢复：

```bash
curl http://127.0.0.1:8080/health
docker ps --filter name=efilter
```

> 建议保留 `test` 标签几天观察期，确认无问题再 `docker rmi efilter/risk-engine:test` 清理（多标签指向同一镜像时删标签不会删内容）。

## 4B. 验证不通过 → 清理，生产零影响

```bash
docker rm -f efilter-app-test
docker rmi efilter/risk-engine:test
# 生产容器自始至终没动过
```

## 常见问题

| 现象 | 原因 / 处理 |
|------|------------|
| 测试容器起不来，日志报 config error | env_file 路径不对，确认 `/opt/efilter/docker/.env` 存在 |
| 测试容器连不上数据库 | `--network` 没加或网络名不对，`docker network ls \| grep efilter` 核对；DSN 里 host 必须是 `postgres`（容器名） |
| results 返回 200 但 check 500 | 数据库未就绪时的降级行为，新版已修复；若重现，看 `docker logs efilter-app-test` |
| 8081 外网不通 | 宝塔「安全」+ 云安全组都要放行；测试完回收 |
| 想再测一版 | 重复 1-3 节即可，`test` 标签会被新构建覆盖 |
