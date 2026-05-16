package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pbMerchant "go-commerce/api/merchant"
	pbOrder "go-commerce/api/order"
	pbPayment "go-commerce/api/payment"
	appjwt "go-commerce/pkg/jwt"
)

type fakeOrderClient struct {
	lastCreateOrderReq *pbOrder.CreateOrderRequest
	lastCancelOrderReq *pbOrder.CancelOrderRequest
	lastShipOrderReq   *pbOrder.ShipOrderRequest
}

type conflictOrderClient struct {
	fakeOrderClient
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
	f.lastCancelOrderReq = in
	return &pbOrder.CancelOrderResponse{Success: true}, nil
}

func (f *fakeOrderClient) ShipOrder(ctx context.Context, in *pbOrder.ShipOrderRequest, opts ...grpc.CallOption) (*pbOrder.ShipOrderResponse, error) {
	f.lastShipOrderReq = in
	return &pbOrder.ShipOrderResponse{Success: true}, nil
}

func (f *fakeOrderClient) CompleteOrder(ctx context.Context, in *pbOrder.CompleteOrderRequest, opts ...grpc.CallOption) (*pbOrder.CompleteOrderResponse, error) {
	return &pbOrder.CompleteOrderResponse{Success: true}, nil
}

func (f *conflictOrderClient) CreateOrder(context.Context, *pbOrder.CreateOrderRequest, ...grpc.CallOption) (*pbOrder.CreateOrderResponse, error) {
	return nil, status.Error(codes.FailedPrecondition, "idempotency key conflict")
}

