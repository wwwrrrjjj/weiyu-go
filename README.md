# weiyu-go · 轻量社交内容平台后端

基于 **Go + Gin + GORM + MySQL + Redis + RabbitMQ** 的社区动态后端，覆盖用户认证、动态发布、评论、点赞互动等核心链路，并落地了缓存防护、消息异步解耦、幂等消费、优雅关闭等生产化实践。

---

## 1. 功能特性

| 模块 | 能力 |
|---|---|
| 用户与认证 | 注册（bcrypt 哈希）、登录签发 JWT、Bearer 鉴权中间件、当前用户信息 |
| 动态 | 发布、详情、分页列表、内容长度校验 |
| 评论 | 发表、列表、动态存在性校验 |
| 点赞 | 点赞 / 取消点赞 / 点赞数统计，唯一索引防重复 |
| 缓存 | 详情 Cache-Aside、空值缓存防穿透、singleflight 防击穿、TTL 抖动防雪崩 |
| 消息 | 点赞/评论事件投递 RabbitMQ，异步写通知，confirm + 手动 ack + 幂等 + 重试 + 死信 |
| 工程化 | 三层分层、统一响应、健康检查、优雅关闭、Docker 多阶段构建、compose 编排 |

**接口规模**：13 个 RESTful 接口 / 5 张表（users、posts、comments、likes、notifications）。

---

## 2. 技术栈

| 层次 | 选型 | 说明 |
|---|---|---|
| 语言 | Go 1.22+ | goroutine 处理 HTTP 与 MQ 消费并发 |
| Web | Gin | 路由分组、中间件链、参数绑定校验 |
| ORM | GORM | AutoMigrate、`WithContext`、`TranslateError` 语义化错误 |
| 存储 | MySQL 8.0 | 唯一索引兜底并发、连接池参数 |
| 缓存 | Redis 7.x | 详情缓存、点赞计数、消费幂等 |
| 消息 | RabbitMQ 3.12 | topic 交换机、DLX 死信、发布确认 |
| 认证 | JWT (HS256) + bcrypt | 算法白名单、issuer/exp 强校验 |
| 部署 | Docker / docker-compose | 多阶段构建、非 root 运行、健康检查 |

---

## 3. 系统架构

```
                     ┌──────────────────────────────┐
   HTTP Client ─────▶│  Gin Router                  │
                     │  ├── JWT 中间件（写接口）      │
                     │  ├── Controller（参数/状态码） │
                     │  └── Service（业务/缓存/MQ）   │
                     └───────┬──────────┬───────────┘
                             │          │
              ┌──────────────▼──┐   ┌───▼─────────────┐
              │     Redis       │   │     MySQL       │
              │ 详情缓存         │   │ users/posts     │
              │ 点赞计数         │   │ comments/likes  │
              │ 消费幂等         │   │ notifications   │
              └─────────────────┘   └─────────────────┘
                             │
                    事件投递（confirm）
                             │
                     ┌───────▼────────┐
                     │   RabbitMQ     │
                     │ weiyu.events  │──▶ weiyu.notify ──▶ 消费者 ──▶ notifications
                     │ weiyu.dlx     │◀── 重试超限（死信）
                     └────────────────┘
```

**点赞链路时序**：

1. 校验动态是否存在（走缓存，未命中回源）
2. `INSERT likes`，唯一索引冲突 → 返回 409（不做「先查后插」）
3. Redis 点赞计数 +1（Lua 脚本，key 不存在时跳过，交给读请求重建）
4. 投递 `like.created` 事件（confirm 模式，2s 超时，失败仅告警不阻塞主流程）
5. 消费者 `SETNX` 幂等去重 → 查动态作者 → 写入 `notifications`
6. 消费失败：重试 ≤ 3 次重新入队，超限 reject 进入死信队列

---

## 4. 目录结构

```
weiyu-go/
├── main.go                 # 启动入口：基础设施初始化、消费者、HTTP、优雅关闭
├── router/                 # 路由注册（公开接口 / 鉴权分组）
├── middleware/             # JWT 鉴权中间件
├── controller/             # HTTP 层：参数解析、状态码映射、统一响应
├── service/                # 业务层：领域错误、缓存、事件投递、MQ 消费处理
├── models/                 # GORM 模型（含 Notification）
├── config/                 # MySQL / Redis 初始化与连接池
├── mq/                     # RabbitMQ 连接、拓扑声明、发布、消费者
├── utils/                  # JWT 签发校验、统一响应结构
├── docker-compose.yml      # MySQL + Redis + RabbitMQ + App 一键编排
├── Dockerfile              # 多阶段构建
└── .env.example            # 环境变量模板
```

