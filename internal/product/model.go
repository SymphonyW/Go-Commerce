package product

import (
	"gorm.io/gorm"
)

type Product struct {
	gorm.Model
	Name        string  `gorm:"not null"`
	Description string
	Price       float64 `gorm:"not null"`
	Stock       int32   `gorm:"not null;default:0"`
	Category    string  `gorm:"index"`
	ImageURL    string
	MerchantID  uint    `gorm:"not null"`
}
