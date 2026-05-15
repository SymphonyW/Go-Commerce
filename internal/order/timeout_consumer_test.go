package order

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/streadway/amqp"

	pb "go-commerce/api/order"
	"go-commerce/internal/product"
	"go-commerce/pkg/events"
)

func TestOrderTimeoutConsumerCancelsPendingOrderRestoresStockAndPublishesEvents(t *testing.T) {
	publisher := &recordingPublisher{}
	service, db := newTestServiceWithPublisher(t, publisher)
	item := createTestProduct(t, db, "超时取消商品", 10, 5)

	resp, err := service.CreateOrder(t.Context(), &pb.CreateOrderRequest{
		UserId: 1,
		Items: []*pb.CreateOrderItem{
			{ProductId: int64(item.ID), Quantity: 2},
		},
	})
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}
	publisher.events = nil

	ack := &fakeAcknowledger{}
	consumer := NewOrderTimeoutConsumer(db, publisher, nil)
	if err := consumer.HandleDelivery(timeoutDelivery(t, ack, resp.Order.Id, 1)); err != nil {
		t.Fatalf("HandleDelivery returned error: %v", err)
	}

	var latestOrder Order
	if err := db.First(&latestOrder, resp.Order.Id).Error; err != nil {
		t.Fatalf("failed to reload order: %v", err)
	}
	if got, want := latestOrder.Status, OrderStatusCancelled; got != want {
		t.Fatalf("unexpected order status: got %q want %q", got, want)
	}
	if got, want := latestOrder.CancelReason, OrderCancelReasonPaymentTimeout; got != want {
		t.Fatalf("unexpected cancel reason: got %q want %q", got, want)
	}

	var latestProduct product.Product
	if err := db.First(&latestProduct, item.ID).Error; err != nil {
		t.Fatalf("failed to reload product: %v", err)
	}
	if got, want := latestProduct.Stock, int32(5); got != want {
		t.Fatalf("unexpected restored stock: got %d want %d", got, want)
	}
	if !ack.acked {
		t.Fatal("expected timeout event to be acked")
	}
	if got, want := len(publisher.events), 2; got != want {
		t.Fatalf("unexpected published event count: got %d want %d", got, want)
	}
	if got, want := publisher.events[0].routingKey, events.OrderCancelledType; got != want {
		t.Fatalf("unexpected first routing key: got %q want %q", got, want)
	}
	if got, want := publisher.events[1].routingKey, events.OrderTimeoutCancelledType; got != want {
		t.Fatalf("unexpected second routing key: got %q want %q", got, want)
	}
}

func TestOrderTimeoutConsumerSkipsPaidOrder(t *testing.T) {
	publisher := &recordingPublisher{}
	service, db := newTestServiceWithPublisher(t, publisher)
	item := createTestProduct(t, db, "已支付商品", 10, 5)

	resp, err := service.CreateOrder(t.Context(), &pb.CreateOrderRequest{
		UserId: 1,
		Items: []*pb.CreateOrderItem{
			{ProductId: int64(item.ID), Quantity: 1},
		},
	})
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}
	if _, _, err := MarkOrderPaid(db, resp.Order.Id, 1, 10); err != nil {
		t.Fatalf("MarkOrderPaid returned error: %v", err)
	}
	publisher.events = nil

	ack := &fakeAcknowledger{}
	consumer := NewOrderTimeoutConsumer(db, publisher, nil)
	if err := consumer.HandleDelivery(timeoutDelivery(t, ack, resp.Order.Id, 1)); err != nil {
		t.Fatalf("HandleDelivery returned error: %v", err)
	}

	var latestOrder Order
	if err := db.First(&latestOrder, resp.Order.Id).Error; err != nil {
		t.Fatalf("failed to reload order: %v", err)
	}
	if got, want := latestOrder.Status, OrderStatusPaid; got != want {
		t.Fatalf("unexpected order status: got %q want %q", got, want)
	}

	var latestProduct product.Product
	if err := db.First(&latestProduct, item.ID).Error; err != nil {
		t.Fatalf("failed to reload product: %v", err)
	}
	if got, want := latestProduct.Stock, int32(4); got != want {
		t.Fatalf("unexpected stock after paid timeout check: got %d want %d", got, want)
	}
	if !ack.acked {
		t.Fatal("expected timeout event to be acked")
	}
	if got := len(publisher.events); got != 0 {
		t.Fatalf("unexpected published event count: got %d want 0", got)
	}
}

