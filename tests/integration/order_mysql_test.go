//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pbOrder "go-commerce/api/order"
	"go-commerce/internal/auth"
	"go-commerce/internal/idempotency"
	"go-commerce/internal/order"
	"go-commerce/internal/outbox"
	"go-commerce/internal/product"
	"go-commerce/pkg/events"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

func TestMySQLOrderPersistsAndDeductsStock(t *testing.T) {
	ctx := context.Background()
	db := openIntegrationDB(t)
	if err := db.AutoMigrate(&product.Product{}, &order.Order{}, &order.OrderItem{}, &idempotency.Record{}, &outbox.Event{}); err != nil {
		t.Fatalf("failed to migrate integration schema: %v", err)
	}
	ensureIntegrationOrderIndexes(t, db)

	item := product.Product{
		Name:        "integration-order-" + uniqueSuffix(t),
		Description: "mysql integration fixture",
		Price:       88.5,
		Stock:       5,
		MerchantID:  1,
	}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("failed to create integration product: %v", err)
	}

	var createdOrderID int64
	t.Cleanup(func() {
		if createdOrderID > 0 {
			_ = db.Where("order_id = ?", createdOrderID).Delete(&order.OrderItem{}).Error
			_ = db.Delete(&order.Order{}, createdOrderID).Error
		}
		_ = db.Where("aggregate_type = ? AND aggregate_id = ?", "order", fmt.Sprintf("%d", createdOrderID)).Delete(&outbox.Event{}).Error
		_ = db.Where("request_path = ? AND idempotency_key = ?", "/api/orders", "integration-order-"+fmt.Sprintf("%d", item.ID)).Delete(&idempotency.Record{}).Error
		_ = db.Delete(&product.Product{}, item.ID).Error
	})

	service := order.NewService(db, nil)
	resp, err := service.CreateOrder(ctx, &pbOrder.CreateOrderRequest{
		UserId:         7,
		IdempotencyKey: "integration-order-" + fmt.Sprintf("%d", item.ID),
		Items: []*pbOrder.CreateOrderItem{
			{ProductId: int64(item.ID), Quantity: 2},
		},
	})
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}
	createdOrderID = resp.Order.Id

	var persisted order.Order
	if err := db.First(&persisted, createdOrderID).Error; err != nil {
		t.Fatalf("failed to reload order: %v", err)
	}
	if got, want := persisted.TotalAmount, 177.0; got != want {
		t.Fatalf("unexpected total amount: got %.2f want %.2f", got, want)
	}

	var latest product.Product
	if err := db.First(&latest, item.ID).Error; err != nil {
		t.Fatalf("failed to reload product: %v", err)
	}
	if got, want := latest.Stock, int32(3); got != want {
		t.Fatalf("unexpected stock after order: got %d want %d", got, want)
	}
}

func TestMySQLCancelOrderIdempotencyReplaysWithoutDuplicateSideEffects(t *testing.T) {
	ctx := context.Background()
	db := openIntegrationDB(t)
	if err := db.AutoMigrate(&product.Product{}, &order.Order{}, &order.OrderItem{}, &idempotency.Record{}, &outbox.Event{}); err != nil {
		t.Fatalf("failed to migrate integration schema: %v", err)
	}
	ensureIntegrationOrderIndexes(t, db)

	item := product.Product{
		Name:        "integration-cancel-" + uniqueSuffix(t),
		Description: "mysql cancel idempotency fixture",
		Price:       30,
		Stock:       5,
		MerchantID:  1,
	}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("failed to create integration product: %v", err)
	}

	service := order.NewService(db, nil)
	createResp, err := service.CreateOrder(ctx, &pbOrder.CreateOrderRequest{
		UserId:         7,
		IdempotencyKey: "integration-create-before-cancel-" + fmt.Sprintf("%d", item.ID),
		Items: []*pbOrder.CreateOrderItem{
			{ProductId: int64(item.ID), Quantity: 2},
		},
	})
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}

	req := &pbOrder.CancelOrderRequest{
		Id:             createResp.Order.Id,
		UserId:         7,
		IdempotencyKey: "integration-cancel-" + fmt.Sprintf("%d", createResp.Order.Id),
	}
	first, err := service.CancelOrder(ctx, req)
	if err != nil {
		t.Fatalf("first CancelOrder returned error: %v", err)
	}
	second, err := service.CancelOrder(ctx, req)
	if err != nil {
		t.Fatalf("second CancelOrder returned error: %v", err)
	}

	var latest product.Product
	if err := db.First(&latest, item.ID).Error; err != nil {
		t.Fatalf("failed to reload product: %v", err)
	}
	if got, want := latest.Stock, int32(5); got != want {
		t.Fatalf("unexpected stock after repeated cancel: got %d want %d", got, want)
	}

	firstBytes, err := proto.Marshal(first)
	if err != nil {
		t.Fatalf("failed to marshal first response: %v", err)
	}
	secondBytes, err := proto.Marshal(second)
	if err != nil {
		t.Fatalf("failed to marshal second response: %v", err)
	}
	if !bytes.Equal(firstBytes, secondBytes) {
		t.Fatalf("expected serialized cancel responses to match: first=%q second=%q", firstBytes, secondBytes)
	}

	var eventCount int64
	if err := db.Model(&outbox.Event{}).
		Where("aggregate_type = ? AND aggregate_id = ? AND event_type = ?", "order", fmt.Sprintf("%d", createResp.Order.Id), events.OrderCancelledType).
		Count(&eventCount).Error; err != nil {
		t.Fatalf("failed to count cancelled outbox events: %v", err)
	}
	if got, want := eventCount, int64(1); got != want {
		t.Fatalf("unexpected cancelled outbox event count: got %d want %d", got, want)
	}
}

