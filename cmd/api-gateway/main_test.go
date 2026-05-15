package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"

	pbMerchant "go-commerce/api/merchant"
	pbOrder "go-commerce/api/order"
	appjwt "go-commerce/pkg/jwt"
)

type fakeOrderClient struct {
	lastCreateOrderReq *pbOrder.CreateOrderRequest
}

func (f *fakeOrderClient) CreateOrder(ctx context.Context, in *pbOrder.CreateOrderRequest, opts ...grpc.CallOption) (*pbOrder.CreateOrderResponse, error) {
	f.lastCreateOrderReq = in
	return &pbOrder.CreateOrderResponse{
		Order: &pbOrder.Order{
			Id: 1,
			Items: []*pbOrder.OrderItem{
				{
					ProductId:   in.Items[0].ProductId,
					ProductName: "真实商品",
					Price:       99,
					Quantity:    in.Items[0].Quantity,
				},
			},
			TotalAmount: 99,
		},
	}, nil
}

func (f *fakeOrderClient) GetOrder(ctx context.Context, in *pbOrder.GetOrderRequest, opts ...grpc.CallOption) (*pbOrder.GetOrderResponse, error) {
	return nil, nil
}

func (f *fakeOrderClient) ListOrders(ctx context.Context, in *pbOrder.ListOrdersRequest, opts ...grpc.CallOption) (*pbOrder.ListOrdersResponse, error) {
	return nil, nil
}

func (f *fakeOrderClient) CancelOrder(ctx context.Context, in *pbOrder.CancelOrderRequest, opts ...grpc.CallOption) (*pbOrder.CancelOrderResponse, error) {
	return nil, nil
}

func TestHandleCreateOrderIgnoresForgedClientFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	client := &fakeOrderClient{}
	gateway := &APIGateway{orderClient: client}
	router := gin.New()
	router.POST("/api/orders", func(c *gin.Context) {
		c.Set("user_id", int64(1))
		gateway.handleCreateOrder(c)
	})

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/orders",
		strings.NewReader(`{"items":[{"product_id":1,"product_name":"伪造商品","price":0.01,"quantity":2}]}`),
	)
	req.Header.Set("Content-Type", "application/json")

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if got, want := resp.Code, http.StatusOK; got != want {
		t.Fatalf("unexpected status code: got %d want %d", got, want)
	}
	if client.lastCreateOrderReq == nil {
		t.Fatal("expected CreateOrder to be called")
	}
	if len(client.lastCreateOrderReq.Items) != 1 {
		t.Fatalf("unexpected forwarded item count: got %d want 1", len(client.lastCreateOrderReq.Items))
	}
	if got, want := client.lastCreateOrderReq.Items[0].ProductId, int64(1); got != want {
		t.Fatalf("unexpected forwarded product id: got %d want %d", got, want)
	}
	if got, want := client.lastCreateOrderReq.Items[0].Quantity, int32(2); got != want {
		t.Fatalf("unexpected forwarded quantity: got %d want %d", got, want)
	}
}

type fakeMerchantClient struct {
	lastCreateMerchantReq *pbMerchant.CreateMerchantRequest
}

func (f *fakeMerchantClient) CreateMerchant(ctx context.Context, in *pbMerchant.CreateMerchantRequest, opts ...grpc.CallOption) (*pbMerchant.CreateMerchantResponse, error) {
	f.lastCreateMerchantReq = in
	return &pbMerchant.CreateMerchantResponse{
		Merchant: &pbMerchant.Merchant{
			Id:          1,
			Name:        in.Name,
			ContactInfo: in.ContactInfo,
			OwnerUserId: in.OwnerUserId,
		},
	}, nil
}

func (f *fakeMerchantClient) GetMerchant(ctx context.Context, in *pbMerchant.GetMerchantRequest, opts ...grpc.CallOption) (*pbMerchant.GetMerchantResponse, error) {
	return &pbMerchant.GetMerchantResponse{
		Merchant: &pbMerchant.Merchant{Id: in.Id, Name: "Public Shop"},
	}, nil
}

