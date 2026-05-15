# 技术文档

## 1. 技术栈

| 技术/框架 | 版本 | 用途 |
|---------|------|------|
| Go | 1.24.0 | 后端服务开发 |
| React | 18.2.0 | 前端应用开发 |
| gRPC | 1.59.0 | 服务间通信 |
| Gin | 1.9.1 | API网关路由 |
| MySQL | 8.0 | 数据存储 |
| Redis | 7.0+ | 缓存与购物车存储 |
| RabbitMQ | 3.0+ | 消息队列 |
| Docker | 20.0+ | 容器化部署 |
| Vite | 8.0.1 | 前端构建工具 |
| TypeScript | 5.9.3 | 前端类型系统 |
| Axios | 1.13.6 | 前端HTTP客户端 |
| React Router | 7.13.2 | 前端路由管理 |

## 2. 系统架构

项目采用微服务架构，主要包含以下组件：

1. **API网关**：处理所有客户端请求，路由到相应的微服务
2. **认证服务**：处理用户注册、登录和认证
3. **产品服务**：管理产品信息
4. **订单服务**：处理订单创建和管理
5. **购物车服务**：管理用户购物车
6. **商户服务**：管理商户信息和商户产品
7. **通知服务**：异步消费订单事件并执行通知动作

各服务之间通过gRPC进行通信，数据存储使用MySQL和Redis，消息传递使用RabbitMQ。前端通过API网关与后端服务交互。

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│  前端应用   │────>│  API网关    │────>│  认证服务   │
└─────────────┘     └─────────────┘     └─────────────┘
        ↑                  │                  ↑
        │                  │                  │
        │                  ↓                  │
        │            ┌─────────────┐          │
        │            │  产品服务   │          │
        │            └─────────────┘          │
        │                  │                  │
        │                  ↓                  │
        │            ┌─────────────┐          │
        │            │  订单服务   │◄─────────┘
        │            └─────────────┘
        │                  │
        │                  ↓
        │            ┌─────────────┐
        │            │  商户服务   │
        │            └─────────────┘
        │                  │
        │                  ↓
        │            ┌─────────────┐
        └────────────│  购物车服务  │
                     └─────────────┘
