package order

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/streadway/amqp"
	"gorm.io/gorm"

	pb "go-commerce/api/order"
	"go-commerce/internal/inbox"
	"go-commerce/internal/outbox"
	"go-commerce/internal/product"
	"go-commerce/pkg/events"
)

func TestOrderTimeoutConsumerCancelsPendingOrderRestoresStockAndPublishesEvents(t *testing.T) {
	publisher := &recordingPublisher{}
	service, db := newTestServiceWithPublisher(t, publisher)
	item := createTestProduct(t, db, "超时取消商品", 10, 5)

	resp, err := service.CreateOrder(t.Context(), &pb.CreateOrderRequest{
		UserId:         1,
		IdempotencyKey: "test-key",
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
	if got := len(publisher.events); got != 0 {
		t.Fatalf("unexpected direct publish count: got %d want 0", got)
	}
	var saved []outbox.Event
	if err := db.Where("event_type IN ?", []string{events.OrderCancelledType, events.OrderTimeoutCancelledType}).
		Order("id ASC").
		Find(&saved).Error; err != nil {
		t.Fatalf("failed to load timeout outbox events: %v", err)
	}
	if got, want := len(saved), 2; got != want {
		t.Fatalf("unexpected outbox event count: got %d want %d", got, want)
	}
	if got, want := saved[0].EventType, events.OrderCancelledType; got != want {
		t.Fatalf("unexpected first outbox event type: got %q want %q", got, want)
	}
	if got, want := saved[1].EventType, events.OrderTimeoutCancelledType; got != want {
		t.Fatalf("unexpected second outbox event type: got %q want %q", got, want)
	}
}

func TestOrderTimeoutConsumerSkipsPaidOrder(t *testing.T) {
	publisher := &recordingPublisher{}
	service, db := newTestServiceWithPublisher(t, publisher)
	item := createTestProduct(t, db, "已支付商品", 10, 5)

	resp, err := service.CreateOrder(t.Context(), &pb.CreateOrderRequest{
		UserId:         1,
		IdempotencyKey: "test-key",
		Items: []*pb.CreateOrderItem{
			{ProductId: int64(item.ID), Quantity: 1},
		},
	})
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}
	if _, _, err := MarkOrderPaid(db, resp.Order.Id, 1, 10, nil); err != nil {
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
		UserId:         1,
		IdempotencyKey: "test-key",
		Items: []*pb.CreateOrderItem{
			{ProductId: int64(item.ID), Quantity: 2},
		},
	})
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}
	publisher.events = nil

	consumer := NewOrderTimeoutConsumer(db, publisher, nil)
	if err := consumer.HandleDelivery(timeoutDeliveryWithEventID(t, &fakeAcknowledger{}, "evt-timeout-duplicate-1", resp.Order.Id, 1)); err != nil {
		t.Fatalf("first HandleDelivery returned error: %v", err)
	}
	secondAck := &fakeAcknowledger{}
	if err := consumer.HandleDelivery(timeoutDeliveryWithEventID(t, secondAck, "evt-timeout-duplicate-1", resp.Order.Id, 1)); err != nil {
		t.Fatalf("second HandleDelivery returned error: %v", err)
	}
	if !secondAck.acked {
		t.Fatal("expected duplicate timeout event to be acked")
	}
	if secondAck.nacked {
		t.Fatal("did not expect duplicate timeout event to be nacked")
	}

	var latestProduct product.Product
	if err := db.First(&latestProduct, item.ID).Error; err != nil {
		t.Fatalf("failed to reload product: %v", err)
	}
	if got, want := latestProduct.Stock, int32(5); got != want {
		t.Fatalf("unexpected restored stock after duplicate timeout: got %d want %d", got, want)
	}
	var saved []outbox.Event
	if err := db.Where("event_type IN ?", []string{events.OrderCancelledType, events.OrderTimeoutCancelledType}).Find(&saved).Error; err != nil {
		t.Fatalf("failed to load outbox events: %v", err)
	}
	if got, want := len(saved), 2; got != want {
		t.Fatalf("unexpected outbox event count after duplicate timeout: got %d want %d", got, want)
	}
	assertConsumedTimeoutEventCount(t, db, "evt-timeout-duplicate-1", 1)
}