---

## 5. 快速开始

### 5.1 Docker Compose 一键启动（推荐）

```bash
git clone https://github.com/wwwrrrjjj/weiyu-go.git
cd weiyu
cp .env.example .env        # 仅本地直跑需要；compose 已内置环境变量
docker compose up -d --build
docker compose logs -f app
```

服务地址：

| 组件 | 地址 |
|---|---|
| API | http://127.0.0.1:8080 |
| RabbitMQ 管理台 | http://127.0.0.1:15672 （weiyu / weiyu123456） |
| MySQL | 127.0.0.1:3306 |
| Redis | 127.0.0.1:6379 |

### 5.2 本地运行

```bash
# 1. 启动依赖（或使用 docker compose up -d mysql redis rabbitmq）
docker compose up -d mysql redis rabbitmq

# 2. 配置环境变量（在仓库根目录执行）
cp .env.example .env
set -a && source .env && set +a    # 或使用 direnv / IDE 运行配置

# 3. 启动服务（表结构由 AutoMigrate 自动创建）
go run .
```

> JWT_SECRET 必须 ≥ 32 字节，否则启动后签发 token 会直接报错。

---

## 6. 环境变量

| 变量 | 必填 | 默认值 | 说明 |
|---|---|---|---|
| `APP_ENV` | 否 | development | `production` 时监听 `:8080` |
| `HTTP_ADDR` | 否 | 见上 | 显式指定监听地址，优先级最高 |
| `MYSQL_DSN` | **是** | — | `user:pass@tcp(host:port)/db?charset=utf8mb4&parseTime=True&loc=Local` |
| `REDIS_ADDR` | 否 | 127.0.0.1:6379 | Redis 地址 |
| `REDIS_PASSWORD` | 否 | 空 | Redis 密码 |
| `REDIS_DB` | 否 | 0 | Redis 库号 |
| `RABBITMQ_URL` | 否 | amqp://guest:guest@127.0.0.1:5672/ | AMQP 连接串 |
| `JWT_SECRET` | **是** | — | 签名密钥，长度 ≥ 32 字节 |

---

## 7. API 文档

### 7.1 接口总览

| 方法 | 路径 | 鉴权 | 说明 |
|---|---|:---:|---|
| GET | `/api/v1/health` | — | 健康检查（探测 MySQL/Redis/RabbitMQ） |
| POST | `/api/v1/users/register` | — | 注册 |
| POST | `/api/v1/users/login` | — | 登录，返回 JWT |
| GET | `/api/v1/users/me` | ✅ | 当前用户信息 |
| GET | `/api/v1/posts` | — | 动态列表（`page`、`page_size`） |
| POST | `/api/v1/posts` | ✅ | 发布动态 |
| GET | `/api/v1/posts/:id` | — | 动态详情（走缓存） |
| GET | `/api/v1/posts/:id/comments` | — | 评论列表 |
| POST | `/api/v1/posts/:id/comments` | ✅ | 发表评论 |
| GET | `/api/v1/posts/:id/likes/count` | — | 点赞数（走缓存） |
| POST | `/api/v1/posts/:id/likes` | ✅ | 点赞 |
| DELETE | `/api/v1/posts/:id/likes` | ✅ | 取消点赞 |
| GET | `/api/v1/notifications` | ✅ | 我的通知（MQ 异步写入） |

### 7.2 统一响应

```json
{
  "code": 0,
  "message": "success",
  "data": {}
}
```

- `code = 0`：成功（HTTP 200 / 201）
- `code = HTTP 状态码`：失败，常见 400 参数错误、401 未登录/凭证失效、404 资源不存在、409 重复操作、500 服务端错误

### 7.3 调用示例

