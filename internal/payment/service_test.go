package payment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	pbOrder "go-commerce/api/order"
	"go-commerce/internal/idempotency"
	orderdomain "go-commerce/internal/order"
	"go-commerce/internal/outbox"
	"go-commerce/pkg/events"
	"go-commerce/pkg/mq"

	"github.com/glebarez/sqlite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

type fakeOrderClient struct {
	orders map[int64]*pbOrder.Order
}

func (f *fakeOrderClient) CreateOrder(context.Context, *pbOrder.CreateOrderRequest, ...grpc.CallOption) (*pbOrder.CreateOrderResponse, error) {
	return nil, nil
}

func (f *fakeOrderClient) GetOrder(ctx context.Context, req *pbOrder.GetOrderRequest, opts ...grpc.CallOption) (*pbOrder.GetOrderResponse, error) {
	order, ok := f.orders[req.Id]
	if !ok || order.UserId != req.UserId {
		return nil, status.Error(codes.NotFound, "order not found")
	}
	return &pbOrder.GetOrderResponse{Order: order}, nil
}

func (f *fakeOrderClient) ListOrders(context.Context, *pbOrder.ListOrdersRequest, ...grpc.CallOption) (*pbOrder.ListOrdersResponse, error) {
	return nil, nil
}

func (f *fakeOrderClient) ListMerchantOrders(context.Context, *pbOrder.ListMerchantOrdersRequest, ...grpc.CallOption) (*pbOrder.ListMerchantOrdersResponse, error) {
	return nil, nil
}

func (f *fakeOrderClient) CancelOrder(context.Context, *pbOrder.CancelOrderRequest, ...grpc.CallOption) (*pbOrder.CancelOrderResponse, error) {
	return nil, nil
}

func (f *fakeOrderClient) ShipOrder(context.Context, *pbOrder.ShipOrderRequest, ...grpc.CallOption) (*pbOrder.ShipOrderResponse, error) {
	return nil, nil
}

func (f *fakeOrderClient) CompleteOrder(context.Context, *pbOrder.CompleteOrderRequest, ...grpc.CallOption) (*pbOrder.CompleteOrderResponse, error) {
	return nil, nil
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
	p.events = append(p.events, publishedEvent{routingKey: routingKey, event: event})
	return p.err
}

func newTestService(t *testing.T, orderClient pbOrder.OrderServiceClient, publisher mq.Publisher) (*Service, *gorm.DB) {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	if err := db.AutoMigrate(&Payment{}, &idempotency.Record{}, &outbox.Event{}); err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}

	return NewService(db, orderClient, publisher), db
}

