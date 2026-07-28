package response

import (
	"net/http"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go-commerce/pkg/observability"
)

type ErrorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Error     string `json:"error"`
	RequestID string `json:"request_id"`
}

func BadRequest(c *gin.Context, message string) {
	WriteError(c, http.StatusBadRequest, message)
}

func Unauthorized(c *gin.Context, message string) {
	WriteError(c, http.StatusUnauthorized, message)
}

func Forbidden(c *gin.Context, message string) {
	WriteError(c, http.StatusForbidden, message)
}

func WriteGRPCError(c *gin.Context, err error) {
	grpcStatus := status.Convert(err)
	message := strings.TrimSpace(grpcStatus.Message())
	if message == "" {
		message = grpcStatus.Code().String()
	}
	WriteErrorWithCode(c, HTTPStatusFromGRPC(grpcStatus.Code()), CodeFromMessage(message, grpcStatus.Code().String()), message)
}

func WriteError(c *gin.Context, statusCode int, message string) {
	WriteErrorWithCode(c, statusCode, CodeFromMessage(message, defaultCode(statusCode)), message)
}

func WriteErrorWithCode(c *gin.Context, statusCode int, code string, message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		message = http.StatusText(statusCode)
	}
	code = strings.TrimSpace(code)
	if code == "" {
		code = defaultCode(statusCode)
	}
	c.JSON(statusCode, ErrorBody{
		Code:      code,
		Message:   message,
		Error:     message,
		RequestID: requestID(c),
	})
}

func HTTPStatusFromGRPC(code codes.Code) int {
	switch code {
	case codes.InvalidArgument:
		return http.StatusBadRequest
	case codes.Unauthenticated:
		return http.StatusUnauthorized
	case codes.PermissionDenied:
		return http.StatusForbidden
	case codes.NotFound:
		return http.StatusNotFound
	case codes.AlreadyExists, codes.FailedPrecondition:
		return http.StatusConflict
	case codes.ResourceExhausted:
		return http.StatusTooManyRequests
	case codes.DeadlineExceeded:
		return http.StatusGatewayTimeout
	case codes.Unavailable:
		return http.StatusServiceUnavailable
	case codes.Internal:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}

func CodeFromMessage(message, fallback string) string {
	var builder strings.Builder
	lastUnderscore := false
	for _, r := range strings.ToUpper(strings.TrimSpace(message)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore && builder.Len() > 0 {
			builder.WriteByte('_')
			lastUnderscore = true
		}
	}

	code := strings.Trim(builder.String(), "_")
	if code == "" {
		code = strings.ToUpper(strings.TrimSpace(fallback))
	}
	if code == "" {
		code = "ERROR"
	}
	return code
}

func requestID(c *gin.Context) string {
	if requestID := c.GetString("request_id"); requestID != "" {
		return requestID
	}
	if c.Request != nil {
		if requestID := observability.RequestIDFromContext(c.Request.Context()); requestID != "" {
			return requestID
		}
		if requestID := c.GetHeader(observability.RequestIDHeader); requestID != "" {
			return requestID
		}
	}
	return c.Writer.Header().Get(observability.RequestIDHeader)
}

func defaultCode(statusCode int) string {
	switch statusCode {
	case http.StatusBadRequest:
		return "BAD_REQUEST"
	case http.StatusUnauthorized:
		return "UNAUTHORIZED"
	case http.StatusForbidden:
		return "FORBIDDEN"
	case http.StatusNotFound:
		return "NOT_FOUND"
	case http.StatusConflict:
		return "CONFLICT"
	case http.StatusTooManyRequests:
		return "RESOURCE_EXHAUSTED"
	case http.StatusGatewayTimeout:
		return "DEADLINE_EXCEEDED"
	case http.StatusServiceUnavailable:
		return "UNAVAILABLE"
	default:
		return "INTERNAL"
	}
}