func (f *fakeMerchantClient) ListMerchants(ctx context.Context, in *pbMerchant.ListMerchantsRequest, opts ...grpc.CallOption) (*pbMerchant.ListMerchantsResponse, error) {
	return &pbMerchant.ListMerchantsResponse{
		Merchants: []*pbMerchant.Merchant{{Id: 1, Name: "Public Shop"}},
		Total:     1,
	}, nil
}

func (f *fakeMerchantClient) AddProduct(ctx context.Context, in *pbMerchant.AddProductRequest, opts ...grpc.CallOption) (*pbMerchant.AddProductResponse, error) {
	return &pbMerchant.AddProductResponse{ProductId: 1}, nil
}

func (f *fakeMerchantClient) DeleteProduct(ctx context.Context, in *pbMerchant.DeleteProductRequest, opts ...grpc.CallOption) (*pbMerchant.DeleteProductResponse, error) {
	return &pbMerchant.DeleteProductResponse{Success: true}, nil
}

func TestMerchantWriteRoutesRequireAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)

	gateway := &APIGateway{merchantClient: &fakeMerchantClient{}}
	router := gin.New()
	router.POST("/api/merchants", gateway.authMiddleware(), gateway.requireRole("merchant", "admin"), gateway.handleCreateMerchant)

	req := httptest.NewRequest(http.MethodPost, "/api/merchants", strings.NewReader(`{"name":"Shop","contact_info":"shop@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if got, want := resp.Code, http.StatusUnauthorized; got != want {
		t.Fatalf("unexpected status code: got %d want %d", got, want)
	}
}

func TestCustomerCannotAccessMerchantWriteRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	token, err := appjwt.GenerateToken(7, "customer")
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	gateway := &APIGateway{merchantClient: &fakeMerchantClient{}}
	router := gin.New()
	router.POST("/api/merchants", gateway.authMiddleware(), gateway.requireRole("merchant", "admin"), gateway.handleCreateMerchant)

	req := httptest.NewRequest(http.MethodPost, "/api/merchants", strings.NewReader(`{"name":"Shop","contact_info":"shop@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if got, want := resp.Code, http.StatusForbidden; got != want {
		t.Fatalf("unexpected status code: got %d want %d", got, want)
	}
}

func TestMerchantCreateRouteInjectsCurrentUserAsOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)

	token, err := appjwt.GenerateToken(8, "merchant")
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	client := &fakeMerchantClient{}
	gateway := &APIGateway{merchantClient: client}
	router := gin.New()
	router.POST("/api/merchants", gateway.authMiddleware(), gateway.requireRole("merchant", "admin"), gateway.handleCreateMerchant)

	req := httptest.NewRequest(http.MethodPost, "/api/merchants", strings.NewReader(`{"name":"Shop","contact_info":"shop@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if got, want := resp.Code, http.StatusOK; got != want {
		t.Fatalf("unexpected status code: got %d want %d", got, want)
	}
	if client.lastCreateMerchantReq == nil {
		t.Fatal("expected CreateMerchant to be called")
	}
	if got, want := client.lastCreateMerchantReq.OwnerUserId, int64(8); got != want {
		t.Fatalf("unexpected owner user id: got %d want %d", got, want)
	}
}

func TestPublicMerchantReadRoutesRemainAccessible(t *testing.T) {
	gin.SetMode(gin.TestMode)

	gateway := &APIGateway{merchantClient: &fakeMerchantClient{}}
	router := gin.New()
	router.GET("/api/merchants", gateway.handleListMerchants)

	req := httptest.NewRequest(http.MethodGet, "/api/merchants", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if got, want := resp.Code, http.StatusOK; got != want {
		t.Fatalf("unexpected status code: got %d want %d", got, want)
	}
}
