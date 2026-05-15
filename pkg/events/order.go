package events

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

const (
	OrderCreatedType     = "order.created"
	OrderPaidType        = "order.paid"
	OrderShippedType     = "order.shipped"
	OrderCompletedType   = "order.completed"
	OrderCancelledType   = "order.cancelled"
	PaymentSucceededType = "payment.succeeded"
)

// BaseEvent 为所有领域事件提供统一元数据，消费者无需依赖 routing key 也能识别消息类型。
type BaseEvent struct {
	EventID    string `json:"event_id"`
	EventType  string `json:"event_type"`
	OccurredAt string `json:"occurred_at"`
}

// GetEventID 让消息发布器可以把业务事件 ID 同步写入 AMQP MessageId。
func (e BaseEvent) GetEventID() string {
	return e.EventID
}

// OrderItemSnapshot 表示下单瞬间的商品快照。
type OrderItemSnapshot struct {
	ProductID   int64   `json:"product_id"`
	MerchantID  int64   `json:"merchant_id"`
	ProductName string  `json:"product_name"`
	Price       float64 `json:"price"`
	Quantity    int32   `json:"quantity"`
}

// OrderCreatedEvent 表示订单创建成功后的领域事件。
type OrderCreatedEvent struct {
	BaseEvent
	OrderID     int64               `json:"order_id"`
	UserID      int64               `json:"user_id"`
	TotalAmount float64             `json:"total_amount"`
	Items       []OrderItemSnapshot `json:"items,omitempty"`
}

// OrderCancelledEvent 表示订单取消成功后的领域事件。
type OrderCancelledEvent struct {
	BaseEvent
	OrderID int64  `json:"order_id"`
	UserID  int64  `json:"user_id"`
	Reason  string `json:"reason,omitempty"`
}

// OrderStatusChangedEvent 表示订单主状态发生了一次合法迁移。
type OrderStatusChangedEvent struct {
	BaseEvent
	OrderID    int64  `json:"order_id"`
	UserID     int64  `json:"user_id"`
	FromStatus string `json:"from_status"`
	ToStatus   string `json:"to_status"`
}

// PaymentSucceededEvent 表示一次支付已经成功完成。
type PaymentSucceededEvent struct {
	BaseEvent
	PaymentID int64   `json:"payment_id"`
	PaymentNo string  `json:"payment_no"`
	OrderID   int64   `json:"order_id"`
	UserID    int64   `json:"user_id"`
	Amount    float64 `json:"amount"`
}

// NewBaseEvent 统一生成事件元数据，时间使用 UTC RFC3339Nano。
func NewBaseEvent(eventType string, occurredAt time.Time) BaseEvent {
	return BaseEvent{
		EventID:    newEventID(),
		EventType:  eventType,
		OccurredAt: occurredAt.UTC().Format(time.RFC3339Nano),
	}
}

func newEventID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("evt-%d", time.Now().UTC().UnixNano())
	}
	return hex.EncodeToString(buf)
}
