package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRouterRegistersExistingRoutes(t *testing.T) {
	router := NewRouter(RouterConfig{})
	routes := make(map[string]struct{})
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}

	expected := []string{
		"GET /metrics",
		"GET /healthz",
		"GET /readyz",
		"POST /api/register",
		"POST /api/login",
		"GET /api/products",
		"GET /api/products/:id",
		"GET /api/merchants/:id",
		"GET /api/merchants",
		"POST /api/orders",
		"GET /api/orders/:id",
		"GET /api/orders",
		"PUT /api/orders/:id/cancel",
		"PUT /api/orders/:id/ship",
		"PUT /api/orders/:id/complete",
		"POST /api/payments",
		"GET /api/payments/:id",
		"POST /api/payments/:id/success",
		"POST /api/payments/:id/fail",
		"POST /api/cart/items",
		"GET /api/cart",
		"PUT /api/cart/items",
		"DELETE /api/cart/items",
		"DELETE /api/cart",
		"POST /api/merchants",
		"POST /api/merchants/products",
		"DELETE /api/merchants/products",
		"GET /api/merchant/profile",
		"GET /api/merchant/products",
		"POST /api/merchant/products",
		"PUT /api/merchant/products/:id",
		"DELETE /api/merchant/products/:id",
		"GET /api/merchant/orders",
	}
	for _, route := range expected {
		if _, ok := routes[route]; !ok {
			t.Fatalf("missing route %s", route)
		}
	}
}

func TestRouterHandlesAllowedOPTIONSPreflight(t *testing.T) {
	router := NewRouter(RouterConfig{CORSAllowedOrigins: []string{"http://localhost:5173"}})

	req := httptest.NewRequest(http.MethodOptions, "/api/products", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if got, want := resp.Code, http.StatusNoContent; got != want {
		t.Fatalf("unexpected preflight status: got %d want %d", got, want)
	}
	if got, want := resp.Header().Get("Access-Control-Allow-Origin"), "http://localhost:5173"; got != want {
		t.Fatalf("unexpected allow origin: got %q want %q", got, want)
	}
}

func TestRouterRejectsDisallowedOPTIONSPreflight(t *testing.T) {
	router := NewRouter(RouterConfig{CORSAllowedOrigins: []string{"http://localhost:5173"}})

	req := httptest.NewRequest(http.MethodOptions, "/api/products", nil)
	req.Header.Set("Origin", "http://evil.example")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if got, want := resp.Code, http.StatusForbidden; got != want {
		t.Fatalf("unexpected preflight status: got %d want %d", got, want)
	}
	if got := resp.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("did not expect disallowed origin header, got %q", got)
	}
}
