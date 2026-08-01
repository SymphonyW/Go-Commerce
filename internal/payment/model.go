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
	// 支付单号会参与唯一约束，必须显式限制长度，避免 MySQL 把它推断成 longtext 后无法建索引。
	// 同时使用 unique，避免重复 AutoMigrate 时再次触发 GORM/MySQL 的唯一索引迁移问题。
	PaymentNo     string `gorm:"size:64;not null;unique"`
	OrderID       uint   `gorm:"not null;index"`
	ActiveOrderID *uint
	UserID        uint   `gorm:"not null;index"`
	AmountCents   int64  `gorm:"not null"`
	Status        string `gorm:"not null;default:'created';index"`
	PaymentMethod string `gorm:"not null"`
}
