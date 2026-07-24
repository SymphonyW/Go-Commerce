//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"testing"

	pbOrder "go-commerce/api/order"
	pbPayment "go-commerce/api/payment"
	"go-commerce/internal/idempotency"
	orderdomain "go-commerce/internal/order"
	"go-commerce/internal/outbox"
	"go-commerce/internal/payment"
	"go-commerce/internal/product"
	"go-commerce/pkg/events"

	"github.com/streadway/amqp"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

func TestMySQLPaymentAutoMigrateCanRunRepeatedly(t *testing.T) {
	db := openIntegrationDB(t)

	if err := db.AutoMigrate(&payment.Payment{}); err != nil {
		t.Fatalf("first payment migration failed: %v", err)
	}
	if err := db.AutoMigrate(&payment.Payment{}); err != nil {
		t.Fatalf("second payment migration failed: %v", err)
	}
}

type integrationOrderClient struct {
	service *orderdomain.Service
}

func (c integrationOrderClient) CreateOrder(ctx context.Context, req *pbOrder.CreateOrderRequest, opts ...grpc.CallOption) (*pbOrder.CreateOrderResponse, error) {
	return c.service.CreateOrder(ctx, req)
}

func (c integrationOrderClient) GetOrder(ctx context.Context, req *pbOrder.GetOrderRequest, opts ...grpc.CallOption) (*pbOrder.GetOrderResponse, error) {
	return c.service.GetOrder(ctx, req)
}

func (c integrationOrderClient) ListOrders(context.Context, *pbOrder.ListOrdersRequest, ...grpc.CallOption) (*pbOrder.ListOrdersResponse, error) {
	return nil, nil
}

func (c integrationOrderClient) ListMerchantOrders(context.Context, *pbOrder.ListMerchantOrdersRequest, ...grpc.CallOption) (*pbOrder.ListMerchantOrdersResponse, error) {
	return nil, nil
}

func (c integrationOrderClient) CancelOrder(context.Context, *pbOrder.CancelOrderRequest, ...grpc.CallOption) (*pbOrder.CancelOrderResponse, error) {
	return nil, nil
}

func (c integrationOrderClient) ShipOrder(context.Context, *pbOrder.ShipOrderRequest, ...grpc.CallOption) (*pbOrder.ShipOrderResponse, error) {
	return nil, nil
}

func (c integrationOrderClient) CompleteOrder(context.Context, *pbOrder.CompleteOrderRequest, ...grpc.CallOption) (*pbOrder.CompleteOrderResponse, error) {
	return nil, nil
}

type integrationAcknowledger struct {
	acked int
}

func (a *integrationAcknowledger) Ack(uint64, bool) error {
	a.acked++
	return nil
}

func (a *integrationAcknowledger) Nack(uint64, bool, bool) error {
	return nil
}

func (a *integrationAcknowledger) Reject(uint64, bool) error {
	return nil
}