func TestCreatePaymentRejectsMissingOrder(t *testing.T) {
	service, _ := newTestService(t, &fakeOrderClient{orders: map[int64]*pbOrder.Order{}}, nil)

	_, err := service.CreatePayment(context.Background(), 1, 999, PaymentMethodMockBalance)
	if !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreatePaymentRejectsOtherUsersOrder(t *testing.T) {
	service, _ := newTestService(t, &fakeOrderClient{orders: map[int64]*pbOrder.Order{
		1: {Id: 1, UserId: 2, Status: orderdomain.OrderStatusPending, TotalAmount: 99},
	}}, nil)

	_, err := service.CreatePayment(context.Background(), 1, 1, PaymentMethodMockBalance)
	if !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreatePaymentForPendingOrder(t *testing.T) {
	service, _ := newTestService(t, &fakeOrderClient{orders: map[int64]*pbOrder.Order{
		1: {Id: 1, UserId: 1, Status: orderdomain.OrderStatusPending, TotalAmount: 99},
	}}, nil)

	payment, err := service.CreatePayment(context.Background(), 1, 1, PaymentMethodMockBalance)
	if err != nil {
		t.Fatalf("CreatePayment returned error: %v", err)
	}
	if payment.PaymentNo == "" {
		t.Fatal("expected payment no")
	}
	if got, want := payment.Amount, 99.0; got != want {
		t.Fatalf("unexpected amount: got %.2f want %.2f", got, want)
	}
	if got, want := payment.Status, PaymentStatusCreated; got != want {
		t.Fatalf("unexpected status: got %q want %q", got, want)
	}
}

func TestCreatePaymentRejectsDuplicateActivePayment(t *testing.T) {
	service, _ := newTestService(t, &fakeOrderClient{orders: map[int64]*pbOrder.Order{
		1: {Id: 1, UserId: 1, Status: orderdomain.OrderStatusPending, TotalAmount: 99},
	}}, nil)

	if _, err := service.CreatePayment(context.Background(), 1, 1, PaymentMethodMockBalance); err != nil {
		t.Fatalf("first CreatePayment returned error: %v", err)
	}
	_, err := service.CreatePayment(context.Background(), 1, 1, PaymentMethodMockBalance)
	if !errors.Is(err, ErrActivePaymentExists) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreatePaymentRejectsNonPendingOrders(t *testing.T) {
	tests := []string{orderdomain.OrderStatusCancelled, orderdomain.OrderStatusPaid}
	for _, orderStatus := range tests {
		t.Run(orderStatus, func(t *testing.T) {
			service, _ := newTestService(t, &fakeOrderClient{orders: map[int64]*pbOrder.Order{
				1: {Id: 1, UserId: 1, Status: orderStatus, TotalAmount: 99},
			}}, nil)

			_, err := service.CreatePayment(context.Background(), 1, 1, PaymentMethodMockBalance)
			if !errors.Is(err, ErrOrderNotPayable) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestSucceedPaymentStoresEventInOutbox(t *testing.T) {
	publisher := &recordingPublisher{}
	service, db := newTestService(t, &fakeOrderClient{orders: map[int64]*pbOrder.Order{
		1: {Id: 1, UserId: 1, Status: orderdomain.OrderStatusPending, TotalAmount: 99},
	}}, publisher)

	payment, err := service.CreatePayment(context.Background(), 1, 1, PaymentMethodMockBalance)
	if err != nil {
		t.Fatalf("CreatePayment returned error: %v", err)
	}
	updated, err := service.SucceedPayment(context.Background(), 1, payment.ID)
	if err != nil {
		t.Fatalf("SucceedPayment returned error: %v", err)
	}
	if got, want := updated.Status, PaymentStatusSucceeded; got != want {
		t.Fatalf("unexpected status: got %q want %q", got, want)
	}
	if got := len(publisher.events); got != 0 {
		t.Fatalf("unexpected direct publish count: got %d want 0", got)
	}

	var saved outbox.Event
	if err := db.Where("event_type = ?", events.PaymentSucceededType).First(&saved).Error; err != nil {
		t.Fatalf("failed to load outbox event: %v", err)
	}
	if got, want := saved.Status, outbox.StatusPending; got != want {
		t.Fatalf("unexpected outbox status: got %q want %q", got, want)
	}

	var event events.PaymentSucceededEvent
	if err := json.Unmarshal([]byte(saved.Payload), &event); err != nil {
		t.Fatalf("failed to decode outbox payload: %v", err)
	}
	if got, want := event.OrderID, int64(1); got != want {
		t.Fatalf("unexpected order id: got %d want %d", got, want)
	}
}

func TestFailPaymentMarksRecordFailed(t *testing.T) {
	service, _ := newTestService(t, &fakeOrderClient{orders: map[int64]*pbOrder.Order{
		1: {Id: 1, UserId: 1, Status: orderdomain.OrderStatusPending, TotalAmount: 99},
	}}, nil)

	payment, err := service.CreatePayment(context.Background(), 1, 1, PaymentMethodMockBalance)
	if err != nil {
		t.Fatalf("CreatePayment returned error: %v", err)
	}
	updated, err := service.FailPayment(1, payment.ID)
	if err != nil {
		t.Fatalf("FailPayment returned error: %v", err)
	}
	if got, want := updated.Status, PaymentStatusFailed; got != want {
		t.Fatalf("unexpected status: got %q want %q", got, want)
	}
}

func TestSucceedPaymentRejectsCancelledOrder(t *testing.T) {
	service, _ := newTestService(t, &fakeOrderClient{orders: map[int64]*pbOrder.Order{
		1: {Id: 1, UserId: 1, Status: orderdomain.OrderStatusPending, TotalAmount: 99},
	}}, nil)

	payment, err := service.CreatePayment(context.Background(), 1, 1, PaymentMethodMockBalance)
	if err != nil {
		t.Fatalf("CreatePayment returned error: %v", err)
	}

	service.orderClient = &fakeOrderClient{orders: map[int64]*pbOrder.Order{
		1: {Id: 1, UserId: 1, Status: orderdomain.OrderStatusCancelled, TotalAmount: 99},
	}}
	_, err = service.SucceedPayment(context.Background(), 1, payment.ID)
	if !errors.Is(err, ErrOrderNotPayable) {
		t.Fatalf("unexpected error: %v", err)
	}
}
