package observability

import (
	"context"
	"strconv"
	"strings"

	"google.golang.org/grpc/metadata"
)

const (
	RequestIDHeader      = "X-Request-ID"
	TraceIDHeader        = "X-Trace-ID"
	RequestIDMetadataKey = "x-request-id"
	TraceIDMetadataKey   = "x-trace-id"
	UserIDMetadataKey    = "x-user-id"
)

type contextKey string

const (
	requestIDKey contextKey = "request_id"
	traceIDKey   contextKey = "trace_id"
	spanIDKey    contextKey = "span_id"
	userIDKey    contextKey = "user_id"
)

func WithRequestID(ctx context.Context, requestID string) context.Context {
	if strings.TrimSpace(requestID) == "" {
		return ctx
	}
	return context.WithValue(ctx, requestIDKey, strings.TrimSpace(requestID))
}

func RequestIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey).(string)
	return value
}

func WithTraceID(ctx context.Context, traceID string) context.Context {
	if strings.TrimSpace(traceID) == "" {
		return ctx
	}
	return context.WithValue(ctx, traceIDKey, strings.TrimSpace(traceID))
}

func TraceIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(traceIDKey).(string)
	return value
}

func WithSpanID(ctx context.Context, spanID string) context.Context {
	if strings.TrimSpace(spanID) == "" {
		return ctx
	}
	return context.WithValue(ctx, spanIDKey, strings.TrimSpace(spanID))
}

func SpanIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(spanIDKey).(string)
	return value
}

func WithUserID(ctx context.Context, userID int64) context.Context {
	if userID <= 0 {
		return ctx
	}
	return context.WithValue(ctx, userIDKey, userID)
}

func UserIDFromContext(ctx context.Context) (int64, bool) {
	value, ok := ctx.Value(userIDKey).(int64)
	return value, ok
}

// ContextFromIncomingMetadata 把 gRPC metadata 中的关联字段装回 context，
// 这样服务端日志、下游 gRPC 调用和业务事件可以继续复用同一组 ID。
func ContextFromIncomingMetadata(ctx context.Context) context.Context {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ctx
	}

	if values := md.Get(RequestIDMetadataKey); len(values) > 0 {
		ctx = WithRequestID(ctx, values[0])
	}
	if values := md.Get(TraceIDMetadataKey); len(values) > 0 {
		ctx = WithTraceID(ctx, values[0])
	}
	if values := md.Get(UserIDMetadataKey); len(values) > 0 {
		if userID, err := strconv.ParseInt(values[0], 10, 64); err == nil {
			ctx = WithUserID(ctx, userID)
		}
	}
	return ctx
}

// WithOutgoingMetadata 统一把关联字段附着到 gRPC 出站请求上。
func WithOutgoingMetadata(ctx context.Context) context.Context {
	if requestID := RequestIDFromContext(ctx); requestID != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, RequestIDMetadataKey, requestID)
	}
	if traceID := TraceIDFromContext(ctx); traceID != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, TraceIDMetadataKey, traceID)
	}
	if userID, ok := UserIDFromContext(ctx); ok {
		ctx = metadata.AppendToOutgoingContext(ctx, UserIDMetadataKey, strconv.FormatInt(userID, 10))
	}
	return ctx
}
