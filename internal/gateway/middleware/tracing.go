package middleware

import (
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"go-commerce/pkg/observability"
)

func Tracing(service string) gin.HandlerFunc {
	if service == "" {
		service = "api-gateway"
	}
	return func(c *gin.Context) {
		ctx := otel.GetTextMapPropagator().Extract(c.Request.Context(), propagation.HeaderCarrier(c.Request.Header))
		spanName := c.Request.Method + " " + c.Request.URL.Path
		ctx, span := observability.StartSpan(ctx,
			spanName,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("service.name", service),
				attribute.String("http.method", c.Request.Method),
				attribute.String("url.path", c.Request.URL.Path),
			),
		)
		ctx = observability.WithSpanContext(ctx, span.SpanContext())
		c.Request = c.Request.WithContext(ctx)
		if spanContext := span.SpanContext(); spanContext.IsValid() {
			traceID := spanContext.TraceID().String()
			c.Set(TraceIDContextKey, traceID)
			c.Writer.Header().Set(observability.TraceIDHeader, traceID)
		}

		c.Next()

		route := c.FullPath()
		if route == "" {
			route = c.Request.URL.Path
		}
		span.SetAttributes(
			attribute.String("http.route", route),
			attribute.Int("http.status_code", c.Writer.Status()),
		)
		if len(c.Errors) > 0 {
			span.RecordError(fmt.Errorf("%s", c.Errors.String()))
		}
		if status := c.Writer.Status(); status >= 500 {
			span.SetAttributes(attribute.String("error.type", strconv.Itoa(status)))
		}
		span.End()
	}
}
