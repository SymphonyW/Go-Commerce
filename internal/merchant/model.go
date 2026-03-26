// merchant 包包含商家服务的模型和业务逻辑
package merchant

import (
	// gorm.Model：GORM的基础模型，包含ID、CreatedAt、UpdatedAt、DeletedAt字段
	"gorm.io/gorm"
)

// Merchant 商家模型
// 用于存储商家的基本信息
// 对应数据库中的merchants表
type Merchant struct {
	gorm.Model          // 嵌入GORM基础模型
	Name        string `gorm:"not null"` // 商家名称，非空
	ContactInfo string `gorm:"not null"` // 联系信息，非空
}
