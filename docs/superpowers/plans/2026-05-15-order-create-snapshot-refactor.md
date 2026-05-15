# 订单创建快照化重构 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将订单创建改造成仅信任 `product_id` 与 `quantity`，由后端生成商品快照、计算金额并完成库存校验。

**Architecture:** 请求层新增 `CreateOrderItem`，与响应用 `OrderItem` 分离；订单服务在事务内先聚合重复商品，再基于真实商品记录生成快照、计算金额并扣减库存；网关、文档与测试同步更新。现阶段继续沿用共享数据库事务，避免在缺少原子扣库存接口时引入跨服务一致性问题。

**Tech Stack:** Go 1.24、gRPC/protobuf、GORM、MySQL、Gin、Go testing、SQLite 测试库

---

## 文件结构

- `api/order/order.proto`：定义请求专用 `CreateOrderItem`
- `api/order/order.pb.go` / `api/order/order_grpc.pb.go`：同步生成后的协议代码
- `internal/order/service.go`：重构订单创建流程，新增输入聚合与快照生成逻辑
- `internal/order/service_test.go`：新增订单创建测试
- `cmd/api-gateway/main.go`：收紧创建订单请求体与转发字段
- `README.md`：更新使用示例
- `doc/API_Documentation.md`：更新接口说明
- `doc/TECHNICAL_DOCUMENT.md`：补充订单快照流程说明

### Task 1: 建立订单创建行为测试

**Files:**
- Create: `internal/order/service_test.go`
- Modify: `go.mod`

- [ ] **Step 1: 为测试引入 SQLite 依赖**

```bash
go get gorm.io/driver/sqlite
```

- [ ] **Step 2: 编写失败测试**

在 `internal/order/service_test.go` 中覆盖：

```go
func TestCreateOrderUsesDatabaseSnapshot(t *testing.T) {}
func TestCreateOrderRejectsMissingProduct(t *testing.T) {}
func TestCreateOrderRejectsInsufficientStock(t *testing.T) {}
func TestCreateOrderRejectsInvalidQuantity(t *testing.T) {}
func TestCreateOrderCalculatesTotalAcrossProducts(t *testing.T) {}
func TestCreateOrderMergesDuplicateProducts(t *testing.T) {}
```

这些测试要明确断言：

- 请求中即便出现伪造名称/价格，也不会进入最终订单；
- 总价使用数据库中的真实价格；
- 重复商品会被合并为一条订单项；
- 非法输入返回 `InvalidArgument`，商品缺失返回 `NotFound`。

- [ ] **Step 3: 运行测试并确认失败**

```bash
go test ./internal/order -run TestCreateOrder -v
```

预期：测试因当前协议与实现仍依赖旧输入结构而失败。

### Task 2: 升级订单创建协议

**Files:**
- Modify: `api/order/order.proto`
- Modify: `api/order/order.pb.go`
- Modify: `api/order/order_grpc.pb.go`

- [ ] **Step 1: 更新 proto**

```proto
message CreateOrderItem {
  int64 product_id = 1;
  int32 quantity = 2;
}

message CreateOrderRequest {
  int64 user_id = 1;
  repeated CreateOrderItem items = 2;
}
```

- [ ] **Step 2: 重新生成 Go 协议代码**

```bash
protoc --go_out=. --go-grpc_out=. api/order/order.proto
```

- [ ] **Step 3: 运行测试确认仍为红灯**

```bash
go test ./internal/order -run TestCreateOrder -v
```

预期：协议已更新，但服务实现尚未适配，测试继续失败。

### Task 3: 重构订单创建主流程

**Files:**
- Modify: `internal/order/service.go`

- [ ] **Step 1: 增加输入聚合函数**

```go
func aggregateCreateOrderItems(items []*pb.CreateOrderItem) (map[int64]int32, error)
```

要求：

- items 为空时报错；
- `product_id <= 0` 或 `quantity <= 0` 时返回参数错误；
- 相同 `product_id` 的数量累加。

- [ ] **Step 2: 增加快照准备函数**

```go
func buildOrderSnapshots(tx *gorm.DB, aggregatedItems map[int64]int32) ([]OrderItem, float64, error)
```

要求：

- 逐个查询商品真实记录；
- 商品不存在时返回 `NotFound`；
- 库存不足时返回 `InvalidArgument`；
- 订单项中的 `ProductName` 与 `Price` 来自数据库；
- 同步计算总金额；
- 对商品库存做事务内扣减。

- [ ] **Step 3: 重写 `CreateOrder`**

主流程应只负责：

1. 聚合输入；
2. 开启事务；
3. 准备快照与金额；
4. 创建订单；
5. 写入订单项；
6. 发布事件；
7. 提交事务并返回响应。

- [ ] **Step 4: 运行测试确认转绿**

```bash
go test ./internal/order -run TestCreateOrder -v
```

预期：订单创建相关测试全部通过。

### Task 4: 收紧 API 网关请求

**Files:**
- Modify: `cmd/api-gateway/main.go`

- [ ] **Step 1: 更新请求绑定结构**

```go
Items []struct {
    ProductId int64 `json:"product_id"`
    Quantity  int32 `json:"quantity"`
} `json:"items"`
```

- [ ] **Step 2: 更新转发协议**

```go
orderItems[i] = &pbOrder.CreateOrderItem{
    ProductId: item.ProductId,
    Quantity:  item.Quantity,
}
```

- [ ] **Step 3: 编译确认网关与订单协议协同正常**

```bash
go test ./cmd/api-gateway ./internal/order
```

预期：相关包可以编译并测试通过。

### Task 5: 更新文档

**Files:**
- Modify: `README.md`
- Modify: `doc/API_Documentation.md`
- Modify: `doc/TECHNICAL_DOCUMENT.md`

- [ ] **Step 1: 更新 README 下单示例**

```json
{"items": [{"product_id": 1, "quantity": 1}]}
```

- [ ] **Step 2: 更新 API 文档**

明确写出：

- 创建订单仅接收 `product_id` 与 `quantity`
- 价格以后端商品真实价格为准
- 响应中的名称与价格是下单快照

- [ ] **Step 3: 更新技术文档**

补充订单服务职责说明：

- 下单时读取真实商品信息
- 生成订单快照
- 历史订单不受后续商品改价影响

### Task 6: 全量验证

**Files:**
- Verify only

- [ ] **Step 1: 执行格式化**

```bash
gofmt -w internal/order/service.go internal/order/service_test.go cmd/api-gateway/main.go
```

- [ ] **Step 2: 执行全量测试**

```bash
go test ./...
```

- [ ] **Step 3: 检查 Git 变更**

```bash
git status --short
git diff --stat
```

- [ ] **Step 4: 提交实现**

```bash
git add api/order/order.proto api/order/order.pb.go api/order/order_grpc.pb.go internal/order/service.go internal/order/service_test.go cmd/api-gateway/main.go README.md doc/API_Documentation.md doc/TECHNICAL_DOCUMENT.md go.mod go.sum docs/superpowers/specs/2026-05-15-order-create-snapshot-design.md docs/superpowers/plans/2026-05-15-order-create-snapshot-refactor.md
git commit -m "refactor: secure order creation with item snapshots"
```
