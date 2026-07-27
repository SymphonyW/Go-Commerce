package inbox

import (
	"context"
	"errors"
	"strings"
	"time"

	gomysql "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

var (
	ErrMissingConsumerName = errors.New("inbox consumer name is required")
	ErrMissingEventID      = errors.New("inbox event id is required")
	ErrMissingEventType    = errors.New("inbox event type is required")
	ErrMissingDB           = errors.New("inbox database handle is required")
)

func ProcessOnce(
	ctx context.Context,
	db *gorm.DB,
	consumerName string,
	eventID string,
	eventType string,
	handler func(tx *gorm.DB) error,
) (processed bool, err error) {
	consumerName = strings.TrimSpace(consumerName)
	eventID = strings.TrimSpace(eventID)
	eventType = strings.TrimSpace(eventType)
	if consumerName == "" {
		return false, ErrMissingConsumerName
	}
	if eventID == "" {
		return false, ErrMissingEventID
	}
	if eventType == "" {
		return false, ErrMissingEventType
	}
	if db == nil {
		return false, ErrMissingDB
	}

	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		record := ConsumedEvent{
			ConsumerName: consumerName,
			EventID:      eventID,
			EventType:    eventType,
			ConsumedAt:   time.Now().UTC(),
		}
		if err := tx.Create(&record).Error; err != nil {
			if IsDuplicateConsume(err) {
				processed = false
				return nil
			}
			return err
		}

		if handler != nil {
			if err := handler(tx); err != nil {
				return err
			}
		}

		// Keep the reservation insert before the handler for dedupe, then do a
		// final inbox write so late persistence errors roll back business changes.
		if err := tx.Model(&ConsumedEvent{}).
			Where("id = ?", record.ID).
			Update("consumed_at", time.Now().UTC()).Error; err != nil {
			return err
		}
		processed = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return processed, nil
}

func IsDuplicateConsume(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}

	var mysqlErr *gomysql.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		return true
	}

	message := err.Error()
	return strings.Contains(message, "Duplicate entry") ||
		strings.Contains(message, "UNIQUE constraint failed") ||
		strings.Contains(message, "constraint failed: UNIQUE")
}
