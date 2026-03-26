// auth 包包含认证服务的模型和业务逻辑
package auth

import (
	// gorm.Model：GORM的基础模型，包含ID、CreatedAt、UpdatedAt、DeletedAt字段
	"gorm.io/gorm"
)

// User 用户模型
// 用于存储用户的基本信息
// 对应数据库中的users表
type User struct {
	gorm.Model          // 嵌入GORM基础模型
	Username string `gorm:"unique;not null"` // 用户名，唯一且非空
	Password string `gorm:"not null"`        // 密码，非空
	Email    string `gorm:"unique;not null"` // 邮箱，唯一且非空
}
