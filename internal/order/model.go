package order

import (
	"gorm.io/gorm"
)

type Order struct {
	gorm.Model
	UserID       int64       `gorm:"index"`
	TotalAmount  float64     `gorm:"not null"`
	Status       string      `gorm:"not null;default:'pending'"`
	OrderItems   []OrderItem `gorm:"foreignKey:OrderID"`
}

type OrderItem struct {
	gorm.Model
	OrderID    int64   `gorm:"index"`
	ProductID  int64   `gorm:"index"`
	ProductName string `gorm:"not null"`
	Price      float64 `gorm:"not null"`
	Quantity   int32   `gorm:"not null"`
}
