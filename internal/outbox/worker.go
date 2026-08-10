package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"go-commerce/pkg/mq"
	"go-commerce/pkg/observability"
)

type Worker struct {
	repo      EventRepository
	publisher mq.Publisher
	config    Config
	logger    *log.Logger
	metrics   MetricsRecorder
	now       func() time.Time
	polling   atomic.Bool
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
		metrics:   NopMetrics{},
		now:       time.Now,
	}
}

func (w *Worker) SetMetrics(metrics MetricsRecorder) {
	if metrics == nil {
		metrics = NopMetrics{}
	}
	w.metrics = metrics
}

func (w *Worker) WorkerID() string {
	if w == nil {
		return ""
	}
	return w.config.WorkerID
}

func (w *Worker) IsPolling() bool {
	return w != nil && w.polling.Load()
}

func (w *Worker) CheckPolling(context.Context) error {
	if !w.IsPolling() {
		return fmt.Errorf("outbox worker polling loop is not running")
	}
	return nil
}

func (w *Worker) Run(ctx context.Context) error {
	w.polling.Store(true)
	defer w.polling.Store(false)

	ticker := time.NewTicker(w.config.PollInterval)
	defer ticker.Stop()

	for {
		if err := w.ProcessOnce(ctx); err != nil {
			w.logger.Printf("outbox_process_failed worker_id=%s error=%v", w.config.WorkerID, err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (w *Worker) ProcessOnce(ctx context.Context) error {
	claim, err := w.repo.ClaimDueEvents(ctx, w.now(), w.config.BatchSize, w.config.WorkerID, w.config.LeaseDuration)
	if err != nil {
		return err
	}
	w.metrics.RecordClaimed(len(claim.Events))
	w.metrics.RecordLeaseRecovered(claim.LeaseRecoveredCount)

	for _, event := range claim.Events {
		requestID := extractRequestID(event.Payload)
		traceID := extractTraceID(event.Payload)
		eventCtx := observability.WithTraceID(observability.WithRequestID(ctx, requestID), traceID)
		eventCtx, span := observability.StartSpan(eventCtx,
			"outbox publish "+event.EventType,
			trace.WithSpanKind(trace.SpanKindProducer),
			trace.WithAttributes(
				attribute.String("event.id", event.EventID),
				attribute.String("event.type", event.EventType),
				attribute.String("correlation.request_id", requestID),
				attribute.String("correlation.trace_id", traceID),
				attribute.String("messaging.system", "rabbitmq"),
			),
		)
		raw := mq.RawEvent{
			EventID:   event.EventID,
			EventType: event.EventType,
			RequestID: requestID,
			TraceID:   traceID,
			Body:      json.RawMessage(event.Payload),
		}
		if err := w.publisher.Publish(eventCtx, event.EventType, raw); err != nil {
			observability.EndSpan(span, err)
			w.metrics.RecordPublishFailure()
			failedAt := w.now()
			retryCount := event.RetryCount + 1
			markAsFailed := retryCount >= w.config.MaxRetry
			nextRetryAt := failedAt.Add(w.config.RetryDelay(retryCount))
			if markErr := w.repo.MarkRetry(ctx, event.ID, w.config.WorkerID, failedAt, RetryUpdate{
				RetryCount:   retryCount,
				NextRetryAt:  nextRetryAt,
				LastError:    err.Error(),
				MarkAsFailed: markAsFailed,
			}); markErr != nil {
				return markErr
			}
			if markAsFailed {
				w.metrics.RecordFailed()
				w.logger.Printf(
					"outbox_event_failed worker_id=%s event_id=%s event_type=%s retry_count=%d error=%v",
					w.config.WorkerID,
					event.EventID,
					event.EventType,
					retryCount,
					err,
				)
			} else {
				w.metrics.RecordRetry()
				w.logger.Printf(
					"outbox_event_retry_scheduled worker_id=%s event_id=%s event_type=%s retry_count=%d next_retry_at=%s error=%v",
					w.config.WorkerID,
					event.EventID,
					event.EventType,
					retryCount,
					nextRetryAt.UTC().Format(time.RFC3339Nano),
					err,
				)
			}
			continue
		}

		publishedAt := w.now()
		if err := w.repo.MarkPublished(ctx, event.ID, w.config.WorkerID, publishedAt); err != nil {
			observability.EndSpan(span, err)
			return err
		}
		observability.EndSpan(span, nil)
		w.metrics.RecordPublished()
		w.logger.Printf(
			"outbox_event_published worker_id=%s event_id=%s event_type=%s aggregate_type=%s aggregate_id=%s",
			w.config.WorkerID,
			event.EventID,
			event.EventType,
			event.AggregateType,
			event.AggregateID,
		)
	}

	return nil
}

type eventCorrelation struct {
	RequestID string `json:"request_id"`
	TraceID   string `json:"trace_id"`
}

func extractRequestID(payload string) string {
	var event eventCorrelation
	_ = json.Unmarshal([]byte(payload), &event)
	return event.RequestID
}

func extractTraceID(payload string) string {
	var event eventCorrelation
	_ = json.Unmarshal([]byte(payload), &event)
	return event.TraceID
}
