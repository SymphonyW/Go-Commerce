package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"

	"go-commerce/pkg/observability"
)

func Logging(logger *slog.Logger) gin.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}
	return func(c *gin.Context) {
		startedAt := time.Now()
		c.Next()

		ctx := GatewayContext(c)
		attrs := append(observability.ContextAttrs(ctx),
			slog.String("path", c.FullPath()),
			slog.String("method", c.Request.Method),
			slog.Int("status", c.Writer.Status()),
			slog.Duration("duration", time.Since(startedAt)),
		)
		if len(c.Errors) > 0 {
			attrs = append(attrs, slog.Any("error", c.Errors.Last().Err))
			logger.LogAttrs(ctx, slog.LevelError, "http_request_failed", attrs...)
			return
		}
		logger.LogAttrs(ctx, slog.LevelInfo, "http_request_completed", attrs...)
	}
}
