# weiyu-go · 轻量社交内容平台后端

> 基于 Go + Gin + GORM + MySQL + Redis + RabbitMQ 的社区动态后端。
> 覆盖认证、动态、评论、点赞、通知，重点落在缓存防护、可靠消息与接口正确性。

**规模**：15 个 REST 接口 · 5 张表 · 3 类基础设施（MySQL / Redis / RabbitMQ）。

---

## 1. 功能

| 模块 | 能力 |
|---|---|
| 账号 | 注册（bcrypt）、登录签发 JWT、当前用户、登出（jti 吊销） |
| 动态 | 发布、游标分页列表、详情、删除自己的动态 |
| 评论 | 发表评论、按动态查询评论列表 |
| 点赞 | 点赞/取消、点赞数查询、唯一索引防重复 |
| 通知 | 点赞/评论事件经 RabbitMQ 异步写通知，支持拉取 |
| 平台 | 健康检查、IP/用户两级限流、统一响应、优雅关闭 |

---

## 2. 接口

**公开**

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/v1/health` | 健康检查（MySQL / Redis / RabbitMQ） |
| POST | `/api/v1/users/register` | 注册 |
| POST | `/api/v1/users/login` | 登录，返回 JWT |
| GET | `/api/v1/posts?limit=10&cursor=...` | 动态列表，游标分页 |
| GET | `/api/v1/posts/:id` | 动态详情（缓存） |
| GET | `/api/v1/posts/:id/comments` | 评论列表 |
| GET | `/api/v1/posts/:id/likes/count` | 点赞数 |

**登录后**

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/v1/users/me` | 当前用户 |
| POST | `/api/v1/users/logout` | 登出，jti 进入吊销名单 |
| GET | `/api/v1/notifications` | 我的通知 |
| POST | `/api/v1/posts` | 发布动态 |
| DELETE | `/api/v1/posts/:id` | 删除自己的动态（级联清理） |
| POST | `/api/v1/posts/:id/comments` | 发表评论 |
| POST | `/api/v1/posts/:id/likes` | 点赞 |
| DELETE | `/api/v1/posts/:id/likes` | 取消点赞 |

统一响应：`{"code":0,"message":"success","data":...}`。

---

## 3. 架构

```
                     ┌──────────────────────────────┐
 HTTP Client ───────▶│ Gin Router                   │
                     │  ├── RateLimit（IP / 用户）   │
                     │  ├── JWT 鉴权（jti 吊销检查） │
                     │  ├── Controller（参数/状态码）│
                     │  └── Service（业务/缓存/MQ）  │
                     └───────┬──────────┬───────────┘
                             │          │
                  ┌──────────▼──┐   ┌───▼────────────┐
                  │   Redis     │   │     MySQL      │
                  │ 详情缓存     │   │ users/posts    │
                  │ 空值/计数    │   │ comments/likes │
                  │ 限流/吊销    │   │ notifications  │
                  └─────────────┘   └────────────────┘
                             │
                    点赞/评论事件（confirm）
                             │
                     ┌───────▼────────┐
                     │   RabbitMQ     │
                     │ weiyu.events   │──▶ weiyu.notify ──▶ 消费者 ──▶ notifications
                     │ weiyu.dlx      │◀── 重试超限
                     └────────────────┘
```

**动态详情读路径**

```
Redis 命中 ──▶ 直接返回
   │ miss
   ▼
空值缓存命中 ──▶ 404（防穿透）
   │ miss
   ▼
singleflight 合并同 key 请求（防击穿）
   │
   ▼
MySQL 回源 ──▶ 回填缓存（TTL + 0~50% 抖动，防雪崩）
```

---

## 4. 数据模型（5 张表）

| 表 | 关键字段/索引 |
|---|---|
| `users` | `username` 唯一；密码只存 bcrypt 哈希 |
| `posts` | 作者、内容；`(created_at, id)` 复合索引服务游标分页 |
| `comments` | `post_id` 索引 |
| `likes` | `(post_id, user_id)` 唯一索引，防重复点赞 |
| `notifications` | `(user_id, created_at)` 索引；`event_id` 唯一索引做消息幂等兜底 |

表由 GORM `AutoMigrate` 在启动时创建，当前没有 migrations/seed 目录。

---

## 5. 关键设计

| 主题 | 做法 | 代码 |
|---|---|---|
| 缓存三防护 | Cache-Aside + 空值缓存防穿透 + singleflight 防击穿 + TTL 抖动防雪崩 | `service/cache.go`、`service/post.go` |
| 缓存一致性 | 删除动态在事务提交后删除详情/空值/计数三个 key，避免回滚留下脏状态 | `service/post.go` `DeletePost` |
| 游标分页 | `(created_at, id)` keyset，取 `limit+1` 判断 `has_more`，返回 `next_cursor` | `service/cursor.go`、`service/post.go` |
| 两级限流 | Redis Lua 固定窗口，INCR+EXPIRE 原子；IP 300/min、用户 60/min | `middleware/ratelimit.go` |
| 可靠通知 | 发布确认、手动 ack、Redis SETNX 幂等 + `event_id` 唯一索引、重试 3 次后进死信 | `mq/event.go`、`mq/consumer.go` |
| JWT 登出 | jti 写入 Redis 吊销名单，TTL 对齐 token 过期；鉴权时检查 | `service/token.go`、`middleware/jwt.go` |
| 点赞计数 | Redis 缓存 + Lua 只在 key 存在时增减，避免覆盖历史计数 | `service/like.go`、`service/cache.go` |

**设计取舍**

