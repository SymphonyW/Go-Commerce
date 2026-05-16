package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
)

var ErrMissingEventID = errors.New("outbox payload must expose a non-empty event id")

type identifiedPayload interface {
	GetEventID() string
}

// NewEventInput 描述一条即将写入本地消息表的领域事件。
type NewEventInput struct {
	AggregateType string
	AggregateID   string
	EventType     string
	Payload       interface{}
	CreatedAt     time.Time
}

type RetryUpdate struct {
	RetryCount   int
	NextRetryAt  time.Time
	LastError    string
	MarkAsFailed bool
}

// EventRepository 抽象出 worker 与业务侧共同依赖的最小仓储能力。
type EventRepository interface {
	Create(ctx context.Context, tx *gorm.DB, input NewEventInput) (*Event, error)
	ListDuePending(ctx context.Context, now time.Time, limit int) ([]Event, error)
	MarkPublished(ctx context.Context, id uint, publishedAt time.Time) error
	MarkRetry(ctx context.Context, id uint, update RetryUpdate) error
}

type GormRepository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *GormRepository {
	return &GormRepository{db: db}
}

func (r *GormRepository) Create(ctx context.Context, tx *gorm.DB, input NewEventInput) (*Event, error) {
	if tx == nil {
		tx = r.db
	}

	identified, ok := input.Payload.(identifiedPayload)
	if !ok || identified.GetEventID() == "" {
		return nil, ErrMissingEventID
	}

	payload, err := json.Marshal(input.Payload)
	if err != nil {
		return nil, err
	}

	createdAt := input.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}

	event := &Event{
		EventID:       identified.GetEventID(),
		AggregateType: input.AggregateType,
		AggregateID:   input.AggregateID,
		EventType:     input.EventType,
		Payload:       string(payload),
		Status:        StatusPending,
		NextRetryAt:   createdAt,
		CreatedAt:     createdAt,
	}
	if err := tx.WithContext(ctx).Create(event).Error; err != nil {
		return nil, err
	}
	return event, nil
}

func (r *GormRepository) ListDuePending(ctx context.Context, now time.Time, limit int) ([]Event, error) {
	if limit <= 0 {
		limit = DefaultBatchSize
	}

	var events []Event
	err := r.db.WithContext(ctx).
		Where("status = ? AND next_retry_at <= ?", StatusPending, now).
		Order("created_at ASC").
		Order("id ASC").
		Limit(limit).
		Find(&events).Error
	return events, err
}

func (r *GormRepository) MarkPublished(ctx context.Context, id uint, publishedAt time.Time) error {
	return r.db.WithContext(ctx).
		Model(&Event{}).
		Where("id = ? AND status = ?", id, StatusPending).
		Updates(map[string]interface{}{
			"status":       StatusPublished,
			"published_at": publishedAt,
			"last_error":   "",
		}).Error
}

func (r *GormRepository) MarkRetry(ctx context.Context, id uint, update RetryUpdate) error {
	status := StatusPending
	if update.MarkAsFailed {
		status = StatusFailed
	}

	return r.db.WithContext(ctx).
		Model(&Event{}).
		Where("id = ? AND status = ?", id, StatusPending).
		Updates(map[string]interface{}{
			"status":        status,
			"retry_count":   update.RetryCount,
			"next_retry_at": update.NextRetryAt,
			"last_error":    update.LastError,
		}).Error
}
