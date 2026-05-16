package idempotency

import (
	"time"

	"gorm.io/gorm"
)

const (
	StateProcessing = "processing"
	StateCompleted  = "completed"
)

// Record 保存一次幂等请求的指纹与首次成功响应快照。
type Record struct {
	gorm.Model
	IdempotencyKey string    `gorm:"size:128;not null;uniqueIndex:idx_user_path_key"`
	UserID         uint      `gorm:"not null;uniqueIndex:idx_user_path_key"`
	RequestPath    string    `gorm:"size:128;not null;uniqueIndex:idx_user_path_key"`
	RequestHash    string    `gorm:"size:64;not null"`
	ResponseBody   string    `gorm:"type:longtext"`
	StatusCode     int       `gorm:"not null;default:0"`
	State          string    `gorm:"size:16;not null;index"`
	ExpiredAt      time.Time `gorm:"not null;index"`
}