func TestOrderTimeoutConsumerIsIdempotentForRepeatedMessages(t *testing.T) {
	publisher := &recordingPublisher{}
	service, db := newTestServiceWithPublisher(t, publisher)
	item := createTestProduct(t, db, "重复超时商品", 10, 5)

	resp, err := service.CreateOrder(t.Context(), &pb.CreateOrderRequest{
		UserId: 1,
		Items: []*pb.CreateOrderItem{
			{ProductId: int64(item.ID), Quantity: 2},
		},
	})
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}
	publisher.events = nil

	consumer := NewOrderTimeoutConsumer(db, publisher, nil)
	if err := consumer.HandleDelivery(timeoutDelivery(t, &fakeAcknowledger{}, resp.Order.Id, 1)); err != nil {
		t.Fatalf("first HandleDelivery returned error: %v", err)
	}
	if err := consumer.HandleDelivery(timeoutDelivery(t, &fakeAcknowledger{}, resp.Order.Id, 1)); err != nil {
		t.Fatalf("second HandleDelivery returned error: %v", err)
	}

	var latestProduct product.Product
	if err := db.First(&latestProduct, item.ID).Error; err != nil {
		t.Fatalf("failed to reload product: %v", err)
	}
	if got, want := latestProduct.Stock, int32(5); got != want {
		t.Fatalf("unexpected restored stock after duplicate timeout: got %d want %d", got, want)
	}
	if got, want := len(publisher.events), 2; got != want {
		t.Fatalf("unexpected published event count: got %d want %d", got, want)
	}
}

func TestOrderTimeoutConsumerSkipsAlreadyCancelledOrder(t *testing.T) {
	publisher := &recordingPublisher{}
	service, db := newTestServiceWithPublisher(t, publisher)
	item := createTestProduct(t, db, "已取消商品", 10, 5)

	resp, err := service.CreateOrder(t.Context(), &pb.CreateOrderRequest{
		UserId: 1,
		Items: []*pb.CreateOrderItem{
			{ProductId: int64(item.ID), Quantity: 1},
		},
	})
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}
	if cancelResp, err := service.CancelOrder(t.Context(), &pb.CancelOrderRequest{
		Id:     resp.Order.Id,
		UserId: 1,
	}); err != nil {
		t.Fatalf("CancelOrder returned error: %v", err)
	} else if !cancelResp.Success {
		t.Fatalf("expected cancellation to succeed, got message %q", cancelResp.Message)
	}
	publisher.events = nil

	consumer := NewOrderTimeoutConsumer(db, publisher, nil)
	if err := consumer.HandleDelivery(timeoutDelivery(t, &fakeAcknowledger{}, resp.Order.Id, 1)); err != nil {
		t.Fatalf("HandleDelivery returned error: %v", err)
	}

	var latestProduct product.Product
	if err := db.First(&latestProduct, item.ID).Error; err != nil {
		t.Fatalf("failed to reload product: %v", err)
	}
	if got, want := latestProduct.Stock, int32(5); got != want {
		t.Fatalf("unexpected stock after already-cancelled timeout: got %d want %d", got, want)
	}
	if got := len(publisher.events); got != 0 {
		t.Fatalf("unexpected published event count: got %d want 0", got)
	}
}

func timeoutDelivery(t *testing.T, ack *fakeAcknowledger, orderID, userID int64) amqp.Delivery {
	t.Helper()

	body, err := json.Marshal(events.OrderTimeoutCheckEvent{
		BaseEvent:      events.NewBaseEvent(events.OrderTimeoutCheckType, time.Now()),
		OrderID:        orderID,
		UserID:         userID,
		CreatedAt:      time.Now().UTC().Format(time.RFC3339Nano),
		ExpireAt:       time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano),
		TimeoutMinutes: 1,
	})
	if err != nil {
		t.Fatalf("failed to marshal timeout event: %v", err)
	}

	return amqp.Delivery{
		Acknowledger: ack,
		DeliveryTag:  1,
		Body:         body,
	}
}
