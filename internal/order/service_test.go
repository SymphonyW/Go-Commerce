package order

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pb "go-commerce/api/order"
	"go-commerce/internal/auth"
	"go-commerce/internal/idempotency"
	"go-commerce/internal/inbox"
	"go-commerce/internal/merchant"
	"go-commerce/internal/outbox"
	"go-commerce/internal/product"
	"go-commerce/pkg/events"
	"go-commerce/pkg/mq"

	"github.com/glebarez/sqlite"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

// newTestService 为订单服务创建独立的内存数据库，避免测试之间互相污染。
func newTestService(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()

	dsn := fmt.Sprintf(
		"file:%s?mode=memory&cache=shared",
		strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()),
	)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	if err := db.AutoMigrate(&auth.User{}, &merchant.Merchant{}, &product.Product{}, &Order{}, &OrderItem{}, &idempotency.Record{}, &outbox.Event{}, &inbox.ConsumedEvent{}); err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}

	return NewService(db, nil), db
}

type publishedEvent struct {
	routingKey string
	event      interface{}
}

type recordingPublisher struct {
	events []publishedEvent
	err    error
}

func (p *recordingPublisher) Publish(ctx context.Context, routingKey string, event interface{}) error {
	p.events = append(p.events, publishedEvent{
		routingKey: routingKey,
		event:      event,
	})
	return p.err
}

type scheduledTimeout struct {
	event events.OrderTimeoutCheckEvent
	delay time.Duration
}

type recordingTimeoutScheduler struct {
	events []scheduledTimeout
	err    error
}

func (s *recordingTimeoutScheduler) Schedule(ctx context.Context, event events.OrderTimeoutCheckEvent, delay time.Duration) error {
	s.events = append(s.events, scheduledTimeout{
		event: event,
		delay: delay,
	})
	return s.err
}

func newTestServiceWithPublisher(t *testing.T, publisher mq.Publisher) (*Service, *gorm.DB) {
	t.Helper()

	dsn := fmt.Sprintf(
		"file:%s?mode=memory&cache=shared",
		strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()),
	)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	if err := db.AutoMigrate(&auth.User{}, &merchant.Merchant{}, &product.Product{}, &Order{}, &OrderItem{}, &idempotency.Record{}, &outbox.Event{}, &inbox.ConsumedEvent{}); err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}

	return NewService(db, publisher), db
}

func newTestServiceWithTimeout(t *testing.T, publisher mq.Publisher, scheduler TimeoutScheduler, timeout time.Duration) (*Service, *gorm.DB) {
	t.Helper()

	dsn := fmt.Sprintf(
		"file:%s?mode=memory&cache=shared",
		strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()),
	)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	if err := db.AutoMigrate(&auth.User{}, &merchant.Merchant{}, &product.Product{}, &Order{}, &OrderItem{}, &idempotency.Record{}, &outbox.Event{}, &inbox.ConsumedEvent{}); err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}

	return NewServiceWithTimeout(db, publisher, scheduler, timeout), db
}

func newConcurrentTestService(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()

	dsn := filepath.Join(t.TempDir(), "orders.db") + "?_busy_timeout=5000&_journal_mode=WAL"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open concurrent test database: %v", err)
	}
	if err := db.AutoMigrate(&auth.User{}, &merchant.Merchant{}, &product.Product{}, &Order{}, &OrderItem{}, &idempotency.Record{}, &outbox.Event{}, &inbox.ConsumedEvent{}); err != nil {
		t.Fatalf("failed to migrate concurrent test database: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to open sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		// 关闭文件型 sqlite 连接，避免 Windows 上临时目录清理时文件仍被占用。
		_ = sqlDB.Close()
	})

	return NewService(db, nil), db
}

// createTestProduct 写入真实商品数据，供订单服务生成快照时读取。
func createTestProduct(t *testing.T, db *gorm.DB, name string, price float64, stock int32) product.Product {
	t.Helper()

	item := product.Product{
		Name:       name,
		Price:      price,
		Stock:      stock,
		MerchantID: 1,
	}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("failed to create product: %v", err)
	}

	return item
}

func createTestUser(t *testing.T, db *gorm.DB, role string) auth.User {
	t.Helper()

	var count int64
	if err := db.Model(&auth.User{}).Count(&count).Error; err != nil {
		t.Fatalf("failed to count users: %v", err)
	}

	user := auth.User{
		Username: fmt.Sprintf("%s-user-%s-%d", role, strings.NewReplacer("/", "-", " ", "-").Replace(t.Name()), count+1),
		Password: "password",
		Email:    fmt.Sprintf("%s-%s-%d@example.com", role, strings.NewReplacer("/", "-", " ", "-").Replace(t.Name()), count+1),
		Role:     role,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	return user
}

func createTestMerchant(t *testing.T, db *gorm.DB, ownerUserID uint) merchant.Merchant {
	t.Helper()

	shop := merchant.Merchant{
		Name:        "测试商家",
		ContactInfo: "merchant@example.com",
		OwnerUserID: &ownerUserID,
	}
	if err := db.Create(&shop).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}
	return shop
}

func countOrderOutboxEvents(t *testing.T, db *gorm.DB, orderID uint, eventType string) int64 {
	t.Helper()

	var count int64
	if err := db.Model(&outbox.Event{}).
		Where("aggregate_type = ? AND aggregate_id = ? AND event_type = ?", "order", fmt.Sprintf("%d", orderID), eventType).
		Count(&count).Error; err != nil {
		t.Fatalf("failed to count order outbox events: %v", err)
	}
	return count
}

func createOrderRequest(userID int64, key string, items ...*pb.CreateOrderItem) *pb.CreateOrderRequest {
	return &pb.CreateOrderRequest{
		UserId:         userID,
		IdempotencyKey: key,
		Items:          items,
	}
}

func cancelOrderRequest(orderID, userID int64, key string) *pb.CancelOrderRequest {
	return &pb.CancelOrderRequest{
		Id:             orderID,
		UserId:         userID,
		IdempotencyKey: key,
	}
}

func createCancellableOrder(t *testing.T, service *Service, userID int64, item product.Product, quantity int32, key string) *pb.Order {
	t.Helper()

	resp, err := service.CreateOrder(context.Background(), createOrderRequest(
		userID,
		key,
		&pb.CreateOrderItem{ProductId: int64(item.ID), Quantity: quantity},
	))
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}
	return resp.Order
}

func TestCreateOrderIsIdempotentForRepeatedRequests(t *testing.T) {
	service, db := newTestService(t)
	item := createTestProduct(t, db, "幂等商品", 25, 5)
	req := createOrderRequest(1, "repeat-create-order", &pb.CreateOrderItem{ProductId: int64(item.ID), Quantity: 2})

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
		t.Fatalf("failed to reload product: %v", err)
	}
	if got, want := latest.Stock, int32(3); got != want {
		t.Fatalf("unexpected stock after replay: got %d want %d", got, want)
	}
}

