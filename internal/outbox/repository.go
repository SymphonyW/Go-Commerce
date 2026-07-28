package outbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
)

var ErrMissingEventID = errors.New("outbox payload must expose a non-empty event id")
var ErrLeaseNotOwned = errors.New("outbox event lease is not owned by worker")

type identifiedPayload interface {
	GetEventID() string
}

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

type ClaimResult struct {
	Events              []Event
	LeaseRecoveredCount int
}

type EventRepository interface {
	Create(ctx context.Context, tx *gorm.DB, input NewEventInput) (*Event, error)
	ClaimDueEvents(ctx context.Context, now time.Time, limit int, workerID string, leaseDuration time.Duration) (ClaimResult, error)
	MarkPublished(ctx context.Context, id uint, workerID string, publishedAt time.Time) error
	MarkRetry(ctx context.Context, id uint, workerID string, now time.Time, update RetryUpdate) error
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

func (r *GormRepository) ClaimDueEvents(ctx context.Context, now time.Time, limit int, workerID string, leaseDuration time.Duration) (ClaimResult, error) {
	if limit <= 0 {
		limit = DefaultBatchSize
	}
	if workerID == "" || leaseDuration <= 0 {
		return ClaimResult{}, ErrLeaseNotOwned
	}

	var result ClaimResult
	var txOptions []*sql.TxOptions
	if r.supportsSkipLocked() {
		txOptions = append(txOptions, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		selected, err := r.selectPendingClaimable(tx, now, limit)
		if err != nil {
			return err
		}

		if remaining := limit - len(selected); remaining > 0 {
			expired, err := r.selectExpiredProcessingClaimable(tx, now, remaining)
			if err != nil {
				return err
			}
			selected = append(selected, expired...)
		}

		if len(selected) == 0 {
			return nil
		}

		ids := make([]uint, 0, len(selected))
		statusByID := make(map[uint]string, len(selected))
		for _, event := range selected {
			ids = append(ids, event.ID)
			statusByID[event.ID] = event.Status
		}

		leaseExpiresAt := now.Add(leaseDuration)
		updateResult := tx.Model(&Event{}).
			Where(
				"id IN ? AND ((status = ? AND next_retry_at <= ?) OR (status = ? AND lease_expires_at IS NOT NULL AND lease_expires_at <= ?))",
				ids,
				StatusPending,
				now,
				StatusProcessing,
				now,
			).
			Updates(map[string]interface{}{
				"status":           StatusProcessing,
				"locked_by":        workerID,
				"locked_at":        now,
				"lease_expires_at": leaseExpiresAt,
			})
		if updateResult.Error != nil {
			return updateResult.Error
		}
		if updateResult.RowsAffected == 0 {
			return nil
		}

		var claimed []Event
		if err := tx.
			Where("id IN ? AND status = ? AND locked_by = ?", ids, StatusProcessing, workerID).
			Order("id ASC").
			Find(&claimed).Error; err != nil {
			return err
		}

		for _, event := range claimed {
			if statusByID[event.ID] == StatusProcessing {
				result.LeaseRecoveredCount++
			}
		}
		result.Events = claimed
		return nil
	}, txOptions...)
	return result, err
}

type claimIDRow struct {
	ID uint
}

func (r *GormRepository) selectPendingClaimable(tx *gorm.DB, now time.Time, limit int) ([]Event, error) {
	if r.supportsSkipLocked() {
		var rows []claimIDRow
		err := tx.Raw(
			`SELECT id FROM outbox_events FORCE INDEX (idx_outbox_pending_claim)
WHERE status = ? AND next_retry_at <= ?
ORDER BY next_retry_at ASC, id ASC
LIMIT ? FOR UPDATE SKIP LOCKED`,
			StatusPending,
			now,
			limit,
		).Scan(&rows).Error
		if err != nil {
			return nil, err
		}
		return loadClaimedEventsByID(tx, rows)
	}

	var events []Event
	err := tx.
		Where("status = ? AND next_retry_at <= ?", StatusPending, now).
		Order("next_retry_at ASC").
		Order("id ASC").
		Limit(limit).
		Find(&events).Error
	return events, err
}

func (r *GormRepository) selectExpiredProcessingClaimable(tx *gorm.DB, now time.Time, limit int) ([]Event, error) {
	if r.supportsSkipLocked() {
		var rows []claimIDRow
		err := tx.Raw(
			`SELECT id FROM outbox_events FORCE INDEX (idx_outbox_processing_claim)
WHERE status = ? AND lease_expires_at IS NOT NULL AND lease_expires_at <= ?
ORDER BY lease_expires_at ASC, id ASC
LIMIT ? FOR UPDATE SKIP LOCKED`,
			StatusProcessing,
			now,
			limit,
		).Scan(&rows).Error
		if err != nil {
			return nil, err
		}
		return loadClaimedEventsByID(tx, rows)
	}

	var events []Event
	err := tx.
		Where("status = ? AND lease_expires_at IS NOT NULL AND lease_expires_at <= ?", StatusProcessing, now).
		Order("lease_expires_at ASC").
		Order("id ASC").
		Limit(limit).
		Find(&events).Error
	return events, err
}

func loadClaimedEventsByID(tx *gorm.DB, rows []claimIDRow) ([]Event, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	ids := make([]uint, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}

	var loaded []Event
	if err := tx.Where("id IN ?", ids).Find(&loaded).Error; err != nil {
		return nil, err
	}
	byID := make(map[uint]Event, len(loaded))
	for _, event := range loaded {
		byID[event.ID] = event
	}

	events := make([]Event, 0, len(rows))
	for _, row := range rows {
		event, ok := byID[row.ID]
		if ok {
			events = append(events, event)
		}
	}
	return events, nil
}

func (r *GormRepository) MarkPublished(ctx context.Context, id uint, workerID string, publishedAt time.Time) error {
	if workerID == "" {
		return ErrLeaseNotOwned
	}
	result := r.db.WithContext(ctx).
		Model(&Event{}).
		Where("id = ? AND status = ? AND locked_by = ? AND lease_expires_at > ?", id, StatusProcessing, workerID, publishedAt).
		Updates(map[string]interface{}{
			"status":           StatusPublished,
			"published_at":     publishedAt,
			"last_error":       "",
			"locked_by":        "",
			"locked_at":        nil,
			"lease_expires_at": nil,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrLeaseNotOwned
	}
	return nil
}

func (r *GormRepository) MarkRetry(ctx context.Context, id uint, workerID string, now time.Time, update RetryUpdate) error {
	if workerID == "" {
		return ErrLeaseNotOwned
	}
	status := StatusPending
	if update.MarkAsFailed {
		status = StatusFailed
	}

	result := r.db.WithContext(ctx).
		Model(&Event{}).
		Where("id = ? AND status = ? AND locked_by = ? AND lease_expires_at > ?", id, StatusProcessing, workerID, now).
		Updates(map[string]interface{}{
			"status":           status,
			"retry_count":      update.RetryCount,
			"next_retry_at":    update.NextRetryAt,
			"last_error":       update.LastError,
			"locked_by":        "",
			"locked_at":        nil,
			"lease_expires_at": nil,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrLeaseNotOwned
	}
	return nil
}

func (r *GormRepository) supportsSkipLocked() bool {
	return r.db != nil && r.db.Dialector != nil && r.db.Dialector.Name() == "mysql"
}
