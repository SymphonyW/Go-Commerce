// order 包包含订单服务的模型和业务逻辑
package order

import (
	// gorm.Model：GORM的基础模型，包含ID、CreatedAt、UpdatedAt、DeletedAt字段
	"gorm.io/gorm"
	"time"
)

// Order 订单模型
// 用于存储订单的基本信息
// 对应数据库中的orders表
type Order struct {
	gorm.Model          // 嵌入GORM基础模型
	UserID      uint      `gorm:"not null"`          // 用户ID，非空，关联到用户表
	TotalAmount float64   `gorm:"not null"`          // 订单总金额，非空
	Status      string    `gorm:"not null;default:'pending'"` // 订单状态，非空，默认值为'pending'
	OrderDate   time.Time `gorm:"not null"`          // 订单日期，非空
}

// OrderItem 订单项模型
// 用于存储订单中每个商品的详细信息
// 对应数据库中的order_items表
type OrderItem struct {
	gorm.Model        // 嵌入GORM基础模型
	OrderID     uint    `gorm:"not null"`  // 订单ID，非空，关联到订单表
	ProductID   int64   `gorm:"not null"`  // 产品ID，非空
	ProductName string  `gorm:"not null"`  // 产品名称，非空
	Price       float64 `gorm:"not null"`  // 产品价格，非空
	Quantity    int32   `gorm:"not null"`  // 产品数量，非空
}
