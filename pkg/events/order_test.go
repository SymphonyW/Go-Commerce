package events

import (
	"context"
	"testing"
	"time"

	"go-commerce/pkg/observability"
)

func TestNewBaseEventWithContextCarriesCorrelationIDs(t *testing.T) {
	ctx := observability.WithTraceID(observability.WithRequestID(context.Background(), "req-1"), "trace-1")
	event := NewBaseEventWithContext(ctx, OrderCreatedType, time.Date(2026, time.May, 16, 8, 0, 0, 0, time.UTC))

	if got, want := event.RequestID, "req-1"; got != want {
		t.Fatalf("unexpected request id: got %q want %q", got, want)
	}
	if got, want := event.TraceID, "trace-1"; got != want {
		t.Fatalf("unexpected trace id: got %q want %q", got, want)
	}
}
