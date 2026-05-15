# 关键交易接口幂等性 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为创建订单、模拟支付成功、取消订单补齐统一的请求级幂等机制，并确保重复 HTTP 请求与重复消息消费都不会造成重复副作用。

**Architecture:** 新增独立的 `internal/idempotency` 模块，统一负责幂等记录、请求指纹、重复键判定与首次响应快照。HTTP 网关强制要求 `Idempotency-Key` 并透传到 gRPC；订单服务在 `CreateOrder` / `CancelOrder` 外层接入幂等控制，支付服务在 `MarkPaymentSucceeded` 的 gRPC 边界接入幂等控制；订单状态机、支付单状态与库存事务继续承担领域级防重职责。

**Tech Stack:** Go 1.24、gRPC/protobuf、Gin、GORM、MySQL、SQLite test DB、Go testing、RabbitMQ event consumers

---

## 文件结构

- `internal/idempotency/model.go`
  - 定义 `idempotency_records` 对应模型与状态常量
- `internal/idempotency/hash.go`
  - 提供稳定请求指纹计算
- `internal/idempotency/service.go`
  - 提供首次抢占、重复判定、响应快照写回与重放能力
- `internal/idempotency/service_test.go`
  - 覆盖首次抢占、同参回放、异参冲突、处理中冲突、哈希稳定性
- `api/order/order.proto`
  - 为创建订单、取消订单补充 `idempotency_key`
- `api/order/order.pb.go`
  - 重新生成订单 proto 代码
- `api/order/order_grpc.pb.go`
  - 重新生成订单 gRPC 代码
- `api/payment/payment.proto`
  - 为支付动作补充 `idempotency_key`
- `api/payment/payment.pb.go`
  - 重新生成支付 proto 代码
- `api/payment/payment_grpc.pb.go`
  - 重新生成支付 gRPC 代码
- `cmd/api-gateway/main.go`
  - 校验 `Idempotency-Key`、加入 CORS 允许头、透传下游
- `cmd/api-gateway/main_test.go`
  - 覆盖三个写接口的 header 必填与透传
- `internal/order/service.go`
  - 在创建订单、取消订单外层接入幂等控制，并统一重复取消语义
- `internal/order/service_test.go`
  - 覆盖下单重复请求、同键异参、20 并发同键、重复取消
- `internal/order/payment_consumer_test.go`
  - 补充 `payment.succeeded` 重复消费测试
- `internal/order/timeout_consumer_test.go`
  - 继续沿用并验证超时消息重复消费幂等行为
- `internal/payment/grpc_service.go`
  - 在 `MarkPaymentSucceeded` gRPC 边界接入幂等控制
- `internal/payment/grpc_service_test.go`
  - 覆盖支付成功同键重放、同键异参冲突
- `cmd/order-service/main.go`
  - 自动迁移幂等记录表
- `cmd/payment-service/main.go`
  - 自动迁移幂等记录表
- `README.md`
  - 增加 `Idempotency-Key` 使用示例
- `doc/API_Documentation.md`
  - 标明哪些接口必须传 `Idempotency-Key`
- `doc/TECHNICAL_DOCUMENT.md`
  - 说明请求级幂等与领域级幂等协作
- `doc/INTERVIEW.md`
  - 增加“如何保证订单接口幂等”

## Task 1: 建立通用幂等核心模块

**Files:**
- Create: `internal/idempotency/model.go`
- Create: `internal/idempotency/hash.go`
- Create: `internal/idempotency/service.go`
- Create: `internal/idempotency/service_test.go`

- [ ] **Step 1: 先写失败测试**

在 `internal/idempotency/service_test.go` 中新增：

```go
package idempotency

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/wrapperspb"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestService(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	if err := db.AutoMigrate(&Record{}); err != nil {
		t.Fatalf("failed to migrate db: %v", err)
	}
	return NewService(db, 24*time.Hour), db
}

func TestBeginCreatesProcessingRecord(t *testing.T) {
	service, db := newTestService(t)

	result, err := service.Begin(context.Background(), BeginRequest{
		UserID:         1,
		RequestPath:    "POST /api/orders",
		IdempotencyKey: "order-key",
		RequestHash:    "hash-a",
	})
	if err != nil {
		t.Fatalf("Begin returned error: %v", err)
	}
	if got, want := result.Action, ActionProceed; got != want {
		t.Fatalf("unexpected action: got %q want %q", got, want)
	}

	var record Record
	if err := db.First(&record, result.Record.ID).Error; err != nil {
		t.Fatalf("failed to load record: %v", err)
	}
	if got, want := record.State, StateProcessing; got != want {
		t.Fatalf("unexpected state: got %q want %q", got, want)
	}
}

func TestBeginReplaysCompletedRecord(t *testing.T) {
	service, _ := newTestService(t)
	first, err := service.Begin(context.Background(), BeginRequest{
		UserID:         1,
		RequestPath:    "POST /api/orders",
		IdempotencyKey: "order-key",
		RequestHash:    "hash-a",
	})
	if err != nil {
		t.Fatalf("Begin returned error: %v", err)
	}
	if err := service.Complete(context.Background(), first.Record.ID, 200, wrapperspb.String("ok")); err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}

	replay, err := service.Begin(context.Background(), BeginRequest{
		UserID:         1,
		RequestPath:    "POST /api/orders",
		IdempotencyKey: "order-key",
		RequestHash:    "hash-a",
	})
	if err != nil {
		t.Fatalf("second Begin returned error: %v", err)
	}
	if got, want := replay.Action, ActionReplay; got != want {
		t.Fatalf("unexpected action: got %q want %q", got, want)
	}

	var restored wrapperspb.StringValue
	if err := ReplayInto(replay.Record, &restored); err != nil {
		t.Fatalf("ReplayInto returned error: %v", err)
	}
	if got, want := restored.Value, "ok"; got != want {
		t.Fatalf("unexpected restored value: got %q want %q", got, want)
	}
}

func TestBeginRejectsHashMismatch(t *testing.T) {
	service, _ := newTestService(t)
	if _, err := service.Begin(context.Background(), BeginRequest{
		UserID:         1,
		RequestPath:    "POST /api/orders",
		IdempotencyKey: "order-key",
		RequestHash:    "hash-a",
	}); err != nil {
		t.Fatalf("Begin returned error: %v", err)
	}

	_, err := service.Begin(context.Background(), BeginRequest{
		UserID:         1,
		RequestPath:    "POST /api/orders",
		IdempotencyKey: "order-key",
		RequestHash:    "hash-b",
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBeginRejectsProcessingDuplicate(t *testing.T) {
	service, _ := newTestService(t)
	if _, err := service.Begin(context.Background(), BeginRequest{
		UserID:         1,
		RequestPath:    "POST /api/orders",
		IdempotencyKey: "order-key",
		RequestHash:    "hash-a",
	}); err != nil {
		t.Fatalf("Begin returned error: %v", err)
	}

	_, err := service.Begin(context.Background(), BeginRequest{
		UserID:         1,
		RequestPath:    "POST /api/orders",
		IdempotencyKey: "order-key",
		RequestHash:    "hash-a",
	})
	if !errors.Is(err, ErrInProgress) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHashPayloadIsStable(t *testing.T) {
	first, err := HashPayload(map[string]any{
		"user_id": 1,
		"items": []map[string]any{
			{"product_id": 1, "quantity": 2},
		},
	})
	if err != nil {
		t.Fatalf("HashPayload returned error: %v", err)
	}
	second, err := HashPayload(map[string]any{
		"items": []map[string]any{
			{"quantity": 2, "product_id": 1},
		},
		"user_id": 1,
	})
	if err != nil {
		t.Fatalf("HashPayload returned error: %v", err)
	}
	if first != second {
		t.Fatalf("expected stable hash, got %q and %q", first, second)
	}
}
```

