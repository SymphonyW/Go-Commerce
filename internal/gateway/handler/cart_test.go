package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pbCart "go-commerce/api/cart"
	"go-commerce/internal/gateway/middleware"
	"go-commerce/internal/gateway/response"
)

type fakeCartClient struct {
	addCalls  int
	getErr    error
	addErr    error
	lastAdd   *pbCart.AddCartItemRequest
	lastGet   *pbCart.GetCartRequest
	lastClear *pbCart.ClearCartRequest
}

func (f *fakeCartClient) AddCartItem(ctx context.Context, in *pbCart.AddCartItemRequest, opts ...grpc.CallOption) (*pbCart.AddCartItemResponse, error) {
	f.addCalls++
	f.lastAdd = in
	if f.addErr != nil {
		return nil, f.addErr
	}
	return &pbCart.AddCartItemResponse{}, nil
}

func (f *fakeCartClient) GetCart(ctx context.Context, in *pbCart.GetCartRequest, opts ...grpc.CallOption) (*pbCart.GetCartResponse, error) {
	f.lastGet = in
	if f.getErr != nil {
		return nil, f.getErr
	}
	return &pbCart.GetCartResponse{}, nil
}

func (f *fakeCartClient) UpdateCartItem(context.Context, *pbCart.UpdateCartItemRequest, ...grpc.CallOption) (*pbCart.UpdateCartItemResponse, error) {
	return &pbCart.UpdateCartItemResponse{}, nil
}

func (f *fakeCartClient) RemoveCartItem(context.Context, *pbCart.RemoveCartItemRequest, ...grpc.CallOption) (*pbCart.RemoveCartItemResponse, error) {
	return &pbCart.RemoveCartItemResponse{}, nil
}

func (f *fakeCartClient) ClearCart(ctx context.Context, in *pbCart.ClearCartRequest, opts ...grpc.CallOption) (*pbCart.ClearCartResponse, error) {
	f.lastClear = in
	return &pbCart.ClearCartResponse{}, nil
}

func TestGetCartMapsNotFoundTo404(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := New(Clients{Cart: &fakeCartClient{getErr: status.Error(codes.NotFound, "cart item not found")}})
	router := gin.New()
	router.Use(middleware.RequestContext())
	router.GET("/api/cart", func(c *gin.Context) {
		c.Set("user_id", int64(7))
		h.GetCart(c)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/cart", nil)
	req.Header.Set("X-Request-ID", "req-cart")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if got, want := resp.Code, http.StatusNotFound; got != want {
		t.Fatalf("unexpected status: got %d want %d", got, want)
	}
	var body response.ErrorBody
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode error body: %v", err)
	}
	if got, want := body.RequestID, "req-cart"; got != want {
		t.Fatalf("unexpected request id: got %q want %q", got, want)
	}
	if got, want := body.Error, "cart item not found"; got != want {
		t.Fatalf("unexpected error: got %q want %q", got, want)
	}
}

func TestAddCartItemRejectsInvalidQuantity(t *testing.T) {
	gin.SetMode(gin.TestMode)

	client := &fakeCartClient{}
	h := New(Clients{Cart: client})
	router := gin.New()
	router.POST("/api/cart/items", func(c *gin.Context) {
		c.Set("user_id", int64(7))
		h.AddCartItem(c)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/cart/items", strings.NewReader(`{"product_id":1,"quantity":0}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if got, want := resp.Code, http.StatusBadRequest; got != want {
		t.Fatalf("unexpected status: got %d want %d", got, want)
	}
	if got := client.addCalls; got != 0 {
		t.Fatalf("expected cart service not to be called, got %d calls", got)
	}
}