func TestCreateOrderRejectsSameKeyWithDifferentPayload(t *testing.T) {
	service, db := newTestService(t)
	item := createTestProduct(t, db, "冲突商品", 25, 5)

	if _, err := service.CreateOrder(context.Background(), createOrderRequest(
		1,
		"conflicting-create-order",
		&pb.CreateOrderItem{ProductId: int64(item.ID), Quantity: 1},
	)); err != nil {
		t.Fatalf("first CreateOrder returned error: %v", err)
	}

	_, err := service.CreateOrder(context.Background(), createOrderRequest(
		1,
		"conflicting-create-order",
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
	start := make(chan struct{})
	var wg sync.WaitGroup

	errCh := make(chan error, requestCount)

	for i := 0; i < requestCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start

			_, err := service.CreateOrder(context.Background(), createOrderRequest(
				1,
				"concurrent-create-order",
				&pb.CreateOrderItem{ProductId: int64(item.ID), Quantity: 1},
			))

			// 关键：不要吞掉错误，先把它记录下来
			errCh <- err
		}()
	}

	close(start)
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Logf("concurrent CreateOrder error: %v", err)
		}
	}

	var orderCount int64
	if err := db.Model(&Order{}).Count(&orderCount).Error; err != nil {
		t.Fatalf("failed to count orders: %v", err)
	}
	if got, want := orderCount, int64(1); got != want {
		t.Fatalf("unexpected order count: got %d want %d", got, want)
	}
}

func TestCancelOrderRejectsMissingIdempotencyKey(t *testing.T) {
	service, _ := newTestService(t)

	_, err := service.CancelOrder(context.Background(), &pb.CancelOrderRequest{
		Id:     1,
		UserId: 1,
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("unexpected error code: got %v want %v", status.Code(err), codes.InvalidArgument)
	}
}

func TestCancelOrderCompletesIdempotencyRecordOnSuccess(t *testing.T) {
	service, db := newTestService(t)
	item := createTestProduct(t, db, "取消幂等记录商品", 10, 5)
	order := createCancellableOrder(t, service, 1, item, 2, "create-before-cancel-record")

	resp, err := service.CancelOrder(context.Background(), cancelOrderRequest(order.Id, 1, "cancel-record-key"))
	if err != nil {
		t.Fatalf("CancelOrder returned error: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected cancellation to succeed, got message %q", resp.Message)
	}

	var record idempotency.Record
	if err := db.Where("user_id = ? AND request_path = ? AND idempotency_key = ?", 1, cancelOrderRequestPath, "cancel-record-key").First(&record).Error; err != nil {
		t.Fatalf("failed to load idempotency record: %v", err)
	}
	if got, want := record.State, idempotency.StateCompleted; got != want {
		t.Fatalf("unexpected idempotency state: got %q want %q", got, want)
	}
	if got, want := record.StatusCode, http.StatusOK; got != want {
		t.Fatalf("unexpected status code: got %d want %d", got, want)
	}
	var replay pb.CancelOrderResponse
	if err := idempotency.ReplayInto(&record, &replay); err != nil {
		t.Fatalf("failed to replay stored response: %v", err)
	}
	if !proto.Equal(resp, &replay) {
		t.Fatalf("stored response mismatch: got %+v want %+v", &replay, resp)
	}
}

func TestCancelOrderReplaysSameKeySameRequest(t *testing.T) {
	service, db := newTestService(t)
	item := createTestProduct(t, db, "取消重放商品", 10, 5)
	order := createCancellableOrder(t, service, 1, item, 2, "create-before-cancel-replay")
	req := cancelOrderRequest(order.Id, 1, "repeat-cancel-key")

	first, err := service.CancelOrder(context.Background(), req)
	if err != nil {
		t.Fatalf("first CancelOrder returned error: %v", err)
	}
	second, err := service.CancelOrder(context.Background(), req)
	if err != nil {
		t.Fatalf("second CancelOrder returned error: %v", err)
	}

	if !proto.Equal(first, second) {
		t.Fatalf("expected replayed response to match first response: first=%+v second=%+v", first, second)
	}
	if !second.Success || second.Message != "订单取消成功" {
		t.Fatalf("expected cached success response, got success=%v message=%q", second.Success, second.Message)
	}
}

func TestCancelOrderRejectsSameKeyWithDifferentOrderID(t *testing.T) {
	service, db := newTestService(t)
	item := createTestProduct(t, db, "取消冲突商品", 10, 10)
	firstOrder := createCancellableOrder(t, service, 1, item, 1, "create-before-cancel-conflict-a")
	secondOrder := createCancellableOrder(t, service, 1, item, 1, "create-before-cancel-conflict-b")

	if _, err := service.CancelOrder(context.Background(), cancelOrderRequest(firstOrder.Id, 1, "conflicting-cancel-key")); err != nil {
		t.Fatalf("first CancelOrder returned error: %v", err)
	}

	_, err := service.CancelOrder(context.Background(), cancelOrderRequest(secondOrder.Id, 1, "conflicting-cancel-key"))
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("unexpected error code: got %v want %v", status.Code(err), codes.FailedPrecondition)
	}
}

func TestCancelOrderRejectsSameKeyWhileProcessing(t *testing.T) {
	service, db := newTestService(t)
	item := createTestProduct(t, db, "取消处理中商品", 10, 5)
	order := createCancellableOrder(t, service, 1, item, 1, "create-before-processing-cancel")
	requestHash, err := idempotency.HashPayload(newCancelOrderFingerprint(1, order.Id))
	if err != nil {
		t.Fatalf("failed to hash cancel request: %v", err)
	}
	if err := db.Create(&idempotency.Record{
		IdempotencyKey: "processing-cancel-key",
		UserID:         1,
		RequestPath:    cancelOrderRequestPath,
		RequestHash:    requestHash,
		State:          idempotency.StateProcessing,
		ExpiredAt:      time.Now().Add(time.Hour),
	}).Error; err != nil {
		t.Fatalf("failed to create processing idempotency record: %v", err)
	}

	_, err = service.CancelOrder(context.Background(), cancelOrderRequest(order.Id, 1, "processing-cancel-key"))
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("unexpected error code: got %v want %v", status.Code(err), codes.FailedPrecondition)
	}
}

func TestCancelOrderSameKeyDifferentUserDoesNotConflict(t *testing.T) {
	service, db := newTestService(t)
	item := createTestProduct(t, db, "取消跨用户商品", 10, 10)
	firstOrder := createCancellableOrder(t, service, 1, item, 1, "create-before-cancel-user-a")
	secondOrder := createCancellableOrder(t, service, 2, item, 1, "create-before-cancel-user-b")

	first, err := service.CancelOrder(context.Background(), cancelOrderRequest(firstOrder.Id, 1, "shared-cancel-key"))
	if err != nil {
		t.Fatalf("first CancelOrder returned error: %v", err)
	}
	second, err := service.CancelOrder(context.Background(), cancelOrderRequest(secondOrder.Id, 2, "shared-cancel-key"))
	if err != nil {
		t.Fatalf("second CancelOrder returned error: %v", err)
	}
	if !first.Success || !second.Success {
		t.Fatalf("expected both users to cancel successfully, first=%+v second=%+v", first, second)
	}
}

