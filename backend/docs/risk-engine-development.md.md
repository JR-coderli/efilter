下面整理成一份可以直接给技术同事执行的 **《广告流量风控 API 系统 0-1 开发文档（Go 本地部署版）》**。

建议保存为：

```
docs/
 └── risk-engine-development.md
```

---

```md
# 广告流量风控 API 系统开发文档

版本：V1.0
技术栈：Go + MySQL + Redis + IP2Location + IP2Proxy

---

# 1. 项目背景

## 1.1 项目目标

开发一套本地部署的广告流量风控系统，用于：

- IP 信息查询
- VPN / Proxy / Residential Proxy 检测
- 数据中心 IP 检测
- ASN / ISP 判断
- 国家地区判断
- 用户访问风险评分
- 广告流量质量过滤

系统定位类似：

- ClickFlare 风控模块
- Voluum Traffic Filtering
- Binom Traffic Rules


---

# 2. 系统整体架构


                 用户请求

                    |

                  Nginx

                    |

                 Go API

                    |

        +-----------+-----------+

        |           |           |

      Redis       MySQL     IP Database

      Cache       Config    IP2Location

                             IP2Proxy


                    |

              Risk Engine

                    |

              返回评分结果



---

# 3. 技术选型


## 后端

语言：

Go 1.24+


Web Framework:

推荐：

- Gin

或者：

- Fiber


原因：

- 高并发
- 内存占用低
- 部署简单


---

## 数据库


### MySQL 8

用途：

存储业务数据：

- 用户
- API Key
- 风控规则
- 黑名单
- 白名单
- 配置


### Redis

用途：

- IP查询缓存
- API限流
- 临时数据


### IP数据库

本地文件：

IP2Location

IP2Proxy


格式：

BIN


---

# 4. 开发环境


## Server

Linux:

Ubuntu 22.04+

或者：

CentOS 8+


## 软件

安装：

```

Go
Git
MySQL 8
Redis
Nginx

```


---

# 5. 项目目录结构


```

risk-engine/

├── cmd/
│   └── server/
│       └── main.go
│

├── internal/

│   ├── api/
│   │   ├── handler.go
│   │   └── router.go
│
│   ├── service/
│   │   ├── risk.go
│   │   └── ip.go
│
│   ├── database/
│   │   ├── mysql.go
│   │   ├── redis.go
│   │   └── ipdb.go
│
│   ├── models/
│   │
│   ├── middleware/
│   │
│   └── config/

├── configs/

│   └── config.yaml

├── logs/

├── go.mod

└── README.md

```


---

# 6. 数据库设计


## users


```

id

username

password

status

created_at

```



---

## api_keys


```

id

user_id

api_key

rate_limit

status

created_at

```



---

## risk_rules


```

id

name

condition

score

action

status

created_at

```



示例：

```

VPN = true

score +30

Datacenter=true

score +40

```


---

## ip_blacklist


```

id

ip

reason

created_at

```


---

## ip_whitelist


```

id

ip

remark

created_at

```


---

# 7. IP数据库模块


## IP2Location


功能：

获取：

```

country

region

city

latitude

longitude

timezone

isp

asn

```



---

## IP2Proxy


功能：

检测：

```

proxy

vpn

tor

datacenter

residential

mobile

usage_type

fraud_score

```


---

# 8. API设计


## 8.1 IP检测接口


URL:

```

POST /api/v1/check

````


Request:

```json
{
 "ip":"8.8.8.8",

 "user_agent":"Chrome",

 "cookie_id":"xxxx",

 "campaign":"test"
}

````

Response:

```json
{

"ip":"8.8.8.8",

"country":"US",

"city":"California",

"isp":"Google",

"asn":"15169",

"is_proxy":false,

"is_vpn":false,

"is_datacenter":true,


"risk_score":60,


"action":"review"

}

```

---

# 9. Risk Engine规则

评分模型：

基础分：

```
0-100
```

规则：

## VPN

```
VPN=true

+30
```

## Proxy

```
Proxy=true

+40

```

## Datacenter

```
Hosting=true

+30

```

## Tor

```
Tor=true

+80

```

## 黑名单

```
Blacklist=true

+100

```

最终：

```
0-30

Safe


30-70

Review


70-100

Block

```

---

# 10. Redis设计

## IP缓存

Key:

```
risk:ip:{ip}
```

Value:

JSON

TTL:

```
600 seconds
```

例如：

```
risk:ip:8.8.8.8

{
vpn:false,
proxy:false,
score:20
}

```

---

# 11. MySQL配置

连接池：

```
max_connections=100

idle_connections=20

```

---

# 12. 性能要求

目标：

## 第一阶段

QPS:

1000

## 第二阶段

QPS:

10000+

要求：

单IP查询：

<5ms

---

# 13. 日志系统

记录：

```
request_id

ip

country

risk_score

rule_hit

response_time

created_at

```

日志：

```
logs/app.log

```

---

# 14. 部署方式

## 编译

```
go build

```

生成：

```
risk-engine

```

---

## Systemd

服务：

```
/etc/systemd/system/risk.service

```

启动：

```
systemctl start risk

```

---

# 15. Nginx

代理：

```
client

 |

nginx

 |

localhost:8080

```

开启：

* HTTPS
* Access Log

---

# 16. 开发阶段计划

## Phase 1

基础API

时间：

1周

完成：

* Go项目初始化
* Gin
* API
* MySQL
* Redis

---

## Phase 2

IP检测

时间：

1周

完成：

* IP2Location
* IP2Proxy
* 查询接口

---

## Phase 3

规则系统

时间：

1-2周

完成：

* 风控规则
* 黑名单
* 白名单
* Risk Score

---

## Phase 4

生产化

时间：

2周

完成：

* 日志
* 性能优化
* Docker
* 部署

---

# 17. 后续扩展

未来增加：

## Device Fingerprint

采集：

* Canvas
* WebGL
* Audio
* Font
* Screen
* Browser

---

## Click Tracking

增加：

* click_id
* sub_id
* campaign_id

---

## 数据分析

增加：

ClickHouse

用于：

* 点击分析
* 媒体质量分析
* 风险趋势

---

# 最终技术架构

Go

*

Gin

*

MySQL

*

Redis

*

IP2Location

*

IP2Proxy

*

Nginx

实现：

广告流量实时风险检测系统。

```

---

这个版本偏**技术落地 PRD + 开发设计文档**，技术拿到后可以直接拆任务。

后续如果继续开发，我建议下一份补充：

1. **数据库 ER 图**
2. **Go 项目代码骨架**
3. **IP2Proxy 接入示例代码**
4. **Risk Engine 规则引擎设计**
5. **后台管理系统 PRD**

这样就可以真正进入开发阶段。
```
