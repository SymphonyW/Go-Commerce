package middleware

import (
	"context"

	"github.com/gin-gonic/gin"

	"go-commerce/pkg/observability"
)

const (
	RequestIDContextKey = "request_id"
	TraceIDContextKey   = "trace_id"
)

func RequestContext() gin.HandlerFunc {
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
		c.Set(RequestIDContextKey, requestID)
		c.Set(TraceIDContextKey, traceID)
		c.Writer.Header().Set(observability.RequestIDHeader, requestID)
		c.Writer.Header().Set(observability.TraceIDHeader, traceID)
		c.Next()
	}
}

func GatewayContext(c *gin.Context) context.Context {
	ctx := c.Request.Context()
	if userID, ok := CurrentUserID(c); ok {
		ctx = observability.WithUserID(ctx, userID)
		c.Request = c.Request.WithContext(ctx)
	}
	return ctx
}

func CurrentUserID(c *gin.Context) (int64, bool) {
	value, exists := c.Get("user_id")
	if !exists {
		return 0, false
	}
	userID, ok := value.(int64)
	return userID, ok
}
