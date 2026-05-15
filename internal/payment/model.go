package payment

import "gorm.io/gorm"

const (
	PaymentStatusCreated   = "created"
	PaymentStatusSucceeded = "succeeded"
	PaymentStatusFailed    = "failed"

	PaymentMethodMockBalance = "mock_balance"
	PaymentMethodMockWechat  = "mock_wechat"
	PaymentMethodMockAlipay  = "mock_alipay"
)

// Payment 记录一次独立支付尝试，订单和支付通过 order_id 关联但职责分离。
type Payment struct {
	gorm.Model
	PaymentNo     string  `gorm:"not null;uniqueIndex"`
	OrderID       uint    `gorm:"not null;index"`
	UserID        uint    `gorm:"not null;index"`
	Amount        float64 `gorm:"not null"`
	Status        string  `gorm:"not null;default:'created';index"`
	PaymentMethod string  `gorm:"not null"`
}
