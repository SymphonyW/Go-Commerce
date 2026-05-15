# 订单超时自动取消设计

## 目标

为待支付订单增加“到期自动关闭”能力：订单创建后先保持 `pending`，若在配置窗口内仍未支付，则自动取消、恢复库存，并记录 `payment_timeout` 取消原因。

## 方案选择

本次采用 RabbitMQ **TTL + DLX**：

```text
CreateOrder -> order.timeout.check -> delay queue
                                      |
                                      | TTL 到期
                                      v
                                  dead-letter exchange
                                      |
                                      v
                                  cancel queue -> timeout consumer
```

这样不依赖 delayed-message 插件，和当前项目已经使用 RabbitMQ 的形态最一致。

## 组件边界

- `order-service`
  - 创建订单后发送延迟消息
  - 声明延迟交换机、延迟队列、DLX 与消费队列
  - 内部 goroutine 消费超时消息
- `internal/order`
  - 提供共享取消函数，人工取消与支付超时复用同一库存回补逻辑
  - 提供超时消息结构、调度器与消费者
- 前端
  - 根据 `cancel_reason = payment_timeout` 展示“已取消（支付超时）”

暂不拆独立 worker，因为订单状态机、库存回补和事务都属于订单服务；当前规模下留在同一服务里更清楚，未来需要独立扩容时再拆不迟。

## 消息结构

`order.timeout.check` 至少包含：

- `event_id`
- `order_id`
- `user_id`
- `created_at`
- `expire_at`
- `timeout_minutes`

超时取消成功后发布 `order.timeout.cancelled`，并保留通用的 `order.cancelled` 事件，便于后续订阅方既能监听“任何取消”，也能监听“支付超时取消”。

## 数据流

```text
订单事务提交成功
        |
        v
发送延迟消息
        |
        v
TTL 到期，经 DLX 转发
        |
        v
查询订单
   | pending
   v
统一取消流程
   |- status = cancelled
   |- cancel_reason = payment_timeout
   |- 恢复库存
   `- 发布取消事件
```

## 幂等与并发

- 只有 `pending -> cancelled` 的首次迁移会触发库存回补。
- 超时消息重复到达时，若订单已取消，消费者只记录日志并 ACK。
- 若订单已支付，状态机阻止其再进入 `cancelled`。
- 支付成功与取消逻辑都在事务中读取并锁定订单，避免并发下互相覆盖。

## 一致性说明

当前版本沿用项目现状：先提交数据库事务，再发送 RabbitMQ 消息。这样可以避免事务未提交时消息先被消费，但如果事务成功后 RabbitMQ 发送失败，超时检查消息仍可能丢失。后续如需更强可靠性，应引入本地消息表 / Outbox 与重试补偿。