func (f *conflictOrderClient) CancelOrder(context.Context, *pbOrder.CancelOrderRequest, ...grpc.CallOption) (*pbOrder.CancelOrderResponse, error) {
	return nil, status.Error(codes.FailedPrecondition, "idempotency key conflict")
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
	req.Header.Set("Idempotency-Key", "order-key")

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

func TestHandleCreateOrderRequiresIdempotencyKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	gateway := &APIGateway{orderClient: &fakeOrderClient{}}
	router := gin.New()
	router.POST("/api/orders", func(c *gin.Context) {
		c.Set("user_id", int64(1))
		gateway.handleCreateOrder(c)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/orders", strings.NewReader(`{"items":[{"product_id":1,"quantity":1}]}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if got, want := resp.Code, http.StatusBadRequest; got != want {
		t.Fatalf("unexpected status code: got %d want %d", got, want)
	}
}

func TestHandleCreateOrderForwardsIdempotencyKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	client := &fakeOrderClient{}
	gateway := &APIGateway{orderClient: client}
	router := gin.New()
	router.POST("/api/orders", func(c *gin.Context) {
		c.Set("user_id", int64(1))
		gateway.handleCreateOrder(c)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/orders", strings.NewReader(`{"items":[{"product_id":1,"quantity":1}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "order-key")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if got, want := resp.Code, http.StatusOK; got != want {
		t.Fatalf("unexpected status code: got %d want %d", got, want)
	}
	if got, want := client.lastCreateOrderReq.IdempotencyKey, "order-key"; got != want {
		t.Fatalf("unexpected idempotency key: got %q want %q", got, want)
	}
}

func TestHandleCreateOrderMapsIdempotencyConflictToHTTP409(t *testing.T) {
	gin.SetMode(gin.TestMode)

	gateway := &APIGateway{orderClient: &conflictOrderClient{}}
	router := gin.New()
	router.POST("/api/orders", func(c *gin.Context) {
		c.Set("user_id", int64(1))
		gateway.handleCreateOrder(c)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/orders", strings.NewReader(`{"items":[{"product_id":1,"quantity":1}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "order-key")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if got, want := resp.Code, http.StatusConflict; got != want {
		t.Fatalf("unexpected status code: got %d want %d", got, want)
	}
}

type fakeMerchantClient struct {
	lastCreateMerchantReq *pbMerchant.CreateMerchantRequest
}

type fakePaymentClient struct {
	lastCreatePaymentReq *pbPayment.CreatePaymentRequest
	lastSucceededReq     *pbPayment.PaymentActionRequest
}

func (f *fakePaymentClient) CreatePayment(ctx context.Context, in *pbPayment.CreatePaymentRequest, opts ...grpc.CallOption) (*pbPayment.CreatePaymentResponse, error) {
	f.lastCreatePaymentReq = in
	return &pbPayment.CreatePaymentResponse{
		Payment: &pbPayment.Payment{
			Id:            1,
			OrderId:       in.OrderId,
			UserId:        in.UserId,
			PaymentMethod: in.PaymentMethod,
		},
	}, nil
}

func (f *fakePaymentClient) GetPayment(ctx context.Context, in *pbPayment.GetPaymentRequest, opts ...grpc.CallOption) (*pbPayment.GetPaymentResponse, error) {
	return nil, nil
}

func (f *fakePaymentClient) MarkPaymentSucceeded(ctx context.Context, in *pbPayment.PaymentActionRequest, opts ...grpc.CallOption) (*pbPayment.PaymentActionResponse, error) {
	f.lastSucceededReq = in
	return &pbPayment.PaymentActionResponse{Payment: &pbPayment.Payment{Id: in.Id, UserId: in.UserId, Status: "succeeded"}}, nil
}

func (f *fakePaymentClient) MarkPaymentFailed(ctx context.Context, in *pbPayment.PaymentActionRequest, opts ...grpc.CallOption) (*pbPayment.PaymentActionResponse, error) {
	return nil, nil
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

func TestHandleCreatePaymentInjectsCurrentUser(t *testing.T) {
	gin.SetMode(gin.TestMode)

	client := &fakePaymentClient{}
	gateway := &APIGateway{paymentClient: client}
	router := gin.New()
	router.POST("/api/payments", func(c *gin.Context) {
		c.Set("user_id", int64(7))
		gateway.handleCreatePayment(c)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/payments", strings.NewReader(`{"order_id":1,"payment_method":"mock_balance","user_id":999}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if got, want := resp.Code, http.StatusOK; got != want {
		t.Fatalf("unexpected status code: got %d want %d", got, want)
	}
	if client.lastCreatePaymentReq == nil {
		t.Fatal("expected CreatePayment to be called")
	}
	if got, want := client.lastCreatePaymentReq.UserId, int64(7); got != want {
		t.Fatalf("unexpected user id: got %d want %d", got, want)
	}
}

func TestHandleMarkPaymentSucceededRequiresIdempotencyKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	gateway := &APIGateway{paymentClient: &fakePaymentClient{}}
	router := gin.New()
	router.POST("/api/payments/:id/success", func(c *gin.Context) {
		c.Set("user_id", int64(1))
		gateway.handleMarkPaymentSucceeded(c)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/payments/1/success", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if got, want := resp.Code, http.StatusBadRequest; got != want {
		t.Fatalf("unexpected status code: got %d want %d", got, want)
	}
}

func TestHandleMarkPaymentSucceededForwardsIdempotencyKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	client := &fakePaymentClient{}
	gateway := &APIGateway{paymentClient: client}
	router := gin.New()
	router.POST("/api/payments/:id/success", func(c *gin.Context) {
		c.Set("user_id", int64(1))
		gateway.handleMarkPaymentSucceeded(c)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/payments/1/success", nil)
	req.Header.Set("Idempotency-Key", "payment-key")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if got, want := resp.Code, http.StatusOK; got != want {
		t.Fatalf("unexpected status code: got %d want %d", got, want)
	}
	if got, want := client.lastSucceededReq.IdempotencyKey, "payment-key"; got != want {
		t.Fatalf("unexpected idempotency key: got %q want %q", got, want)
	}
}

func TestHandleCancelOrderRequiresIdempotencyKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	gateway := &APIGateway{orderClient: &fakeOrderClient{}}
	router := gin.New()
	router.PUT("/api/orders/:id/cancel", func(c *gin.Context) {
		c.Set("user_id", int64(1))
		gateway.handleCancelOrder(c)
	})

	req := httptest.NewRequest(http.MethodPut, "/api/orders/1/cancel", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if got, want := resp.Code, http.StatusBadRequest; got != want {
		t.Fatalf("unexpected status code: got %d want %d", got, want)
	}
}

func TestHandleCancelOrderForwardsIdempotencyKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	client := &fakeOrderClient{}
	gateway := &APIGateway{orderClient: client}
	router := gin.New()
	router.PUT("/api/orders/:id/cancel", func(c *gin.Context) {
		c.Set("user_id", int64(1))
		gateway.handleCancelOrder(c)
	})

	req := httptest.NewRequest(http.MethodPut, "/api/orders/1/cancel", nil)
	req.Header.Set("Idempotency-Key", "cancel-key")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if got, want := resp.Code, http.StatusOK; got != want {
		t.Fatalf("unexpected status code: got %d want %d", got, want)
	}
	if got, want := client.lastCancelOrderReq.IdempotencyKey, "cancel-key"; got != want {
		t.Fatalf("unexpected idempotency key: got %q want %q", got, want)
	}
}

func TestHandleCancelOrderMapsIdempotencyConflictToHTTP409(t *testing.T) {
	gin.SetMode(gin.TestMode)

	gateway := &APIGateway{orderClient: &conflictOrderClient{}}
	router := gin.New()
	router.PUT("/api/orders/:id/cancel", func(c *gin.Context) {
		c.Set("user_id", int64(1))
		gateway.handleCancelOrder(c)
	})

	req := httptest.NewRequest(http.MethodPut, "/api/orders/1/cancel", nil)
	req.Header.Set("Idempotency-Key", "cancel-key")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if got, want := resp.Code, http.StatusConflict; got != want {
		t.Fatalf("unexpected status code: got %d want %d", got, want)
	}
}

func TestCustomerCannotShipOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)

	token, err := appjwt.GenerateToken(7, "customer")
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	gateway := &APIGateway{orderClient: &fakeOrderClient{}}
	router := gin.New()
	router.PUT("/api/orders/:id/ship", gateway.authMiddleware(), gateway.requireRole("merchant", "admin"), gateway.handleShipOrder)

	req := httptest.NewRequest(http.MethodPut, "/api/orders/1/ship", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if got, want := resp.Code, http.StatusForbidden; got != want {
		t.Fatalf("unexpected status code: got %d want %d", got, want)
	}
}

func TestShipOrderInjectsCurrentActor(t *testing.T) {
	gin.SetMode(gin.TestMode)

	token, err := appjwt.GenerateToken(9, "merchant")
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	client := &fakeOrderClient{}
	gateway := &APIGateway{orderClient: client}
	router := gin.New()
	router.PUT("/api/orders/:id/ship", gateway.authMiddleware(), gateway.requireRole("merchant", "admin"), gateway.handleShipOrder)

	req := httptest.NewRequest(http.MethodPut, "/api/orders/1/ship", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if got, want := resp.Code, http.StatusOK; got != want {
		t.Fatalf("unexpected status code: got %d want %d", got, want)
	}
	if client.lastShipOrderReq == nil {
		t.Fatal("expected ShipOrder to be called")
	}
	if got, want := client.lastShipOrderReq.ActorUserId, int64(9); got != want {
		t.Fatalf("unexpected actor user id: got %d want %d", got, want)
	}
}
