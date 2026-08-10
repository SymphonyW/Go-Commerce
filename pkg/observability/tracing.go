package observability

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"github.com/streadway/amqp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/metadata"
)

const tracerName = "go-commerce"

type TraceShutdown func(context.Context) error

func InitTracing(ctx context.Context, service string, logger *slog.Logger) TraceShutdown {
	if !enabled(os.Getenv("OTEL_ENABLED")) {
		otel.SetTextMapPropagator(newPropagator())
		return func(context.Context) error { return nil }
	}
	if logger == nil {
		logger = slog.Default()
	}

	endpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	if endpoint == "" {
		endpoint = "localhost:4317"
	}
	endpoint = strings.TrimPrefix(strings.TrimPrefix(endpoint, "http://"), "https://")

	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		logger.Error("otel_exporter_init_failed", "endpoint", endpoint, "error", err)
		otel.SetTextMapPropagator(newPropagator())
		return func(context.Context) error { return nil }
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(
			"",
			attribute.String("service.name", service),
		)),
	)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(newPropagator())
	logger.Info("otel_tracing_enabled", "endpoint", endpoint)
	return provider.Shutdown
}

func enabled(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on", "enabled":
		return true
	default:
		return false
	}
}

func newPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

func Tracer() trace.Tracer {
	return otel.Tracer(tracerName)
}

func StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	ctx, span := Tracer().Start(ctx, name, opts...)
	return WithSpanContext(ctx, span.SpanContext()), span
}

func WithSpanContext(ctx context.Context, spanContext trace.SpanContext) context.Context {
	if !spanContext.IsValid() {
		return ctx
	}
	ctx = WithTraceID(ctx, spanContext.TraceID().String())
	return WithSpanID(ctx, spanContext.SpanID().String())
}

func EndSpan(span trace.Span, err error) {
	if span == nil {
		return
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}

type metadataCarrier metadata.MD

func (c metadataCarrier) Get(key string) string {
	values := metadata.MD(c).Get(strings.ToLower(key))
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func (c metadataCarrier) Set(key, value string) {
	metadata.MD(c).Set(strings.ToLower(key), value)
}

func (c metadataCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for key := range c {
		keys = append(keys, key)
	}
	return keys
}

func ExtractFromMetadata(ctx context.Context) context.Context {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ctx
	}
	return otel.GetTextMapPropagator().Extract(ctx, metadataCarrier(md))
}

func InjectIntoMetadata(ctx context.Context) context.Context {
	md, _ := metadata.FromOutgoingContext(ctx)
	md = md.Copy()
	otel.GetTextMapPropagator().Inject(ctx, metadataCarrier(md))
	return metadata.NewOutgoingContext(ctx, md)
}

type AMQPCarrier amqp.Table

func (c AMQPCarrier) Get(key string) string {
	value, ok := amqp.Table(c)[strings.ToLower(key)]
	if !ok {
		value = amqp.Table(c)[key]
	}
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return ""
	}
}

func (c AMQPCarrier) Set(key, value string) {
	amqp.Table(c)[strings.ToLower(key)] = value
}

func (c AMQPCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for key := range c {
		keys = append(keys, key)
	}
	return keys
}

func ExtractFromAMQP(ctx context.Context, headers amqp.Table) context.Context {
	if headers == nil {
		headers = amqp.Table{}
	}
	return otel.GetTextMapPropagator().Extract(ctx, AMQPCarrier(headers))
}

func InjectIntoAMQP(ctx context.Context, headers amqp.Table) amqp.Table {
	if headers == nil {
		headers = amqp.Table{}
	}
	otel.GetTextMapPropagator().Inject(ctx, AMQPCarrier(headers))
	return headers
}

func ContextFromAMQPDelivery(ctx context.Context, delivery amqp.Delivery) context.Context {
	ctx = ExtractFromAMQP(ctx, delivery.Headers)
	if requestID := amqpString(delivery.Headers, RequestIDMetadataKey, "request_id"); requestID != "" {
		ctx = WithRequestID(ctx, requestID)
	} else if delivery.CorrelationId != "" {
		ctx = WithRequestID(ctx, delivery.CorrelationId)
	}
	if traceID := amqpString(delivery.Headers, TraceIDMetadataKey, "trace_id"); traceID != "" {
		ctx = WithTraceID(ctx, traceID)
	}
	if spanContext := trace.SpanContextFromContext(ctx); spanContext.IsValid() {
		ctx = WithSpanContext(ctx, spanContext)
	}
	return ctx
}

func amqpString(headers amqp.Table, keys ...string) string {
	for _, key := range keys {
		value := AMQPCarrier(headers).Get(key)
		if value != "" {
			return value
		}
	}
	return ""
}