func TestCancelOrderAbortsIdempotencyRecordAfterBusinessFailure(t *testing.T) {
	service, db := newTestService(t)
	item := createTestProduct(t, db, "取消失败释放商品", 10, 5)
	order := createCancellableOrder(t, service, 1, item, 2, "create-before-cancel-abort")

	if err := db.Exec(`
		CREATE TRIGGER fail_cancel_outbox_insert
		BEFORE INSERT ON outbox_events
		WHEN NEW.event_type = 'order.cancelled'
		BEGIN
			SELECT RAISE(FAIL, 'forced cancel outbox failure');
		END;
	`).Error; err != nil {
		t.Fatalf("failed to create trigger: %v", err)
	}

	resp, err := service.CancelOrder(context.Background(), cancelOrderRequest(order.Id, 1, "abort-cancel-key"))
	if err != nil {
		t.Fatalf("CancelOrder returned error: %v", err)
	}
	if resp.Success || resp.Message != "取消订单失败" {
		t.Fatalf("unexpected failure response: %+v", resp)
	}

	var recordCount int64
	if err := db.Unscoped().
		Model(&idempotency.Record{}).
		Where("user_id = ? AND request_path = ? AND idempotency_key = ?", 1, cancelOrderRequestPath, "abort-cancel-key").
		Count(&recordCount).Error; err != nil {
		t.Fatalf("failed to count idempotency records: %v", err)
	}
	if got, want := recordCount, int64(0); got != want {
		t.Fatalf("unexpected idempotency record count after abort: got %d want %d", got, want)
	}

	var persisted Order
	if err := db.First(&persisted, order.Id).Error; err != nil {
		t.Fatalf("failed to reload order: %v", err)
	}
	if got, want := persisted.Status, OrderStatusPending; got != want {
		t.Fatalf("unexpected order status after rollback: got %q want %q", got, want)
	}
	var latest product.Product
	if err := db.First(&latest, item.ID).Error; err != nil {
		t.Fatalf("failed to reload product: %v", err)
	}
	if got, want := latest.Stock, int32(3); got != want {
		t.Fatalf("unexpected stock after rollback: got %d want %d", got, want)
	}

	if err := db.Exec("DROP TRIGGER fail_cancel_outbox_insert").Error; err != nil {
		t.Fatalf("failed to drop trigger: %v", err)
	}
	retry, err := service.CancelOrder(context.Background(), cancelOrderRequest(order.Id, 1, "abort-cancel-key"))
	if err != nil {
		t.Fatalf("retry CancelOrder returned error: %v", err)
	}
	if !retry.Success {
		t.Fatalf("expected retry after abort to succeed, got message %q", retry.Message)
	}
}

func TestCancelOrderReplayDoesNotRestoreStockTwice(t *testing.T) {
	service, db := newTestService(t)
	item := createTestProduct(t, db, "取消库存重放商品", 10, 5)
	order := createCancellableOrder(t, service, 1, item, 2, "create-before-cancel-stock-replay")
	req := cancelOrderRequest(order.Id, 1, "cancel-stock-replay-key")

	if _, err := service.CancelOrder(context.Background(), req); err != nil {
		t.Fatalf("first CancelOrder returned error: %v", err)
	}
	if _, err := service.CancelOrder(context.Background(), req); err != nil {
		t.Fatalf("replay CancelOrder returned error: %v", err)
	}

	var latest product.Product
	if err := db.First(&latest, item.ID).Error; err != nil {
		t.Fatalf("failed to reload product: %v", err)
	}
	if got, want := latest.Stock, int32(5); got != want {
		t.Fatalf("unexpected stock after replay: got %d want %d", got, want)
	}
}

func TestCancelOrderReplayDoesNotCreateSecondCancelledOutboxEvent(t *testing.T) {
	service, db := newTestService(t)
	item := createTestProduct(t, db, "取消事件重放商品", 10, 5)
	order := createCancellableOrder(t, service, 1, item, 1, "create-before-cancel-outbox-replay")
	req := cancelOrderRequest(order.Id, 1, "cancel-outbox-replay-key")

	if _, err := service.CancelOrder(context.Background(), req); err != nil {
		t.Fatalf("first CancelOrder returned error: %v", err)
	}
	if _, err := service.CancelOrder(context.Background(), req); err != nil {
		t.Fatalf("replay CancelOrder returned error: %v", err)
	}

	var eventCount int64
	if err := db.Model(&outbox.Event{}).
		Where("aggregate_type = ? AND aggregate_id = ? AND event_type = ?", "order", fmt.Sprintf("%d", order.Id), events.OrderCancelledType).
		Count(&eventCount).Error; err != nil {
		t.Fatalf("failed to count cancelled outbox events: %v", err)
	}
	if got, want := eventCount, int64(1); got != want {
		t.Fatalf("unexpected cancelled outbox event count: got %d want %d", got, want)
	}
}

func TestCreateOrderUsesDatabaseSnapshot(t *testing.T) {
	service, db := newTestService(t)
	item := createTestProduct(t, db, "真实商品", 88.5, 10)

	resp, err := service.CreateOrder(context.Background(), &pb.CreateOrderRequest{
		UserId:         1,
		IdempotencyKey: "test-key",
		Items: []*pb.CreateOrderItem{
			{ProductId: int64(item.ID), Quantity: 2},
		},
	})
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}

	if got, want := resp.Order.TotalAmount, float32(177); got != want {
		t.Fatalf("unexpected total amount: got %.2f want %.2f", got, want)
	}
	if len(resp.Order.Items) != 1 {
		t.Fatalf("unexpected order item count: got %d want 1", len(resp.Order.Items))
	}
	if got, want := resp.Order.Items[0].ProductName, "真实商品"; got != want {
		t.Fatalf("unexpected snapshot product name: got %q want %q", got, want)
	}
	if got, want := resp.Order.Items[0].Price, float32(88.5); got != want {
		t.Fatalf("unexpected snapshot price: got %.2f want %.2f", got, want)
	}

	var saved OrderItem
	if err := db.First(&saved).Error; err != nil {
		t.Fatalf("failed to query saved order item: %v", err)
	}
	if got, want := saved.ProductName, "真实商品"; got != want {
		t.Fatalf("unexpected saved snapshot name: got %q want %q", got, want)
	}
	if got, want := saved.Price, 88.5; got != want {
		t.Fatalf("unexpected saved snapshot price: got %.2f want %.2f", got, want)
	}
}

func TestCreateOrderStartsPending(t *testing.T) {
	service, db := newTestService(t)
	item := createTestProduct(t, db, "默认状态商品", 10, 5)

	resp, err := service.CreateOrder(context.Background(), &pb.CreateOrderRequest{
		UserId:         1,
		IdempotencyKey: "test-key",
		Items: []*pb.CreateOrderItem{
			{ProductId: int64(item.ID), Quantity: 1},
		},
	})
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}
	if got, want := resp.Order.Status, OrderStatusPending; got != want {
		t.Fatalf("unexpected order status: got %q want %q", got, want)
	}
}

