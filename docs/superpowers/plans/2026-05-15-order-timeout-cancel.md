# Order Timeout Cancellation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为待支付订单增加基于 RabbitMQ TTL + DLX 的超时自动取消机制，并在超时后幂等回补库存。

**Architecture:** 订单创建事务提交后，由 `order-service` 发送 `order.timeout.check` 延迟消息到 TTL 队列；消息过期后经 DLX 投递到超时消费队列，仍由 `order-service` 内部 consumer 处理。取消流程收束到统一的订单取消函数，人工取消与支付超时共享状态迁移和库存回补逻辑。

**Tech Stack:** Go, GORM, RabbitMQ (TTL + DLX), gRPC/Protobuf, React, Docker Compose

---

### Task 1: 定义超时消息、取消原因与共享取消流程

**Files:**
- Modify: `pkg/events/order.go`
- Modify: `internal/order/model.go`
- Modify: `internal/order/service.go`
- Modify: `internal/order/payment_consumer.go`
- Test: `internal/order/service_test.go`

- [ ] 编写失败测试，覆盖订单创建后会安排超时检查消息、取消原因会落库、重复取消不会重复回补。
- [ ] 运行定向测试，确认当前实现因缺少调度器/取消原因/幂等取消而失败。
- [ ] 增加超时事件结构、取消原因常量和统一取消函数，并让人工取消改走共享逻辑。
- [ ] 让 `MarkOrderPaid` 与取消流程都在事务内加锁，避免支付与超时取消并发互相踩踏。
- [ ] 重新运行定向测试，确认通过。

### Task 2: 实现 RabbitMQ 超时调度器与消费者

**Files:**
- Create: `internal/order/timeout.go`
- Create: `internal/order/timeout_consumer.go`
- Create: `internal/order/timeout_consumer_test.go`
- Modify: `cmd/order-service/main.go`

- [ ] 编写失败测试，覆盖待支付订单超时取消、已支付订单跳过、重复消息幂等、已取消订单跳过、超时取消事件发布。
- [ ] 运行定向测试，确认缺少消费者实现时失败。
- [ ] 实现 `order.timeout.check` 调度器、TTL + DLX 拓扑声明和超时 consumer。
- [ ] 将 consumer 接入 `order-service` 启动流程，并从 `ORDER_PAYMENT_TIMEOUT_MINUTES` 读取支持小数分钟的配置。
- [ ] 重新运行定向测试，确认通过。

### Task 3: 暴露取消原因并更新前端展示

**Files:**
- Modify: `api/order/order.proto`
- Modify: `api/order/order.pb.go`
- Modify: `internal/order/service.go`
- Modify: `frontend/src/pages/OrderDetail.jsx`
- Modify: `frontend/src/pages/Orders.jsx`

- [ ] 编写或补充失败测试，确认返回给前端的订单包含 `cancel_reason`。
- [ ] 运行相关测试，确认当前 proto/转换逻辑缺失该字段。
- [ ] 增加 `cancel_reason` 字段并把它透传到前端。
- [ ] 将支付超时取消展示为“已取消（支付超时）”。
- [ ] 重新运行相关测试，确认通过。

### Task 4: 更新部署配置与文档

**Files:**
- Modify: `docker-compose.yml`
- Modify: `README.md`
- Modify: `doc/TECHNICAL_DOCUMENT.md`
- Modify: `doc/INTERVIEW.md`
- Create: `docs/superpowers/specs/2026-05-15-order-timeout-cancel-design.md`

- [ ] 将 `ORDER_PAYMENT_TIMEOUT_MINUTES` 加入 `order-service` 配置。
- [ ] 写入“延迟取消订单设计”、ASCII 流程图、弱一致说明和面试题答案。
- [ ] 自检文档是否覆盖 TTL/DLX、幂等、库存回补、为什么暂不拆 worker。

### Task 5: 全量验证

**Files:**
- Verify only

- [ ] 运行 `go test ./...`。
- [ ] 运行前端构建 `npm run build`。
- [ ] 检查 `git diff`，确认需求点都已落地且没有无关改动。
