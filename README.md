# Go Commerce

[![CI](https://github.com/SymphonyW/Go-Commerce/actions/workflows/ci.yml/badge.svg)](https://github.com/SymphonyW/Go-Commerce/actions/workflows/ci.yml)

一个用于展示电商核心链路的 Go 微服务项目。它覆盖了商品浏览、购物车、下单、模拟支付、商家后台、异步事件、订单超时关闭、可靠消息与基础可观测性，适合用来演示“交易链路如何从接口一直落到状态机和消息机制”。

> 当前项目更接近“可运行的工程化演示系统”，不是生产级商城。文档会严格区分已经实现的能力和后续计划，避免把设计愿景写成既成事实。

## 1. 项目简介

Go Commerce 采用 `API Gateway + gRPC 微服务 + RabbitMQ 事件总线` 的结构，对外提供 REST API，对内使用 gRPC。
当前已实现的主链路：

```text
注册/登录 -> 浏览商品 -> 加入购物车 -> 创建订单 -> 创建支付 -> 支付成功 -> 订单变为 paid
```

同时补齐了几条更能体现工程能力的支线：

- 下单时由后端读取真实商品价格，生成订单快照，前端不能传金额。
- 库存扣减采用数据库条件更新，避免并发超卖。
- 订单创建、支付成功、取消、发货、完成等事件通过 Outbox Pattern 可靠投递。
- 未支付订单通过 RabbitMQ `TTL + DLX` 自动关闭并回补库存。
- 商家后台支持当前店铺资料、商品维护、相关订单查看，并有资源归属校验。

## 快速体验

```bash
# 启动后端与基础设施
docker compose up -d --build

# 初始化演示数据（也可使用 make seed-demo）
go run ./cmd/seed-data

# 启动前端
cd frontend
npm install
npm run dev
```

| 入口 | 地址 |
| --- | --- |
| 前端 | `http://localhost:5173` |
| API Gateway | `http://localhost:8080` |

演示商户账号：

- 用户名：`demo_merchant`
- 密码：`password123`

> 演示数据初始化命令需要本机安装 Go 1.24+；详细启动与调试方式见后文“快速启动”。

## 2. 核心亮点

| 亮点 | 当前实现 |
| --- | --- |
| 后端定价 | 下单只接收 `product_id + quantity`，总价由订单服务基于真实商品数据计算 |
| 防超卖 | `UPDATE products SET stock = stock - ? WHERE id = ? AND stock >= ?` 原子扣减 |
| 订单状态机 | `pending -> paid -> shipped -> completed`，另有 `pending -> cancelled` |
| 超时关单 | RabbitMQ 延迟队列 + 死信交换机，超时后自动取消并回补库存 |
| 可靠事件 | 业务事务与 `outbox_events` 同事务提交，独立 worker 异步投递 RabbitMQ |
| 商家权限 | 网关先校验角色，服务端再按 `owner_user_id` 做资源归属授权 |
| 可观测性 | API Gateway 已接入请求 ID、Trace ID、结构化日志、`/metrics`、`/healthz`、`/readyz` |
| 测试体系 | 单元测试、真实依赖集成测试、端到端主链路回归测试 |

## 3. 技术栈

| 层次 | 技术 |
| --- | --- |
| 后端 | Go 1.24、Gin、gRPC、GORM |
| 前端 | React 18、Vite 8、Axios、React Router |
| 数据 | MySQL 8、Redis 7 |
| 消息 | RabbitMQ 3 |
| 观测 | Prometheus client、结构化日志、健康检查 |
| 工程化 | Docker Compose、GitHub Actions、Makefile |

## 4. 系统架构图

```mermaid
flowchart LR
    U["Browser / React"] --> G["API Gateway<br/>REST / JWT"]

    G --> A["auth-service"]
    G --> P["product-service"]
    G --> C["cart-service"]
    G --> O["order-service"]
    G --> PAY["payment-service"]
    G --> M["merchant-service"]

    C --> P
    PAY --> O

    A --> DB[("MySQL ecommerce")]
    P --> DB
    O --> DB
    PAY --> DB
    M --> DB
    C --> R[("Redis")]

    O --> OB[("outbox_events")]
    PAY --> OB
    OB --> W["outbox-worker"]
    W --> MQ["RabbitMQ<br/>ecommerce.events"]

    O --> TQ["RabbitMQ TTL / DLX<br/>order timeout"]
    TQ --> O
    MQ --> N["notification-service"]
    MQ --> O
```

当前演示环境中，多个服务共享同一个 `ecommerce` MySQL 数据库；服务拆分已经落地，但“每服务独立数据库”尚未实现。

## 5. 功能模块总览

| 模块 | 能力 |
| --- | --- |
| 认证 | 用户注册、登录、JWT、`customer / merchant / admin` 角色 |
| 商品 | 商品列表、详情、分页、分类、关键词、价格区间、排序 |
| 购物车 | Redis 持久化购物车、加购、修改、删除、清空 |
| 订单 | 创建、查询、取消、发货、完成、状态机、超时关闭 |
| 支付 | 模拟支付创建、成功、失败，成功后异步驱动订单变更 |
| 商家后台 | 当前店铺、商品维护、商家订单查看、跨店权限隔离 |
| 事件总线 | `order.*`、`payment.succeeded` 事件与通知消费者 |
| 可靠消息 | Outbox 本地消息表、重试、失败落库 |
| 观测 | 健康检查、就绪检查、网关指标、链路关联 ID |

## 6. 目录结构

```text
Go-Commerce/
├── api/                    # gRPC proto 与生成代码
├── cmd/                    # 各服务入口
│   ├── api-gateway/
│   ├── auth-service/
│   ├── cart-service/
│   ├── merchant-service/
│   ├── notification-service/
│   ├── order-service/
│   ├── outbox-worker/
│   ├── payment-service/
│   └── product-service/
├── internal/               # 各领域服务实现
├── pkg/                    # 公共包：JWT、MQ、观测、健康检查
├── frontend/               # React 前端
├── tests/                  # integration / e2e
├── doc/                    # 技术文档、API 文档、面试文档
├── docker-compose.yml
├── Makefile
└── openapi.yaml            # OpenAPI 3.0 描述
```

## 7. 快速启动

### 7.1 前置条件

- Docker 与 Docker Compose（用于启动基础设施和后端服务）
- Node.js 22+（用于启动前端）
- Go 1.24+（用于手动启动后端，或执行本地演示数据初始化命令 `go run ./cmd/seed-data` / `make seed-demo`）

### 7.2 推荐方式：Docker Compose 启动后端与基础设施

```bash
docker compose up -d --build
docker compose ps
```

启动完成后：

| 入口 | 地址 |
| --- | --- |
| API Gateway | `http://localhost:8080` |
| RabbitMQ 控制台 | `http://localhost:15672`（`guest / guest`） |
| MySQL | `127.0.0.1:3307` |
| Redis | `127.0.0.1:6379` |

### 7.3 启动前端

```bash
cd frontend
npm install
npm run dev
```

浏览器访问 `http://localhost:5173`。

### 7.4 初始化演示数据

为了让首页、商品列表、商品详情、购物车和商家列表在首次打开时就具备完整展示效果，建议先导入演示数据。以下导入命令在本机执行，因此也需要 Go 1.24+：

```bash
# 如果你只想先启动数据库
docker compose up -d mysql

# 或直接启动后端与基础设施
docker compose up -d

# 导入演示商家与演示商品
go run ./cmd/seed-data
```

也可以使用快捷命令：

```bash
make seed-demo
```

该命令会初始化 1 个可登录演示商户账号、4 个演示商家与 20 个演示商品，覆盖：

- `数码科技`
- `居家生活`
- `户外骑行`
- `图书学习`

演示商家包括：

- 森屿数码馆
- 北屿生活家
- 远野出行社
- 纸上工坊

演示商户账号：

- 用户名：`demo_merchant`
- 密码：`password123`

这 4 个演示商家都会绑定到该账号，可直接登录进入商家后台，并通过“我的店铺”切换到不同店铺继续维护商品。演示商品直接写入后端 MySQL，而不是在前端写死 mock 数据，因此首页、列表、详情、购物车和订单链路都能基于真实接口运行。命令可重复执行；再次运行时会同步演示账号、商家归属与商品图片地址，不会无限重复插入。

> 如果你希望最快看到完整视觉效果，推荐顺序是：`docker compose up -d` → `go run ./cmd/seed-data` → 启动前端。

### 7.5 手动启动后端（用于调试）

先启动基础设施：

```bash
docker compose up -d mysql redis rabbitmq
```

然后分别启动：

```bash
go run ./cmd/auth-service
go run ./cmd/product-service
go run ./cmd/order-service
go run ./cmd/payment-service
go run ./cmd/outbox-worker
go run ./cmd/notification-service
go run ./cmd/cart-service
go run ./cmd/merchant-service
go run ./cmd/api-gateway
```

> 手动启动时，程序直接读取系统环境变量；仓库没有内置 `.env` 自动加载器，配置方式见下文“8. 环境变量”。

## 8. 环境变量

`.env.example` 用于说明本地手动启动时可配置的环境变量。当前项目不会自动加载 `.env` 文件；如需手动启动后端服务，请自行通过 shell、IDE 或运行环境导出这些变量。使用 Docker Compose 启动时，默认环境变量已在 `docker-compose.yml` 中配置。

| 变量 | 作用 | 默认值 |
| --- | --- | --- |
| `DB_DSN` | MySQL 连接串 | `root:password@tcp(127.0.0.1:3307)/ecommerce?...` |
| `REDIS_ADDR` | Redis 地址 | `127.0.0.1:6379` |
| `RABBITMQ_URL` | RabbitMQ 连接地址 | `amqp://guest:guest@127.0.0.1:5672/` |
| `EVENT_EXCHANGE` | 统一事件交换机 | `ecommerce.events` |
| `HTTP_ADDR` | API Gateway 地址 | `:8080` |
| `*_SERVICE_ADDR` | 服务间 gRPC 地址 | 见 `.env.example` |
| `*_GRPC_ADDR` | 各服务监听地址 | `:50051` ~ `:50056` |
| `*_HEALTH_ADDR` | 各服务健康检查地址 | `:8081` ~ `:8088` |
| `GATEWAY_GRPC_TIMEOUT` | 网关下游调用超时 | `3s` |
| `SERVICE_GRPC_TIMEOUT` | 服务间调用超时 | `3s` |
| `ORDER_PAYMENT_TIMEOUT_MINUTES` | 订单支付窗口 | `15` |
| `OUTBOX_POLL_INTERVAL` | Outbox 扫描间隔 | `5s` |
| `OUTBOX_BATCH_SIZE` | Outbox 单批数量 | `100` |
| `OUTBOX_MAX_RETRY` | Outbox 最大重试次数 | `5` |
| `OUTBOX_RETRY_BASE_DELAY` | Outbox 退避基准 | `1s` |

完整参考配置见 `.env.example`。

## 9. API 示例

### 注册并登录

```bash
curl -X POST http://localhost:8080/api/register \
  -H "Content-Type: application/json" \
  -d '{"username":"demo_customer","password":"password123","email":"demo@example.com","role":"customer"}'

curl -X POST http://localhost:8080/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"demo_customer","password":"password123"}'
```

### 查询商品

```bash
curl "http://localhost:8080/api/products?page=1&page_size=10&keyword=Go&sort_by=price&order=asc"
```

### 创建订单

```bash
curl -X POST http://localhost:8080/api/orders \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Idempotency-Key: order-demo-001" \
  -d '{"items":[{"product_id":1,"quantity":1}]}'
```

### 创建并完成模拟支付

```bash
curl -X POST http://localhost:8080/api/payments \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{"order_id":1,"payment_method":"mock_balance"}'

curl -X POST http://localhost:8080/api/payments/1/success \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Idempotency-Key: payment-demo-001"
```

更完整的接口说明见 [API 文档](doc/API_Documentation.md) 与 [OpenAPI 描述](openapi.yaml)。
其中，`doc/API_Documentation.md` 用于面向阅读地解释接口，`openapi.yaml` 则是机器可读的 OpenAPI 3.0 接口描述；二者都以当前已经实现的 Gateway 接口为准，不描述未来能力。

## 10. 测试方式

```bash
make test
make test-integration
make test-e2e
```

| 测试层级 | 覆盖内容 |
| --- | --- |
| Unit | 业务逻辑、状态机、幂等记录、库存、商家权限 |
| Integration | MySQL、Redis、RabbitMQ 的真实交互 |
| E2E | 通过 API Gateway 回归下单到支付成功主链路 |

CI 当前自动执行：

- `backend-check`
- `frontend-check`
- `docker-build`
- `integration-test`

`E2E` 测试已提供，但尚未纳入默认 GitHub Actions 工作流。

## 11. 监控与运维入口

### 已实现

- API Gateway：`/metrics`、`/healthz`、`/readyz`
- 各后端服务：独立 `/healthz` 与 `/readyz`
- 网关链路字段：`X-Request-ID`、`X-Trace-ID`
- 网关结构化 JSON 日志
- RabbitMQ 管理台：查看交换机、队列与投递情况

### 尚未实现

- 仓库未内置 Prometheus / Grafana Compose 服务
- 结构化日志、gRPC 指标与统一 trace 传播尚未铺到全部服务
- 当前更接近“链路关联 ID”，还不是完整的 OpenTelemetry 分布式追踪

## 12. 前端页面

当前前端已提供：

- 首页 `/`
- 商品列表与详情 `/products`
- 购物车 `/cart`
- 订单列表与详情 `/orders`
- 商家列表 `/merchants`
- 商家后台 `/merchant`
- 商家商品管理 `/merchant/products`
- 商家订单 `/merchant/orders`

> 下图基于本地演示数据截取，用于展示当前已实现的前端页面形态。
> 为获得最接近截图的展示效果，建议先执行上一节的“初始化演示数据”命令。

| 首页 | 商品列表 | 商家后台 |
| --- | --- | --- |
| ![首页截图](doc/assets/homepage.png) | ![商品列表截图](doc/assets/products.png) | ![商家后台截图](doc/assets/merchant-dashboard.png) |

## 13. Roadmap

### 已实现

- 可靠事件投递（Outbox）
- 下单幂等记录
- 订单超时自动关闭
- 商家后台与资源归属校验
- 交易主链路 E2E

### 计划继续完善

- 为 `取消订单`、`支付成功` 补齐真正落库的幂等重放，而不仅是状态天然幂等
- 统一 API Gateway 的错误码映射，减少历史接口直接返回 `500`
- 为公开商家列表、用户订单列表暴露真实分页参数
- 把观测能力铺到所有服务，并接入 Prometheus / Grafana / OpenTelemetry
- 让前端自动生成并携带 `Idempotency-Key`
- 逐步演进到更清晰的数据库边界与迁移机制
- 为 OpenAPI 补充 Swagger UI

## 14. 相关文档

- [技术文档](doc/TECHNICAL_DOCUMENT.md)
- [API 文档](doc/API_Documentation.md)：面向阅读的接口说明文档
- [面试文档](doc/INTERVIEW.md)
- [OpenAPI 3.0](openapi.yaml)：机器可读的接口契约

`doc/API_Documentation.md` 与 `openapi.yaml` 均以当前已经实现的 Gateway 接口为准，不描述未来能力。