func TestCreateOrderStoresCommittedOrderCreatedEventInOutbox(t *testing.T) {
	publisher := &recordingPublisher{}
	service, db := newTestServiceWithPublisher(t, publisher)
	item := createTestProduct(t, db, "事件商品", 88.5, 10)

	resp, err := service.CreateOrder(context.Background(), &pb.CreateOrderRequest{
		UserId:         1,
		IdempotencyKey: "test-key",
		Items: []*pb.CreateOrderItem{
			{ProductId: int64(item.ID), Quantity: 2},
		},
	})
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}
	if got := len(publisher.events); got != 0 {
		t.Fatalf("unexpected direct publish count: got %d want 0", got)
	}

	var saved outbox.Event
	if err := db.Where("event_type = ?", events.OrderCreatedType).First(&saved).Error; err != nil {
		t.Fatalf("failed to load outbox event: %v", err)
	}
	if got, want := saved.Status, outbox.StatusPending; got != want {
		t.Fatalf("unexpected outbox status: got %q want %q", got, want)
	}

	var event events.OrderCreatedEvent
	if err := json.Unmarshal([]byte(saved.Payload), &event); err != nil {
		t.Fatalf("failed to decode outbox payload: %v", err)
	}
	if event.EventID == "" {
		t.Fatal("expected event id to be set")
	}
	if got, want := event.EventType, events.OrderCreatedType; got != want {
		t.Fatalf("unexpected event type field: got %q want %q", got, want)
	}
	if got, want := event.OrderID, resp.Order.Id; got != want {
		t.Fatalf("unexpected order id: got %d want %d", got, want)
	}
	if got, want := len(event.Items), 1; got != want {
		t.Fatalf("unexpected event item count: got %d want %d", got, want)
	}
}

func TestCreateOrderSchedulesTimeoutCheckAfterCommit(t *testing.T) {
	publisher := &recordingPublisher{}
	scheduler := &recordingTimeoutScheduler{}
	service, db := newTestServiceWithTimeout(t, publisher, scheduler, 30*time.Second)
	item := createTestProduct(t, db, "超时调度商品", 20, 5)

	resp, err := service.CreateOrder(context.Background(), &pb.CreateOrderRequest{
		UserId:         9,
		IdempotencyKey: "test-key",
		Items: []*pb.CreateOrderItem{
			{ProductId: int64(item.ID), Quantity: 1},
		},
	})
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}

	if len(scheduler.events) != 1 {
		t.Fatalf("unexpected scheduled timeout count: got %d want 1", len(scheduler.events))
	}

	scheduled := scheduler.events[0]
	if got, want := scheduled.delay, 30*time.Second; got != want {
		t.Fatalf("unexpected timeout delay: got %v want %v", got, want)
	}
	if got, want := scheduled.event.EventType, events.OrderTimeoutCheckType; got != want {
		t.Fatalf("unexpected timeout event type: got %q want %q", got, want)
	}
	if got, want := scheduled.event.OrderID, resp.Order.Id; got != want {
		t.Fatalf("unexpected timeout event order id: got %d want %d", got, want)
	}
	if got, want := scheduled.event.UserID, int64(9); got != want {
		t.Fatalf("unexpected timeout event user id: got %d want %d", got, want)
	}
	if got, want := scheduled.event.TimeoutMinutes, 0.5; got != want {
		t.Fatalf("unexpected timeout minutes: got %.2f want %.2f", got, want)
	}

	createdAt, err := time.Parse(time.RFC3339Nano, scheduled.event.CreatedAt)
	if err != nil {
		t.Fatalf("failed to parse timeout created_at: %v", err)
	}
	expireAt, err := time.Parse(time.RFC3339Nano, scheduled.event.ExpireAt)
	if err != nil {
		t.Fatalf("failed to parse timeout expire_at: %v", err)
	}
	if got, want := expireAt.Sub(createdAt), 30*time.Second; got != want {
		t.Fatalf("unexpected timeout window: got %v want %v", got, want)
	}
}

func TestCreateOrderKeepsPendingOutboxEventWhenPublisherUnavailable(t *testing.T) {
	publisher := &recordingPublisher{err: errors.New("rabbitmq unavailable")}
	service, db := newTestServiceWithPublisher(t, publisher)
	item := createTestProduct(t, db, "弱一致商品", 10, 5)

	resp, err := service.CreateOrder(context.Background(), &pb.CreateOrderRequest{
		UserId:         1,
		IdempotencyKey: "test-key",
		Items: []*pb.CreateOrderItem{
			{ProductId: int64(item.ID), Quantity: 1},
		},
	})
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}
	if resp.Order.Id == 0 {
		t.Fatal("expected order to be created")
	}

	var count int64
	if err := db.Model(&Order{}).Count(&count).Error; err != nil {
		t.Fatalf("failed to count orders: %v", err)
	}
	if got, want := count, int64(1); got != want {
		t.Fatalf("unexpected order count: got %d want %d", got, want)
	}

	var saved outbox.Event
	if err := db.Where("event_type = ?", events.OrderCreatedType).First(&saved).Error; err != nil {
		t.Fatalf("failed to load outbox event: %v", err)
	}
	if got, want := saved.Status, outbox.StatusPending; got != want {
		t.Fatalf("unexpected outbox status: got %q want %q", got, want)
	}
}

func TestCreateOrderRejectsMissingProduct(t *testing.T) {
	service, _ := newTestService(t)

	_, err := service.CreateOrder(context.Background(), &pb.CreateOrderRequest{
		UserId:         1,
		IdempotencyKey: "test-key",
		Items: []*pb.CreateOrderItem{
			{ProductId: 999, Quantity: 1},
		},
	})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("unexpected error code: got %v want %v", status.Code(err), codes.NotFound)
	}
}

func TestCreateOrderRejectsInsufficientStock(t *testing.T) {
	service, db := newTestService(t)
	item := createTestProduct(t, db, "库存商品", 12, 1)

	_, err := service.CreateOrder(context.Background(), &pb.CreateOrderRequest{
		UserId:         1,
		IdempotencyKey: "test-key",
		Items: []*pb.CreateOrderItem{
			{ProductId: int64(item.ID), Quantity: 2},
		},
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("unexpected error code: got %v want %v", status.Code(err), codes.InvalidArgument)
	}
}

func TestCreateOrderRejectsInvalidQuantity(t *testing.T) {
	tests := []struct {
		name     string
		quantity int32
	}{
		{name: "zero", quantity: 0},
		{name: "negative", quantity: -1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service, db := newTestService(t)
			item := createTestProduct(t, db, "数量商品", 10, 5)

			_, err := service.CreateOrder(context.Background(), &pb.CreateOrderRequest{
				UserId:         1,
				IdempotencyKey: "test-key",
				Items: []*pb.CreateOrderItem{
					{ProductId: int64(item.ID), Quantity: tc.quantity},
				},
			})
			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("unexpected error code: got %v want %v", status.Code(err), codes.InvalidArgument)
			}
		})
	}
}

