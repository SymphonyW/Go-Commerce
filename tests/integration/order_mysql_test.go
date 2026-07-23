//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	pbOrder "go-commerce/api/order"
	"go-commerce/internal/idempotency"
	"go-commerce/internal/order"
	"go-commerce/internal/outbox"
	"go-commerce/internal/product"
	"go-commerce/pkg/events"

	"google.golang.org/protobuf/proto"
)

func TestMySQLOrderPersistsAndDeductsStock(t *testing.T) {
	ctx := context.Background()
	db := openIntegrationDB(t)
	if err := db.AutoMigrate(&product.Product{}, &order.Order{}, &order.OrderItem{}, &idempotency.Record{}, &outbox.Event{}); err != nil {
		t.Fatalf("failed to migrate integration schema: %v", err)
	}

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
