package outbox

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"go-commerce/pkg/mq"
)

// Worker 负责把已提交的本地事件异步投递到 RabbitMQ。
// 业务事务只承担“写事件”的责任，真正的网络 IO 留给这里重试。
type Worker struct {
	repo      EventRepository
	publisher mq.Publisher
	config    Config
	logger    *log.Logger
	now       func() time.Time
}

func NewWorker(repo EventRepository, publisher mq.Publisher, config Config, logger *log.Logger) *Worker {
	if publisher == nil {
		publisher = mq.NopPublisher{}
	}
	if logger == nil {
		logger = log.Default()
	}
	return &Worker{
		repo:      repo,
		publisher: publisher,
		config:    config.withDefaults(),
		logger:    logger,
		now:       time.Now,
	}
}

func (w *Worker) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.config.PollInterval)
	defer ticker.Stop()

	for {
		if err := w.ProcessOnce(ctx); err != nil {
			w.logger.Printf("outbox_process_failed error=%v", err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (w *Worker) ProcessOnce(ctx context.Context) error {
	now := w.now()
	pendingEvents, err := w.repo.ListDuePending(ctx, now, w.config.BatchSize)
	if err != nil {
		return err
	}

	for _, event := range pendingEvents {
		raw := mq.RawEvent{
			EventID: event.EventID,
			Body:    json.RawMessage(event.Payload),
		}
		if err := w.publisher.Publish(ctx, event.EventType, raw); err != nil {
			retryCount := event.RetryCount + 1
			markAsFailed := retryCount >= w.config.MaxRetry
			nextRetryAt := now.Add(w.config.RetryDelay(retryCount))
			if markErr := w.repo.MarkRetry(ctx, event.ID, RetryUpdate{
				RetryCount:   retryCount,
				NextRetryAt:  nextRetryAt,
				LastError:    err.Error(),
				MarkAsFailed: markAsFailed,
			}); markErr != nil {
				return markErr
			}
			if markAsFailed {
				w.logger.Printf(
					"outbox_event_failed event_id=%s event_type=%s retry_count=%d error=%v",
					event.EventID,
					event.EventType,
					retryCount,
					err,
				)
			} else {
				w.logger.Printf(
					"outbox_event_retry_scheduled event_id=%s event_type=%s retry_count=%d next_retry_at=%s error=%v",
					event.EventID,
					event.EventType,
					retryCount,
					nextRetryAt.UTC().Format(time.RFC3339Nano),
					err,
				)
			}
			continue
		}

		if err := w.repo.MarkPublished(ctx, event.ID, now); err != nil {
			return err
		}
		w.logger.Printf(
			"outbox_event_published event_id=%s event_type=%s aggregate_type=%s aggregate_id=%s",
			event.EventID,
			event.EventType,
			event.AggregateType,
			event.AggregateID,
		)
	}

	return nil
}
