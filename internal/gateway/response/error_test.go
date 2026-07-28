package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go-commerce/pkg/observability"
)

func TestGRPCErrorMapsCodesToHTTPStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		code       codes.Code
		wantStatus int
	}{
		{"invalid argument", codes.InvalidArgument, http.StatusBadRequest},
		{"unauthenticated", codes.Unauthenticated, http.StatusUnauthorized},
		{"permission denied", codes.PermissionDenied, http.StatusForbidden},
		{"not found", codes.NotFound, http.StatusNotFound},
		{"already exists", codes.AlreadyExists, http.StatusConflict},
		{"failed precondition", codes.FailedPrecondition, http.StatusConflict},
		{"resource exhausted", codes.ResourceExhausted, http.StatusTooManyRequests},
		{"deadline exceeded", codes.DeadlineExceeded, http.StatusGatewayTimeout},
		{"unavailable", codes.Unavailable, http.StatusServiceUnavailable},
		{"internal", codes.Internal, http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/err", func(c *gin.Context) {
				c.Set("request_id", "req-map")
				WriteGRPCError(c, status.Error(tt.code, "order is not payable"))
			})

			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/err", nil))

			if got := resp.Code; got != tt.wantStatus {
				t.Fatalf("unexpected status: got %d want %d", got, tt.wantStatus)
			}
			var body ErrorBody
			if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
				t.Fatalf("failed to decode error body: %v", err)
			}
			if got, want := body.RequestID, "req-map"; got != want {
				t.Fatalf("unexpected request id: got %q want %q", got, want)
			}
			if got, want := body.Message, "order is not payable"; got != want {
				t.Fatalf("unexpected message: got %q want %q", got, want)
			}
			if got, want := body.Error, body.Message; got != want {
				t.Fatalf("unexpected legacy error field: got %q want %q", got, want)
			}
			if got, want := body.Code, "ORDER_IS_NOT_PAYABLE"; got != want {
				t.Fatalf("unexpected code: got %q want %q", got, want)
			}
		})
	}
}

func TestWriteErrorIncludesRequestIDFromHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/err", func(c *gin.Context) {
		c.Writer.Header().Set(observability.RequestIDHeader, "req-header")
		WriteError(c, http.StatusBadRequest, "invalid order id")
	})

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/err", nil))

	var body ErrorBody
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode error body: %v", err)
	}
	if got, want := body.RequestID, "req-header"; got != want {
		t.Fatalf("unexpected request id: got %q want %q", got, want)
	}
}
