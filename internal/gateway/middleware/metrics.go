package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"go-commerce/pkg/observability"
)

func Metrics(metrics *observability.Metrics) gin.HandlerFunc {
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