- [ ] **Step 2: 运行测试确认红灯**

```bash
go test ./internal/idempotency -v
```

Expected: FAIL，原因是 `Record`、`Service`、`HashPayload`、`ReplayInto` 尚未定义。

- [ ] **Step 3: 写最小实现**

在 `internal/idempotency/model.go` 中写入：

```go
package idempotency

import (
	"time"

	"gorm.io/gorm"
)

const (
	StateProcessing = "processing"
	StateCompleted  = "completed"
)

// Record 保存一次幂等请求的指纹与首次成功响应快照。
type Record struct {
	gorm.Model
	IdempotencyKey string    `gorm:"size:128;not null;uniqueIndex:idx_user_path_key"`
	UserID         uint      `gorm:"not null;uniqueIndex:idx_user_path_key"`
	RequestPath    string    `gorm:"size:128;not null;uniqueIndex:idx_user_path_key"`
	RequestHash    string    `gorm:"size:64;not null"`
	ResponseBody   string    `gorm:"type:longtext"`
	StatusCode     int       `gorm:"not null;default:0"`
	State          string    `gorm:"size:16;not null;index"`
	ExpiredAt      time.Time `gorm:"not null;index"`
}
```

在 `internal/idempotency/hash.go` 中写入：

```go
package idempotency

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// HashPayload 基于稳定 JSON 编码生成请求摘要，屏蔽字段顺序差异。
func HashPayload(payload any) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}
```

在 `internal/idempotency/service.go` 中写入：

```go
package idempotency

import (
	"context"
	"errors"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

var (
	ErrConflict   = errors.New("idempotency key reused with different request")
	ErrInProgress = errors.New("idempotent request is still processing")
)

type Action string

const (
	ActionProceed Action = "proceed"
	ActionReplay  Action = "replay"
)

type BeginRequest struct {
	UserID         uint
	RequestPath    string
	IdempotencyKey string
	RequestHash    string
}

type BeginResult struct {
	Action Action
	Record *Record
}

type Service struct {
	db  *gorm.DB
	ttl time.Duration
	now func() time.Time
}

func NewService(db *gorm.DB, ttl time.Duration) *Service {
	return &Service{
		db:  db,
		ttl: ttl,
		now: time.Now,
	}
}

func (s *Service) Begin(ctx context.Context, req BeginRequest) (*BeginResult, error) {
	record := &Record{
		IdempotencyKey: req.IdempotencyKey,
		UserID:         req.UserID,
		RequestPath:    req.RequestPath,
		RequestHash:    req.RequestHash,
		State:          StateProcessing,
		ExpiredAt:      s.now().Add(s.ttl),
	}
	createErr := s.db.WithContext(ctx).Create(record).Error
	if createErr == nil {
		return &BeginResult{Action: ActionProceed, Record: record}, nil
	}

	var existing Record
	if err := s.db.WithContext(ctx).
		Where("user_id = ? AND request_path = ? AND idempotency_key = ?", req.UserID, req.RequestPath, req.IdempotencyKey).
		First(&existing).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, createErr
		}
		return nil, err
	}
	if existing.RequestHash != req.RequestHash {
		return nil, ErrConflict
	}
	if existing.State == StateProcessing {
		return nil, ErrInProgress
	}
	return &BeginResult{Action: ActionReplay, Record: &existing}, nil
}

func (s *Service) Complete(ctx context.Context, recordID uint, statusCode int, response proto.Message) error {
	body, err := protojson.Marshal(response)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).
		Model(&Record{}).
		Where("id = ?", recordID).
		Updates(map[string]any{
			"response_body": string(body),
			"status_code":   statusCode,
			"state":         StateCompleted,
		}).Error
}

func ReplayInto(record *Record, response proto.Message) error {
	return protojson.Unmarshal([]byte(record.ResponseBody), response)
}
```

- [ ] **Step 4: 运行测试确认转绿**

```bash
go test ./internal/idempotency -v
```

Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/idempotency
git commit -m "feat: add reusable idempotency core"
```

## Task 2: 扩展 gRPC 协议并重新生成代码

**Files:**
- Modify: `api/order/order.proto`
- Modify: `api/order/order.pb.go`
- Modify: `api/order/order_grpc.pb.go`
- Modify: `api/payment/payment.proto`
- Modify: `api/payment/payment.pb.go`
- Modify: `api/payment/payment_grpc.pb.go`

- [ ] **Step 1: 先让下游测试引用新字段并进入红灯**

在后续将要修改的测试文件中，统一按以下方式构造请求：

```go
&pb.CreateOrderRequest{
	UserId:         1,
	IdempotencyKey: "order-key",
	Items:          []*pb.CreateOrderItem{{ProductId: 1, Quantity: 1}},
}

&pb.CancelOrderRequest{
	Id:             1,
	UserId:         1,
	IdempotencyKey: "cancel-key",
}

&pbPayment.PaymentActionRequest{
	Id:             1,
	UserId:         1,
	IdempotencyKey: "payment-key",
}
```

- [ ] **Step 2: 运行编译型测试确认红灯**

```bash
go test ./cmd/api-gateway ./internal/order ./internal/payment
```

Expected: FAIL，原因是 proto 请求结构体尚无 `IdempotencyKey` 字段。

- [ ] **Step 3: 更新 proto**

在 `api/order/order.proto` 中改为：

```proto
message CreateOrderRequest {
  int64 user_id = 1;
  repeated CreateOrderItem items = 2;
  string idempotency_key = 3;
}

