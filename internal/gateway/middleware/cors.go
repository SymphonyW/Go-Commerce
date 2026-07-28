package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const DefaultAllowedOrigin = "http://localhost:5173"

func ParseAllowedOrigins(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{DefaultAllowedOrigin}
	}

	origins := make([]string, 0)
	seen := make(map[string]struct{})
	for _, part := range strings.Split(raw, ",") {
		origin := strings.TrimSpace(part)
		if origin == "" || origin == "*" {
			continue
		}
		if _, exists := seen[origin]; exists {
			continue
		}
		seen[origin] = struct{}{}
		origins = append(origins, origin)
	}
	if len(origins) == 0 {
		return []string{DefaultAllowedOrigin}
	}
	return origins
}

func CORS(allowedOrigins []string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		origin = strings.TrimSpace(origin)
		if origin != "" && origin != "*" {
			allowed[origin] = struct{}{}
		}
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		_, originAllowed := allowed[origin]
		if origin != "" && originAllowed {
			headers := c.Writer.Header()
			headers.Set("Access-Control-Allow-Origin", origin)
			headers.Set("Access-Control-Allow-Credentials", "true")
			headers.Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, Idempotency-Key, accept, origin, Cache-Control, X-Requested-With, X-Request-ID, X-Trace-ID")
			headers.Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")
			headers.Add("Vary", "Origin")
		}

		if c.Request.Method == http.MethodOptions {
			if origin != "" && !originAllowed {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
