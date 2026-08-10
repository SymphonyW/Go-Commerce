package observability

import (
	"context"
	"testing"
	"time"

	"github.com/streadway/amqp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestUnaryClientInterceptorPropagatesRequestMetadata(t *testing.T) {
	ctx := WithUserID(WithTraceID(WithRequestID(context.Background(), "req-123"), "trace-123"), 42)

	var captured metadata.MD
	invoker := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		var ok bool
		captured, ok = metadata.FromOutgoingContext(ctx)
		if !ok {
			t.Fatal("expected outgoing metadata to be present")
		}
		return nil
	}

	if err := UnaryClientInterceptor()(ctx, "/order.OrderService/CreateOrder", nil, nil, nil, invoker); err != nil {
		t.Fatalf("interceptor returned error: %v", err)
	}

	if got, want := captured.Get(RequestIDMetadataKey), []string{"req-123"}; len(got) != 1 || got[0] != want[0] {
		t.Fatalf("unexpected request id metadata: got %v want %v", got, want)
	}
	if got, want := captured.Get(TraceIDMetadataKey), []string{"trace-123"}; len(got) != 1 || got[0] != want[0] {
		t.Fatalf("unexpected trace id metadata: got %v want %v", got, want)
	}
	if got, want := captured.Get(UserIDMetadataKey), []string{"42"}; len(got) != 1 || got[0] != want[0] {
		t.Fatalf("unexpected user id metadata: got %v want %v", got, want)
	}
}

func TestUnaryClientTimeoutInterceptorAddsDeadlineWhenMissing(t *testing.T) {
	var sawDeadline bool
	invoker := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		_, sawDeadline = ctx.Deadline()
		return nil
	}

	if err := UnaryClientTimeoutInterceptor(time.Second)(context.Background(), "/demo.Service/Call", nil, nil, nil, invoker); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sawDeadline {
		t.Fatal("expected interceptor to add a deadline")
	}
}

func TestContextFromIncomingMetadataExtractsCorrelationFields(t *testing.T) {
	incoming := metadata.New(map[string]string{
		RequestIDMetadataKey: "req-456",
		TraceIDMetadataKey:   "trace-456",
		UserIDMetadataKey:    "7",
	})

	ctx := ContextFromIncomingMetadata(metadata.NewIncomingContext(context.Background(), incoming))

	if got, want := RequestIDFromContext(ctx), "req-456"; got != want {
		t.Fatalf("unexpected request id: got %q want %q", got, want)
	}
	if got, want := TraceIDFromContext(ctx), "trace-456"; got != want {
		t.Fatalf("unexpected trace id: got %q want %q", got, want)
	}
	if got, ok := UserIDFromContext(ctx); !ok || got != 7 {
		t.Fatalf("unexpected user id: got %d ok=%v want 7 true", got, ok)
	}
}

func TestTraceContextPropagatesAcrossGRPCMetadata(t *testing.T) {
	setupTraceTest(t)

	ctx, parent := Tracer().Start(context.Background(), "parent")
	defer parent.End()
	parentTraceID := parent.SpanContext().TraceID()

	var captured metadata.MD
	invoker := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		var ok bool
		captured, ok = metadata.FromOutgoingContext(ctx)
		if !ok {
			t.Fatal("expected outgoing metadata to be present")
		}
		return nil
	}

	if err := UnaryClientInterceptor()(ctx, "/order.OrderService/CreateOrder", nil, nil, nil, invoker); err != nil {
		t.Fatalf("interceptor returned error: %v", err)
	}

	extracted := ExtractFromMetadata(metadata.NewIncomingContext(context.Background(), captured))
	if got := oteltrace.SpanContextFromContext(extracted).TraceID(); got != parentTraceID {
		t.Fatalf("unexpected propagated trace id: got %s want %s", got, parentTraceID)
	}
}

func TestTraceContextPropagatesAcrossAMQPHeaders(t *testing.T) {
	setupTraceTest(t)

	ctx, parent := Tracer().Start(context.Background(), "parent")
	defer parent.End()
	parentTraceID := parent.SpanContext().TraceID()

	headers := InjectIntoAMQP(WithRequestID(ctx, "req-1"), amqp.Table{})
	delivery := amqp.Delivery{Headers: headers, CorrelationId: "req-1"}

	extracted := ContextFromAMQPDelivery(context.Background(), delivery)
	if got := oteltrace.SpanContextFromContext(extracted).TraceID(); got != parentTraceID {
		t.Fatalf("unexpected propagated trace id: got %s want %s", got, parentTraceID)
	}
	if got, want := RequestIDFromContext(extracted), "req-1"; got != want {
		t.Fatalf("unexpected propagated request id: got %q want %q", got, want)
	}
}

func setupTraceTest(t *testing.T) {
	t.Helper()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
		otel.SetTracerProvider(oteltrace.NewNoopTracerProvider())
		otel.SetTextMapPropagator(propagation.TraceContext{})
	})
}
