package main

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"go-commerce/pkg/observability"
)

const (
	requestIDContextKey = "request_id"
	traceIDContextKey   = "trace_id"
)

// requestContextMiddleware 负责在 HTTP 入口处建立整条链路共用的关联 ID。
func requestContextMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader(observability.RequestIDHeader)
		if requestID == "" {
			requestID = observability.NewRequestID()
		}

		traceID := c.GetHeader(observability.TraceIDHeader)
		if traceID == "" {
			traceID = requestID
		}

		ctx := observability.WithTraceID(observability.WithRequestID(c.Request.Context(), requestID), traceID)
		c.Request = c.Request.WithContext(ctx)
		c.Set(requestIDContextKey, requestID)
		c.Set(traceIDContextKey, traceID)
		c.Writer.Header().Set(observability.RequestIDHeader, requestID)
		c.Writer.Header().Set(observability.TraceIDHeader, traceID)
		c.Next()
	}
}

func gatewayContext(c *gin.Context) context.Context {
	ctx := c.Request.Context()
	if userID, ok := currentUserID(c); ok {
		ctx = observability.WithUserID(ctx, userID)
		c.Request = c.Request.WithContext(ctx)
	}
	return ctx
}

func currentUserID(c *gin.Context) (int64, bool) {
	value, exists := c.Get("user_id")
	if !exists {
		return 0, false
	}
	userID, ok := value.(int64)
	return userID, ok
}

func httpMetricsMiddleware(metrics *observability.Metrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		startedAt := time.Now()
		c.Next()

		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}
		metrics.ObserveHTTP(c.Request.Method, path, strconv.Itoa(c.Writer.Status()), time.Since(startedAt))
	}
}

func httpLoggingMiddleware(logger *slog.Logger) gin.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}
	return func(c *gin.Context) {
		startedAt := time.Now()
		c.Next()

		ctx := gatewayContext(c)
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
