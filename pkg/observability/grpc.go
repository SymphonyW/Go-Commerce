package observability

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

func UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		return invoker(WithOutgoingMetadata(ctx), method, req, reply, cc, opts...)
	}
}

func UnaryServerInterceptor(logger *slog.Logger, metrics *Metrics) grpc.UnaryServerInterceptor {
	if logger == nil {
		logger = slog.Default()
	}
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		ctx = ContextFromIncomingMetadata(ctx)
		startedAt := time.Now()
		resp, err := handler(ctx, req)
		duration := time.Since(startedAt)
		code := status.Code(err).String()
		metrics.ObserveGRPC(info.FullMethod, code, duration)

		attrs := append(ContextAttrs(ctx),
			slog.String("grpc_method", info.FullMethod),
			slog.String("grpc_code", code),
			slog.Duration("duration", duration),
		)
		if err != nil {
			attrs = append(attrs, slog.Any("error", err))
			logger.LogAttrs(ctx, slog.LevelError, "grpc_request_failed", attrs...)
		} else {
			logger.LogAttrs(ctx, slog.LevelInfo, "grpc_request_completed", attrs...)
		}
		return resp, err
	}
}
