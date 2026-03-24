package order

import (
	"gorm.io/gorm"
	"time"
)

// Order 订单模型
type Order struct {
	gorm.Model
	UserID      uint      `gorm:"not null"`
	TotalAmount float64   `gorm:"not null"`
	Status      string    `gorm:"not null;default:'pending'"`
	OrderDate   time.Time `gorm:"not null"`
}

// OrderItem 订单项模型
type OrderItem struct {
	gorm.Model
	OrderID     uint    `gorm:"not null"`
	ProductID   int64   `gorm:"not null"`
	ProductName string  `gorm:"not null"`
	Price       float64 `gorm:"not null"`
	Quantity    int32   `gorm:"not null"`
}
