//go:build integration

package integration_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go-commerce/internal/inbox"

	"gorm.io/gorm"
)

func TestMySQLInboxAutoMigrateCanRunRepeatedly(t *testing.T) {
	db := openIntegrationDB(t)

	if err := db.AutoMigrate(&inbox.ConsumedEvent{}); err != nil {
		t.Fatalf("first inbox migration failed: %v", err)
	}
	if err := db.AutoMigrate(&inbox.ConsumedEvent{}); err != nil {
		t.Fatalf("second inbox migration failed: %v", err)
	}
}

func TestMySQLInboxConcurrentDuplicateEventRunsHandlerOnce(t *testing.T) {
	db := openMigratedInboxDB(t)
	ctx := context.Background()

	start := make(chan struct{})
	errs := make(chan error, 20)
	processed := make(chan bool, 20)
	var handlerCalls int32
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			didProcess, err := inbox.ProcessOnce(ctx, db, "mysql-consumer", "evt-concurrent-1", "event.type", func(tx *gorm.DB) error {
				atomic.AddInt32(&handlerCalls, 1)
				time.Sleep(25 * time.Millisecond)
				return nil
			})
			errs <- err
			processed <- didProcess
		}()
	}

	close(start)
	wg.Wait()
	close(errs)
	close(processed)

	for err := range errs {
		if err != nil {
			t.Fatalf("ProcessOnce returned error: %v", err)
		}
	}

	processedCount := 0
	for didProcess := range processed {
		if didProcess {
			processedCount++
		}
	}
	if got, want := processedCount, 1; got != want {
		t.Fatalf("unexpected processed count: got %d want %d", got, want)
	}
	if got, want := atomic.LoadInt32(&handlerCalls), int32(1); got != want {
		t.Fatalf("unexpected handler call count: got %d want %d", got, want)
	}

	var count int64
	if err := db.Model(&inbox.ConsumedEvent{}).
		Where("consumer_name = ? AND event_id = ?", "mysql-consumer", "evt-concurrent-1").
		Count(&count).Error; err != nil {
		t.Fatalf("failed to count consumed events: %v", err)
	}
	if got, want := count, int64(1); got != want {
		t.Fatalf("unexpected consumed event count: got %d want %d", got, want)
	}
}

func openMigratedInboxDB(t *testing.T) *gorm.DB {
	t.Helper()

	db := openIntegrationDB(t)
	if err := db.AutoMigrate(&inbox.ConsumedEvent{}); err != nil {
		t.Fatalf("failed to migrate inbox schema: %v", err)
	}
	return db
}
