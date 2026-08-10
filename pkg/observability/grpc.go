package observability

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

func UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		service, rpcMethod := splitFullMethod(method)
		ctx, span := StartSpan(ctx,
			"grpc.client "+method,
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(
				attribute.String("rpc.system", "grpc"),
				attribute.String("rpc.service", service),
				attribute.String("rpc.method", rpcMethod),
			),
		)
		ctx = InjectIntoMetadata(WithOutgoingMetadata(ctx))
		err := invoker(ctx, method, req, reply, cc, opts...)
		if err != nil {
			span.SetAttributes(attribute.String("rpc.grpc.status_code", status.Code(err).String()))
		}
		EndSpan(span, err)
		return err
	}
}

// UnaryClientTimeoutInterceptor 为未显式设置 deadline 的调用补上一层默认超时。
// 这样可以避免网关或服务间调用因为下游异常而无限悬挂。
func UnaryClientTimeoutInterceptor(timeout time.Duration) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if timeout <= 0 {
			return invoker(ctx, method, req, reply, cc, opts...)
		}
		if _, ok := ctx.Deadline(); ok {
			return invoker(ctx, method, req, reply, cc, opts...)
		}
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

func UnaryServerInterceptor(logger *slog.Logger, metrics *Metrics) grpc.UnaryServerInterceptor {
	if logger == nil {
		logger = slog.Default()
	}
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		ctx = ContextFromIncomingMetadata(ExtractFromMetadata(ctx))
		service, rpcMethod := splitFullMethod(info.FullMethod)
		ctx, span := StartSpan(ctx,
			"grpc.server "+info.FullMethod,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("rpc.system", "grpc"),
				attribute.String("rpc.service", service),
				attribute.String("rpc.method", rpcMethod),
			),
		)
		startedAt := time.Now()
		resp, err := handler(ctx, req)
		duration := time.Since(startedAt)
		code := status.Code(err).String()
		span.SetAttributes(attribute.String("rpc.grpc.status_code", code))
		EndSpan(span, err)
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

func splitFullMethod(fullMethod string) (service, method string) {
	trimmed := strings.TrimPrefix(fullMethod, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) != 2 {
		return trimmed, ""
	}
	return parts[0], parts[1]
}
