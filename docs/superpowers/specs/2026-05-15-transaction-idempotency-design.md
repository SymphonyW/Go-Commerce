# 关键交易接口幂等性设计

## 目标

为 Go-Commerce 的关键交易链路补齐请求级幂等机制，优先覆盖：

1. 创建订单
2. 模拟支付成功 / 支付成功回调语义
3. 取消订单

设计目标是让客户端重试、网络抖动、消息重复消费都不会造成重复副作用：

- 不重复创建订单
- 不重复扣减库存
- 不重复处理支付成功
- 不重复取消订单
- 不重复恢复库存

## 方案选择

本次采用“**通用幂等记录表 + 现有领域状态约束**”的双层方案。

### 采用方案

1. HTTP 写接口统一要求 `Idempotency-Key` 请求头。
2. 订单服务与支付服务内部统一维护幂等记录。
3. 请求首次到达时先抢占幂等键，再执行真实业务。
4. 重复请求时：
   - 请求内容一致且已完成：直接回放首次响应
   - 请求内容不一致：返回冲突
   - 首次请求仍在处理中：返回冲突
5. 现有业务侧继续保留：
   - 订单状态机
   - 支付单 `payment_no` 唯一约束
   - 取消逻辑中的行锁与库存回补事务

### 选择理由

- 仅依赖状态机无法覆盖“重复创建订单”和“重复请求返回首次结果”。
- 仅做消息去重又无法覆盖 HTTP 重试。
- 通用幂等层负责“请求级唯一”，领域状态约束负责“业务级唯一”，两者边界清晰、互相补位。

## 接口契约

### 需要 `Idempotency-Key` 的接口

- `POST /api/orders`
- `POST /api/payments/:id/success`
- `PUT /api/orders/:id/cancel`

### 统一规则

- 缺少 `Idempotency-Key`：返回 `400 Bad Request`
- 首次请求成功：返回原接口成功响应
- 同一用户、同一路径、同一键、同一请求：返回首次响应快照
- 同一用户、同一路径、同一键、不同请求：返回 `409 Conflict`
- 同一用户、同一路径、同一键、首次请求仍处理中：返回 `409 Conflict`

### 协议传递

HTTP 网关读取 `Idempotency-Key`，并将其透传到下游 gRPC：

- `CreateOrderRequest.idempotency_key`
- `CancelOrderRequest.idempotency_key`
- `PaymentActionRequest.idempotency_key`

## 幂等记录模型

新增表：`idempotency_records`

建议字段：

- `id`
- `idempotency_key`
- `user_id`
- `request_path`
- `request_hash`
- `response_body`
- `status_code`
- `state`：`processing` / `completed`
- `created_at`
- `expired_at`

### 唯一约束

建立唯一索引：

```text
UNIQUE(user_id, request_path, idempotency_key)
```

这样同一用户在同一路径下的同一个键只能表达一个请求；同时也允许同一字符串在不同接口上安全复用。

### 过期策略

- 默认幂等窗口：24 小时
- `expired_at = created_at + 24h`
- 当前版本先写入过期时间，不引入定时清理任务；后续可再演进自动清理

## 请求指纹

`request_hash` 不基于原始 JSON 字符串，而基于“稳定化后的业务语义”计算，避免仅因字段顺序变化就误判为不同请求。

### 指纹内容

- 创建订单：
  - `user_id`
  - 归并后的 `items(product_id, quantity)`
- 模拟支付成功：
  - `user_id`
  - `payment_id`
- 取消订单：
  - `user_id`
  - `order_id`

### 设计说明

- 创建订单会先按 `product_id` 聚合请求项，再计算指纹。
- 这样 `[{1,1},{1,2}]` 与 `[{1,3}]` 会被视为同一业务请求。
- 请求体字段顺序不同、JSON 空白不同，不影响指纹。

## 通用处理流程

```text
HTTP 请求
  -> 校验 Idempotency-Key
  -> 计算 request_hash
  -> 尝试插入 idempotency_records(processing)
       ├─ 插入成功：继续真实业务
       ├─ 唯一键冲突 + hash 相同 + completed：回放首次响应
       ├─ 唯一键冲突 + hash 不同：409 Conflict
       └─ 唯一键冲突 + processing：409 Conflict
  -> 业务成功
  -> 保存 response_body / status_code
  -> 标记 completed
```

### 事务边界

- 幂等记录先于真实业务创建，用唯一索引抢占执行权。
- 真实业务继续使用各自领域事务：
  - 订单：订单主表、订单项、库存扣减同事务
  - 取消：状态流转、库存回补同事务
  - 支付：支付状态推进保持原有逻辑，并补齐并发保护
- 业务成功后，同步写回响应快照并将记录置为 `completed`

### 失败语义

- 如果真实业务失败，幂等记录仍保留为 `processing`
- 后续同键请求统一返回 `409 Conflict`
- 这样宁可短暂拒绝重试，也不冒重复执行副作用的风险

> 后续若要进一步优化，可引入失败态、补偿或超时回收；本次先优先保证交易安全。

## 创建订单幂等

### 目标

- 同一个下单请求重复发送，只创建一张订单
- 只扣减一次库存
- 重复请求返回相同订单结果
- 并发 20 次相同请求时，只允许一个请求进入真实业务

### 执行顺序

1. 校验 `Idempotency-Key`
2. 聚合订单项并计算 `request_hash`
3. 抢占幂等记录
4. 首次请求进入现有下单事务
5. 创建订单、创建订单项、扣减库存
6. 事务提交成功后保存首次响应快照
7. 重复同参请求直接回放该快照

