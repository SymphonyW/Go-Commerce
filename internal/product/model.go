// product 包包含产品服务的模型和业务逻辑
package product

import (
	// gorm.Model：GORM的基础模型，包含ID、CreatedAt、UpdatedAt、DeletedAt字段
	"gorm.io/gorm"
)

// Product 产品模型
// 用于存储产品的详细信息
// 对应数据库中的products表
type Product struct {
	gorm.Model          // 嵌入GORM基础模型
	Name        string  `gorm:"not null"`        // 产品名称，非空
	Description string                             // 产品描述
	Price       float64 `gorm:"not null"`        // 产品价格，非空
	Stock       int32   `gorm:"not null;default:0"` // 产品库存，非空，默认值为0
	Category    string  `gorm:"index"`           // 产品分类，创建索引以提高查询性能
	ImageURL    string                             // 产品图片URL
	MerchantID  uint    `gorm:"not null"`        // 商家ID，非空，关联到商家表
}
