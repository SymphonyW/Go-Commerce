package payment

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	pbOrder "go-commerce/api/order"
	"go-commerce/internal/idempotency"
	orderdomain "go-commerce/internal/order"
	"go-commerce/internal/outbox"
	"go-commerce/pkg/events"
	"go-commerce/pkg/mq"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

var (
	ErrOrderNotFound        = errors.New("order not found")
	ErrOrderNotPayable      = errors.New("order is not payable")
	ErrActivePaymentExists  = errors.New("active payment already exists")
	ErrPaymentNotFound      = errors.New("payment not found")
	ErrPaymentNotActionable = errors.New("payment cannot change status")
	ErrInvalidPaymentMethod = errors.New("invalid payment method")
)

// Service 聚合支付核心逻辑，避免把支付状态机散落在 gRPC 或网关层。
type Service struct {
	db          *gorm.DB
	orderClient pbOrder.OrderServiceClient
	publisher   mq.Publisher
	outboxRepo  outbox.EventRepository
	idempotency *idempotency.Service
}

func NewService(db *gorm.DB, orderClient pbOrder.OrderServiceClient, publisher mq.Publisher) *Service {
	if publisher == nil {
		publisher = mq.NopPublisher{}
	}
	return &Service{
		db:          db,
		orderClient: orderClient,
		publisher:   publisher,
		outboxRepo:  outbox.NewRepository(db),
		idempotency: idempotency.NewService(db, 24*time.Hour),
	}
}

func (s *Service) CreatePayment(ctx context.Context, userID, orderID int64, method string) (*Payment, error) {
	if !isSupportedPaymentMethod(method) {
		return nil, ErrInvalidPaymentMethod
	}

	order, err := s.fetchOrder(ctx, userID, orderID)
	if err != nil {
		return nil, err
	}
	if order.Status != orderdomain.OrderStatusPending {
		return nil, ErrOrderNotPayable
	}

	var activeCount int64
	if err := s.db.Model(&Payment{}).
		Where("order_id = ? AND status IN ?", orderID, []string{PaymentStatusCreated, PaymentStatusSucceeded}).
		Count(&activeCount).Error; err != nil {
		return nil, err
	}
	if activeCount > 0 {
		return nil, ErrActivePaymentExists
	}

	payment := Payment{
		PaymentNo:     newPaymentNo(),
		OrderID:       uint(orderID),
		UserID:        uint(userID),
		Amount:        float64(order.TotalAmount),
		Status:        PaymentStatusCreated,
		PaymentMethod: method,
	}
	if err := s.db.Create(&payment).Error; err != nil {
		return nil, err
	}
	return &payment, nil
}

func (s *Service) GetPayment(userID, paymentID int64) (*Payment, error) {
	var payment Payment
	if err := s.db.Where("id = ? AND user_id = ?", paymentID, userID).First(&payment).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPaymentNotFound
		}
		return nil, err
	}
	return &payment, nil
}

func (s *Service) SucceedPayment(ctx context.Context, userID, paymentID uint) (*Payment, error) {
	payment, err := s.GetPayment(int64(userID), int64(paymentID))
	if err != nil {
		return nil, err
	}
	if payment.Status != PaymentStatusCreated {
		return nil, ErrPaymentNotActionable
	}

	order, err := s.fetchOrder(ctx, int64(userID), int64(payment.OrderID))
	if err != nil {
		return nil, err
	}
	if order.Status != orderdomain.OrderStatusPending || float64(order.TotalAmount) != payment.Amount {
		return nil, ErrOrderNotPayable
	}

	payment.Status = PaymentStatusSucceeded
	event := events.PaymentSucceededEvent{
		BaseEvent: events.NewBaseEvent(events.PaymentSucceededType, time.Now()),
		PaymentID: int64(payment.ID),
		PaymentNo: payment.PaymentNo,
		OrderID:   int64(payment.OrderID),
		UserID:    int64(payment.UserID),
		Amount:    payment.Amount,
	}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(payment).Error; err != nil {
			return err
		}
		_, err := s.outboxRepo.Create(ctx, tx, outbox.NewEventInput{
			AggregateType: "payment",
			AggregateID:   strconv.FormatUint(uint64(payment.ID), 10),
			EventType:     events.PaymentSucceededType,
			Payload:       event,
		})
		return err
	}); err != nil {
		return nil, err
	}

	return payment, nil
}

func (s *Service) FailPayment(userID, paymentID uint) (*Payment, error) {
	payment, err := s.GetPayment(int64(userID), int64(paymentID))
	if err != nil {
		return nil, err
	}
	if payment.Status != PaymentStatusCreated {
		return nil, ErrPaymentNotActionable
	}

	payment.Status = PaymentStatusFailed
	if err := s.db.Save(payment).Error; err != nil {
		return nil, err
	}
	return payment, nil
}

func (s *Service) fetchOrder(ctx context.Context, userID, orderID int64) (*pbOrder.Order, error) {
	if s.orderClient == nil {
		return nil, fmt.Errorf("order client is nil")
	}
	resp, err := s.orderClient.GetOrder(ctx, &pbOrder.GetOrderRequest{Id: orderID, UserId: userID})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}
	if resp == nil || resp.Order == nil {
		return nil, ErrOrderNotFound
	}
	return resp.Order, nil
}

func isSupportedPaymentMethod(method string) bool {
	switch method {
	case PaymentMethodMockBalance, PaymentMethodMockWechat, PaymentMethodMockAlipay:
		return true
	default:
		return false
	}
}

func newPaymentNo() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("pay-%d", time.Now().UTC().UnixNano())
	}
	return "pay-" + hex.EncodeToString(buf)
}
