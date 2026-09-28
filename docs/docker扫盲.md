# Docker 扫盲：rm / tag / 宝塔编排与命令行的关系

> 面向刚接触 Docker 的同学，把 efilter 部署流程里用到的几个核心概念讲清楚。
> 配套文档：[docker-deployment.md](docker-deployment.md)（标准部署）、[docker-bt-deployment.md](docker-bt-deployment.md)（宝塔部署）、[docker部署测试版本.md](docker部署测试版本.md)（测试版本流程）。

## 1. `rm` 和 `docker rm`：删的到底是什么

`rm` 是 Linux 通用命令 remove（删除文件）。`docker rm` 是它的 Docker 版本，但删除的对象是**容器**——可以理解为"删除一个正在运行（或已停止）的进程实例 + 它的可写层"。

关键：**镜像和容器是两样东西**。

```text
镜像 (image)     = 类 / 模板   → docker rmi 删除
容器 (container) = 对象 / 实例 → docker rm  删除
```

类比 Java：`Image` 是 class，`Container` 是 new 出来的对象。`docker rm` 只是销毁实例，**镜像还在**，下次 `docker run` 还能从同一个镜像再创建。

部署流程里 `docker rm -f efilter-app`（删生产容器）之后宝塔点"启动"能重建，就是因为镜像没动——只是用同一个模板 new 了一个新实例。

常用命令区分：

| 命令 | 删什么 | 后果 |
|------|--------|------|
| `docker rm 容器名` | 容器（实例） | 进程没了，镜像还在，可重建 |
| `docker rm -f 容器名` | 运行中的容器 | 先 SIGKILL 再删，等同于强制 |
| `docker rmi 镜像名` | 镜像（模板） | 有容器在用时会拒绝，需先 rm 容器 |
| `docker volume rm 卷名` | 数据卷 | **慎用**：postgres 的数据就在卷里，删了数据清空 |

efilter 里唯一不能乱删的是 `pgdata` 卷——那是 PostgreSQL 的全部数据。

## 2. `docker tag` 的原理：只是改指针，不搬数据

每个镜像有一个**唯一 ID（sha256 哈希）**，而 `efilter/risk-engine:latest` 这样的名字只是一个"指针标签"（tag），指向某个哈希：

```text
efilter/risk-engine:latest ──┐
                             ├──→ 镜像 ID: a1b2c3d4...（同一份内容）
efilter/risk-engine:test   ──┘
```

所以测试转正这一步：

```bash
docker tag efilter/risk-engine:test efilter/risk-engine:latest
```

**没有任何数据复制**——只是把 `latest` 标签的指向从旧镜像改到 test 指向的哈希，一瞬间完成，1MB 都不搬。这就是转正不需要重新构建的原因。

同理，`docker rmi efilter/risk-engine:test` 在多标签指向同一镜像时**只是撕掉一个标签**，真正的镜像文件要等所有标签都撕掉、且没有容器在使用，才会被 Docker 回收。所以观察期内删 test 标签是安全的。

动手验证：

```bash
docker images                 # 看每个镜像 ID 和标签
docker inspect efilter/risk-engine:latest --format '{{.Id}}'
docker inspect efilter/risk-engine:test   --format '{{.Id}}'   # 转正后两者相同
```

## 3. 宝塔编排 = docker compose 的网页皮肤，可全 bash 替代

宝塔编排只做两件事：替你保存 compose 文件 + 替你执行 compose 命令。SSH 里敲的命令和它**完全等价**。

宝塔环境直接用仓库里的 compose 文件操作：

```bash
cd /opt/efilter
docker compose -f docker/docker-compose.bt.yml up -d    # 启动/创建缺失的容器
docker compose -f docker/docker-compose.bt.yml stop     # 停止
docker compose -f docker/docker-compose.bt.yml down     # 停止并删容器（数据卷保留）
docker compose -f docker/docker-compose.bt.yml ps       # 状态
```

### 一个必须避开的坑：项目名（project name）

compose 靠**项目名**认亲——项目名决定容器名、网络名。宝塔创建的栈项目名可能和默认值不同，直接跑 `docker compose -f ...` 会用默认项目名**建出第二套平行容器**（比如 `efilter-app-1`），新旧两套并存、端口冲突、各连各的数据库。

先核对宝塔栈的项目名：

```bash
docker compose ls          # 列出所有 compose 项目
docker inspect efilter-app --format '{{index .Config.Labels "com.docker.compose.project"}}'
```

然后显式指定项目名再操作：

```bash
docker compose -p <项目名> -f docker/docker-compose.bt.yml up -d
```

### 实用结论

| 操作 | 建议方式 |
|------|---------|
| 看状态、日志、资源 | 全 bash 更顺手（`docker ps` / `docker logs` / `docker stats`） |
| 日常重启 | bash：`docker restart efilter-app` |
| 升级重建 | 二选一：① `docker rm -f efilter-app` 后宝塔点"启动"（容器是宝塔建的，让它重建最稳）；② 全套 bash，记得加 `-p 项目名` |

两条路完全等价，选自己不容易出错的那条。
