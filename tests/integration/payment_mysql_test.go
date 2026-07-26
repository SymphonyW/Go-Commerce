//go:build integration

package integration_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

func TestMySQLPaymentAutoMigrateCanRunRepeatedly(t *testing.T) {
	db := openIntegrationDB(t)

	if err := payment.Migrate(db); err != nil {
		t.Fatalf("first payment migration failed: %v", err)
	}
	if err := payment.Migrate(db); err != nil {
		t.Fatalf("second payment migration failed: %v", err)
	}
	if !db.Migrator().HasIndex(&payment.Payment{}, "idx_payments_active_order") {
		t.Fatal("expected active order unique index to exist")
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
	if err := db.AutoMigrate(&product.Product{}, &orderdomain.Order{}, &orderdomain.OrderItem{}, &idempotency.Record{}, &outbox.Event{}); err != nil {
		t.Fatalf("failed to migrate integration schema: %v", err)
	}
	if err := payment.Migrate(db); err != nil {
		t.Fatalf("failed to migrate payment schema: %v", err)
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

func TestMySQLConcurrentCreatePaymentAllowsOnlyOneActivePayment(t *testing.T) {
	ctx := context.Background()
	db := openIntegrationDB(t)
	if err := db.AutoMigrate(&orderdomain.Order{}, &outbox.Event{}); err != nil {
		t.Fatalf("failed to migrate integration schema: %v", err)
	}
	if err := payment.Migrate(db); err != nil {
		t.Fatalf("failed to migrate payment schema: %v", err)
	}

	placed := createIntegrationPaymentOrder(t, db, 7, orderdomain.OrderStatusPending, 99)
	service := payment.NewService(db, nil, nil)

	var successes int32
	unexpected := runConcurrentIntegrationPaymentActions(50, func() error {
		created, err := service.CreatePayment(ctx, int64(placed.UserID), int64(placed.ID), payment.PaymentMethodMockBalance)
		if err == nil {
			if created == nil {
				return fmt.Errorf("nil payment")
			}
			atomic.AddInt32(&successes, 1)
			return nil
		}
		if errors.Is(err, payment.ErrActivePaymentExists) {
			return nil
		}
		return err
	})
	for _, err := range unexpected {
		t.Errorf("unexpected CreatePayment error: %v", err)
	}
	if got, want := atomic.LoadInt32(&successes), int32(1); got != want {
		t.Fatalf("unexpected successful create count: got %d want %d", got, want)
	}

	var paymentCount int64
	if err := db.Model(&payment.Payment{}).Where("order_id = ?", placed.ID).Count(&paymentCount).Error; err != nil {
		t.Fatalf("failed to count payments: %v", err)
	}
	if got, want := paymentCount, int64(1); got != want {
		t.Fatalf("unexpected payment count: got %d want %d", got, want)
	}

	var activeCount int64
	if err := db.Model(&payment.Payment{}).
		Where("active_order_id = ? AND status IN ?", placed.ID, []string{payment.PaymentStatusCreated, payment.PaymentStatusSucceeded}).
		Count(&activeCount).Error; err != nil {
		t.Fatalf("failed to count active payments: %v", err)
	}
	if got, want := activeCount, int64(1); got != want {
		t.Fatalf("unexpected active payment count: got %d want %d", got, want)
	}
}

func TestMySQLDuplicateActivePaymentMapsToFailedPrecondition(t *testing.T) {
	ctx := context.Background()
	db := openIntegrationDB(t)
	if err := db.AutoMigrate(&orderdomain.Order{}, &outbox.Event{}); err != nil {
		t.Fatalf("failed to migrate integration schema: %v", err)
	}
	if err := payment.Migrate(db); err != nil {
		t.Fatalf("failed to migrate payment schema: %v", err)
	}

	placed := createIntegrationPaymentOrder(t, db, 7, orderdomain.OrderStatusPending, 99)
	grpcService := payment.NewGRPCService(payment.NewService(db, nil, nil))
	req := &pbPayment.CreatePaymentRequest{
		UserId:        int64(placed.UserID),
		OrderId:       int64(placed.ID),
		PaymentMethod: payment.PaymentMethodMockBalance,
	}
	if _, err := grpcService.CreatePayment(ctx, req); err != nil {
		t.Fatalf("first CreatePayment returned error: %v", err)
	}
	_, err := grpcService.CreatePayment(ctx, req)
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("unexpected error code: got %v want %v", status.Code(err), codes.FailedPrecondition)
	}
}

func TestMySQLConcurrentPaymentSuccessAndFailOnlyOneWins(t *testing.T) {
	ctx := context.Background()
	db := openIntegrationDB(t)
	if err := db.AutoMigrate(&orderdomain.Order{}, &outbox.Event{}); err != nil {
		t.Fatalf("failed to migrate integration schema: %v", err)
	}
	if err := payment.Migrate(db); err != nil {
		t.Fatalf("failed to migrate payment schema: %v", err)
	}

	placed := createIntegrationPaymentOrder(t, db, 7, orderdomain.OrderStatusPending, 99)
	service := payment.NewService(db, nil, nil)
	created, err := service.CreatePayment(ctx, int64(placed.UserID), int64(placed.ID), payment.PaymentMethodMockBalance)
	if err != nil {
		t.Fatalf("CreatePayment returned error: %v", err)
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	var successErr, failErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, successErr = service.SucceedPayment(ctx, placed.UserID, created.ID)
	}()
	go func() {
		defer wg.Done()
		<-start
		_, failErr = service.FailPayment(placed.UserID, created.ID)
	}()
	close(start)
	wg.Wait()

	successfulUpdates := 0
	for _, err := range []error{successErr, failErr} {
		if err == nil {
			successfulUpdates++
			continue
		}
		if !errors.Is(err, payment.ErrPaymentNotActionable) {
			t.Fatalf("unexpected concurrent status update error: %v", err)
		}
	}
	if got, want := successfulUpdates, 1; got != want {
		t.Fatalf("unexpected successful status update count: got %d want %d", got, want)
	}

	var latest payment.Payment
	if err := db.First(&latest, created.ID).Error; err != nil {
		t.Fatalf("failed to reload payment: %v", err)
	}
	switch latest.Status {
	case payment.PaymentStatusSucceeded:
		if latest.ActiveOrderID == nil || *latest.ActiveOrderID != placed.ID {
			t.Fatalf("unexpected active order id after success: got %v want %d", latest.ActiveOrderID, placed.ID)
		}
		if got, want := countIntegrationPaymentOutboxEvents(t, db, created.ID, events.PaymentSucceededType), int64(1); got != want {
			t.Fatalf("unexpected payment succeeded outbox count: got %d want %d", got, want)
		}
	case payment.PaymentStatusFailed:
		if latest.ActiveOrderID != nil {
			t.Fatalf("expected failed payment to release active order id, got %v", *latest.ActiveOrderID)
		}
		if got, want := countIntegrationPaymentOutboxEvents(t, db, created.ID, events.PaymentSucceededType), int64(0); got != want {
			t.Fatalf("unexpected payment succeeded outbox count after failure: got %d want %d", got, want)
		}
	default:
		t.Fatalf("unexpected final payment status: %q", latest.Status)
	}
}

func createIntegrationPaymentOrder(t *testing.T, db *gorm.DB, userID uint, status string, amount float64) orderdomain.Order {
	t.Helper()

	placed := orderdomain.Order{
		UserID:      userID,
		TotalAmount: amount,
		Status:      status,
		OrderDate:   time.Now(),
	}
	if err := db.Create(&placed).Error; err != nil {
		t.Fatalf("failed to create order: %v", err)
	}
	return placed
}

func countIntegrationPaymentOutboxEvents(t *testing.T, db *gorm.DB, paymentID uint, eventType string) int64 {
	t.Helper()

	var count int64
	if err := db.Model(&outbox.Event{}).
		Where("aggregate_type = ? AND aggregate_id = ? AND event_type = ?", "payment", fmt.Sprintf("%d", paymentID), eventType).
		Count(&count).Error; err != nil {
		t.Fatalf("failed to count payment outbox events: %v", err)
	}
	return count
}

func runConcurrentIntegrationPaymentActions(workers int, fn func() error) []error {
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
