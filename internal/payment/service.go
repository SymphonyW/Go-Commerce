package payment

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	gomysql "github.com/go-sql-driver/mysql"
	pbOrder "go-commerce/api/order"
	"go-commerce/internal/idempotency"
	orderdomain "go-commerce/internal/order"
	"go-commerce/internal/outbox"
	"go-commerce/pkg/events"
	"go-commerce/pkg/mq"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

func Migrate(db *gorm.DB) error {
	if db.Migrator().HasIndex(&Payment{}, activeOrderIndexName) {
		return backfillActiveOrderIDs(db)
	}
	if err := db.AutoMigrate(&Payment{}); err != nil {
		return err
	}
	if err := ensureActiveOrderUniqueIndex(db); err != nil {
		return err
	}
	return backfillActiveOrderIDs(db)
}

const activeOrderIndexName = "idx_payments_active_order"

func ensureActiveOrderUniqueIndex(db *gorm.DB) error {
	if db.Migrator().HasIndex(&Payment{}, activeOrderIndexName) {
		return nil
	}
	return db.Exec("CREATE UNIQUE INDEX " + activeOrderIndexName + " ON payments (active_order_id)").Error
}

func backfillActiveOrderIDs(db *gorm.DB) error {
	return db.Model(&Payment{}).
		Where("active_order_id IS NULL AND status IN ?", []string{PaymentStatusCreated, PaymentStatusSucceeded}).
		Update("active_order_id", gorm.Expr("order_id")).Error
}

func (s *Service) CreatePayment(ctx context.Context, userID, orderID int64, method string) (*Payment, error) {
	if !isSupportedPaymentMethod(method) {
		return nil, ErrInvalidPaymentMethod
	}

	var payment Payment
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		order, err := s.lockOrderForPayment(tx, userID, orderID)
		if err != nil {
			return err
		}
		if order.Status != orderdomain.OrderStatusPending {
			return ErrOrderNotPayable
		}

		activeOrderID := order.ID
		payment = Payment{
			PaymentNo:     newPaymentNo(),
			OrderID:       order.ID,
			ActiveOrderID: &activeOrderID,
			UserID:        uint(userID),
			Amount:        order.TotalAmount,
			Status:        PaymentStatusCreated,
			PaymentMethod: method,
		}
		if err := tx.Create(&payment).Error; err != nil {
			if isActivePaymentUniqueConstraintError(err) {
				return ErrActivePaymentExists
			}
			return err
		}
		return nil
	}); err != nil {
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
	var payment Payment
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.lockPayment(tx, userID, paymentID, &payment); err != nil {
			return err
		}
		if payment.Status != PaymentStatusCreated {
			return ErrPaymentNotActionable
		}

		order, err := s.lockOrderForPayment(tx, int64(userID), int64(payment.OrderID))
		if err != nil {
			return err
		}
		if order.Status != orderdomain.OrderStatusPending || order.TotalAmount != payment.Amount {
			return ErrOrderNotPayable
		}

		activeOrderID := payment.OrderID
		payment.Status = PaymentStatusSucceeded
		payment.ActiveOrderID = &activeOrderID
		if err := tx.Save(&payment).Error; err != nil {
			if isActivePaymentUniqueConstraintError(err) {
				return ErrActivePaymentExists
			}
			return err
		}
		event := events.PaymentSucceededEvent{
			BaseEvent: events.NewBaseEvent(events.PaymentSucceededType, time.Now()),
			PaymentID: int64(payment.ID),
			PaymentNo: payment.PaymentNo,
			OrderID:   int64(payment.OrderID),
			UserID:    int64(payment.UserID),
			Amount:    payment.Amount,
		}
		_, err = s.outboxRepo.Create(ctx, tx, outbox.NewEventInput{
			AggregateType: "payment",
			AggregateID:   strconv.FormatUint(uint64(payment.ID), 10),
			EventType:     events.PaymentSucceededType,
			Payload:       event,
		})
		return err
	}); err != nil {
		return nil, err
	}

	return &payment, nil
}

func (s *Service) FailPayment(userID, paymentID uint) (*Payment, error) {
	var payment Payment
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := s.lockPayment(tx, userID, paymentID, &payment); err != nil {
			return err
		}
		if payment.Status != PaymentStatusCreated {
			return ErrPaymentNotActionable
		}

		payment.Status = PaymentStatusFailed
		payment.ActiveOrderID = nil
		return tx.Save(&payment).Error
	}); err != nil {
		return nil, err
	}
	return &payment, nil
}

func (s *Service) lockOrderForPayment(tx *gorm.DB, userID, orderID int64) (*orderdomain.Order, error) {
	var order orderdomain.Order
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND user_id = ?", orderID, userID).
		First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}
	return &order, nil
}

func (s *Service) lockPayment(tx *gorm.DB, userID, paymentID uint, payment *Payment) error {
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND user_id = ?", paymentID, userID).
		First(payment).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrPaymentNotFound
		}
		return err
	}
	return nil
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

func isActivePaymentUniqueConstraintError(err error) bool {
	if !isUniqueConstraintError(err) {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "idx_payments_active_order") ||
		strings.Contains(message, "active_order_id")
}

func isUniqueConstraintError(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}

	var mysqlErr *gomysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1062
	}

	var sqliteErr interface{ Code() int }
	if errors.As(err, &sqliteErr) {
		switch sqliteErr.Code() {
		case 1555, 2067:
			return true
		}
	}

	message := strings.ToLower(err.Error())
	return strings.Contains(message, "duplicate entry") ||
		strings.Contains(message, "unique constraint failed") ||
		strings.Contains(message, "constraint failed: unique")
}