message CancelOrderRequest {
  int64 id = 1;
  int64 user_id = 2;
  string idempotency_key = 3;
}
```

在 `api/payment/payment.proto` 中改为：

```proto
message PaymentActionRequest {
  int64 id = 1;
  int64 user_id = 2;
  string idempotency_key = 3;
}
```

- [ ] **Step 4: 重新生成 Go 协议代码**

```bash
protoc --go_out=. --go-grpc_out=. api/order/order.proto
protoc --go_out=. --go-grpc_out=. api/payment/payment.proto
```

Expected: `api/order/*.pb.go` 与 `api/payment/*.pb.go` 被刷新。

- [ ] **Step 5: 运行编译型测试确认协议已生效**

```bash
go test ./cmd/api-gateway ./internal/order ./internal/payment
```

Expected: 不再因为 `IdempotencyKey` 字段缺失而失败；后续仍会因业务逻辑尚未接入而出现功能红灯。

- [ ] **Step 6: 提交**

```bash
git add api/order api/payment
git commit -m "feat: propagate idempotency keys through grpc contracts"
```

## Task 3: 在 API 网关强制要求并透传 `Idempotency-Key`

**Files:**
- Modify: `cmd/api-gateway/main.go`
- Modify: `cmd/api-gateway/main_test.go`

- [ ] **Step 1: 先写失败测试**

在 `cmd/api-gateway/main_test.go` 中补充：

```go
func TestHandleCreateOrderRequiresIdempotencyKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	gateway := &APIGateway{orderClient: &fakeOrderClient{}}
	router := gin.New()
	router.POST("/api/orders", func(c *gin.Context) {
		c.Set("user_id", int64(1))
		gateway.handleCreateOrder(c)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/orders", strings.NewReader(`{"items":[{"product_id":1,"quantity":1}]}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if got, want := resp.Code, http.StatusBadRequest; got != want {
		t.Fatalf("unexpected status code: got %d want %d", got, want)
	}
}

func TestHandleCreateOrderForwardsIdempotencyKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	client := &fakeOrderClient{}
	gateway := &APIGateway{orderClient: client}
	router := gin.New()
	router.POST("/api/orders", func(c *gin.Context) {
		c.Set("user_id", int64(1))
		gateway.handleCreateOrder(c)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/orders", strings.NewReader(`{"items":[{"product_id":1,"quantity":1}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "order-key")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if got, want := resp.Code, http.StatusOK; got != want {
		t.Fatalf("unexpected status code: got %d want %d", got, want)
	}
	if got, want := client.lastCreateOrderReq.IdempotencyKey, "order-key"; got != want {
		t.Fatalf("unexpected idempotency key: got %q want %q", got, want)
	}
}

func TestHandleMarkPaymentSucceededRequiresIdempotencyKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	gateway := &APIGateway{paymentClient: &fakePaymentClient{}}
	router := gin.New()
	router.POST("/api/payments/:id/success", func(c *gin.Context) {
		c.Set("user_id", int64(1))
		gateway.handleMarkPaymentSucceeded(c)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/payments/1/success", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if got, want := resp.Code, http.StatusBadRequest; got != want {
		t.Fatalf("unexpected status code: got %d want %d", got, want)
	}
}

func TestHandleMarkPaymentSucceededForwardsIdempotencyKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	client := &fakePaymentClient{}
	gateway := &APIGateway{paymentClient: client}
	router := gin.New()
	router.POST("/api/payments/:id/success", func(c *gin.Context) {
		c.Set("user_id", int64(1))
		gateway.handleMarkPaymentSucceeded(c)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/payments/1/success", nil)
	req.Header.Set("Idempotency-Key", "payment-key")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if got, want := resp.Code, http.StatusOK; got != want {
		t.Fatalf("unexpected status code: got %d want %d", got, want)
	}
	if got, want := client.lastSucceededReq.IdempotencyKey, "payment-key"; got != want {
		t.Fatalf("unexpected idempotency key: got %q want %q", got, want)
	}
}

func TestHandleCancelOrderRequiresIdempotencyKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	gateway := &APIGateway{orderClient: &fakeOrderClient{}}
	router := gin.New()
	router.PUT("/api/orders/:id/cancel", func(c *gin.Context) {
		c.Set("user_id", int64(1))
		gateway.handleCancelOrder(c)
	})

	req := httptest.NewRequest(http.MethodPut, "/api/orders/1/cancel", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if got, want := resp.Code, http.StatusBadRequest; got != want {
		t.Fatalf("unexpected status code: got %d want %d", got, want)
	}
}

func TestHandleCancelOrderForwardsIdempotencyKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	client := &fakeOrderClient{}
	gateway := &APIGateway{orderClient: client}
	router := gin.New()
	router.PUT("/api/orders/:id/cancel", func(c *gin.Context) {
		c.Set("user_id", int64(1))
		gateway.handleCancelOrder(c)
	})

	req := httptest.NewRequest(http.MethodPut, "/api/orders/1/cancel", nil)
	req.Header.Set("Idempotency-Key", "cancel-key")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if got, want := resp.Code, http.StatusOK; got != want {
		t.Fatalf("unexpected status code: got %d want %d", got, want)
	}
	if got, want := client.lastCancelOrderReq.IdempotencyKey, "cancel-key"; got != want {
		t.Fatalf("unexpected idempotency key: got %q want %q", got, want)
	}
}

func TestHandleCreateOrderMapsIdempotencyConflictToHTTP409(t *testing.T) {
	gin.SetMode(gin.TestMode)

	gateway := &APIGateway{orderClient: &conflictOrderClient{}}
	router := gin.New()
	router.POST("/api/orders", func(c *gin.Context) {
		c.Set("user_id", int64(1))
		gateway.handleCreateOrder(c)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/orders", strings.NewReader(`{"items":[{"product_id":1,"quantity":1}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "order-key")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if got, want := resp.Code, http.StatusConflict; got != want {
		t.Fatalf("unexpected status code: got %d want %d", got, want)
	}
}

func TestHandleCancelOrderMapsIdempotencyConflictToHTTP409(t *testing.T) {
	gin.SetMode(gin.TestMode)

	gateway := &APIGateway{orderClient: &conflictOrderClient{}}
	router := gin.New()
	router.PUT("/api/orders/:id/cancel", func(c *gin.Context) {
		c.Set("user_id", int64(1))
		gateway.handleCancelOrder(c)
	})

	req := httptest.NewRequest(http.MethodPut, "/api/orders/1/cancel", nil)
	req.Header.Set("Idempotency-Key", "cancel-key")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if got, want := resp.Code, http.StatusConflict; got != want {
		t.Fatalf("unexpected status code: got %d want %d", got, want)
	}
}
```

同时把现有 fake client 扩展为：

```go
type fakeOrderClient struct {
	lastCreateOrderReq *pbOrder.CreateOrderRequest
	lastCancelOrderReq *pbOrder.CancelOrderRequest
	lastShipOrderReq   *pbOrder.ShipOrderRequest
}

func (f *fakeOrderClient) CancelOrder(ctx context.Context, in *pbOrder.CancelOrderRequest, opts ...grpc.CallOption) (*pbOrder.CancelOrderResponse, error) {
	f.lastCancelOrderReq = in
	return &pbOrder.CancelOrderResponse{Success: true}, nil
}

type conflictOrderClient struct {
	fakeOrderClient
}

func (f *conflictOrderClient) CreateOrder(context.Context, *pbOrder.CreateOrderRequest, ...grpc.CallOption) (*pbOrder.CreateOrderResponse, error) {
	return nil, status.Error(codes.FailedPrecondition, "idempotency conflict")
}

func (f *conflictOrderClient) CancelOrder(context.Context, *pbOrder.CancelOrderRequest, ...grpc.CallOption) (*pbOrder.CancelOrderResponse, error) {
	return nil, status.Error(codes.FailedPrecondition, "idempotency conflict")
}

type fakePaymentClient struct {
	lastCreatePaymentReq *pbPayment.CreatePaymentRequest
	lastSucceededReq     *pbPayment.PaymentActionRequest
}

func (f *fakePaymentClient) MarkPaymentSucceeded(ctx context.Context, in *pbPayment.PaymentActionRequest, opts ...grpc.CallOption) (*pbPayment.PaymentActionResponse, error) {
	f.lastSucceededReq = in
	return &pbPayment.PaymentActionResponse{Payment: &pbPayment.Payment{Id: in.Id, UserId: in.UserId, Status: "succeeded"}}, nil
}
```

- [ ] **Step 2: 运行测试确认红灯**

```bash
go test ./cmd/api-gateway -run "TestHandle(CreateOrder|MarkPaymentSucceeded|CancelOrder)" -v
```

Expected: FAIL，因为网关还未校验 header、也未透传新字段。

- [ ] **Step 3: 写最小实现**

在 `cmd/api-gateway/main.go` 中新增 helper：

```go
func readRequiredIdempotencyKey(c *gin.Context) (string, bool) {
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Idempotency-Key header required"})
		return "", false
	}
	return key, true
}
```

把 CORS 允许头扩展为：

```go
c.Writer.Header().Set(
	"Access-Control-Allow-Headers",
	"Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, Idempotency-Key, accept, origin, Cache-Control, X-Requested-With",
)
```

在 `handleCreateOrder` 中透传：

```go
idempotencyKey, ok := readRequiredIdempotencyKey(c)
if !ok {
	return
}

resp, err := g.orderClient.CreateOrder(context.Background(), &pbOrder.CreateOrderRequest{
	UserId:         userID.(int64),
	Items:          orderItems,
	IdempotencyKey: idempotencyKey,
})
if err != nil {
	writeGRPCError(c, err)
	return
}
```

在 `handlePaymentAction` 中只对成功动作强制要求：

```go
req := &pbPayment.PaymentActionRequest{Id: paymentID, UserId: userID.(int64)}
if succeed {
	idempotencyKey, ok := readRequiredIdempotencyKey(c)
	if !ok {
		return
	}
	req.IdempotencyKey = idempotencyKey
}
```

在 `handleCancelOrder` 中透传：

```go
idempotencyKey, ok := readRequiredIdempotencyKey(c)
if !ok {
	return
}

resp, err := g.orderClient.CancelOrder(context.Background(), &pbOrder.CancelOrderRequest{
	Id:             orderId,
	UserId:         userID.(int64),
	IdempotencyKey: idempotencyKey,
})
if err != nil {
	writeGRPCError(c, err)
	return
}
```

- [ ] **Step 4: 调整现有 happy-path 测试**

将已有 `TestHandleCreateOrderIgnoresForgedClientFields` 的请求补上：

```go
req.Header.Set("Idempotency-Key", "order-key")
```

- [ ] **Step 5: 运行测试确认转绿**

```bash
go test ./cmd/api-gateway -v
```

Expected: PASS

- [ ] **Step 6: 提交**

```bash
git add cmd/api-gateway/main.go cmd/api-gateway/main_test.go
git commit -m "feat: require idempotency keys at api gateway"
```

## Task 4: 为创建订单接入幂等控制

**Files:**
- Modify: `internal/order/service.go`
- Modify: `internal/order/service_test.go`
- Modify: `internal/order/timeout_consumer_test.go`

- [ ] **Step 1: 先补测试夹具与失败测试**

先在 `internal/order/service_test.go` 中把测试数据库迁移扩展为：

```go
if err := db.AutoMigrate(&auth.User{}, &merchant.Merchant{}, &product.Product{}, &Order{}, &OrderItem{}, &idempotency.Record{}); err != nil {
	t.Fatalf("failed to migrate test database: %v", err)
}
```

并新增 helper：

```go
func createOrderRequest(userID int64, key string, items ...*pb.CreateOrderItem) *pb.CreateOrderRequest {
	return &pb.CreateOrderRequest{
		UserId:         userID,
		IdempotencyKey: key,
		Items:          items,
	}
}
```

然后新增：

```go
func TestCreateOrderIsIdempotentForRepeatedRequests(t *testing.T) {
	service, db := newTestService(t)
	item := createTestProduct(t, db, "幂等商品", 10, 5)
	req := createOrderRequest(1, "order-key", &pb.CreateOrderItem{ProductId: int64(item.ID), Quantity: 2})

	first, err := service.CreateOrder(context.Background(), req)
	if err != nil {
		t.Fatalf("first CreateOrder returned error: %v", err)
	}
	second, err := service.CreateOrder(context.Background(), req)
	if err != nil {
		t.Fatalf("second CreateOrder returned error: %v", err)
	}
	if got, want := second.Order.Id, first.Order.Id; got != want {
		t.Fatalf("unexpected replayed order id: got %d want %d", got, want)
	}

	var orderCount int64
	if err := db.Model(&Order{}).Count(&orderCount).Error; err != nil {
		t.Fatalf("failed to count orders: %v", err)
	}
	if got, want := orderCount, int64(1); got != want {
		t.Fatalf("unexpected order count: got %d want %d", got, want)
	}

	var latest product.Product
	if err := db.First(&latest, item.ID).Error; err != nil {
		t.Fatalf("failed to load product: %v", err)
	}
	if got, want := latest.Stock, int32(3); got != want {
		t.Fatalf("unexpected stock: got %d want %d", got, want)
	}
}

func TestCreateOrderRejectsSameKeyWithDifferentPayload(t *testing.T) {
	service, db := newTestService(t)
	item := createTestProduct(t, db, "幂等冲突商品", 10, 5)

	if _, err := service.CreateOrder(context.Background(), createOrderRequest(
		1,
		"order-key",
		&pb.CreateOrderItem{ProductId: int64(item.ID), Quantity: 1},
	)); err != nil {
		t.Fatalf("first CreateOrder returned error: %v", err)
	}

	_, err := service.CreateOrder(context.Background(), createOrderRequest(
		1,
		"order-key",
		&pb.CreateOrderItem{ProductId: int64(item.ID), Quantity: 2},
	))
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("unexpected error code: got %v want %v", status.Code(err), codes.FailedPrecondition)
	}
}

func TestCreateOrderConcurrentSameKeyCreatesOneOrder(t *testing.T) {
	service, db := newConcurrentTestService(t)
	item := createTestProduct(t, db, "并发幂等商品", 10, 20)

	const requestCount = 20
	var successCount int32
	var wg sync.WaitGroup
	start := make(chan struct{})

	for i := 0; i < requestCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, err := service.CreateOrder(context.Background(), createOrderRequest(
				1,
				"same-key",
				&pb.CreateOrderItem{ProductId: int64(item.ID), Quantity: 1},
			)); err == nil {
				atomic.AddInt32(&successCount, 1)
			}
		}()
	}

	close(start)
	wg.Wait()

	var orderCount int64
	if err := db.Model(&Order{}).Count(&orderCount).Error; err != nil {
		t.Fatalf("failed to count orders: %v", err)
	}
	if got, want := orderCount, int64(1); got != want {
		t.Fatalf("unexpected order count: got %d want %d", got, want)
	}
	if successCount < 1 {
		t.Fatal("expected at least one successful request")
	}
}
```

把 `internal/order/timeout_consumer_test.go` 里直接创建订单的地方改成使用：

```go
createOrderRequest(1, "timeout-order-key", &pb.CreateOrderItem{ProductId: int64(item.ID), Quantity: 2})
```

- [ ] **Step 2: 批量调整既有创建订单测试**

把 `internal/order/service_test.go` 中所有直接构造的：

```go
&pb.CreateOrderRequest{
	UserId: 1,
	Items:  []*pb.CreateOrderItem{...},
}
```

替换为：

```go
createOrderRequest(1, "<该测试唯一 key>", ...items)
```

每个测试使用不同 key，例如：

```go
"snapshot-key"
"pending-key"
"publish-key"
"rollback-order-key"
"rollback-item-key"
```

- [ ] **Step 3: 运行测试确认红灯**

```bash
go test ./internal/order -run "TestCreateOrder(IsIdempotentForRepeatedRequests|RejectsSameKeyWithDifferentPayload|ConcurrentSameKeyCreatesOneOrder)" -v
```

Expected: FAIL，因为 `CreateOrder` 还没有幂等处理，重复请求会建多单或重复扣库存。

- [ ] **Step 4: 写最小实现**

在 `internal/order/service.go` 中：

1. 引入依赖：

```go
import (
	"net/http"
	"sort"

	"go-commerce/internal/idempotency"
)
```

2. 为服务增加字段：

```go
idempotency *idempotency.Service
```

3. 在 `NewServiceWithTimeout` 中初始化：

```go
idempotency: idempotency.NewService(db, 24*time.Hour),
```

4. 增加请求路径常量与指纹类型：

```go
const createOrderRequestPath = "POST /api/orders"

type createOrderFingerprint struct {
	UserID int64                        `json:"user_id"`
	Items  []createOrderFingerprintItem `json:"items"`
}

type createOrderFingerprintItem struct {
	ProductID int64 `json:"product_id"`
	Quantity  int32 `json:"quantity"`
}

func newCreateOrderFingerprint(userID int64, items []aggregatedCreateOrderItem) createOrderFingerprint {
	normalized := make([]createOrderFingerprintItem, len(items))
	for i, item := range items {
		normalized[i] = createOrderFingerprintItem{
			ProductID: item.ProductID,
			Quantity:  item.Quantity,
		}
	}
	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i].ProductID < normalized[j].ProductID
	})
	return createOrderFingerprint{UserID: userID, Items: normalized}
}
```

5. 在 `CreateOrder` 中最前面插入：

```go
if req.IdempotencyKey == "" {
	return nil, status.Error(codes.InvalidArgument, "idempotency key is required")
}

aggregatedItems, err := aggregateCreateOrderItems(req.Items)
if err != nil {
	return nil, err
}

requestHash, err := idempotency.HashPayload(newCreateOrderFingerprint(req.UserId, aggregatedItems))
if err != nil {
	return nil, status.Errorf(codes.Internal, "failed to hash create order request: %v", err)
}

decision, err := s.idempotency.Begin(ctx, idempotency.BeginRequest{
	UserID:         uint(req.UserId),
	RequestPath:    createOrderRequestPath,
	IdempotencyKey: req.IdempotencyKey,
	RequestHash:    requestHash,
})
if err != nil {
	switch {
	case errors.Is(err, idempotency.ErrConflict), errors.Is(err, idempotency.ErrInProgress):
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	default:
		return nil, status.Errorf(codes.Internal, "failed to begin idempotent create order request: %v", err)
	}
}
if decision.Action == idempotency.ActionReplay {
	replayed := &pb.CreateOrderResponse{}
	if err := idempotency.ReplayInto(decision.Record, replayed); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to replay create order response: %v", err)
	}
	return replayed, nil
}
```

6. 把原本直接返回的响应提取为变量，并在返回前完成快照写回：

```go
response := &pb.CreateOrderResponse{
	Order: convertToPBOrder(&order, orderItems),
}
if err := s.idempotency.Complete(ctx, decision.Record.ID, http.StatusOK, response); err != nil {
	return nil, status.Errorf(codes.Internal, "failed to complete create order idempotency record: %v", err)
}
return response, nil
```

- [ ] **Step 5: 运行相关测试确认转绿**

```bash
go test ./internal/order -run "TestCreateOrder(IsIdempotentForRepeatedRequests|RejectsSameKeyWithDifferentPayload|ConcurrentSameKeyCreatesOneOrder)" -v
```

Expected: PASS

- [ ] **Step 6: 再跑现有创建订单测试防回归**

```bash
go test ./internal/order -run TestCreateOrder -v
```

Expected: PASS

- [ ] **Step 7: 提交**

```bash
git add internal/order/service.go internal/order/service_test.go internal/order/timeout_consumer_test.go
git commit -m "feat: make order creation idempotent"
```

## Task 5: 为支付成功接入幂等控制

**Files:**
- Modify: `internal/payment/grpc_service.go`
- Create: `internal/payment/grpc_service_test.go`

- [ ] **Step 1: 先写失败测试**

新增 `internal/payment/grpc_service_test.go`：

```go
package payment

import (
	"context"
	"testing"

	pbOrder "go-commerce/api/order"
	pbPayment "go-commerce/api/payment"
	orderdomain "go-commerce/internal/order"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestMarkPaymentSucceededIsIdempotentForSameKey(t *testing.T) {
	publisher := &recordingPublisher{}
	core, _ := newTestService(t, &fakeOrderClient{orders: map[int64]*pbOrder.Order{
		1: {Id: 1, UserId: 1, Status: orderdomain.OrderStatusPending, TotalAmount: 99},
	}}, publisher)
	grpcService := NewGRPCService(core)

	payment, err := core.CreatePayment(context.Background(), 1, 1, PaymentMethodMockBalance)
	if err != nil {
		t.Fatalf("CreatePayment returned error: %v", err)
	}

	req := &pbPayment.PaymentActionRequest{
		Id:             int64(payment.ID),
		UserId:         1,
		IdempotencyKey: "payment-key",
	}
	first, err := grpcService.MarkPaymentSucceeded(context.Background(), req)
	if err != nil {
		t.Fatalf("first MarkPaymentSucceeded returned error: %v", err)
	}
	second, err := grpcService.MarkPaymentSucceeded(context.Background(), req)
	if err != nil {
		t.Fatalf("second MarkPaymentSucceeded returned error: %v", err)
	}
	if got, want := second.Payment.Id, first.Payment.Id; got != want {
		t.Fatalf("unexpected replayed payment id: got %d want %d", got, want)
	}
	if got, want := len(publisher.events), 1; got != want {
		t.Fatalf("unexpected published event count: got %d want %d", got, want)
	}
}

func TestMarkPaymentSucceededRejectsSameKeyForDifferentPayment(t *testing.T) {
	core, _ := newTestService(t, &fakeOrderClient{orders: map[int64]*pbOrder.Order{
		1: {Id: 1, UserId: 1, Status: orderdomain.OrderStatusPending, TotalAmount: 99},
		2: {Id: 2, UserId: 1, Status: orderdomain.OrderStatusPending, TotalAmount: 99},
	}}, nil)
	grpcService := NewGRPCService(core)

	first, err := core.CreatePayment(context.Background(), 1, 1, PaymentMethodMockBalance)
	if err != nil {
		t.Fatalf("CreatePayment returned error: %v", err)
	}
	second, err := core.CreatePayment(context.Background(), 1, 2, PaymentMethodMockBalance)
	if err != nil {
		t.Fatalf("CreatePayment returned error: %v", err)
	}

	if _, err := grpcService.MarkPaymentSucceeded(context.Background(), &pbPayment.PaymentActionRequest{
		Id:             int64(first.ID),
		UserId:         1,
		IdempotencyKey: "payment-key",
	}); err != nil {
		t.Fatalf("first MarkPaymentSucceeded returned error: %v", err)
	}
	_, err = grpcService.MarkPaymentSucceeded(context.Background(), &pbPayment.PaymentActionRequest{
		Id:             int64(second.ID),
		UserId:         1,
		IdempotencyKey: "payment-key",
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("unexpected error code: got %v want %v", status.Code(err), codes.FailedPrecondition)
	}
}
```

- [ ] **Step 2: 扩展测试数据库迁移**

在 `internal/payment/service_test.go` 的 `newTestService` 中改为：

```go
if err := db.AutoMigrate(&Payment{}, &idempotency.Record{}); err != nil {
	t.Fatalf("failed to migrate test database: %v", err)
}
```

- [ ] **Step 3: 运行测试确认红灯**

```bash
go test ./internal/payment -run "TestMarkPaymentSucceeded(IsIdempotentForSameKey|RejectsSameKeyForDifferentPayment)" -v
```

Expected: FAIL，因为 `GRPCService` 尚未接入幂等逻辑。

- [ ] **Step 4: 写最小实现**

在 `internal/payment/grpc_service.go` 中：

1. 引入依赖：

```go
import (
	"net/http"

	"go-commerce/internal/idempotency"
)
```

2. 扩展结构体：

```go
type GRPCService struct {
	pb.UnimplementedPaymentServiceServer
	core        *Service
	idempotency *idempotency.Service
}
```

3. 修改构造函数：

```go
func NewGRPCService(core *Service) *GRPCService {
	return &GRPCService{
		core:        core,
		idempotency: idempotency.NewService(core.db, 24*time.Hour),
	}
}
```

4. 新增请求路径常量：

```go
const succeedPaymentRequestPath = "POST /api/payments/:id/success"
```

5. 重写 `MarkPaymentSucceeded`：

```go
func (s *GRPCService) MarkPaymentSucceeded(ctx context.Context, req *pb.PaymentActionRequest) (*pb.PaymentActionResponse, error) {
	if req.IdempotencyKey == "" {
		return nil, status.Error(codes.InvalidArgument, "idempotency key is required")
	}

	requestHash, err := idempotency.HashPayload(struct {
		UserID    int64 `json:"user_id"`
		PaymentID int64 `json:"payment_id"`
	}{
		UserID:    req.UserId,
		PaymentID: req.Id,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to hash payment success request: %v", err)
	}

	decision, err := s.idempotency.Begin(ctx, idempotency.BeginRequest{
		UserID:         uint(req.UserId),
		RequestPath:    succeedPaymentRequestPath,
		IdempotencyKey: req.IdempotencyKey,
		RequestHash:    requestHash,
	})
	if err != nil {
		switch {
		case errors.Is(err, idempotency.ErrConflict), errors.Is(err, idempotency.ErrInProgress):
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		default:
			return nil, status.Errorf(codes.Internal, "failed to begin payment success idempotency: %v", err)
		}
	}
	if decision.Action == idempotency.ActionReplay {
		replayed := &pb.PaymentActionResponse{}
		if err := idempotency.ReplayInto(decision.Record, replayed); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to replay payment success response: %v", err)
		}
		return replayed, nil
	}

	payment, err := s.core.SucceedPayment(ctx, uint(req.UserId), uint(req.Id))
	if err != nil {
		return nil, paymentStatusError(err)
	}
	response := &pb.PaymentActionResponse{Payment: convertToPBPayment(payment)}
	if err := s.idempotency.Complete(ctx, decision.Record.ID, http.StatusOK, response); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to complete payment success idempotency record: %v", err)
	}
	return response, nil
}
```

- [ ] **Step 5: 运行测试确认转绿**

```bash
go test ./internal/payment -run "TestMarkPaymentSucceeded(IsIdempotentForSameKey|RejectsSameKeyForDifferentPayment)" -v
```

Expected: PASS

- [ ] **Step 6: 跑支付服务全量测试**

```bash
go test ./internal/payment -v
```

Expected: PASS

- [ ] **Step 7: 提交**

```bash
git add internal/payment/grpc_service.go internal/payment/grpc_service_test.go internal/payment/service_test.go
git commit -m "feat: make payment success idempotent"
```

## Task 6: 为取消订单接入幂等控制并统一重复取消语义

**Files:**
- Modify: `internal/order/service.go`
- Modify: `internal/order/service_test.go`
- Modify: `internal/order/timeout_consumer_test.go`

- [ ] **Step 1: 先写失败测试**

在 `internal/order/service_test.go` 中新增 helper：

```go
func cancelOrderRequest(orderID, userID int64, key string) *pb.CancelOrderRequest {
	return &pb.CancelOrderRequest{
		Id:             orderID,
		UserId:         userID,
		IdempotencyKey: key,
	}
}
```

并新增：

```go
func TestCancelOrderIsIdempotentForRepeatedRequests(t *testing.T) {
	service, db := newTestService(t)
	item := createTestProduct(t, db, "重复取消商品", 10, 5)
	orderResp, err := service.CreateOrder(context.Background(), createOrderRequest(
		1,
		"cancel-source-order",
		&pb.CreateOrderItem{ProductId: int64(item.ID), Quantity: 2},
	))
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}

	req := cancelOrderRequest(orderResp.Order.Id, 1, "cancel-key")
	first, err := service.CancelOrder(context.Background(), req)
	if err != nil {
		t.Fatalf("first CancelOrder returned error: %v", err)
	}
	second, err := service.CancelOrder(context.Background(), req)
	if err != nil {
		t.Fatalf("second CancelOrder returned error: %v", err)
	}
	if !first.Success || !second.Success {
		t.Fatalf("expected both responses to be successful: first=%v second=%v", first.Success, second.Success)
	}

	var latest product.Product
	if err := db.First(&latest, item.ID).Error; err != nil {
		t.Fatalf("failed to reload product: %v", err)
	}
	if got, want := latest.Stock, int32(5); got != want {
		t.Fatalf("unexpected stock after repeated cancel: got %d want %d", got, want)
	}
}

func TestCancelOrderRejectsSameKeyForDifferentOrder(t *testing.T) {
	service, db := newTestService(t)
	firstItem := createTestProduct(t, db, "取消冲突一", 10, 5)
	secondItem := createTestProduct(t, db, "取消冲突二", 10, 5)

	firstOrder, err := service.CreateOrder(context.Background(), createOrderRequest(
		1,
		"cancel-order-a",
		&pb.CreateOrderItem{ProductId: int64(firstItem.ID), Quantity: 1},
	))
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}
	secondOrder, err := service.CreateOrder(context.Background(), createOrderRequest(
		1,
		"cancel-order-b",
		&pb.CreateOrderItem{ProductId: int64(secondItem.ID), Quantity: 1},
	))
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}

	if _, err := service.CancelOrder(context.Background(), cancelOrderRequest(firstOrder.Order.Id, 1, "cancel-key")); err != nil {
		t.Fatalf("first CancelOrder returned error: %v", err)
	}
	_, err = service.CancelOrder(context.Background(), cancelOrderRequest(secondOrder.Order.Id, 1, "cancel-key"))
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("unexpected error code: got %v want %v", status.Code(err), codes.FailedPrecondition)
	}
}
```

将已有取消测试中的请求统一改为：

```go
cancelOrderRequest(resp.Order.Id, 1, "<该测试唯一 key>")
```

- [ ] **Step 2: 运行测试确认红灯**

```bash
go test ./internal/order -run "TestCancelOrder(IsIdempotentForRepeatedRequests|RejectsSameKeyForDifferentOrder)" -v
```

Expected: FAIL，因为当前 `CancelOrder` 还没有请求级幂等，且重复取消仍返回 `success=false`。

- [ ] **Step 3: 写最小实现**

在 `internal/order/service.go` 中新增：

```go
const cancelOrderRequestPath = "PUT /api/orders/:id/cancel"
```

然后在 `CancelOrder` 开头写入：

```go
if req.IdempotencyKey == "" {
	return nil, status.Error(codes.InvalidArgument, "idempotency key is required")
}

requestHash, err := idempotency.HashPayload(struct {
	UserID  int64 `json:"user_id"`
	OrderID int64 `json:"order_id"`
}{
	UserID:  req.UserId,
	OrderID: req.Id,
})
if err != nil {
	return nil, status.Errorf(codes.Internal, "failed to hash cancel order request: %v", err)
}

decision, err := s.idempotency.Begin(ctx, idempotency.BeginRequest{
	UserID:         uint(req.UserId),
	RequestPath:    cancelOrderRequestPath,
	IdempotencyKey: req.IdempotencyKey,
	RequestHash:    requestHash,
})
if err != nil {
	switch {
	case errors.Is(err, idempotency.ErrConflict), errors.Is(err, idempotency.ErrInProgress):
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	default:
		return nil, status.Errorf(codes.Internal, "failed to begin cancel order idempotency: %v", err)
	}
}
if decision.Action == idempotency.ActionReplay {
	replayed := &pb.CancelOrderResponse{}
	if err := idempotency.ReplayInto(decision.Record, replayed); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to replay cancel order response: %v", err)
	}
	return replayed, nil
}
```

把重复取消语义改为：

```go
if !changed {
	response := &pb.CancelOrderResponse{
		Success: true,
		Message: "订单已取消",
	}
	if err := s.idempotency.Complete(ctx, decision.Record.ID, http.StatusOK, response); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to complete cancel order idempotency record: %v", err)
	}
	return response, nil
}
```

把首次取消成功响应提取并写回快照：

```go
response := &pb.CancelOrderResponse{
	Success: true,
	Message: "订单取消成功",
}
if err := s.idempotency.Complete(ctx, decision.Record.ID, http.StatusOK, response); err != nil {
	return nil, status.Errorf(codes.Internal, "failed to complete cancel order idempotency record: %v", err)
}
return response, nil
```

- [ ] **Step 4: 运行测试确认转绿**

```bash
go test ./internal/order -run "TestCancelOrder(IsIdempotentForRepeatedRequests|RejectsSameKeyForDifferentOrder)" -v
```

Expected: PASS

- [ ] **Step 5: 跑取消与超时相关回归测试**

```bash
go test ./internal/order -run "TestCancelOrder|TestOrderTimeoutConsumer" -v
```

Expected: PASS

- [ ] **Step 6: 提交**

```bash
git add internal/order/service.go internal/order/service_test.go internal/order/timeout_consumer_test.go
git commit -m "feat: make order cancellation idempotent"
```

## Task 7: 补齐重复消息消费回归测试

**Files:**
- Modify: `internal/order/payment_consumer_test.go`

- [ ] **Step 1: 先写失败测试**

在 `internal/order/payment_consumer_test.go` 中新增：

```go
func TestPaymentSucceededConsumerIsIdempotentForRepeatedEvents(t *testing.T) {
	_, db := newTestService(t)
	order := Order{
		UserID:      1,
		TotalAmount: 99,
		Status:      OrderStatusPending,
		OrderDate:   time.Now(),
	}
	if err := db.Create(&order).Error; err != nil {
		t.Fatalf("failed to create order: %v", err)
	}

	body, err := json.Marshal(events.PaymentSucceededEvent{
		BaseEvent: events.NewBaseEvent(events.PaymentSucceededType, time.Now()),
		OrderID:   int64(order.ID),
		UserID:    1,
		Amount:    99,
	})
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}

	publisher := &recordingPublisher{}
	consumer := NewPaymentSucceededConsumer(db, publisher, nil)
	for i := 0; i < 2; i++ {
		if err := consumer.HandleDelivery(amqp.Delivery{
			Acknowledger: &fakeAcknowledger{},
			DeliveryTag:  uint64(i + 1),
			Body:         body,
		}); err != nil {
			t.Fatalf("HandleDelivery returned error: %v", err)
		}
	}

	var latest Order
	if err := db.First(&latest, order.ID).Error; err != nil {
		t.Fatalf("failed to reload order: %v", err)
	}
	if got, want := latest.Status, OrderStatusPaid; got != want {
		t.Fatalf("unexpected order status: got %q want %q", got, want)
	}
	if got, want := len(publisher.events), 1; got != want {
		t.Fatalf("unexpected published event count: got %d want %d", got, want)
	}
}
```

- [ ] **Step 2: 运行测试确认红灯或保护现状**

```bash
go test ./internal/order -run TestPaymentSucceededConsumerIsIdempotentForRepeatedEvents -v
```

Expected: 如果当前实现已正确防重，则测试直接 PASS；若出现重复发布 `order.paid`，则 FAIL。

- [ ] **Step 3: 如测试失败，修正消费者逻辑**

若当前实现重复发布事件，则把逻辑收敛为：

```go
order, changed, err := MarkOrderPaid(c.db, event.OrderID, event.UserID, event.Amount)
if err != nil {
	_ = delivery.Nack(false, false)
	return err
}
if changed {
	statusEvent := newOrderStatusChangedEvent(events.OrderPaidType, order, OrderStatusPending, OrderStatusPaid)
	if err := c.publisher.Publish(context.Background(), events.OrderPaidType, statusEvent); err != nil {
		c.logger.Printf(
			"event_publish_failed event_type=%s event_id=%s order_id=%d user_id=%d error=%v",
			statusEvent.EventType,
			statusEvent.EventID,
			order.ID,
			order.UserID,
			err,
		)
	}
}
```

- [ ] **Step 4: 再跑支付与超时消费者测试**

```bash
go test ./internal/order -run "TestPaymentSucceededConsumer|TestOrderTimeoutConsumer" -v
```

Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/order/payment_consumer_test.go internal/order/payment_consumer.go
git commit -m "test: cover duplicate transaction events"
```

## Task 8: 接入正式数据库迁移并补齐文档

**Files:**
- Modify: `cmd/order-service/main.go`
- Modify: `cmd/payment-service/main.go`
- Modify: `README.md`
- Modify: `doc/API_Documentation.md`
- Modify: `doc/TECHNICAL_DOCUMENT.md`
- Modify: `doc/INTERVIEW.md`

- [ ] **Step 1: 更新自动迁移**

在 `cmd/order-service/main.go` 中：

```go
if err := db.AutoMigrate(&order.Order{}, &order.OrderItem{}, &idempotency.Record{}); err != nil {
	log.Fatalf("failed to migrate database: %v", err)
}
```

在 `cmd/payment-service/main.go` 中：

```go
if err := db.AutoMigrate(&payment.Payment{}, &idempotency.Record{}); err != nil {
	log.Fatalf("failed to migrate database: %v", err)
}
```

- [ ] **Step 2: 更新 README 示例**

把下单、模拟支付成功、取消订单示例补成：

```bash
curl -X POST http://localhost:8081/api/orders \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Idempotency-Key: order-20260515-001" \
  -d '{"items": [{"product_id": 1, "quantity": 1}]}'

curl -X POST http://localhost:8081/api/payments/1/success \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Idempotency-Key: payment-success-20260515-001"

curl -X PUT http://localhost:8081/api/orders/1/cancel \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Idempotency-Key: cancel-20260515-001"
```

- [ ] **Step 3: 更新 API 文档**

在 `doc/API_Documentation.md` 中明确补充：

```md
#### 幂等要求

以下接口必须携带 `Idempotency-Key` 请求头：

- `POST /api/orders`
- `POST /api/payments/:id/success`
- `PUT /api/orders/:id/cancel`

同键同参请求会返回首次成功响应；同键异参或请求仍处理中时返回 `409 Conflict`。
```

- [ ] **Step 4: 更新技术文档**

在 `doc/TECHNICAL_DOCUMENT.md` 中新增小节：

```md
**请求级幂等设计**：
- API 网关统一校验 `Idempotency-Key`
- 订单服务与支付服务使用 `idempotency_records`
- `UNIQUE(user_id, request_path, idempotency_key)` 阻止同一请求并发重复执行
- 请求级幂等负责吸收客户端重试，订单状态机、支付状态与库存事务继续负责领域级防重
```

- [ ] **Step 5: 更新面试文档**

在 `doc/INTERVIEW.md` 中增加问答：

```md
### Q：如何保证订单接口幂等？
A：本项目采用“请求级幂等 + 领域级状态约束”的双层方案。创建订单、支付成功、取消订单都要求 `Idempotency-Key`，服务端用 `user_id + request_path + idempotency_key` 建唯一索引，并保存请求指纹与首次响应快照；同键同参重试直接回放首次结果，同键异参或处理中请求返回 `409`。同时，订单状态机、支付单状态与库存恢复事务继续防止重复支付、重复取消和重复回补库存。
```

- [ ] **Step 6: 提交**

```bash
git add cmd/order-service/main.go cmd/payment-service/main.go README.md doc/API_Documentation.md doc/TECHNICAL_DOCUMENT.md doc/INTERVIEW.md
git commit -m "docs: document transaction idempotency"
```

## Task 9: 全量验证与收口

**Files:**
- Verify only

- [ ] **Step 1: 统一格式化**

```bash
gofmt -w internal/idempotency/*.go internal/order/*.go internal/payment/*.go cmd/api-gateway/main.go cmd/order-service/main.go cmd/payment-service/main.go
```

- [ ] **Step 2: 运行关键测试集**

```bash
go test ./internal/idempotency ./cmd/api-gateway ./internal/order ./internal/payment -v
```

Expected: PASS

- [ ] **Step 3: 运行全量测试**

```bash
go test ./...
```

Expected: PASS

- [ ] **Step 4: 检查仓库状态**

```bash
git status --short
git diff --stat HEAD~8..HEAD
```

Expected: 只有本功能相关文件发生变更；无未提交改动。

- [ ] **Step 5: 若采用单次总提交而非分步提交，至少保留以下最终提交信息**

```bash
git commit -m "feat: add transaction idempotency safeguards"
```

## 自查清单

### Spec coverage

- [x] 创建订单使用 `Idempotency-Key`
- [x] 支付成功使用 `Idempotency-Key`
- [x] 取消订单使用 `Idempotency-Key`
- [x] 幂等记录表、请求指纹、响应快照、处理中冲突
- [x] 同键同参回放、同键异参 409
- [x] `payment.succeeded` 重复消费幂等
- [x] 重复取消与超时取消不重复恢复库存
- [x] 7 个验收测试场景均有覆盖
- [x] README、API、技术文档、INTERVIEW 文档同步更新

### Placeholder scan

- [x] 无 `TBD`
- [x] 无 `TODO`
- [x] 无“稍后补充”
- [x] 无“参考前文”

### Type consistency

- [x] `IdempotencyKey` 字段名在 proto、网关、订单、支付测试中一致
- [x] 幂等错误统一使用 `ErrConflict` / `ErrInProgress`
- [x] HTTP 冲突统一通过 gRPC `FailedPrecondition` 映射为 `409 Conflict`
