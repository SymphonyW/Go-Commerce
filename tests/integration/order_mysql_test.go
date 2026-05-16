//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"testing"

	pbOrder "go-commerce/api/order"
	"go-commerce/internal/idempotency"
	"go-commerce/internal/order"
	"go-commerce/internal/outbox"
	"go-commerce/internal/product"
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