func TestCreateOrderRejectsEmptyItems(t *testing.T) {
	service, _ := newTestService(t)

	_, err := service.CreateOrder(context.Background(), &pb.CreateOrderRequest{
		UserId:         1,
		IdempotencyKey: "test-key",
		Items:          nil,
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("unexpected error code: got %v want %v", status.Code(err), codes.InvalidArgument)
	}
}

func TestCreateOrderCalculatesTotalAcrossProducts(t *testing.T) {
	service, db := newTestService(t)
	first := createTestProduct(t, db, "商品A", 10, 10)
	second := createTestProduct(t, db, "商品B", 20.5, 10)

	resp, err := service.CreateOrder(context.Background(), &pb.CreateOrderRequest{
		UserId:         1,
		IdempotencyKey: "test-key",
		Items: []*pb.CreateOrderItem{
			{ProductId: int64(first.ID), Quantity: 2},
			{ProductId: int64(second.ID), Quantity: 3},
		},
	})
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}

	if got, want := resp.Order.TotalAmount, float32(81.5); got != want {
		t.Fatalf("unexpected total amount: got %.2f want %.2f", got, want)
	}
}

func TestCreateOrderMergesDuplicateProducts(t *testing.T) {
	service, db := newTestService(t)
	item := createTestProduct(t, db, "重复商品", 15, 10)

	resp, err := service.CreateOrder(context.Background(), &pb.CreateOrderRequest{
		UserId:         1,
		IdempotencyKey: "test-key",
		Items: []*pb.CreateOrderItem{
			{ProductId: int64(item.ID), Quantity: 1},
			{ProductId: int64(item.ID), Quantity: 2},
		},
	})
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}

	if len(resp.Order.Items) != 1 {
		t.Fatalf("unexpected order item count: got %d want 1", len(resp.Order.Items))
	}
	if got, want := resp.Order.Items[0].Quantity, int32(3); got != want {
		t.Fatalf("unexpected merged quantity: got %d want %d", got, want)
	}
	if got, want := resp.Order.TotalAmount, float32(45); got != want {
		t.Fatalf("unexpected total amount: got %.2f want %.2f", got, want)
	}

	var latest product.Product
	if err := db.First(&latest, item.ID).Error; err != nil {
		t.Fatalf("failed to query updated product: %v", err)
	}
	if got, want := latest.Stock, int32(7); got != want {
		t.Fatalf("unexpected remaining stock: got %d want %d", got, want)
	}
}

func TestCreateOrderRollsBackStockWhenOrderInsertFails(t *testing.T) {
	publisher := &recordingPublisher{}
	service, db := newTestServiceWithPublisher(t, publisher)
	item := createTestProduct(t, db, "回滚商品", 10, 5)

	if err := db.Exec(`
		CREATE TRIGGER fail_order_insert
		BEFORE INSERT ON orders
		BEGIN
			SELECT RAISE(FAIL, 'forced order insert failure');
		END;
	`).Error; err != nil {
		t.Fatalf("failed to create trigger: %v", err)
	}

	_, err := service.CreateOrder(context.Background(), &pb.CreateOrderRequest{
		UserId:         1,
		IdempotencyKey: "test-key",
		Items: []*pb.CreateOrderItem{
			{ProductId: int64(item.ID), Quantity: 2},
		},
	})
	if status.Code(err) != codes.Internal {
		t.Fatalf("unexpected error code: got %v want %v", status.Code(err), codes.Internal)
	}

	var latest product.Product
	if err := db.First(&latest, item.ID).Error; err != nil {
		t.Fatalf("failed to query product after rollback: %v", err)
	}
	if got, want := latest.Stock, int32(5); got != want {
		t.Fatalf("unexpected stock after rollback: got %d want %d", got, want)
	}
	var outboxCount int64
	if err := db.Model(&outbox.Event{}).Count(&outboxCount).Error; err != nil {
		t.Fatalf("failed to count outbox events after rollback: %v", err)
	}
	if got, want := outboxCount, int64(0); got != want {
		t.Fatalf("unexpected outbox count after rollback: got %d want %d", got, want)
	}
}

func TestCreateOrderRollsBackStockWhenOrderItemInsertFails(t *testing.T) {
	service, db := newTestService(t)
	item := createTestProduct(t, db, "订单项回滚商品", 10, 5)

	if err := db.Exec(`
		CREATE TRIGGER fail_order_item_insert
		BEFORE INSERT ON order_items
		BEGIN
			SELECT RAISE(FAIL, 'forced order item insert failure');
		END;
	`).Error; err != nil {
		t.Fatalf("failed to create trigger: %v", err)
	}

	_, err := service.CreateOrder(context.Background(), &pb.CreateOrderRequest{
		UserId:         1,
		IdempotencyKey: "test-key",
		Items: []*pb.CreateOrderItem{
			{ProductId: int64(item.ID), Quantity: 2},
		},
	})
	if status.Code(err) != codes.Internal {
		t.Fatalf("unexpected error code: got %v want %v", status.Code(err), codes.Internal)
	}

	var latest product.Product
	if err := db.First(&latest, item.ID).Error; err != nil {
		t.Fatalf("failed to query product after rollback: %v", err)
	}
	if got, want := latest.Stock, int32(5); got != want {
		t.Fatalf("unexpected stock after rollback: got %d want %d", got, want)
	}
}

func TestCreateOrderConcurrentRequestsDoNotOversell(t *testing.T) {
	service, db := newConcurrentTestService(t)
	item := createTestProduct(t, db, "并发商品", 10, 10)

	const requestCount = 100
	var successCount int32
	var wg sync.WaitGroup
	start := make(chan struct{})

	for i := 0; i < requestCount; i++ {
		wg.Add(1)
		go func(userID int64) {
			defer wg.Done()
			<-start

			if _, err := service.CreateOrder(context.Background(), &pb.CreateOrderRequest{
				UserId:         userID,
				IdempotencyKey: "test-key",
				Items: []*pb.CreateOrderItem{
					{ProductId: int64(item.ID), Quantity: 1},
				},
			}); err == nil {
				atomic.AddInt32(&successCount, 1)
			}
		}(int64(i + 1))
	}

	close(start)
	wg.Wait()

	var latest product.Product
	if err := db.First(&latest, item.ID).Error; err != nil {
		t.Fatalf("failed to query latest product: %v", err)
	}
	if successCount > 10 {
		t.Fatalf("oversold inventory: success count %d exceeds stock 10", successCount)
	}
	if latest.Stock < 0 {
		t.Fatalf("stock should never be negative, got %d", latest.Stock)
	}
	if got, want := int(successCount)+int(latest.Stock), 10; got != want {
		t.Fatalf("unexpected stock conservation: got success+stock=%d want %d", got, want)
	}
}

