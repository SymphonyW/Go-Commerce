package outbox

import "time"

const (
	StatusPending   = "pending"
	StatusPublished = "published"
	StatusFailed    = "failed"
)

// Event 对应 outbox_events 表。
// 业务事件先随本地事务一同落库，再由 worker 异步投递到 RabbitMQ。
type Event struct {
	ID uint `gorm:"primaryKey"`
	// 这里使用 unique 而不是 uniqueIndex：
	// MySQL 下 GORM 会把 uniqueIndex 识别成“唯一列”，重复 AutoMigrate 时会误删不存在的约束，导致服务启动失败。
	EventID       string    `gorm:"size:64;not null;unique"`
	AggregateType string    `gorm:"size:64;not null;index:idx_outbox_aggregate"`
	AggregateID   string    `gorm:"size:64;not null;index:idx_outbox_aggregate"`
	EventType     string    `gorm:"size:128;not null;index"`
	Payload       string    `gorm:"type:longtext;not null"`
	Status        string    `gorm:"size:16;not null;default:'pending';index"`
	RetryCount    int       `gorm:"not null;default:0"`
	NextRetryAt   time.Time `gorm:"not null;index"`
	LastError     string    `gorm:"type:text"`
	CreatedAt     time.Time `gorm:"not null;index"`
	UpdatedAt     time.Time
	PublishedAt   *time.Time `gorm:"index"`
}

func (Event) TableName() string {
	return "outbox_events"
}