- Redis 不可用时，读缓存失败会降级查 MySQL；登出校验选择放行（可用性优先），代价是 Redis 故障期间登出失效。
- 缓存一致性是「删缓存 + 短 TTL」的最终一致，不是强一致。
- 限流是固定窗口，窗口边界存在瞬时突刺，适合做第一道粗粒度保护。

---

## 6. 快速开始

```bash
# 依赖：MySQL 8 + Redis + RabbitMQ
export MYSQL_DSN='weiyu:weiyu123456@tcp(127.0.0.1:3306)/weiyu?charset=utf8mb4&parseTime=True&loc=Local'
export REDIS_ADDR='127.0.0.1:6379' REDIS_DB=5
export RABBITMQ_URL='amqp://guest:guest@127.0.0.1:5672/'
export JWT_SECRET='至少32字节的随机字符串'

go run .
# 默认监听 127.0.0.1:8080
```

造一条演示数据：

```bash
BASE=http://127.0.0.1:8080/api/v1
curl -s -X POST $BASE/users/register -H 'Content-Type: application/json' \
  -d '{"username":"demo","password":"password123"}'
TOKEN=$(curl -s -X POST $BASE/users/login -H 'Content-Type: application/json' \
  -d '{"username":"demo","password":"password123"}' | python3 -c 'import sys,json;print(json.load(sys.stdin)["data"]["token"])')
curl -s -X POST $BASE/posts -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' -d '{"content":"第一条动态"}'
```

当前本地库中已有一条压测数据：账号 `bench_user` / `password123`，动态 ID `1`。

---

## 7. 压测：缓存开 vs 关

**环境**：本机 macOS，服务、MySQL、Redis 同机；单实例；100 并发 × **200,000 请求**（约 4 秒）；
预热 1,000 请求；压测时 `RATE_LIMIT_ENABLED=false` 避免限流干扰。

> 为什么不用 5000：5000 请求在 5 万 QPS 下只跑 0.1 秒，计时和连接预热会放大误差；
> 20 万请求约 4 秒，数字更稳。两种请求量下趋势一致，下面用 20 万的结果。

```bash
# 缓存开
CACHE_ENABLED=true RATE_LIMIT_ENABLED=false go run .
go run ./cmd/bench -url http://127.0.0.1:8080/api/v1/posts/1 \
  -n 200000 -c 100 -mysql-dsn "$MYSQL_DSN" -tag cache-on

# 缓存关（重启服务）
CACHE_ENABLED=false RATE_LIMIT_ENABLED=false go run .
go run ./cmd/bench -url http://127.0.0.1:8080/api/v1/posts/1 \
  -n 200000 -c 100 -mysql-dsn "$MYSQL_DSN" -tag cache-off
```

| 指标 | 缓存开 | 缓存关 |
|---|---:|---:|
| QPS | **62,376** | 42,461 |
| P50 | 1.56 ms | 2.05 ms |
| P90 | 2.08 ms | 3.92 ms |
| P95 | 2.24 ms | 4.70 ms |
| P99 | **2.69 ms** | 6.50 ms |
| DB SELECT（20 万请求） | **0** | **200,000** |
| 平均回源 | 0 条/请求 | 1 条/请求 |

**怎么解读**

- 缓存把 MySQL 回源从 **20 万次降到 0**，这是它最核心的价值：保护数据库，而不是单纯刷 QPS。
- QPS 从 42,461 提升到 62,376（约 +47%）；本机 MySQL 单行主键查询很快，
  缓存路径本身有 Redis RTT + JSON 反序列化成本，所以不要吹成几十倍。
- P99 从 6.50ms 降到 2.69ms，尾部延迟更稳定。
- 并发不是越高越好：200/400 并发下 QPS 没有继续上升、P99 反而升高，说明已经到本机 CPU 瓶颈；
  因此取 100 并发作为对比条件。

**局限**：本机同机、单实例、单行数据、无 TLS/负载均衡，数字不能当作生产容量。

---

## 8. 测试与 CI

```bash
go test ./...              # 单元测试
go test -race ./...        # 竞态检测（CI 使用）
go vet ./...
```

现有测试覆盖：JWT 签发/吊销、限流、游标编解码、缓存读写与空值、MQ 事件序列化、
通知消费（部分集成测试需要本机 Redis/RabbitMQ）。

GitHub Actions：`.github/workflows/ci.yml`，每次 push/PR 执行 `go vet` + `go test -race`。

---

## 9. 目录

```
weiyu-go/
├── main.go              启动：MySQL/Redis/RabbitMQ → 消费者 → HTTP → 优雅关闭
├── config/              连接初始化、缓存/限流开关
├── controller/          HTTP 参数、状态码
├── service/             业务、缓存、游标、JWT 吊销、MQ 事件
├── middleware/          JWT、限流
├── models/              5 张表
├── mq/                  交换机/队列/死信、发布确认、消费者
├── router/              15 个路由
├── cmd/bench/           零依赖压测工具（可选统计 DB 回源）
└── docs/                缓存三防护 10 分钟讲解稿
```

---

## 10. 已知限制

- 无前端页面，纯 API 项目；
- 单机 MySQL/Redis，无主从、无集群；
- 表结构用 AutoMigrate，没有版本化迁移和种子脚本；
- 缓存是最终一致，删除后到缓存失效前可能读到旧值；
- 限流为固定窗口，边界有突刺；
- 压测为本机数据，不代表生产性能。

更完整的面试讲法见 [docs/缓存三防护-10分钟讲解.md](docs/缓存三防护-10分钟讲解.md)。