func TestCancelOrderRestoresStockAtomically(t *testing.T) {
	service, db := newTestService(t)
	item := createTestProduct(t, db, "取消回补商品", 10, 5)

	resp, err := service.CreateOrder(context.Background(), &pb.CreateOrderRequest{
		UserId:         1,
		IdempotencyKey: "test-key",
		Items: []*pb.CreateOrderItem{
			{ProductId: int64(item.ID), Quantity: 2},
		},
	})
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}

	cancelResp, err := service.CancelOrder(context.Background(), &pb.CancelOrderRequest{
		Id:             resp.Order.Id,
		UserId:         1,
		IdempotencyKey: "cancel-restore-stock-key",
	})
	if err != nil {
		t.Fatalf("CancelOrder returned error: %v", err)
	}
	if !cancelResp.Success {
		t.Fatalf("expected cancellation to succeed, got message %q", cancelResp.Message)
	}

	var latest product.Product
	if err := db.First(&latest, item.ID).Error; err != nil {
		t.Fatalf("failed to query restored product: %v", err)
	}
	if got, want := latest.Stock, int32(5); got != want {
		t.Fatalf("unexpected restored stock: got %d want %d", got, want)
	}
}

func TestCancelOrderStoresOrderCancelledEventInOutbox(t *testing.T) {
	publisher := &recordingPublisher{}
	service, db := newTestServiceWithPublisher(t, publisher)
	item := createTestProduct(t, db, "取消事件商品", 10, 5)

	resp, err := service.CreateOrder(context.Background(), &pb.CreateOrderRequest{
		UserId:         1,
		IdempotencyKey: "test-key",
		Items: []*pb.CreateOrderItem{
			{ProductId: int64(item.ID), Quantity: 1},
		},
	})
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}
	cancelResp, err := service.CancelOrder(context.Background(), &pb.CancelOrderRequest{
		Id:             resp.Order.Id,
		UserId:         1,
		IdempotencyKey: "cancel-outbox-key",
	})
	if err != nil {
		t.Fatalf("CancelOrder returned error: %v", err)
	}
	if !cancelResp.Success {
		t.Fatalf("expected cancellation to succeed, got message %q", cancelResp.Message)
	}
	if got := len(publisher.events); got != 0 {
		t.Fatalf("unexpected direct publish count: got %d want 0", got)
	}

	var saved outbox.Event
	if err := db.Where("event_type = ?", events.OrderCancelledType).Order("id DESC").First(&saved).Error; err != nil {
		t.Fatalf("failed to load outbox event: %v", err)
	}
	var event events.OrderCancelledEvent
	if err := json.Unmarshal([]byte(saved.Payload), &event); err != nil {
		t.Fatalf("failed to decode outbox payload: %v", err)
	}
	if got, want := event.OrderID, resp.Order.Id; got != want {
		t.Fatalf("unexpected order id: got %d want %d", got, want)
	}
}

func TestCancelOrderPersistsUserCancelReason(t *testing.T) {
	service, db := newTestService(t)
	item := createTestProduct(t, db, "取消原因商品", 10, 5)

	resp, err := service.CreateOrder(context.Background(), &pb.CreateOrderRequest{
		UserId:         1,
		IdempotencyKey: "test-key",
		Items: []*pb.CreateOrderItem{
			{ProductId: int64(item.ID), Quantity: 1},
		},
	})
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}

	cancelResp, err := service.CancelOrder(context.Background(), &pb.CancelOrderRequest{
		Id:             resp.Order.Id,
		UserId:         1,
		IdempotencyKey: "cancel-reason-key",
	})
	if err != nil {
		t.Fatalf("CancelOrder returned error: %v", err)
	}
	if !cancelResp.Success {
		t.Fatalf("expected cancellation to succeed, got message %q", cancelResp.Message)
	}

	var latest Order
	if err := db.First(&latest, resp.Order.Id).Error; err != nil {
		t.Fatalf("failed to reload order: %v", err)
	}
	if got, want := latest.CancelReason, OrderCancelReasonUserCancelled; got != want {
		t.Fatalf("unexpected cancel reason: got %q want %q", got, want)
	}
}

func TestConvertToPBOrderIncludesCancelReason(t *testing.T) {
	order := &Order{
		UserID:       1,
		TotalAmount:  10,
		Status:       OrderStatusCancelled,
		CancelReason: OrderCancelReasonPaymentTimeout,
		OrderDate:    time.Now(),
	}

	converted := convertToPBOrder(order, nil)
	if got, want := converted.CancelReason, OrderCancelReasonPaymentTimeout; got != want {
		t.Fatalf("unexpected cancel reason: got %q want %q", got, want)
	}
}

func TestCancelOrderRejectsCompletedOrder(t *testing.T) {
	service, db := newTestService(t)
	order := Order{
		UserID:      1,
		TotalAmount: 10,
		Status:      OrderStatusCompleted,
		OrderDate:   time.Now(),
	}
	if err := db.Create(&order).Error; err != nil {
		t.Fatalf("failed to create order: %v", err)
	}

	resp, err := service.CancelOrder(context.Background(), &pb.CancelOrderRequest{
		Id:             int64(order.ID),
		UserId:         1,
		IdempotencyKey: "cancel-completed-key",
	})
	if err != nil {
		t.Fatalf("CancelOrder returned error: %v", err)
	}
	if resp.Success {
		t.Fatal("expected completed order cancellation to fail")
	}
	if got, want := resp.Message, "invalid order status transition: completed -> cancelled"; got != want {
		t.Fatalf("unexpected message: got %q want %q", got, want)
	}
}

func TestCancelOrderRejectsPaidOrder(t *testing.T) {
	service, db := newTestService(t)
	order := Order{
		UserID:      1,
		TotalAmount: 10,
		Status:      OrderStatusPaid,
		OrderDate:   time.Now(),
	}
	if err := db.Create(&order).Error; err != nil {
		t.Fatalf("failed to create order: %v", err)
	}

	resp, err := service.CancelOrder(context.Background(), &pb.CancelOrderRequest{
		Id:             int64(order.ID),
		UserId:         1,
		IdempotencyKey: "cancel-paid-key",
	})
	if err != nil {
		t.Fatalf("CancelOrder returned error: %v", err)
	}
	if resp.Success {
		t.Fatal("expected paid order cancellation to fail")
	}
	if got, want := resp.Message, "invalid order status transition: paid -> cancelled"; got != want {
		t.Fatalf("unexpected message: got %q want %q", got, want)
	}
}

func TestShipOrderTransitionsPaidToShipped(t *testing.T) {
	publisher := &recordingPublisher{}
	service, db := newTestServiceWithPublisher(t, publisher)
	merchantUser := createTestUser(t, db, auth.RoleMerchant)
	shop := createTestMerchant(t, db, merchantUser.ID)
	order := Order{
		UserID:      1,
		TotalAmount: 10,
		Status:      OrderStatusPaid,
		OrderDate:   time.Now(),
	}
	if err := db.Create(&order).Error; err != nil {
		t.Fatalf("failed to create order: %v", err)
	}
	if err := db.Create(&OrderItem{OrderID: order.ID, ProductID: 1, ProductName: "商品", Price: 10, Quantity: 1, MerchantID: shop.ID}).Error; err != nil {
		t.Fatalf("failed to create order item: %v", err)
	}

	resp, err := service.ShipOrder(context.Background(), &pb.ShipOrderRequest{
		Id:          int64(order.ID),
		ActorUserId: int64(merchantUser.ID),
	})
	if err != nil {
		t.Fatalf("ShipOrder returned error: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected ship to succeed, got message %q", resp.Message)
	}

	var latest Order
	if err := db.First(&latest, order.ID).Error; err != nil {
		t.Fatalf("failed to reload order: %v", err)
	}
	if got, want := latest.Status, OrderStatusShipped; got != want {
		t.Fatalf("unexpected order status: got %q want %q", got, want)
	}
	if got := len(publisher.events); got != 0 {
		t.Fatalf("unexpected direct publish count: got %d want 0", got)
	}
	var saved outbox.Event
	if err := db.Where("event_type = ?", events.OrderShippedType).First(&saved).Error; err != nil {
		t.Fatalf("failed to load shipped outbox event: %v", err)
	}
}

