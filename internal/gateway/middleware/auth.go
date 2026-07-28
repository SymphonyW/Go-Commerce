package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"

	"go-commerce/internal/gateway/response"
	"go-commerce/pkg/jwt"
)

func Auth() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			response.Unauthorized(c, "authorization header required")
			c.Abort()
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			response.Unauthorized(c, "invalid authorization format")
			c.Abort()
			return
		}

		claims, err := jwt.ValidateToken(parts[1])
		if err != nil {
			response.Unauthorized(c, "invalid token")
			c.Abort()
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("role", claims.Role)
		c.Next()
	}
}

func RequireRole(allowedRoles ...string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(allowedRoles))
	for _, role := range allowedRoles {
		allowed[role] = struct{}{}
	}

	return func(c *gin.Context) {
		roleValue, exists := c.Get("role")
		if !exists {
			response.Forbidden(c, "forbidden")
			c.Abort()
			return
		}

		role, ok := roleValue.(string)
		if !ok {
			response.Forbidden(c, "forbidden")
			c.Abort()
			return
		}

		if _, ok := allowed[role]; !ok {
			response.Forbidden(c, "forbidden")
			c.Abort()
			return
		}

		c.Next()
	}
}