func TestOrderTimeoutConsumerNacksMalformedMessageWithoutRequeue(t *testing.T) {
	consumer := NewOrderTimeoutConsumer(nil, nil, nil)
	ack := &fakeAcknowledger{}

	err := consumer.HandleDelivery(amqp.Delivery{Acknowledger: ack, DeliveryTag: 1, Body: []byte("{bad json")})
	if err == nil {
		t.Fatal("expected malformed message error")
	}
	if !ack.nacked {
		t.Fatal("expected malformed message to be nacked")
	}
	if ack.requeue {
		t.Fatal("expected malformed message not to be requeued")
	}
}

func TestOrderTimeoutConsumerNacksMissingEventIDWithoutRequeue(t *testing.T) {
	consumer := NewOrderTimeoutConsumer(nil, nil, nil)
	ack := &fakeAcknowledger{}
	body, err := json.Marshal(events.OrderTimeoutCheckEvent{
		BaseEvent: events.BaseEvent{EventType: events.OrderTimeoutCheckType},
		OrderID:   1,
		UserID:    1,
	})
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}

	err = consumer.HandleDelivery(amqp.Delivery{Acknowledger: ack, DeliveryTag: 1, Body: body})
	if err == nil {
		t.Fatal("expected missing event id error")
	}
	if !ack.nacked {
		t.Fatal("expected missing event id to be nacked")
	}
	if ack.requeue {
		t.Fatal("expected missing event id not to be requeued")
	}
}

func TestOrderTimeoutConsumerNacksTemporaryDatabaseErrorWithRequeue(t *testing.T) {
	consumer := NewOrderTimeoutConsumer(nil, nil, nil)
	ack := &fakeAcknowledger{}
	body, err := json.Marshal(events.OrderTimeoutCheckEvent{
		BaseEvent: events.BaseEvent{
			EventID:    "evt-timeout-db-error-1",
			EventType:  events.OrderTimeoutCheckType,
			OccurredAt: time.Now().UTC().Format(time.RFC3339Nano),
		},
		OrderID: 1,
		UserID:  1,
	})
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}

	err = consumer.HandleDelivery(amqp.Delivery{Acknowledger: ack, DeliveryTag: 1, Body: body})
	if err == nil {
		t.Fatal("expected database error")
	}
	if !ack.nacked {
		t.Fatal("expected database error to be nacked")
	}
	if !ack.requeue {
		t.Fatal("expected temporary database error to be requeued")
	}
}

func TestOrderTimeoutConsumerSkipsAlreadyCancelledOrder(t *testing.T) {
	publisher := &recordingPublisher{}
	service, db := newTestServiceWithPublisher(t, publisher)
	item := createTestProduct(t, db, "已取消商品", 10, 5)

	resp, err := service.CreateOrder(t.Context(), &pb.CreateOrderRequest{
		UserId:         1,
		IdempotencyKey: "test-key",
		Items: []*pb.CreateOrderItem{
			{ProductId: int64(item.ID), Quantity: 1},
		},
	})
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}
	if cancelResp, err := service.CancelOrder(t.Context(), &pb.CancelOrderRequest{
		Id:             resp.Order.Id,
		UserId:         1,
		IdempotencyKey: "cancel-before-timeout-key",
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
	return timeoutDeliveryWithEventID(t, ack, events.NewBaseEvent(events.OrderTimeoutCheckType, time.Now()).EventID, orderID, userID)
}

func timeoutDeliveryWithEventID(t *testing.T, ack *fakeAcknowledger, eventID string, orderID, userID int64) amqp.Delivery {
	t.Helper()

	body, err := json.Marshal(events.OrderTimeoutCheckEvent{
		BaseEvent:      events.BaseEvent{EventID: eventID, EventType: events.OrderTimeoutCheckType, OccurredAt: time.Now().UTC().Format(time.RFC3339Nano)},
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

func assertConsumedTimeoutEventCount(t *testing.T, db *gorm.DB, eventID string, want int64) {
	t.Helper()

	var count int64
	if err := db.Model(&inbox.ConsumedEvent{}).
		Where("consumer_name = ? AND event_id = ?", orderTimeoutConsumerName, eventID).
		Count(&count).Error; err != nil {
		t.Fatalf("failed to count consumed timeout events: %v", err)
	}
	if got := count; got != want {
		t.Fatalf("unexpected consumed timeout event count: got %d want %d", got, want)
	}
}