func TestShipOrderRejectsForeignMerchant(t *testing.T) {
	service, db := newTestService(t)
	owner := createTestUser(t, db, auth.RoleMerchant)
	otherMerchant := createTestUser(t, db, auth.RoleMerchant)
	shop := createTestMerchant(t, db, owner.ID)
	order := Order{
		UserID:      1,
		TotalAmount: 10,
		Status:      OrderStatusPaid,
		OrderDate:   time.Now(),
	}
	if err := db.Create(&order).Error; err != nil {
		t.Fatalf("failed to create order: %v", err)
	}
	if err := db.Create(&OrderItem{OrderID: order.ID, ProductID: 1, ProductName: "商品", Price: 10, Quantity: 1, MerchantID: shop.ID}).Error; err != nil {
		t.Fatalf("failed to create order item: %v", err)
	}

	_, err := service.ShipOrder(context.Background(), &pb.ShipOrderRequest{
		Id:          int64(order.ID),
		ActorUserId: int64(otherMerchant.ID),
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("unexpected error code: got %v want %v", status.Code(err), codes.PermissionDenied)
	}

	var latest Order
	if err := db.First(&latest, order.ID).Error; err != nil {
		t.Fatalf("failed to reload order: %v", err)
	}
	if got, want := latest.Status, OrderStatusPaid; got != want {
		t.Fatalf("unexpected order status after permission failure: got %q want %q", got, want)
	}
	if got := countOrderOutboxEvents(t, db, order.ID, events.OrderShippedType); got != 0 {
		t.Fatalf("unexpected shipped outbox event count after permission failure: got %d want 0", got)
	}
}

func TestShipOrderRejectsCustomerActor(t *testing.T) {
	service, db := newTestService(t)
	customer := createTestUser(t, db, auth.RoleCustomer)
	order := Order{
		UserID:      1,
		TotalAmount: 10,
		Status:      OrderStatusPaid,
		OrderDate:   time.Now(),
	}
	if err := db.Create(&order).Error; err != nil {
		t.Fatalf("failed to create order: %v", err)
	}

	_, err := service.ShipOrder(context.Background(), &pb.ShipOrderRequest{
		Id:          int64(order.ID),
		ActorUserId: int64(customer.ID),
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("unexpected error code: got %v want %v", status.Code(err), codes.PermissionDenied)
	}

	var latest Order
	if err := db.First(&latest, order.ID).Error; err != nil {
		t.Fatalf("failed to reload order: %v", err)
	}
	if got, want := latest.Status, OrderStatusPaid; got != want {
		t.Fatalf("unexpected order status after customer ship attempt: got %q want %q", got, want)
	}
	if got := countOrderOutboxEvents(t, db, order.ID, events.OrderShippedType); got != 0 {
		t.Fatalf("unexpected shipped outbox event count after customer ship attempt: got %d want 0", got)
	}
}

func TestShipOrderRejectsInvalidStatuses(t *testing.T) {
	statuses := []string{
		OrderStatusPending,
		OrderStatusShipped,
		OrderStatusCompleted,
		OrderStatusCancelled,
	}

	for _, initialStatus := range statuses {
		t.Run(initialStatus, func(t *testing.T) {
			service, db := newTestService(t)
			admin := createTestUser(t, db, auth.RoleAdmin)
			order := Order{
				UserID:      1,
				TotalAmount: 10,
				Status:      initialStatus,
				OrderDate:   time.Now(),
			}
			if err := db.Create(&order).Error; err != nil {
				t.Fatalf("failed to create order: %v", err)
			}

			_, err := service.ShipOrder(context.Background(), &pb.ShipOrderRequest{
				Id:          int64(order.ID),
				ActorUserId: int64(admin.ID),
			})
			if status.Code(err) != codes.FailedPrecondition {
				t.Fatalf("unexpected error code: got %v want %v", status.Code(err), codes.FailedPrecondition)
			}

			var latest Order
			if err := db.First(&latest, order.ID).Error; err != nil {
				t.Fatalf("failed to reload order: %v", err)
			}
			if got, want := latest.Status, initialStatus; got != want {
				t.Fatalf("unexpected order status after failed ship: got %q want %q", got, want)
			}
			if got := countOrderOutboxEvents(t, db, order.ID, events.OrderShippedType); got != 0 {
				t.Fatalf("unexpected shipped outbox event count: got %d want 0", got)
			}
		})
	}
}

func TestShipOrderRollsBackStatusWhenOutboxInsertFails(t *testing.T) {
	service, db := newTestService(t)
	admin := createTestUser(t, db, auth.RoleAdmin)
	order := Order{
		UserID:      1,
		TotalAmount: 10,
		Status:      OrderStatusPaid,
		OrderDate:   time.Now(),
	}
	if err := db.Create(&order).Error; err != nil {
		t.Fatalf("failed to create order: %v", err)
	}
	if err := db.Exec(`
		CREATE TRIGGER fail_ship_outbox_insert
		BEFORE INSERT ON outbox_events
		WHEN NEW.event_type = 'order.shipped'
		BEGIN
			SELECT RAISE(FAIL, 'forced ship outbox failure');
		END;
	`).Error; err != nil {
		t.Fatalf("failed to create trigger: %v", err)
	}

	_, err := service.ShipOrder(context.Background(), &pb.ShipOrderRequest{
		Id:          int64(order.ID),
		ActorUserId: int64(admin.ID),
	})
	if status.Code(err) != codes.Internal {
		t.Fatalf("unexpected error code: got %v want %v", status.Code(err), codes.Internal)
	}

	var latest Order
	if err := db.First(&latest, order.ID).Error; err != nil {
		t.Fatalf("failed to reload order: %v", err)
	}
	if got, want := latest.Status, OrderStatusPaid; got != want {
		t.Fatalf("unexpected order status after outbox failure: got %q want %q", got, want)
	}
	if got := countOrderOutboxEvents(t, db, order.ID, events.OrderShippedType); got != 0 {
		t.Fatalf("unexpected shipped outbox event count after rollback: got %d want 0", got)
	}
}

func TestCompleteOrderTransitionsShippedToCompleted(t *testing.T) {
	publisher := &recordingPublisher{}
	service, db := newTestServiceWithPublisher(t, publisher)
	order := Order{
		UserID:      7,
		TotalAmount: 10,
		Status:      OrderStatusShipped,
		OrderDate:   time.Now(),
	}
	if err := db.Create(&order).Error; err != nil {
		t.Fatalf("failed to create order: %v", err)
	}

	resp, err := service.CompleteOrder(context.Background(), &pb.CompleteOrderRequest{
		Id:     int64(order.ID),
		UserId: 7,
	})
	if err != nil {
		t.Fatalf("CompleteOrder returned error: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected completion to succeed, got message %q", resp.Message)
	}

	var latest Order
	if err := db.First(&latest, order.ID).Error; err != nil {
		t.Fatalf("failed to reload order: %v", err)
	}
	if got, want := latest.Status, OrderStatusCompleted; got != want {
		t.Fatalf("unexpected order status: got %q want %q", got, want)
	}
	if got := len(publisher.events); got != 0 {
		t.Fatalf("unexpected direct publish count: got %d want 0", got)
	}
	var saved outbox.Event
	if err := db.Where("event_type = ?", events.OrderCompletedType).First(&saved).Error; err != nil {
		t.Fatalf("failed to load completed outbox event: %v", err)
	}
}

func TestCompleteOrderRejectsInvalidStatuses(t *testing.T) {
	statuses := []string{
		OrderStatusPending,
		OrderStatusPaid,
		OrderStatusCompleted,
		OrderStatusCancelled,
	}

	for _, initialStatus := range statuses {
		t.Run(initialStatus, func(t *testing.T) {
			service, db := newTestService(t)
			order := Order{
				UserID:      7,
				TotalAmount: 10,
				Status:      initialStatus,
				OrderDate:   time.Now(),
			}
			if err := db.Create(&order).Error; err != nil {
				t.Fatalf("failed to create order: %v", err)
			}

			_, err := service.CompleteOrder(context.Background(), &pb.CompleteOrderRequest{
				Id:     int64(order.ID),
				UserId: 7,
			})
			if status.Code(err) != codes.FailedPrecondition {
				t.Fatalf("unexpected error code: got %v want %v", status.Code(err), codes.FailedPrecondition)
			}

			var latest Order
			if err := db.First(&latest, order.ID).Error; err != nil {
				t.Fatalf("failed to reload order: %v", err)
			}
			if got, want := latest.Status, initialStatus; got != want {
				t.Fatalf("unexpected order status after failed completion: got %q want %q", got, want)
			}
			if got := countOrderOutboxEvents(t, db, order.ID, events.OrderCompletedType); got != 0 {
				t.Fatalf("unexpected completed outbox event count: got %d want 0", got)
			}
		})
	}
}

func TestCompleteOrderRollsBackStatusWhenOutboxInsertFails(t *testing.T) {
	service, db := newTestService(t)
	order := Order{
		UserID:      7,
		TotalAmount: 10,
		Status:      OrderStatusShipped,
		OrderDate:   time.Now(),
	}
	if err := db.Create(&order).Error; err != nil {
		t.Fatalf("failed to create order: %v", err)
	}
	if err := db.Exec(`
		CREATE TRIGGER fail_complete_outbox_insert
		BEFORE INSERT ON outbox_events
		WHEN NEW.event_type = 'order.completed'
		BEGIN
			SELECT RAISE(FAIL, 'forced complete outbox failure');
		END;
	`).Error; err != nil {
		t.Fatalf("failed to create trigger: %v", err)
	}

	_, err := service.CompleteOrder(context.Background(), &pb.CompleteOrderRequest{
		Id:     int64(order.ID),
		UserId: 7,
	})
	if status.Code(err) != codes.Internal {
		t.Fatalf("unexpected error code: got %v want %v", status.Code(err), codes.Internal)
	}

	var latest Order
	if err := db.First(&latest, order.ID).Error; err != nil {
		t.Fatalf("failed to reload order: %v", err)
	}
	if got, want := latest.Status, OrderStatusShipped; got != want {
		t.Fatalf("unexpected order status after outbox failure: got %q want %q", got, want)
	}
	if got := countOrderOutboxEvents(t, db, order.ID, events.OrderCompletedType); got != 0 {
		t.Fatalf("unexpected completed outbox event count after rollback: got %d want 0", got)
	}
}

func TestListMerchantOrdersOnlyReturnsRelatedOrders(t *testing.T) {
	service, db := newTestService(t)
	merchantUser := createTestUser(t, db, auth.RoleMerchant)
	otherUser := createTestUser(t, db, auth.RoleMerchant)
	ownShop := createTestMerchant(t, db, merchantUser.ID)
	otherShop := createTestMerchant(t, db, otherUser.ID)

	ownOrder := Order{UserID: 10, TotalAmount: 30, Status: OrderStatusPaid, OrderDate: time.Now()}
	if err := db.Create(&ownOrder).Error; err != nil {
		t.Fatalf("failed to create own order: %v", err)
	}
	if err := db.Create(&[]OrderItem{
		{OrderID: ownOrder.ID, ProductID: 1, MerchantID: ownShop.ID, ProductName: "Own Item", Price: 10, Quantity: 1},
		{OrderID: ownOrder.ID, ProductID: 2, MerchantID: otherShop.ID, ProductName: "Other Item", Price: 20, Quantity: 1},
	}).Error; err != nil {
		t.Fatalf("failed to create mixed order items: %v", err)
	}

	foreignOrder := Order{UserID: 11, TotalAmount: 40, Status: OrderStatusPaid, OrderDate: time.Now()}
	if err := db.Create(&foreignOrder).Error; err != nil {
		t.Fatalf("failed to create foreign order: %v", err)
	}
	if err := db.Create(&OrderItem{
		OrderID: foreignOrder.ID, ProductID: 3, MerchantID: otherShop.ID, ProductName: "Foreign Item", Price: 40, Quantity: 1,
	}).Error; err != nil {
		t.Fatalf("failed to create foreign order item: %v", err)
	}

	resp, err := service.ListMerchantOrders(context.Background(), &pb.ListMerchantOrdersRequest{
		ActorUserId: int64(merchantUser.ID),
		Page:        1,
		PageSize:    10,
	})
	if err != nil {
		t.Fatalf("ListMerchantOrders returned error: %v", err)
	}
	if got, want := resp.Total, int64(1); got != want {
		t.Fatalf("unexpected merchant order total: got %d want %d", got, want)
	}
	if got, want := len(resp.Orders), 1; got != want {
		t.Fatalf("unexpected merchant order count: got %d want %d", got, want)
	}
	if got, want := resp.Orders[0].Id, int64(ownOrder.ID); got != want {
		t.Fatalf("unexpected order id: got %d want %d", got, want)
	}
	if got, want := len(resp.Orders[0].Items), 1; got != want {
		t.Fatalf("unexpected merchant order item count: got %d want %d", got, want)
	}
	if got, want := resp.Orders[0].Items[0].ProductName, "Own Item"; got != want {
		t.Fatalf("unexpected merchant order item: got %q want %q", got, want)
	}
}