func TestMySQLConcurrentShipOrderSerializesTransition(t *testing.T) {
	ctx := context.Background()
	db := openIntegrationDB(t)
	if err := db.AutoMigrate(&auth.User{}, &order.Order{}, &order.OrderItem{}, &idempotency.Record{}, &outbox.Event{}); err != nil {
		t.Fatalf("failed to migrate integration schema: %v", err)
	}
	ensureIntegrationOrderIndexes(t, db)

	admin := createIntegrationUser(t, db, auth.RoleAdmin)
	placed := order.Order{
		UserID:      7,
		TotalAmount: 10,
		Status:      order.OrderStatusPaid,
		OrderDate:   time.Now(),
	}
	if err := db.Create(&placed).Error; err != nil {
		t.Fatalf("failed to create order: %v", err)
	}

	service := order.NewService(db, nil)
	var successes int32
	unexpected := runConcurrentOrderActions(20, func() error {
		resp, err := service.ShipOrder(ctx, &pbOrder.ShipOrderRequest{
			Id:          int64(placed.ID),
			ActorUserId: int64(admin.ID),
		})
		if err == nil {
			if resp != nil && resp.Success {
				atomic.AddInt32(&successes, 1)
				return nil
			}
			return fmt.Errorf("unexpected ship response: %+v", resp)
		}
		if status.Code(err) == codes.FailedPrecondition {
			return nil
		}
		return err
	})
	for _, err := range unexpected {
		t.Errorf("unexpected ShipOrder error: %v", err)
	}
	if got, want := atomic.LoadInt32(&successes), int32(1); got != want {
		t.Fatalf("unexpected successful ship count: got %d want %d", got, want)
	}
	if got, want := countIntegrationOrderOutboxEvents(t, db, placed.ID, events.OrderShippedType), int64(1); got != want {
		t.Fatalf("unexpected shipped outbox event count: got %d want %d", got, want)
	}
}

func TestMySQLConcurrentCompleteOrderSerializesTransition(t *testing.T) {
	ctx := context.Background()
	db := openIntegrationDB(t)
	if err := db.AutoMigrate(&order.Order{}, &order.OrderItem{}, &idempotency.Record{}, &outbox.Event{}); err != nil {
		t.Fatalf("failed to migrate integration schema: %v", err)
	}
	ensureIntegrationOrderIndexes(t, db)

	placed := order.Order{
		UserID:      7,
		TotalAmount: 10,
		Status:      order.OrderStatusShipped,
		OrderDate:   time.Now(),
	}
	if err := db.Create(&placed).Error; err != nil {
		t.Fatalf("failed to create order: %v", err)
	}

	service := order.NewService(db, nil)
	var successes int32
	unexpected := runConcurrentOrderActions(20, func() error {
		resp, err := service.CompleteOrder(ctx, &pbOrder.CompleteOrderRequest{
			Id:     int64(placed.ID),
			UserId: 7,
		})
		if err == nil {
			if resp != nil && resp.Success {
				atomic.AddInt32(&successes, 1)
				return nil
			}
			return fmt.Errorf("unexpected complete response: %+v", resp)
		}
		if status.Code(err) == codes.FailedPrecondition {
			return nil
		}
		return err
	})
	for _, err := range unexpected {
		t.Errorf("unexpected CompleteOrder error: %v", err)
	}
	if got, want := atomic.LoadInt32(&successes), int32(1); got != want {
		t.Fatalf("unexpected successful completion count: got %d want %d", got, want)
	}
	if got, want := countIntegrationOrderOutboxEvents(t, db, placed.ID, events.OrderCompletedType), int64(1); got != want {
		t.Fatalf("unexpected completed outbox event count: got %d want %d", got, want)
	}
}

