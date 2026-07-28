package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCORSAllowsConfiguredOriginWithCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(CORS([]string{"http://localhost:5173"}))
	router.GET("/ping", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if got, want := resp.Header().Get("Access-Control-Allow-Origin"), "http://localhost:5173"; got != want {
		t.Fatalf("unexpected allow origin: got %q want %q", got, want)
	}
	if got, want := resp.Header().Get("Access-Control-Allow-Credentials"), "true"; got != want {
		t.Fatalf("unexpected credentials header: got %q want %q", got, want)
	}
}

func TestCORSRejectsDisallowedPreflightOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(CORS([]string{"http://localhost:5173"}))
	router.OPTIONS("/ping", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	req := httptest.NewRequest(http.MethodOptions, "/ping", nil)
	req.Header.Set("Origin", "http://evil.example")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if got, want := resp.Code, http.StatusForbidden; got != want {
		t.Fatalf("unexpected status: got %d want %d", got, want)
	}
	if got := resp.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("did not expect disallowed origin header, got %q", got)
	}
}

func TestCORSAllowsConfiguredPreflightOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(CORS([]string{"http://localhost:5173"}))
	router.OPTIONS("/ping", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	req := httptest.NewRequest(http.MethodOptions, "/ping", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if got, want := resp.Code, http.StatusNoContent; got != want {
		t.Fatalf("unexpected status: got %d want %d", got, want)
	}
	if got, want := resp.Header().Get("Access-Control-Allow-Origin"), "http://localhost:5173"; got != want {
		t.Fatalf("unexpected allow origin: got %q want %q", got, want)
	}
}

func TestParseAllowedOriginsDefaultsAndIgnoresWildcard(t *testing.T) {
	if got := ParseAllowedOrigins(""); len(got) != 1 || got[0] != DefaultAllowedOrigin {
		t.Fatalf("unexpected default origins: %#v", got)
	}
	got := ParseAllowedOrigins("*, http://localhost:5173, http://example.test, http://localhost:5173")
	if len(got) != 2 || got[0] != "http://localhost:5173" || got[1] != "http://example.test" {
		t.Fatalf("unexpected parsed origins: %#v", got)
	}
}