func TestMySQLPaymentSuccessIdempotencyReplaysWithoutDuplicateSideEffects(t *testing.T) {
	ctx := context.Background()
	db := openIntegrationDB(t)
	if err := db.AutoMigrate(&product.Product{}, &orderdomain.Order{}, &orderdomain.OrderItem{}, &payment.Payment{}, &idempotency.Record{}, &outbox.Event{}); err != nil {
		t.Fatalf("failed to migrate integration schema: %v", err)
	}
	if err := db.Exec(`
		CREATE TABLE payment_status_update_audits (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			payment_id BIGINT NOT NULL,
			old_status VARCHAR(32) NOT NULL,
			new_status VARCHAR(32) NOT NULL
		)
	`).Error; err != nil {
		t.Fatalf("failed to create audit table: %v", err)
	}
	if err := db.Exec(`
		CREATE TRIGGER audit_payment_status_update
		AFTER UPDATE ON payments
		FOR EACH ROW
		BEGIN
			IF OLD.status <> NEW.status THEN
				INSERT INTO payment_status_update_audits (payment_id, old_status, new_status)
				VALUES (NEW.id, OLD.status, NEW.status);
			END IF;
		END
	`).Error; err != nil {
		t.Fatalf("failed to create audit trigger: %v", err)
	}

	item := product.Product{
		Name:        "integration-payment-" + uniqueSuffix(t),
		Description: "mysql payment idempotency fixture",
		Price:       40,
		Stock:       5,
		MerchantID:  1,
	}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("failed to create integration product: %v", err)
	}

	orderService := orderdomain.NewService(db, nil)
	orderResp, err := orderService.CreateOrder(ctx, &pbOrder.CreateOrderRequest{
		UserId:         7,
		IdempotencyKey: "integration-create-before-payment-" + fmt.Sprintf("%d", item.ID),
		Items: []*pbOrder.CreateOrderItem{
			{ProductId: int64(item.ID), Quantity: 2},
		},
	})
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}

	paymentService := payment.NewService(db, integrationOrderClient{service: orderService}, nil)
	grpcService := payment.NewGRPCService(paymentService)
	createdPayment, err := paymentService.CreatePayment(ctx, 7, orderResp.Order.Id, payment.PaymentMethodMockBalance)
	if err != nil {
		t.Fatalf("CreatePayment returned error: %v", err)
	}

	req := &pbPayment.PaymentActionRequest{
		Id:             int64(createdPayment.ID),
		UserId:         7,
		IdempotencyKey: "integration-payment-success-" + fmt.Sprintf("%d", createdPayment.ID),
	}
	first, err := grpcService.MarkPaymentSucceeded(ctx, req)
	if err != nil {
		t.Fatalf("first MarkPaymentSucceeded returned error: %v", err)
	}
	second, err := grpcService.MarkPaymentSucceeded(ctx, req)
	if err != nil {
		t.Fatalf("second MarkPaymentSucceeded returned error: %v", err)
	}
	if !proto.Equal(first, second) {
		t.Fatalf("expected payment success responses to match: first=%+v second=%+v", first, second)
	}

	var updateCount int64
	if err := db.Table("payment_status_update_audits").Where("payment_id = ?", createdPayment.ID).Count(&updateCount).Error; err != nil {
		t.Fatalf("failed to count payment status updates: %v", err)
	}
	if got, want := updateCount, int64(1); got != want {
		t.Fatalf("unexpected payment status update count: got %d want %d", got, want)
	}

	var paymentEventCount int64
	if err := db.Model(&outbox.Event{}).
		Where("aggregate_type = ? AND aggregate_id = ? AND event_type = ?", "payment", fmt.Sprintf("%d", createdPayment.ID), events.PaymentSucceededType).
		Count(&paymentEventCount).Error; err != nil {
		t.Fatalf("failed to count payment succeeded outbox events: %v", err)
	}
	if got, want := paymentEventCount, int64(1); got != want {
		t.Fatalf("unexpected payment succeeded outbox count: got %d want %d", got, want)
	}

	var paymentEvent outbox.Event
	if err := db.Where("aggregate_type = ? AND aggregate_id = ? AND event_type = ?", "payment", fmt.Sprintf("%d", createdPayment.ID), events.PaymentSucceededType).First(&paymentEvent).Error; err != nil {
		t.Fatalf("failed to load payment succeeded event: %v", err)
	}
	ack := &integrationAcknowledger{}
	consumer := orderdomain.NewPaymentSucceededConsumer(db, nil, nil)
	for i := 0; i < 2; i++ {
		if err := consumer.HandleDelivery(amqp.Delivery{
			Acknowledger: ack,
			DeliveryTag:  uint64(i + 1),
			Body:         []byte(paymentEvent.Payload),
		}); err != nil {
			t.Fatalf("HandleDelivery returned error on attempt %d: %v", i+1, err)
		}
	}
	if got, want := ack.acked, 2; got != want {
		t.Fatalf("unexpected ack count: got %d want %d", got, want)
	}

	var latestOrder orderdomain.Order
	if err := db.First(&latestOrder, orderResp.Order.Id).Error; err != nil {
		t.Fatalf("failed to reload order: %v", err)
	}
	if got, want := latestOrder.Status, orderdomain.OrderStatusPaid; got != want {
		t.Fatalf("unexpected order status after consuming payment event: got %q want %q", got, want)
	}
	var orderPaidEventCount int64
	if err := db.Model(&outbox.Event{}).
		Where("aggregate_type = ? AND aggregate_id = ? AND event_type = ?", "order", fmt.Sprintf("%d", orderResp.Order.Id), events.OrderPaidType).
		Count(&orderPaidEventCount).Error; err != nil {
		t.Fatalf("failed to count order paid events: %v", err)
	}
	if got, want := orderPaidEventCount, int64(1); got != want {
		t.Fatalf("unexpected order paid outbox count after duplicate consumption: got %d want %d", got, want)
	}
}
