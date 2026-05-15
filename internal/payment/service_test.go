package payment

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	pbOrder "go-commerce/api/order"
	"go-commerce/pkg/events"
	"go-commerce/pkg/mq"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/driver/sqlite"
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

func (f *fakeOrderClient) CancelOrder(context.Context, *pbOrder.CancelOrderRequest, ...grpc.CallOption) (*pbOrder.CancelOrderResponse, error) {
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
	if err := db.AutoMigrate(&Payment{}); err != nil {
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
		1: {Id: 1, UserId: 2, Status: OrderStatusPending, TotalAmount: 99},
	}}, nil)

	_, err := service.CreatePayment(context.Background(), 1, 1, PaymentMethodMockBalance)
	if !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreatePaymentForPendingOrder(t *testing.T) {
	service, _ := newTestService(t, &fakeOrderClient{orders: map[int64]*pbOrder.Order{
		1: {Id: 1, UserId: 1, Status: OrderStatusPending, TotalAmount: 99},
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
		1: {Id: 1, UserId: 1, Status: OrderStatusPending, TotalAmount: 99},
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
	tests := []string{OrderStatusCancelled, OrderStatusPaid}
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

func TestSucceedPaymentPublishesEvent(t *testing.T) {
	publisher := &recordingPublisher{}
	service, _ := newTestService(t, &fakeOrderClient{orders: map[int64]*pbOrder.Order{
		1: {Id: 1, UserId: 1, Status: OrderStatusPending, TotalAmount: 99},
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
	if got, want := len(publisher.events), 1; got != want {
		t.Fatalf("unexpected published event count: got %d want %d", got, want)
	}
	if got, want := publisher.events[0].routingKey, events.PaymentSucceededType; got != want {
		t.Fatalf("unexpected routing key: got %q want %q", got, want)
	}
	event, ok := publisher.events[0].event.(events.PaymentSucceededEvent)
	if !ok {
		t.Fatalf("unexpected event type: %T", publisher.events[0].event)
	}
	if got, want := event.OrderID, int64(1); got != want {
		t.Fatalf("unexpected order id: got %d want %d", got, want)
	}
}

func TestFailPaymentMarksRecordFailed(t *testing.T) {
	service, _ := newTestService(t, &fakeOrderClient{orders: map[int64]*pbOrder.Order{
		1: {Id: 1, UserId: 1, Status: OrderStatusPending, TotalAmount: 99},
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
