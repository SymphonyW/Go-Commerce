package payment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	pbOrder "go-commerce/api/order"
	pbPayment "go-commerce/api/payment"
	"go-commerce/internal/idempotency"
	orderdomain "go-commerce/internal/order"
	"go-commerce/internal/outbox"
	"go-commerce/pkg/events"
	"go-commerce/pkg/mq"

	"github.com/glebarez/sqlite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

type fakeOrderClient struct {
	orders   map[int64]*pbOrder.Order
	getCount int
}

func (f *fakeOrderClient) CreateOrder(context.Context, *pbOrder.CreateOrderRequest, ...grpc.CallOption) (*pbOrder.CreateOrderResponse, error) {
	return nil, nil
}

func (f *fakeOrderClient) GetOrder(ctx context.Context, req *pbOrder.GetOrderRequest, opts ...grpc.CallOption) (*pbOrder.GetOrderResponse, error) {
	f.getCount++
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

func paymentActionRequest(userID int64, paymentID uint, key string) *pbPayment.PaymentActionRequest {
	return &pbPayment.PaymentActionRequest{
		Id:             int64(paymentID),
		UserId:         userID,
		IdempotencyKey: key,
	}
}

func createPaymentForOrder(t *testing.T, service *Service, userID, orderID int64) *Payment {
	t.Helper()

	payment, err := service.CreatePayment(context.Background(), userID, orderID, PaymentMethodMockBalance)
	if err != nil {
		t.Fatalf("CreatePayment returned error: %v", err)
	}
	return payment
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

func TestMarkPaymentSucceededRejectsMissingIdempotencyKey(t *testing.T) {
	service, _ := newTestService(t, &fakeOrderClient{orders: map[int64]*pbOrder.Order{}}, nil)
	grpcService := NewGRPCService(service)

	_, err := grpcService.MarkPaymentSucceeded(context.Background(), &pbPayment.PaymentActionRequest{
		Id:     1,
		UserId: 1,
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("unexpected error code: got %v want %v", status.Code(err), codes.InvalidArgument)
	}
}

func TestMarkPaymentSucceededCompletesIdempotencyRecordAndStoresOutbox(t *testing.T) {
	service, db := newTestService(t, &fakeOrderClient{orders: map[int64]*pbOrder.Order{
		1: {Id: 1, UserId: 1, Status: orderdomain.OrderStatusPending, TotalAmount: 99},
	}}, nil)
	grpcService := NewGRPCService(service)
	payment := createPaymentForOrder(t, service, 1, 1)

	resp, err := grpcService.MarkPaymentSucceeded(context.Background(), paymentActionRequest(1, payment.ID, "payment-success-key"))
	if err != nil {
		t.Fatalf("MarkPaymentSucceeded returned error: %v", err)
	}
	if got, want := resp.Payment.Status, PaymentStatusSucceeded; got != want {
		t.Fatalf("unexpected payment status: got %q want %q", got, want)
	}

	var eventCount int64
	if err := db.Model(&outbox.Event{}).
		Where("aggregate_type = ? AND aggregate_id = ? AND event_type = ?", "payment", fmt.Sprintf("%d", payment.ID), events.PaymentSucceededType).
		Count(&eventCount).Error; err != nil {
		t.Fatalf("failed to count payment succeeded events: %v", err)
	}
	if got, want := eventCount, int64(1); got != want {
		t.Fatalf("unexpected outbox event count: got %d want %d", got, want)
	}

	var record idempotency.Record
	if err := db.Where("user_id = ? AND request_path = ? AND idempotency_key = ?", 1, paymentSuccessRequestPath, "payment-success-key").First(&record).Error; err != nil {
		t.Fatalf("failed to load idempotency record: %v", err)
	}
	if got, want := record.State, idempotency.StateCompleted; got != want {
		t.Fatalf("unexpected idempotency state: got %q want %q", got, want)
	}
	if got, want := record.StatusCode, http.StatusOK; got != want {
		t.Fatalf("unexpected status code: got %d want %d", got, want)
	}

	var replay pbPayment.PaymentActionResponse
	if err := idempotency.ReplayInto(&record, &replay); err != nil {
		t.Fatalf("failed to replay stored response: %v", err)
	}
	if !proto.Equal(resp, &replay) {
		t.Fatalf("stored response mismatch: got %+v want %+v", &replay, resp)
	}
}

func TestMarkPaymentSucceededReplaysSameKeySameRequest(t *testing.T) {
	orderClient := &fakeOrderClient{orders: map[int64]*pbOrder.Order{
		1: {Id: 1, UserId: 1, Status: orderdomain.OrderStatusPending, TotalAmount: 99},
	}}
	service, _ := newTestService(t, orderClient, nil)
	grpcService := NewGRPCService(service)
	payment := createPaymentForOrder(t, service, 1, 1)
	req := paymentActionRequest(1, payment.ID, "repeat-payment-success")

	first, err := grpcService.MarkPaymentSucceeded(context.Background(), req)
	if err != nil {
		t.Fatalf("first MarkPaymentSucceeded returned error: %v", err)
	}
	orderClient.orders[1] = &pbOrder.Order{Id: 1, UserId: 1, Status: orderdomain.OrderStatusCancelled, TotalAmount: 99}
	second, err := grpcService.MarkPaymentSucceeded(context.Background(), req)
	if err != nil {
		t.Fatalf("second MarkPaymentSucceeded returned error: %v", err)
	}

	if !proto.Equal(first, second) {
		t.Fatalf("expected replayed response to match first response: first=%+v second=%+v", first, second)
	}
	if got, want := orderClient.getCount, 2; got != want {
		t.Fatalf("unexpected order lookup count: got %d want %d", got, want)
	}
}

func TestMarkPaymentSucceededRejectsSameKeyWithDifferentPaymentID(t *testing.T) {
	service, _ := newTestService(t, &fakeOrderClient{orders: map[int64]*pbOrder.Order{
		1: {Id: 1, UserId: 1, Status: orderdomain.OrderStatusPending, TotalAmount: 99},
		2: {Id: 2, UserId: 1, Status: orderdomain.OrderStatusPending, TotalAmount: 45},
	}}, nil)
	grpcService := NewGRPCService(service)
	firstPayment := createPaymentForOrder(t, service, 1, 1)
	secondPayment := createPaymentForOrder(t, service, 1, 2)

	if _, err := grpcService.MarkPaymentSucceeded(context.Background(), paymentActionRequest(1, firstPayment.ID, "conflicting-payment-success")); err != nil {
		t.Fatalf("first MarkPaymentSucceeded returned error: %v", err)
	}
	_, err := grpcService.MarkPaymentSucceeded(context.Background(), paymentActionRequest(1, secondPayment.ID, "conflicting-payment-success"))
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("unexpected error code: got %v want %v", status.Code(err), codes.FailedPrecondition)
	}
}

func TestMarkPaymentSucceededRejectsSameKeyWhileProcessing(t *testing.T) {
	service, db := newTestService(t, &fakeOrderClient{orders: map[int64]*pbOrder.Order{
		1: {Id: 1, UserId: 1, Status: orderdomain.OrderStatusPending, TotalAmount: 99},
	}}, nil)
	grpcService := NewGRPCService(service)
	payment := createPaymentForOrder(t, service, 1, 1)
	requestHash, err := idempotency.HashPayload(newPaymentSuccessFingerprint(1, int64(payment.ID)))
	if err != nil {
		t.Fatalf("failed to hash payment success request: %v", err)
	}
	if err := db.Create(&idempotency.Record{
		IdempotencyKey: "processing-payment-success",
		UserID:         1,
		RequestPath:    paymentSuccessRequestPath,
		RequestHash:    requestHash,
		State:          idempotency.StateProcessing,
		ExpiredAt:      time.Now().Add(time.Hour),
	}).Error; err != nil {
		t.Fatalf("failed to create processing idempotency record: %v", err)
	}

	_, err = grpcService.MarkPaymentSucceeded(context.Background(), paymentActionRequest(1, payment.ID, "processing-payment-success"))
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("unexpected error code: got %v want %v", status.Code(err), codes.FailedPrecondition)
	}
}

func TestMarkPaymentSucceededReplayDoesNotCreateSecondOutboxEvent(t *testing.T) {
	service, db := newTestService(t, &fakeOrderClient{orders: map[int64]*pbOrder.Order{
		1: {Id: 1, UserId: 1, Status: orderdomain.OrderStatusPending, TotalAmount: 99},
	}}, nil)
	grpcService := NewGRPCService(service)
	payment := createPaymentForOrder(t, service, 1, 1)
	req := paymentActionRequest(1, payment.ID, "payment-success-outbox-replay")

	if _, err := grpcService.MarkPaymentSucceeded(context.Background(), req); err != nil {
		t.Fatalf("first MarkPaymentSucceeded returned error: %v", err)
	}
	if _, err := grpcService.MarkPaymentSucceeded(context.Background(), req); err != nil {
		t.Fatalf("replay MarkPaymentSucceeded returned error: %v", err)
	}

	var eventCount int64
	if err := db.Model(&outbox.Event{}).
		Where("aggregate_type = ? AND aggregate_id = ? AND event_type = ?", "payment", fmt.Sprintf("%d", payment.ID), events.PaymentSucceededType).
		Count(&eventCount).Error; err != nil {
		t.Fatalf("failed to count payment succeeded events: %v", err)
	}
	if got, want := eventCount, int64(1); got != want {
		t.Fatalf("unexpected outbox event count: got %d want %d", got, want)
	}
}

func TestMarkPaymentSucceededAbortsIdempotencyRecordAfterBusinessFailure(t *testing.T) {
	service, db := newTestService(t, &fakeOrderClient{orders: map[int64]*pbOrder.Order{
		1: {Id: 1, UserId: 1, Status: orderdomain.OrderStatusPending, TotalAmount: 99},
	}}, nil)
	grpcService := NewGRPCService(service)
	payment := createPaymentForOrder(t, service, 1, 1)

	if err := db.Exec(`
		CREATE TRIGGER fail_payment_success_outbox_insert
		BEFORE INSERT ON outbox_events
		WHEN NEW.event_type = 'payment.succeeded'
		BEGIN
			SELECT RAISE(FAIL, 'forced payment success outbox failure');
		END;
	`).Error; err != nil {
		t.Fatalf("failed to create trigger: %v", err)
	}

	_, err := grpcService.MarkPaymentSucceeded(context.Background(), paymentActionRequest(1, payment.ID, "abort-payment-success"))
	if status.Code(err) != codes.Internal {
		t.Fatalf("unexpected error code: got %v want %v", status.Code(err), codes.Internal)
	}

	var recordCount int64
	if err := db.Unscoped().
		Model(&idempotency.Record{}).
		Where("user_id = ? AND request_path = ? AND idempotency_key = ?", 1, paymentSuccessRequestPath, "abort-payment-success").
		Count(&recordCount).Error; err != nil {
		t.Fatalf("failed to count idempotency records: %v", err)
	}
	if got, want := recordCount, int64(0); got != want {
		t.Fatalf("unexpected idempotency record count after abort: got %d want %d", got, want)
	}

	var latest Payment
	if err := db.First(&latest, payment.ID).Error; err != nil {
		t.Fatalf("failed to reload payment: %v", err)
	}
	if got, want := latest.Status, PaymentStatusCreated; got != want {
		t.Fatalf("unexpected payment status after rollback: got %q want %q", got, want)
	}

	if err := db.Exec("DROP TRIGGER fail_payment_success_outbox_insert").Error; err != nil {
		t.Fatalf("failed to drop trigger: %v", err)
	}
	retry, err := grpcService.MarkPaymentSucceeded(context.Background(), paymentActionRequest(1, payment.ID, "abort-payment-success"))
	if err != nil {
		t.Fatalf("retry MarkPaymentSucceeded returned error: %v", err)
	}
	if got, want := retry.Payment.Status, PaymentStatusSucceeded; got != want {
		t.Fatalf("unexpected retry status: got %q want %q", got, want)
	}
}

func TestMarkPaymentSucceededNewKeyAfterSuccessReturnsBusinessError(t *testing.T) {
	service, db := newTestService(t, &fakeOrderClient{orders: map[int64]*pbOrder.Order{
		1: {Id: 1, UserId: 1, Status: orderdomain.OrderStatusPending, TotalAmount: 99},
	}}, nil)
	grpcService := NewGRPCService(service)
	payment := createPaymentForOrder(t, service, 1, 1)

	if _, err := grpcService.MarkPaymentSucceeded(context.Background(), paymentActionRequest(1, payment.ID, "first-payment-success-key")); err != nil {
		t.Fatalf("first MarkPaymentSucceeded returned error: %v", err)
	}
	_, err := grpcService.MarkPaymentSucceeded(context.Background(), paymentActionRequest(1, payment.ID, "new-payment-success-key"))
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("unexpected error code: got %v want %v", status.Code(err), codes.FailedPrecondition)
	}

	var eventCount int64
	if err := db.Model(&outbox.Event{}).
		Where("aggregate_type = ? AND aggregate_id = ? AND event_type = ?", "payment", fmt.Sprintf("%d", payment.ID), events.PaymentSucceededType).
		Count(&eventCount).Error; err != nil {
		t.Fatalf("failed to count payment succeeded events: %v", err)
	}
	if got, want := eventCount, int64(1); got != want {
		t.Fatalf("unexpected outbox event count: got %d want %d", got, want)
	}

	var recordCount int64
	if err := db.Unscoped().
		Model(&idempotency.Record{}).
		Where("user_id = ? AND request_path = ? AND idempotency_key = ?", 1, paymentSuccessRequestPath, "new-payment-success-key").
		Count(&recordCount).Error; err != nil {
		t.Fatalf("failed to count new-key idempotency records: %v", err)
	}
	if got, want := recordCount, int64(0); got != want {
		t.Fatalf("unexpected new-key idempotency record count after business error: got %d want %d", got, want)
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
