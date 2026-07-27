package inbox

import "time"

type ConsumedEvent struct {
	ID           uint      `gorm:"primaryKey"`
	ConsumerName string    `gorm:"size:128;not null;uniqueIndex:idx_consumed_events_consumer_event"`
	EventID      string    `gorm:"size:128;not null;uniqueIndex:idx_consumed_events_consumer_event"`
	EventType    string    `gorm:"size:128;not null;index"`
	ConsumedAt   time.Time `gorm:"not null;index"`
}

func (ConsumedEvent) TableName() string {
	return "consumed_events"
}
