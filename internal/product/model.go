package product

import "gorm.io/gorm"

// Product stores catalog data. PriceCents is an integer amount in cents.
type Product struct {
	gorm.Model
	Name        string `gorm:"not null"`
	Description string
	PriceCents  int64  `gorm:"not null"`
	Stock       int32  `gorm:"not null;default:0"`
	Category    string `gorm:"index"`
	ImageURL    string
	MerchantID  uint `gorm:"not null"`
}
