package inbox

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type inboxMutation struct {
	ID    uint `gorm:"primaryKey"`
	Value string
}

func newInboxTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	if err := db.AutoMigrate(&ConsumedEvent{}, &inboxMutation{}); err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}
	return db
}

func TestProcessOnceExecutesHandlerAndRecordsEvent(t *testing.T) {
	db := newInboxTestDB(t)
	calls := 0

	processed, err := ProcessOnce(context.Background(), db, "consumer-a", "evt-1", "event.type", func(tx *gorm.DB) error {
		calls++
		return tx.Create(&inboxMutation{Value: "created"}).Error
	})
	if err != nil {
		t.Fatalf("ProcessOnce returned error: %v", err)
	}
	if !processed {
		t.Fatal("expected first event to be processed")
	}
	if got, want := calls, 1; got != want {
		t.Fatalf("unexpected handler calls: got %d want %d", got, want)
	}
	assertConsumedEventCount(t, db, "consumer-a", "evt-1", 1)
	assertInboxMutationCount(t, db, 1)
}

func TestProcessOnceSkipsDuplicateEvent(t *testing.T) {
	db := newInboxTestDB(t)
	if processed, err := ProcessOnce(context.Background(), db, "consumer-a", "evt-1", "event.type", nil); err != nil {
		t.Fatalf("first ProcessOnce returned error: %v", err)
	} else if !processed {
		t.Fatal("expected first event to be processed")
	}

	calls := 0
	processed, err := ProcessOnce(context.Background(), db, "consumer-a", "evt-1", "event.type", func(tx *gorm.DB) error {
		calls++
		return tx.Create(&inboxMutation{Value: "duplicate"}).Error
	})
	if err != nil {
		t.Fatalf("duplicate ProcessOnce returned error: %v", err)
	}
	if processed {
		t.Fatal("expected duplicate event to be skipped")
	}
	if got, want := calls, 0; got != want {
		t.Fatalf("unexpected handler calls: got %d want %d", got, want)
	}
	assertConsumedEventCount(t, db, "consumer-a", "evt-1", 1)
	assertInboxMutationCount(t, db, 0)
}

func TestProcessOnceAllowsDifferentConsumersForSameEvent(t *testing.T) {
	db := newInboxTestDB(t)
	for _, consumer := range []string{"consumer-a", "consumer-b"} {
		processed, err := ProcessOnce(context.Background(), db, consumer, "evt-1", "event.type", nil)
		if err != nil {
			t.Fatalf("ProcessOnce for %s returned error: %v", consumer, err)
		}
		if !processed {
			t.Fatalf("expected %s to process event", consumer)
		}
	}
	assertConsumedEventCount(t, db, "consumer-a", "evt-1", 1)
	assertConsumedEventCount(t, db, "consumer-b", "evt-1", 1)
}

func TestProcessOnceRollsBackInboxRecordWhenHandlerFails(t *testing.T) {
	db := newInboxTestDB(t)
	handlerErr := errors.New("handler failed")

	processed, err := ProcessOnce(context.Background(), db, "consumer-a", "evt-1", "event.type", func(tx *gorm.DB) error {
		if err := tx.Create(&inboxMutation{Value: "rolled-back"}).Error; err != nil {
			return err
		}
		return handlerErr
	})
	if !errors.Is(err, handlerErr) {
		t.Fatalf("expected handler error, got %v", err)
	}
	if processed {
		t.Fatal("expected failed handler to report processed=false")
	}
	assertConsumedEventCount(t, db, "consumer-a", "evt-1", 0)
	assertInboxMutationCount(t, db, 0)
}

func TestProcessOnceDoesNotRunHandlerWhenInboxInsertFails(t *testing.T) {
	db := newInboxTestDB(t)
	if err := db.Exec(`
		CREATE TRIGGER fail_consumed_event_insert
		BEFORE INSERT ON consumed_events
		BEGIN
			SELECT RAISE(FAIL, 'forced inbox insert failure');
		END;
	`).Error; err != nil {
		t.Fatalf("failed to create trigger: %v", err)
	}

	calls := 0
	processed, err := ProcessOnce(context.Background(), db, "consumer-a", "evt-1", "event.type", func(tx *gorm.DB) error {
		calls++
		return tx.Create(&inboxMutation{Value: "should-not-run"}).Error
	})
	if err == nil {
		t.Fatal("expected inbox insert failure")
	}
	if processed {
		t.Fatal("expected failed inbox insert to report processed=false")
	}
	if got, want := calls, 0; got != want {
		t.Fatalf("unexpected handler calls: got %d want %d", got, want)
	}
	assertInboxMutationCount(t, db, 0)
}

func TestProcessOnceRollsBackBusinessWhenFinalInboxWriteFails(t *testing.T) {
	db := newInboxTestDB(t)
	if err := db.Exec(`
		CREATE TRIGGER fail_consumed_event_update
		BEFORE UPDATE ON consumed_events
		BEGIN
			SELECT RAISE(FAIL, 'forced inbox update failure');
		END;
	`).Error; err != nil {
		t.Fatalf("failed to create trigger: %v", err)
	}

	calls := 0
	processed, err := ProcessOnce(context.Background(), db, "consumer-a", "evt-1", "event.type", func(tx *gorm.DB) error {
		calls++
		return tx.Create(&inboxMutation{Value: "should-roll-back"}).Error
	})
	if err == nil {
		t.Fatal("expected inbox update failure")
	}
	if processed {
		t.Fatal("expected final inbox write failure to report processed=false")
	}
	if got, want := calls, 1; got != want {
		t.Fatalf("unexpected handler calls: got %d want %d", got, want)
	}
	assertConsumedEventCount(t, db, "consumer-a", "evt-1", 0)
	assertInboxMutationCount(t, db, 0)
}

func TestProcessOnceRejectsMissingEventID(t *testing.T) {
	db := newInboxTestDB(t)
	processed, err := ProcessOnce(context.Background(), db, "consumer-a", "", "event.type", nil)
	if !errors.Is(err, ErrMissingEventID) {
		t.Fatalf("expected ErrMissingEventID, got %v", err)
	}
	if processed {
		t.Fatal("expected missing event id to report processed=false")
	}
}

func assertConsumedEventCount(t *testing.T, db *gorm.DB, consumerName, eventID string, want int64) {
	t.Helper()

	var count int64
	if err := db.Model(&ConsumedEvent{}).
		Where("consumer_name = ? AND event_id = ?", consumerName, eventID).
		Count(&count).Error; err != nil {
		t.Fatalf("failed to count consumed events: %v", err)
	}
	if count != want {
		t.Fatalf("unexpected consumed event count: got %d want %d", count, want)
	}
}

func assertInboxMutationCount(t *testing.T, db *gorm.DB, want int64) {
	t.Helper()

	var count int64
	if err := db.Model(&inboxMutation{}).Count(&count).Error; err != nil {
		t.Fatalf("failed to count inbox mutations: %v", err)
	}
	if count != want {
		t.Fatalf("unexpected mutation count: got %d want %d", count, want)
	}
}

func TestConsumedEventTableName(t *testing.T) {
	if got, want := (ConsumedEvent{}).TableName(), "consumed_events"; got != want {
		t.Fatalf("unexpected table name: got %q want %q", got, want)
	}
}