func TestMySQLConcurrentShipAndCancelKeepsInventoryConsistent(t *testing.T) {
	ctx := context.Background()
	db := openIntegrationDB(t)
	if err := db.AutoMigrate(&auth.User{}, &product.Product{}, &order.Order{}, &order.OrderItem{}, &idempotency.Record{}, &outbox.Event{}); err != nil {
		t.Fatalf("failed to migrate integration schema: %v", err)
	}
	ensureIntegrationOrderIndexes(t, db)

	item := product.Product{
		Name:        "integration-ship-cancel-" + uniqueSuffix(t),
		Description: "mysql ship cancel concurrency fixture",
		Price:       50,
		Stock:       5,
		MerchantID:  1,
	}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("failed to create product: %v", err)
	}

	admin := createIntegrationUser(t, db, auth.RoleAdmin)
	service := order.NewService(db, nil)
	createResp, err := service.CreateOrder(ctx, &pbOrder.CreateOrderRequest{
		UserId:         7,
		IdempotencyKey: "integration-create-before-ship-cancel-" + fmt.Sprintf("%d", item.ID),
		Items: []*pbOrder.CreateOrderItem{
			{ProductId: int64(item.ID), Quantity: 2},
		},
	})
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}
	if err := db.Model(&order.Order{}).
		Where("id = ?", createResp.Order.Id).
		Update("status", order.OrderStatusPaid).Error; err != nil {
		t.Fatalf("failed to mark order paid: %v", err)
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	var shipErr, cancelErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, shipErr = service.ShipOrder(ctx, &pbOrder.ShipOrderRequest{
			Id:          createResp.Order.Id,
			ActorUserId: int64(admin.ID),
		})
	}()
	go func() {
		defer wg.Done()
		<-start
		resp, err := service.CancelOrder(ctx, &pbOrder.CancelOrderRequest{
			Id:             createResp.Order.Id,
			UserId:         7,
			IdempotencyKey: "integration-cancel-racing-ship-" + fmt.Sprintf("%d", createResp.Order.Id),
		})
		if err != nil {
			cancelErr = err
			return
		}
		if resp == nil {
			cancelErr = fmt.Errorf("nil cancel response")
		}
	}()
	close(start)
	wg.Wait()

	if shipErr != nil && status.Code(shipErr) != codes.FailedPrecondition {
		t.Fatalf("unexpected ShipOrder error: %v", shipErr)
	}
	if cancelErr != nil {
		t.Fatalf("unexpected CancelOrder error: %v", cancelErr)
	}

	var latestOrder order.Order
	if err := db.First(&latestOrder, createResp.Order.Id).Error; err != nil {
		t.Fatalf("failed to reload order: %v", err)
	}
	if got, want := latestOrder.Status, order.OrderStatusShipped; got != want {
		t.Fatalf("unexpected final order status: got %q want %q", got, want)
	}

	var latestProduct product.Product
	if err := db.First(&latestProduct, item.ID).Error; err != nil {
		t.Fatalf("failed to reload product: %v", err)
	}
	if got, want := latestProduct.Stock, int32(3); got != want {
		t.Fatalf("unexpected stock after ship/cancel race: got %d want %d", got, want)
	}
	if got, want := countIntegrationOrderOutboxEvents(t, db, uint(createResp.Order.Id), events.OrderShippedType), int64(1); got != want {
		t.Fatalf("unexpected shipped outbox event count: got %d want %d", got, want)
	}
	if got, want := countIntegrationOrderOutboxEvents(t, db, uint(createResp.Order.Id), events.OrderCancelledType), int64(0); got != want {
		t.Fatalf("unexpected cancelled outbox event count: got %d want %d", got, want)
	}
}

func createIntegrationUser(t *testing.T, db *gorm.DB, role string) auth.User {
	t.Helper()

	suffix := uniqueSuffix(t)
	user := auth.User{
		Username: fmt.Sprintf("%s-%s", role, suffix),
		Password: "password",
		Email:    fmt.Sprintf("%s-%s@example.com", role, suffix),
		Role:     role,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	return user
}

func countIntegrationOrderOutboxEvents(t *testing.T, db *gorm.DB, orderID uint, eventType string) int64 {
	t.Helper()

	var count int64
	if err := db.Model(&outbox.Event{}).
		Where("aggregate_type = ? AND aggregate_id = ? AND event_type = ?", "order", fmt.Sprintf("%d", orderID), eventType).
		Count(&count).Error; err != nil {
		t.Fatalf("failed to count order outbox events: %v", err)
	}
	return count
}

func runConcurrentOrderActions(workers int, fn func() error) []error {
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, workers)

	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			<-start
			if err := fn(); err != nil {
				errs <- err
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	unexpected := make([]error, 0, len(errs))
	for err := range errs {
		unexpected = append(unexpected, err)
	}
	return unexpected
}