```bash
BASE=http://127.0.0.1:8080/api/v1

# 1. 注册（密码至少 8 位）
curl -s -X POST $BASE/users/register \
  -H 'Content-Type: application/json' \
  -d '{"username":"alice","password":"password123"}'

# 2. 登录拿 token
TOKEN=$(curl -s -X POST $BASE/users/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"alice","password":"password123"}' | jq -r '.data.token')

# 3. 发布动态
curl -s -X POST $BASE/posts \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"content":"hello weiyu"}'

# 4. 点赞（重复点赞返回 409）
curl -s -X POST $BASE/posts/1/likes -H "Authorization: Bearer $TOKEN"

# 5. 点赞数（第二次请求命中 Redis 缓存）
curl -s $BASE/posts/1/likes/count

# 6. 查通知（由 MQ 消费者异步写入，稍等片刻）
curl -s "$BASE/notifications?limit=10" -H "Authorization: Bearer $TOKEN"

# 7. 健康检查
curl -s $BASE/health
```

---

## 8. 核心设计说明

### 8.1 JWT 认证

- 登录成功签发 HS256 Token，有效期 2 小时，`sub` 存用户 ID，`iss` 固定为 `weiyu`
- 校验侧开启：算法白名单（只接受 HS256，防 alg 混淆）、issuer 校验、必须包含 exp、iat 校验
- 中间件解析 `Authorization: Bearer <token>`，把 userID 写入 Gin Context，控制器通过 `middleware.ContextUserID` 读取，全站不再出现硬编码测试用户
- 密钥从环境变量读取并强制 ≥ 32 字节，避免弱密钥
- **取舍**：无状态 JWT 无法主动撤销，当前以 2h 短 TTL 控制风险；主动登出需要引入 Redis 黑名单（见 Roadmap）

### 8.2 缓存设计（Redis）

| Key | 类型 | TTL | 用途 |
|---|---|---|---|
| `weiyu:post:detail:{id}` | String(JSON) | 10min + 0~5min 抖动 | 动态详情 |
| `weiyu:post:null:{id}` | String | 60s | 空值缓存，防穿透 |
| `weiyu:like:count:{id}` | String | 10min + 0~5min 抖动 | 点赞计数 |
| `weiyu:mq:idem:{eventID}` | String | 24h | 消费幂等 |

四类典型问题及处理：

| 问题 | 现象 | 处理 |
|---|---|---|
| 缓存穿透 | 大量请求查询不存在的 ID，全部打到 MySQL | 空值缓存（60s）+ 校验前置 |
| 缓存击穿 | 热点 key 过期瞬间，并发请求同时回源 | `singleflight` 合并同一 key 的回源，只放行一个 |
| 缓存雪崩 | 批量 key 同时过期 | TTL 叠加 0~50% 随机抖动 |
| 双写一致性 | 更新 DB 与缓存不一致 | Cache-Aside：写后删除缓存，读时回填；删除详情与空值两个 key |

**降级策略**：Redis 读写失败只记日志，读路径降级查库、写路径跳过计数更新，不影响主流程可用性。

### 8.3 点赞并发一致性

- `(post_id, user_id)` 联合唯一索引，重复点赞由数据库拒绝，应用层捕获 `gorm.ErrDuplicatedKey` 翻译为 409
- 不使用「先查后插」：两个并发请求都可能查到「未点赞」，从而双双插入，这是典型的 TOCTOU 竞态
- 取消点赞用 `RowsAffected == 0` 判定「本来就没点过」，把影响行数当作业务语义
- 计数缓存通过 Lua 脚本实现「仅在 key 存在时 INCRBY」，避免 key 未命中时把真实计数覆盖成 1

### 8.4 RabbitMQ 可靠性

| 环节 | 措施 |
|---|---|
| 拓扑 | `weiyu.events`(topic) → `weiyu.notify`；队列绑定 `x-dead-letter-exchange` |
| 生产 | 开启 confirm，`PublishWithDeferredConfirm` + `WaitContext` 等待 broker 确认；消息持久化 |
| 投递失败 | 2s 超时后仅告警，不阻塞点赞/评论主流程（旁路能力降级） |
| 消费 | `autoAck=false`，处理成功才 ack；`prefetch=10` |
| 幂等 | Redis `SETNX` 事件 ID（24h）+ `notifications.event_id` 唯一索引双保险 |
| 重试 | 失败后带 `x-retry` 头重新入队，最多 3 次 |
| 死信 | 超过重试次数 `reject(requeue=false)`，经 DLX 进入 `weiyu.notify.dlq` |

### 8.5 错误处理与状态码