### 与现有逻辑的关系

现有 `CreateOrder` 已经具备：

- 服务端快照计算
- 同事务库存扣减
- 防超卖

本次只在其外层补上统一幂等控制，不改变原有库存一致性主干。

## 支付幂等

### 目标

- 同一支付单只能成功一次
- 同一成功动作的重复请求不会重复推进
- `payment.succeeded` 事件重复消费时，订单只会从 `pending` 推进到 `paid` 一次

### HTTP 侧

- `POST /api/payments/:id/success` 强制要求 `Idempotency-Key`
- 相同键、相同请求：回放首次成功响应
- 相同键、不同请求：`409 Conflict`

### 领域侧

- `payment_no` 继续保留唯一约束
- 支付单只允许首次从 `created -> succeeded`
- 订单服务消费 `payment.succeeded` 时：
  - 如果订单是 `pending`：推进到 `paid`
  - 如果订单已经是 `paid`：直接视为已处理，不重复发布 `order.paid`

### 说明

- 同一 key 的重复调用走“响应重放”
- 不同 key 对已成功支付单再次发起成功动作，仍按业务规则拒绝，避免把调用方错误掩盖成正常幂等

## 取消订单幂等

### 目标

- 同一取消请求重复发送，不重复恢复库存
- 已取消订单再次取消，对外表现为幂等成功
- 超时消息重复消费，不会造成二次取消或二次回补

### HTTP 侧

- `PUT /api/orders/:id/cancel` 强制要求 `Idempotency-Key`
- 首次成功后保存响应快照
- 重复同参请求直接回放首次成功响应

### 领域侧

继续复用 `cancelOrderWithReason`：

- 行级锁读取订单
- 仅首次 `pending -> cancelled` 时恢复库存
- 已是 `cancelled` 时直接返回“无新增变更”

### 外部语义调整

- 当前重复取消时返回 `success=false`
- 本次改为幂等成功语义：
  - 首次取消：`success=true`
  - 已取消再次取消：`success=true`

这样能让“用户手动重试取消”和“消息重复投递”都符合调用方直觉。

## 消息重复消费

### `payment.succeeded`

- `MarkOrderPaid` 继续作为领域级防重入口
- 已支付订单再次收到同类事件时不再重复更新，也不再重复发布 `order.paid`

### `order.timeout.check`

- 继续复用共享取消路径
- 若订单已经取消或已支付，则只 ACK、不重复改状态
- 因为库存回补只发生在首次 `pending -> cancelled`，所以重复消息不会导致重复回补

## 数据库与并发

### 唯一索引

- `idempotency_records(user_id, request_path, idempotency_key)` 唯一
- `payments.payment_no` 继续唯一

### Duplicate key 处理

- 首次插入幂等记录时遇到唯一键冲突：
  - 读取已有记录
  - 比对 `request_hash`
  - 决定回放 / 冲突 / processing 冲突

### 竞争条件控制

- 幂等键唯一索引负责阻止同一请求并发进入
- 订单取消与支付推进继续使用事务和行锁保护状态流转
- 库存扣减与恢复继续保留数据库级原子更新

## 错误与返回约定

| 场景 | HTTP 结果 |
| --- | --- |
| 缺少 `Idempotency-Key` | `400 Bad Request` |
| 首次执行成功 | 原接口成功响应 |
| 同键同参重复 | 返回首次响应快照 |
| 同键异参 | `409 Conflict` |
| 同键处理中 | `409 Conflict` |

## 代码结构建议

为避免把判断散落在业务代码里，新增独立幂等模块：

- `internal/idempotency/model.go`
  - 幂等记录模型
- `internal/idempotency/service.go`
  - 抢占、读取、回放、完成写回
- `internal/idempotency/hash.go`
  - 稳定化请求摘要

接入位置：

- `internal/order/service.go`
- `internal/payment/service.go`
- `cmd/api-gateway/main.go`
- `api/order/order.proto`
- `api/payment/payment.proto`

这样业务服务只依赖一个清晰的幂等抽象，而不是在各处堆叠临时 `if`。

## 测试策略

### 创建订单

1. 同一个下单请求重复发送两次，只创建一张订单
2. 重复请求返回相同订单结果
3. 相同幂等键但不同请求体，返回冲突
4. 并发 20 次相同 `Idempotency-Key` 下单，最终只产生一笔订单

### 支付

5. 支付成功接口重复调用，只成功一次
6. 重复 `payment.succeeded` 事件只把订单处理一次

### 取消

7. 取消订单重复调用，库存不重复恢复
8. 超时消息重复消费，订单不会二次取消

### 网关与基础模块

9. 缺少 `Idempotency-Key` 时返回 `400`
10. 网关正确透传幂等键
11. 幂等组件覆盖：
    - 首次抢占
    - 同参回放
    - 异参冲突
    - processing 冲突
    - duplicate key 竞争

## 文档更新

同步更新：

- `README.md`
- `doc/API_Documentation.md`
- `doc/TECHNICAL_DOCUMENT.md`
- `doc/INTERVIEW.md`

### 重点内容

- 哪些接口必须传 `Idempotency-Key`
- `curl -H "Idempotency-Key: xxx"` 示例
- 请求级幂等与领域级幂等如何协作
- “如何保证订单接口幂等”的面试回答

## 非目标

本次不引入：

- Outbox
- 死信重试补偿体系
- 幂等记录自动清理任务
- 为所有写接口一次性铺开幂等

这些都可以作为后续演进点，但不应拖慢当前关键交易链路的收口。