```

## 3. 目录结构详解

```
Go-Commerce/
├── api/                # gRPC服务定义
│   ├── auth/           # 认证服务proto文件
│   ├── cart/           # 购物车服务proto文件
│   ├── merchant/       # 商户服务proto文件
│   ├── order/          # 订单服务proto文件
│   └── product/        # 产品服务proto文件
├── cmd/                # 服务入口点
│   ├── api-gateway/    # API网关服务
│   ├── auth-service/   # 认证服务
│   ├── cart-service/   # 购物车服务
│   ├── merchant-service/ # 商户服务
│   ├── notification-service/ # 通知服务
│   ├── order-service/  # 订单服务
│   └── product-service/ # 产品服务
├── doc/                # 项目文档
├── frontend/           # 前端应用
│   ├── dist/           # 构建输出
│   ├── public/         # 静态资源
│   └── src/            # 源代码
├── internal/           # 内部包
│   ├── auth/           # 认证服务内部实现
│   ├── cart/           # 购物车服务内部实现
│   ├── merchant/       # 商户服务内部实现
│   ├── notification/   # 通知消费逻辑
│   ├── order/          # 订单服务内部实现
│   └── product/        # 产品服务内部实现
├── pkg/                # 公共包
│   └── jwt/            # JWT工具
├── docker-compose.yml  # Docker Compose配置
├── go.mod              # Go模块配置
└── README.md           # 项目说明
```

### 主要目录说明

- **api/**：包含所有gRPC服务的proto定义文件和生成的Go代码
- **cmd/**：包含各个微服务的入口点和Dockerfile
- **doc/**：存放项目文档
- **frontend/**：前端React应用代码
- **internal/**：各服务的内部实现，包含业务逻辑和数据模型
- **pkg/**：公共工具包，可被多个服务使用

## 4. 模块功能说明

### 4.1 API网关

**职责**：作为系统的入口点，处理所有客户端请求，路由到相应的微服务。

**核心功能**：
- 请求路由：将客户端请求转发到对应的微服务
- 认证中间件：验证用户JWT令牌
- CORS处理：处理跨域请求

**主要文件**：`cmd/api-gateway/main.go`

### 4.2 认证服务

**职责**：处理用户注册、登录和认证。

**核心功能**：
- 用户注册：创建新用户账号
- 用户登录：验证用户凭据并生成JWT令牌
- 密码哈希：安全存储用户密码
- 角色写入：JWT 中携带 `user_id` 与 `role`，支持后续权限判断

**主要文件**：
- `internal/auth/service.go`：认证服务实现
- `internal/auth/model.go`：用户模型定义

### 4.3 产品服务

**职责**：管理产品信息。

**核心功能**：
- 产品列表：获取产品列表，支持分页和分类筛选
- 产品详情：获取单个产品的详细信息
- 产品创建：创建新产品（管理员功能）

**主要文件**：
- `internal/product/service.go`：产品服务实现
- `internal/product/model.go`：产品模型定义

### 4.4 订单服务

**职责**：处理订单创建和管理。

**核心功能**：
- 创建订单：基于商品真实信息创建订单，按后端价格计算总额
- 商品快照：保存下单时的商品名称与价格，保证历史订单不受后续改价影响
- 订单列表：获取用户的订单列表
- 订单详情：获取单个订单的详细信息

**主要文件**：
- `internal/order/service.go`：订单服务实现
- `internal/order/model.go`：订单模型定义

**事件发布设计**：
- 订单事务提交成功后，订单服务通过统一事件交换机发布 `order.created` 与 `order.cancelled`。
- 事件体统一包含 `event_id`、`event_type`、`occurred_at`，业务侧无需依赖 routing key 才能识别事件。
- 当前阶段采用弱一致：消息发布失败只记录结构化日志，不回滚已经成功提交的订单事务。

**库存一致性设计**：
- 创建订单时先在服务端合并重复 `product_id`，再基于真实商品信息生成订单快照。
- 扣减库存统一走条件更新：`UPDATE products SET stock = stock - ? WHERE id = ? AND stock >= ?`，通过 `RowsAffected` 判断是否成功，避免并发下出现超卖或负库存。
- 订单主表、订单项与库存扣减处于同一事务中；任一环节失败，库存变化会随事务一并回滚。
- 取消订单时统一走原子回补：`UPDATE products SET stock = stock + ? WHERE id = ?`。
- 可通过 `go test ./internal/order -run TestCreateOrderConcurrentRequestsDoNotOversell -v` 快速验证并发防超卖行为。

### 4.5 购物车服务

**职责**：管理用户购物车。

**核心功能**：
- 添加商品：向购物车添加商品，从产品服务获取实际商品信息
- 移除商品：从购物车移除商品
- 更新数量：更新购物车中商品的数量
- 清空购物车：清空用户的购物车
- 获取购物车：获取用户的完整购物车信息

**主要文件**：
- `internal/cart/service.go`：购物车服务实现
- `cmd/cart-service/main.go`：购物车服务入口

**技术特点**：
- 使用Redis存储购物车数据，提高性能
- 与产品服务集成，确保购物车中的商品信息与系统保持一致
- 支持购物车为空时的正确处理，避免前端显示空白页面

### 4.6 商户服务

**职责**：管理商户信息和商户产品。

**核心功能**：
- 商户管理：创建、查询商户信息
- 产品管理：商户添加、删除产品
- 商户列表：获取商户列表
- 资源归属：商户记录保存 `owner_user_id`
- 权限控制：`merchant` 只能管理自己的商户，`admin` 可以管理全部商户

**主要文件**：
- `internal/merchant/service.go`：商户服务实现
- `internal/merchant/grpc_service.go`：商户服务gRPC实现
- `internal/merchant/model.go`：商户模型定义

**RBAC 与归属校验设计**：
- 用户角色最小集合为 `customer`、`merchant`、`admin`
- API 网关先做粗粒度角色拦截，未登录返回 `401`，无角色权限返回 `403`
- 商户服务再基于数据库中的真实角色和 `owner_user_id` 做最终授权，避免只信任前端传入的 `merchant_id`
- 为兼容历史商户数据，`owner_user_id` 先允许为空；旧数据回填完成后，再考虑收紧为非空约束

### 4.7 通知服务

**职责**：异步消费订单事件，演示订单服务与后续业务服务之间的解耦。

**核心功能**：
- 声明并监听队列 `notification.order.created`
- 绑定统一交换机 `ecommerce.events` 的 `order.created` routing key
- 成功反序列化后打印“发送下单成功通知”日志并 ACK
- 遇到格式错误消息时 NACK 且不重回队列，避免坏消息无限循环

**主要文件**：
- `cmd/notification-service/main.go`：通知服务入口
- `internal/notification/consumer.go`：消息处理逻辑

### 4.8 前端应用

**职责**：提供用户界面，与API网关交互。

**核心功能**：
- 用户认证：注册、登录界面
- 产品浏览：产品列表、详情页面
- 购物车管理：添加、移除商品
- 订单管理：创建订单、查看订单历史
- 商户管理：创建商户、管理商户产品

**主要文件**：
- `frontend/src/main.jsx`：前端应用入口
- `frontend/src/components/Navbar.jsx`：导航组件
- `frontend/src/pages/`：各页面组件，包括新增的Merchants.jsx
- `frontend/src/services/api.js`：API调用服务，包含商户相关API


## 5. 事件驱动设计

| 事件 | 生产者 | 消费者 | 说明 |
|------|--------|--------|------|
| `order.created` | `order-service` | `notification-service` | 下单成功后异步触发通知 |
| `order.cancelled` | `order-service` | 暂无 | 当前先完成发布，为后续库存、营销、风控扩展预留 |

统一交换机使用 `ecommerce.events`（topic）。生产者通过 `pkg/mq.Publisher` 抽象发布能力，RabbitMQ 细节封装在 `pkg/mq`；事件结构定义在 `pkg/events`。这让订单服务只关心“发布什么事件”，而不是“怎样调用 AMQP SDK”。

当前消息链路是弱一致模型：数据库事务先提交，RabbitMQ 发布随后执行。若 RabbitMQ 暂时不可用，订单主流程继续成功，但会输出 `event_publish_failed` 日志，事件可能丢失。若未来要把“订单落库”和“事件必达”绑得更紧，需要继续引入本地消息表 / Outbox、重试与死信队列。