- service 层返回哨兵错误（`ErrAlreadyLiked`、`ErrEmptyContent` 等），controller 用 `errors.Is` 映射为 HTTP 状态码，业务判断不散落在各层
- 用户不存在与密码错误统一返回「用户名或密码错误」，避免枚举已注册用户名
- 全站统一 `{code, message, data}` 响应结构，成功码恒为 0

### 8.6 优雅关闭

收到 `SIGINT/SIGTERM` 后：停止 MQ 消费者 → `srv.Shutdown` 等待在途请求（10s 超时）→ 按 defer 逆序关闭 RabbitMQ、Redis、MySQL 连接池，避免请求处理到一半连接被抽走。

---

## 9. 测试与压测

### 9.1 单元测试（无需外部依赖）

```bash
go test ./... -count=1
```

覆盖：JWT 签发/校验（含 alg=none 与过期 token 拒绝）、鉴权中间件（缺失/错误/合法 token）、缓存 key 与 TTL 抖动、通知内容截断、MQ 事件 ID 与重试头解析。

### 9.2 集成测试（需本地 Redis / RabbitMQ）

```bash
# Redis：缓存读写、空值缓存、Lua 计数器的边界行为
REDIS_ADDR=127.0.0.1:6379 REDIS_DB=15 \
go test -tags integration ./service -run TestRedis -v -count=1

# RabbitMQ：发布确认、消费 ack、幂等去重、重试 3 次后进入死信队列
REDIS_ADDR=127.0.0.1:6379 REDIS_DB=15 \
RABBITMQ_URL=amqp://guest:guest@127.0.0.1:5672/ \
go test -tags integration ./mq -run TestRabbitMQ -v -count=1
```

> 集成测试会清空 `REDIS_DB` 指定的库与 `weiyu.notify` 队列，不要指向生产环境。

### 9.3 压测

```bash
# 接口压测（需先安装：go install github.com/rakyll/hey@latest）
hey -n 20000 -c 100 http://127.0.0.1:8080/api/v1/posts/1

# 对比有无缓存：先清空 key，再压一轮
redis-cli DEL weiyu:post:detail:1

# 观察 Redis 命中情况
redis-cli MONITOR | grep weiyu:post

# 观察队列堆积与死信
docker compose exec rabbitmq rabbitmqctl list_queues name messages consumers

# 健康检查
curl -s http://127.0.0.1:8080/api/v1/health | jq
```

压测后建议记录：QPS、P99、缓存命中率、MySQL `COUNT(*)` 调用次数（可通过 general log 或慢查询观察）。

---

## 10. 已知限制与 Roadmap

| 项 | 现状 | 计划 |
|---|---|---|
| 单元测试 | 覆盖 JWT/中间件/工具函数，MQ 与 Redis 有集成测试 | 补齐 service 业务分支（testify + go-sqlmock） |
| 登出/主动失效 | 无 | Redis jti 黑名单 + 登出接口 |
| 限流 | 无 | 令牌桶中间件（Redis 计数或本地 `golang.org/x/time/rate`） |
| 深分页 | offset 分页 | keyset（游标）分页 |
| 评论分页 | 固定最多 50 条 | 增加 page/page_size |
| 消费端重连 | 进程内单连接 | 断线自动重连 + 消费者重建 |
| 可观测性 | 结构化日志 | requestID 串联、Prometheus 指标、链路追踪 |
| 部署 | 单机 compose | K8s 编排、水平扩缩容 |

---

## 11. 常见问题

**启动报 `JWT_SECRET 未配置或长度不足 32 字节`**
→ 未加载 `.env`，或密钥长度不足。用 `set -a && source .env && set +a` 后再启动。

**启动报 `MySQL 初始化失败` / `redis 连接失败`**
→ 依赖未启动或端口未映射。`docker compose ps` 检查健康状态；compose 内主机名是 `mysql` / `redis` / `rabbitmq`，不是 `127.0.0.1`。

**点赞成功但通知表没数据**
→ 检查 RabbitMQ 是否连通、消费者是否启动（日志关键字「通知消费者已启动」）、队列是否有堆积（管理台 Queues）。投递失败只会打 warn 日志，不会让点赞接口报错。

**重复点赞没有返回 409**
→ 确认 `likes` 表存在 `(post_id, user_id)` 唯一索引（AutoMigrate 会创建，历史表需手动补）。

**修改代码后 compose 没生效**
→ `docker compose up -d --build app`，仅 `restart` 不会重新构建镜像。
