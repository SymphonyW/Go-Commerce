package observability

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
)

// NewLogger 为每个服务创建统一的 JSON logger。
// time 字段显式改名为 timestamp，便于日志平台直接索引。
func NewLogger(service string) *slog.Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
		ReplaceAttr: func(groups []string, attr slog.Attr) slog.Attr {
			if attr.Key == slog.TimeKey {
				attr.Key = "timestamp"
			}
			return attr
		},
	})
	return slog.New(handler).With("service", service)
}

func ContextAttrs(ctx context.Context) []slog.Attr {
	attrs := make([]slog.Attr, 0, 4)
	if requestID := RequestIDFromContext(ctx); requestID != "" {
		attrs = append(attrs, slog.String("request_id", requestID))
	}
	if traceID := TraceIDFromContext(ctx); traceID != "" {
		attrs = append(attrs, slog.String("trace_id", traceID))
	}
	if spanID := SpanIDFromContext(ctx); spanID != "" {
		attrs = append(attrs, slog.String("span_id", spanID))
	}
	if userID, ok := UserIDFromContext(ctx); ok {
		attrs = append(attrs, slog.Int64("user_id", userID))
	}
	return attrs
}

func NewLogWriter(logger *slog.Logger) io.Writer {
	if logger == nil {
		logger = slog.Default()
	}
	return logWriter{logger: logger}
}

type logWriter struct {
	logger *slog.Logger
}

func (w logWriter) Write(p []byte) (int, error) {
	message := strings.TrimSpace(string(p))
	if message != "" {
		w.logger.Info("legacy_log", "message", message)
	}
	return len(p), nil
}
