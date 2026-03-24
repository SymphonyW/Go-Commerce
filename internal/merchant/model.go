package merchant

import (
	"gorm.io/gorm"
)

// Merchant 商家模型
type Merchant struct {
	gorm.Model
	Name        string `gorm:"not null"`
	ContactInfo string `gorm:"not null"`
}
