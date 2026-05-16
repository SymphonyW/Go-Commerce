# Go Commerce

[![CI](https://github.com/SymphonyW/Go-Commerce/actions/workflows/ci.yml/badge.svg)](https://github.com/SymphonyW/Go-Commerce/actions/workflows/ci.yml)

## 项目简介

Go Commerce是一个基于微服务架构的电子商务系统，使用Go语言开发后端服务，React开发前端应用，提供完整的购物体验。

## 持续集成（CI）

仓库已提供 GitHub Actions 工作流，会在 `main`、`master`、`dev` 分支的 `push` 以及所有 `pull_request` 时自动执行：

- `backend-check`：下载依赖、执行 `gofmt` 检查、`go vet`、单元测试、覆盖率统计、构建验证与 `golangci-lint`
- `frontend-check`：在 `frontend` 目录执行依赖安装、可选 lint 与前端构建
- `docker-build`：逐个验证主要服务镜像可以成功构建
- `integration-test`：启动 MySQL、Redis、RabbitMQ 后运行带 `integration` 标签的集成测试，并在结束后自动清理容器和卷

PR 合并前建议以 CI 全绿为最低门槛，并在 GitHub 分支保护中将 `backend-check`、`frontend-check`、`docker-build`、`integration-test` 设为必需检查。

## 主要特性

- **微服务架构**：采用模块化设计，各服务独立部署和扩展
- **用户认证**：基于JWT的安全认证系统
- **角色权限**：支持 `customer`、`merchant`、`admin` 三类角色，并校验商家资源归属
- **产品管理**：完整的产品CRUD操作，支持分类和搜索
- **购物车功能**：基于Redis的购物车管理
- **订单管理**：完整的订单创建和跟踪流程
- **支付模拟**：独立 `payment-service` 提供可演示的创建支付、模拟成功/失败能力
- **商家管理**：支持商家注册、商品管理（增删操作）
- **用户下单**：支持用户下单，后端按真实商品价格计算金额并保存下单快照
- **事件驱动**：订单与支付服务向 RabbitMQ 统一事件交换机发布领域事件，通知服务与订单服务按需异步消费
- **订单超时关闭**：基于 RabbitMQ TTL + DLX 自动取消超时未支付订单，并幂等回补库存
- **响应式前端**：使用React构建的现代化用户界面

## 快速启动

### 环境准备

- Go 1.24.0或更高版本
- Node.js 16.0或更高版本
- Docker和Docker Compose
- MySQL 8.0
- Redis 7.0+
- RabbitMQ 3.0+

### 依赖安装

#### 后端依赖

```bash
# 安装Go依赖
go mod download
```

#### 前端依赖

```bash
# 进入前端目录
cd frontend

# 安装npm依赖
npm install
```

### 配置设置

1. 确保Docker服务正在运行
2. 检查并确认`docker-compose.yml`文件中的环境变量配置正确

### 启动服务

#### 方式一：使用Docker Compose启动整个系统（推荐）

```bash
# 启动所有服务（包括基础服务和应用服务）
docker-compose up -d

# 查看服务状态
docker-compose ps

# 查看服务日志（例如查看API网关日志）
docker-compose logs -f api-gateway

# 启动前端
cd frontend
npm run dev
```

#### 方式二：手动启动服务

1. 启动基础服务：

```bash
docker-compose up -d mysql redis rabbitmq
```

2. 启动后端服务（按顺序）：

```bash
# 启动认证服务
go run ./cmd/auth-service

# 启动产品服务
go run ./cmd/product-service

# 启动订单服务
go run ./cmd/order-service

# 启动支付服务
go run ./cmd/payment-service

# 启动 Outbox Worker
go run ./cmd/outbox-worker

# 启动通知服务
go run ./cmd/notification-service

# 启动购物车服务
go run ./cmd/cart-service

# 启动商家服务
go run ./cmd/merchant-service

# 启动API网关
go run ./cmd/api-gateway
```

3. 启动前端服务：

```bash
# 进入前端目录
cd frontend

# 启动开发服务器
npm run dev
```

### 服务依赖关系

- **基础服务**：MySQL、Redis、RabbitMQ
- **核心服务**：认证服务、产品服务、购物车服务
- **依赖服务**：订单服务（核心交易依赖MySQL，RabbitMQ用于异步链路）、支付服务（核心交易依赖MySQL和订单服务，RabbitMQ用于异步链路）、Outbox Worker（依赖MySQL和RabbitMQ）、商家服务（依赖MySQL和RabbitMQ）、通知服务（依赖RabbitMQ）
- **入口服务**：API网关（依赖所有其他服务）

### 访问应用

- 前端应用：<http://localhost:5173>
- API网关：<http://localhost:8080> (Docker Compose) 或 <http://localhost:8081> (手动启动)
- MySQL：<http://localhost:3307> (用户名: root, 密码: password)
- Redis：<http://localhost:6379>
- RabbitMQ管理界面：<http://localhost:15672> (用户名: guest, 密码: guest)

### 环境变量说明

- **数据库连接**：`DB_DSN` - MySQL数据库连接字符串
- **Redis地址**：`REDIS_ADDR` - Redis服务地址
- **RabbitMQ地址**：`RABBITMQ_URL` - RabbitMQ服务地址
- **事件交换机**：`EVENT_EXCHANGE` - RabbitMQ 统一事件交换机名称，默认 `ecommerce.events`
- **Outbox 扫描间隔**：`OUTBOX_POLL_INTERVAL` - worker 轮询间隔，默认 `5s`
- **Outbox 批量大小**：`OUTBOX_BATCH_SIZE` - 每次最多扫描条数，默认 `100`
- **Outbox 最大重试**：`OUTBOX_MAX_RETRY` - 单条事件最大重试次数，默认 `5`
- **Outbox 基础退避**：`OUTBOX_RETRY_BASE_DELAY` - 指数退避基准值，默认 `1s`
- **订单支付超时**：`ORDER_PAYMENT_TIMEOUT_MINUTES` - 支持整数或小数分钟，默认 `15`；本地演示可设为 `0.5`
- **服务地址**：各服务间通信的地址配置（例如`AUTH_SERVICE_ADDR`）

### 故障排除

1. **服务启动失败**：检查Docker服务是否正常运行，查看服务日志获取详细错误信息
2. **数据库连接失败**：确认MySQL服务已启动，检查数据库连接字符串是否正确
3. **端口冲突**：如果端口已被占用，修改`docker-compose.yml`中的端口映射
4. **依赖服务未就绪**：确保基础服务（MySQL、Redis、RabbitMQ）在启动应用服务前已就绪
5. **前端无法访问API**：检查API网关服务是否正常运行，确认前端API配置中的地址是否正确

## RabbitMQ 事件链路

- 统一交换机：`ecommerce.events`（topic）
- 当前事件：`order.created`、`order.paid`、`order.shipped`、`order.completed`、`order.cancelled`、`order.timeout.check`、`order.timeout.cancelled`、`payment.succeeded`
- 生产者：`order-service`、`payment-service`
- 消费者：
  - `notification-service` 绑定队列 `notification.order.created`，消费 `order.created` 后打印“发送下单成功通知”日志
  - `order-service` 绑定队列 `order.payment.succeeded`，消费 `payment.succeeded` 后把订单从 `pending` 更新为 `paid`
  - `order-service` 同时监听 `order.timeout.cancel.queue`，当延迟消息经 DLX 到达后，仅对仍为 `pending` 的订单执行自动取消

当前所有与数据库状态变更绑定的领域事件都已经采用 **Outbox Pattern**：`order.created`、`order.paid`、`order.shipped`、`order.completed`、`order.cancelled`、`order.timeout.cancelled`、`payment.succeeded` 会与业务数据在同一个数据库事务内写入 `outbox_events`，随后由独立 `outbox-worker` 异步发布到 RabbitMQ。即使 RabbitMQ 暂时不可用，核心交易也能先提交，事件会保留在本地消息表中等待后台重试。

```text
业务事务 -> outbox_events(pending) -> outbox-worker -> RabbitMQ
```

### 事件验证

```bash
# 启动 RabbitMQ、订单服务和通知服务
docker-compose up -d rabbitmq order-service notification-service

# 创建订单后观察消费者日志
docker-compose logs -f notification-service

# 进入 RabbitMQ 管理页检查 exchange / queue
# http://localhost:15672  guest / guest
```

验证要点：创建订单后可在管理界面看到 `ecommerce.events` 与 `notification.order.created`，通知服务会打印 `order.created` 对应日志；支付成功后会出现 `order.payment.succeeded` 队列并把订单状态改为 `paid`；未支付订单会先进入 `order.timeout.delay.queue`，TTL 到期后经 `order.timeout.dlx` 转入 `order.timeout.cancel.queue`，随后自动变为 `cancelled` 并回补库存。

## 使用示例

### 注册新用户

```bash
curl -X POST http://localhost:8081/api/register \
  -H "Content-Type: application/json" \
  -d '{"username": "testuser", "password": "password123", "email": "test@example.com", "role": "customer"}'
```

> 注册时可选择 `customer` 或 `merchant`。`admin` 角色预留给后台配置或手工初始化，不开放公网自助注册。

### 登录获取令牌

```bash
curl -X POST http://localhost:8081/api/login \
  -H "Content-Type: application/json" \
  -d '{"username": "testuser", "password": "password123"}'
```

### 获取产品列表

```bash
curl http://localhost:8081/api/products
```

也可以直接组合分页、分类、搜索、排序与价格区间：

```bash
curl "http://localhost:8081/api/products?page=2&page_size=20&category=book&keyword=Go&sort_by=price&order=asc&min_price=50&max_price=120"
```

### 创建商家

```bash
curl -X POST http://localhost:8081/api/merchants \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer MERCHANT_TOKEN" \
  -d '{"name": "Test Merchant", "contact_info": "test@merchant.com"}'
```

### 商家添加商品

```bash
curl -X POST http://localhost:8081/api/merchants/products \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer MERCHANT_TOKEN" \
  -d '{"merchant_id": 1, "name": "Test Product", "description": "Test Description", "price": 99.99, "stock": 100, "category": "Electronics", "image_url": "https://example.com/image.jpg"}'
```

### 商家删除商品

```bash
curl -X DELETE http://localhost:8081/api/merchants/products \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer MERCHANT_TOKEN" \
  -d '{"merchant_id": 1, "product_id": 1}'
```

> 商家写操作都要求登录。`merchant` 只能管理自己名下的商家，`admin` 可管理全部商家，`customer` 无权执行商家写操作。

### 用户下单

```bash
curl -X POST http://localhost:8081/api/orders \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{"items": [{"product_id": 1, "quantity": 1}]}'
```

> 创建订单时，客户端只需要提交商品 ID 和数量；订单金额以后端查询到的真实商品价格为准，订单详情会保留下单时的商品名称与价格快照。

### 发起并模拟支付

```bash
# 创建支付单
curl -X POST http://localhost:8081/api/payments \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{"order_id": 1, "payment_method": "mock_balance"}'

# 模拟支付成功
curl -X POST http://localhost:8081/api/payments/1/success \
  -H "Authorization: Bearer YOUR_TOKEN"
```

> 支付接口只接受当前登录用户发起的请求。创建支付前会校验订单存在、归属当前用户、状态为 `pending`，支付金额直接取订单总金额；模拟失败时支付记录变为 `failed`，订单继续保持 `pending`，便于再次发起支付。

### 完整订单生命周期

```bash
# 商家或管理员发货
curl -X PUT http://localhost:8081/api/orders/1/ship \
  -H "Authorization: Bearer MERCHANT_OR_ADMIN_TOKEN"

# 用户确认收货
curl -X PUT http://localhost:8081/api/orders/1/complete \
  -H "Authorization: Bearer YOUR_TOKEN"
```

> 当前订单状态主线为 `pending -> paid -> shipped -> completed`，另有 `pending -> cancelled` 旁路。所有状态变化都会先经过统一状态机校验；商家只能为自己名下商品构成的订单发货，混合商家订单在当前最小实现中仅允许 `admin` 发货。

### 购物车操作

#### 添加商品到购物车

```bash
curl -X POST http://localhost:8081/api/cart/items \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{"product_id": 1, "quantity": 1}'
```

#### 获取购物车

```bash
curl -X GET http://localhost:8081/api/cart \
  -H "Authorization: Bearer YOUR_TOKEN"
```

#### 更新购物车商品数量

```bash
curl -X PUT http://localhost:8081/api/cart/items \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{"product_id": 1, "quantity": 2}'
```

#### 删除购物车商品

```bash
curl -X DELETE http://localhost:8081/api/cart/items \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{"product_id": 1}'
```

#### 清空购物车

```bash
curl -X DELETE http://localhost:8081/api/cart \
  -H "Authorization: Bearer YOUR_TOKEN"
```

## 相关文档

- [技术文档](doc/TECHNICAL_DOCUMENT.md)：详细的技术架构和实现说明
- [API文档](doc/API_Documentation.md)：完整的API接口说明


## 如何运行测试

项目现在按三层组织测试：

```text
Unit Test         -> 纯业务逻辑，sqlite in-memory / mock 依赖
Integration Test  -> 真实 MySQL、Redis、RabbitMQ
E2E Test          -> 通过 API Gateway 回归完整交易主链路
```

### 单元测试

```bash
make test
# 或
make test-unit
```

当前重点覆盖：

- `auth`：注册、登录、密码校验
- `product`：商品创建、分页、筛选、排序
- `order`：重复下单、价格快照、库存扣减、取消订单、状态机
- `payment`：创建支付、支付成功、已取消订单不可继续支付
- `merchant`：越权校验、添加商品、删除商品

### 集成测试

```bash
make test-integration
# 等价于先启动 mysql / redis / rabbitmq，再执行：
go test ./... -tags=integration
```

默认连接：

- MySQL：`127.0.0.1:3307`
- Redis：`127.0.0.1:6379`
- RabbitMQ：`127.0.0.1:5672`

如需覆盖默认值，可设置：

- `INTEGRATION_DB_DSN`
- `INTEGRATION_REDIS_ADDR`
- `INTEGRATION_RABBITMQ_URL`

当前集成测试覆盖：

- Redis 购物车读写
- MySQL 订单落库与库存扣减
- RabbitMQ 领域事件发布

### E2E 测试

```bash
make test-e2e
```

默认会先启动整套 Compose 服务，再通过 API 网关自动回归：

1. 注册商家与用户
2. 登录获取 token
3. 商家创建店铺和商品
4. 用户浏览商品并加入购物车
5. 创建订单
6. 发起支付并标记成功
7. 等待订单状态变为 `paid`
8. 查询订单详情

如需覆盖默认值，可设置：

- `E2E_BASE_URL`
- `E2E_DB_DSN`

### CI 建议

GitHub Actions 中可直接使用：

```bash
make test-unit
make test-integration
make test-e2e
```

其中集成测试依赖基础设施容器，E2E 测试依赖完整服务栈。若 CI 平台没有预装 `make`，也可以直接调用上面的 `go test` 命令。
