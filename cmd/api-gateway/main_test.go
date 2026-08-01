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
	pbProduct "go-commerce/api/product"
	gwHandler "go-commerce/internal/gateway/handler"
	gwMiddleware "go-commerce/internal/gateway/middleware"
	appjwt "go-commerce/pkg/jwt"
)

type fakeOrderClient struct {
	lastCreateOrderReq *pbOrder.CreateOrderRequest
	lastCancelOrderReq *pbOrder.CancelOrderRequest
	lastShipOrderReq   *pbOrder.ShipOrderRequest
	lastMerchantOrders *pbOrder.ListMerchantOrdersRequest
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
					PriceCents:  9900,
					Quantity:    in.Items[0].Quantity,
				},
			},
			TotalAmountCents: 9900,
		},
	}, nil
}

func (f *fakeOrderClient) GetOrder(ctx context.Context, in *pbOrder.GetOrderRequest, opts ...grpc.CallOption) (*pbOrder.GetOrderResponse, error) {
	return nil, nil
}

func (f *fakeOrderClient) ListOrders(ctx context.Context, in *pbOrder.ListOrdersRequest, opts ...grpc.CallOption) (*pbOrder.ListOrdersResponse, error) {
	return nil, nil
}

func (f *fakeOrderClient) ListMerchantOrders(ctx context.Context, in *pbOrder.ListMerchantOrdersRequest, opts ...grpc.CallOption) (*pbOrder.ListMerchantOrdersResponse, error) {
	f.lastMerchantOrders = in
	return &pbOrder.ListMerchantOrdersResponse{}, nil
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
	gateway := gwHandler.New(gwHandler.Clients{Order: client})
	router := gin.New()
	router.POST("/api/orders", func(c *gin.Context) {
		c.Set("user_id", int64(1))
		gateway.CreateOrder(c)
	})

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/orders",
		strings.NewReader(`{"items":[{"product_id":1,"product_name":"伪造商品","price_cents":1,"quantity":2}]}`),
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

	gateway := gwHandler.New(gwHandler.Clients{Order: &fakeOrderClient{}})
	router := gin.New()
	router.POST("/api/orders", func(c *gin.Context) {
		c.Set("user_id", int64(1))
		gateway.CreateOrder(c)
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
	gateway := gwHandler.New(gwHandler.Clients{Order: client})
	router := gin.New()
	router.POST("/api/orders", func(c *gin.Context) {
		c.Set("user_id", int64(1))
		gateway.CreateOrder(c)
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

	gateway := gwHandler.New(gwHandler.Clients{Order: &conflictOrderClient{}})
	router := gin.New()
	router.POST("/api/orders", func(c *gin.Context) {
		c.Set("user_id", int64(1))
		gateway.CreateOrder(c)
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

func TestRequestContextMiddlewareReusesHeaderAndExposesResponseID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(gwMiddleware.RequestContext())
	router.GET("/ping", func(c *gin.Context) {
		if got, want := c.GetString(gwMiddleware.RequestIDContextKey), "req-from-client"; got != want {
			t.Fatalf("unexpected request id in gin context: got %q want %q", got, want)
		}
		if got, want := c.GetString(gwMiddleware.TraceIDContextKey), "req-from-client"; got != want {
			t.Fatalf("unexpected trace id in gin context: got %q want %q", got, want)
		}
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("X-Request-ID", "req-from-client")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if got, want := resp.Header().Get("X-Request-ID"), "req-from-client"; got != want {
		t.Fatalf("unexpected response request id: got %q want %q", got, want)
	}
}

type fakeMerchantClient struct {
	lastCreateMerchantReq  *pbMerchant.CreateMerchantRequest
	lastCurrentMerchantReq *pbMerchant.CurrentMerchantRequest
}

type fakePaymentClient struct {
	lastCreatePaymentReq *pbPayment.CreatePaymentRequest
	lastSucceededReq     *pbPayment.PaymentActionRequest
}

type fakeProductClient struct {
	lastListProductsReq *pbProduct.ListProductsRequest
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

func (f *fakeMerchantClient) GetCurrentMerchant(ctx context.Context, in *pbMerchant.CurrentMerchantRequest, opts ...grpc.CallOption) (*pbMerchant.CurrentMerchantResponse, error) {
	f.lastCurrentMerchantReq = in
	return &pbMerchant.CurrentMerchantResponse{
		Merchant: &pbMerchant.Merchant{Id: 1, Name: "Current Shop", OwnerUserId: in.ActorUserId},
	}, nil
}

func (f *fakeMerchantClient) ListMerchantProducts(ctx context.Context, in *pbMerchant.ListMerchantProductsRequest, opts ...grpc.CallOption) (*pbMerchant.ListMerchantProductsResponse, error) {
	return &pbMerchant.ListMerchantProductsResponse{}, nil
}

func (f *fakeMerchantClient) UpdateMerchantProduct(ctx context.Context, in *pbMerchant.UpdateMerchantProductRequest, opts ...grpc.CallOption) (*pbMerchant.UpdateMerchantProductResponse, error) {
	return &pbMerchant.UpdateMerchantProductResponse{}, nil
}

func (f *fakeProductClient) CreateProduct(context.Context, *pbProduct.CreateProductRequest, ...grpc.CallOption) (*pbProduct.CreateProductResponse, error) {
	return nil, nil
}

func (f *fakeProductClient) GetProduct(context.Context, *pbProduct.GetProductRequest, ...grpc.CallOption) (*pbProduct.GetProductResponse, error) {
	return nil, nil
}

func (f *fakeProductClient) ListProducts(ctx context.Context, in *pbProduct.ListProductsRequest, opts ...grpc.CallOption) (*pbProduct.ListProductsResponse, error) {
	f.lastListProductsReq = in
	return &pbProduct.ListProductsResponse{}, nil
}

func (f *fakeProductClient) UpdateProduct(context.Context, *pbProduct.UpdateProductRequest, ...grpc.CallOption) (*pbProduct.UpdateProductResponse, error) {
	return nil, nil
}

func (f *fakeProductClient) DeleteProduct(context.Context, *pbProduct.DeleteProductRequest, ...grpc.CallOption) (*pbProduct.DeleteProductResponse, error) {
	return nil, nil
}

func TestMerchantWriteRoutesRequireAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)

	gateway := gwHandler.New(gwHandler.Clients{Merchant: &fakeMerchantClient{}})
	router := gin.New()
	router.POST("/api/merchants", gwMiddleware.Auth(), gwMiddleware.RequireRole("merchant", "admin"), gateway.CreateMerchant)

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

	gateway := gwHandler.New(gwHandler.Clients{Merchant: &fakeMerchantClient{}})
	router := gin.New()
	router.POST("/api/merchants", gwMiddleware.Auth(), gwMiddleware.RequireRole("merchant", "admin"), gateway.CreateMerchant)

	req := httptest.NewRequest(http.MethodPost, "/api/merchants", strings.NewReader(`{"name":"Shop","contact_info":"shop@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if got, want := resp.Code, http.StatusForbidden; got != want {
		t.Fatalf("unexpected status code: got %d want %d", got, want)
	}
}

func TestCustomerCannotAccessMerchantConsoleRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	token, err := appjwt.GenerateToken(17, "customer")
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	gateway := gwHandler.New(gwHandler.Clients{Merchant: &fakeMerchantClient{}})
	router := gin.New()
	router.GET("/api/merchant/profile", gwMiddleware.Auth(), gwMiddleware.RequireRole("merchant", "admin"), gateway.CurrentMerchantProfile)

	req := httptest.NewRequest(http.MethodGet, "/api/merchant/profile", nil)
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
	gateway := gwHandler.New(gwHandler.Clients{Merchant: client})
	router := gin.New()
	router.POST("/api/merchants", gwMiddleware.Auth(), gwMiddleware.RequireRole("merchant", "admin"), gateway.CreateMerchant)

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

	gateway := gwHandler.New(gwHandler.Clients{Merchant: &fakeMerchantClient{}})
	router := gin.New()
	router.GET("/api/merchants", gateway.ListMerchants)

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
	gateway := gwHandler.New(gwHandler.Clients{Payment: client})
	router := gin.New()
	router.POST("/api/payments", func(c *gin.Context) {
		c.Set("user_id", int64(7))
		gateway.CreatePayment(c)
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

func TestHandleListProductsForwardsQueryParameters(t *testing.T) {
	gin.SetMode(gin.TestMode)

	client := &fakeProductClient{}
	gateway := gwHandler.New(gwHandler.Clients{Product: client})
	router := gin.New()
	router.GET("/api/products", gateway.ListProducts)

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/products?page=2&page_size=25&category=book&keyword=Go&sort_by=price&order=asc&min_price_cents=1050&max_price_cents=9990",
		nil,
	)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if got, want := resp.Code, http.StatusOK; got != want {
		t.Fatalf("unexpected status code: got %d want %d", got, want)
	}
	if client.lastListProductsReq == nil {
		t.Fatal("expected ListProducts to be called")
	}
	if got, want := client.lastListProductsReq.Page, int32(2); got != want {
		t.Fatalf("unexpected page: got %d want %d", got, want)
	}
	if got, want := client.lastListProductsReq.PageSize, int32(25); got != want {
		t.Fatalf("unexpected page size: got %d want %d", got, want)
	}
	if got, want := client.lastListProductsReq.Category, "book"; got != want {
		t.Fatalf("unexpected category: got %q want %q", got, want)
	}
	if got, want := client.lastListProductsReq.Keyword, "Go"; got != want {
		t.Fatalf("unexpected keyword: got %q want %q", got, want)
	}
	if got, want := client.lastListProductsReq.SortBy, "price_cents"; got != want {
		t.Fatalf("unexpected sort_by: got %q want %q", got, want)
	}
	if got, want := client.lastListProductsReq.Order, "asc"; got != want {
		t.Fatalf("unexpected order: got %q want %q", got, want)
	}
	if client.lastListProductsReq.MinPriceCents == nil || *client.lastListProductsReq.MinPriceCents != int64(1050) {
		t.Fatalf("unexpected min_price_cents: got %v want 1050", client.lastListProductsReq.MinPriceCents)
	}
	if client.lastListProductsReq.MaxPriceCents == nil || *client.lastListProductsReq.MaxPriceCents != int64(9990) {
		t.Fatalf("unexpected max_price_cents: got %v want 9990", client.lastListProductsReq.MaxPriceCents)
	}
}

func TestHandleListProductsCapsLargePageSizeAndSanitizesSort(t *testing.T) {
	gin.SetMode(gin.TestMode)

	client := &fakeProductClient{}
	gateway := gwHandler.New(gwHandler.Clients{Product: client})
	router := gin.New()
	router.GET("/api/products", gateway.ListProducts)

	req := httptest.NewRequest(http.MethodGet, "/api/products?page=1&page_size=999&sort_by=name&order=random", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if got, want := resp.Code, http.StatusOK; got != want {
		t.Fatalf("unexpected status code: got %d want %d", got, want)
	}
	if got, want := client.lastListProductsReq.Page, int32(1); got != want {
		t.Fatalf("unexpected default page: got %d want %d", got, want)
	}
	if got, want := client.lastListProductsReq.PageSize, int32(100); got != want {
		t.Fatalf("unexpected capped page size: got %d want %d", got, want)
	}
	if got, want := client.lastListProductsReq.SortBy, "created_at"; got != want {
		t.Fatalf("unexpected default sort_by: got %q want %q", got, want)
	}
	if got, want := client.lastListProductsReq.Order, "desc"; got != want {
		t.Fatalf("unexpected default order: got %q want %q", got, want)
	}
}

func TestHandleListProductsRejectsNonPositivePagination(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, target := range []string{
		"/api/products?page=-3&page_size=10",
		"/api/products?page=1&page_size=-5",
	} {
		client := &fakeProductClient{}
		gateway := gwHandler.New(gwHandler.Clients{Product: client})
		router := gin.New()
		router.GET("/api/products", gateway.ListProducts)

		req := httptest.NewRequest(http.MethodGet, target, nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		if got, want := resp.Code, http.StatusBadRequest; got != want {
			t.Fatalf("%s unexpected status code: got %d want %d", target, got, want)
		}
		if client.lastListProductsReq != nil {
			t.Fatalf("%s expected ListProducts not to be called", target)
		}
	}
}

func TestHandleListProductsRejectsInvalidPriceRange(t *testing.T) {
	gin.SetMode(gin.TestMode)

	client := &fakeProductClient{}
	gateway := gwHandler.New(gwHandler.Clients{Product: client})
	router := gin.New()
	router.GET("/api/products", gateway.ListProducts)

	req := httptest.NewRequest(http.MethodGet, "/api/products?min_price_cents=10000&max_price_cents=1000", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if got, want := resp.Code, http.StatusBadRequest; got != want {
		t.Fatalf("unexpected status code: got %d want %d", got, want)
	}
	if client.lastListProductsReq != nil {
		t.Fatal("expected ListProducts not to be called for invalid price range")
	}
}

func TestHandleMarkPaymentSucceededRequiresIdempotencyKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	gateway := gwHandler.New(gwHandler.Clients{Payment: &fakePaymentClient{}})
	router := gin.New()
	router.POST("/api/payments/:id/success", func(c *gin.Context) {
		c.Set("user_id", int64(1))
		gateway.MarkPaymentSucceeded(c)
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
	gateway := gwHandler.New(gwHandler.Clients{Payment: client})
	router := gin.New()
	router.POST("/api/payments/:id/success", func(c *gin.Context) {
		c.Set("user_id", int64(1))
		gateway.MarkPaymentSucceeded(c)
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

	gateway := gwHandler.New(gwHandler.Clients{Order: &fakeOrderClient{}})
	router := gin.New()
	router.PUT("/api/orders/:id/cancel", func(c *gin.Context) {
		c.Set("user_id", int64(1))
		gateway.CancelOrder(c)
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
	gateway := gwHandler.New(gwHandler.Clients{Order: client})
	router := gin.New()
	router.PUT("/api/orders/:id/cancel", func(c *gin.Context) {
		c.Set("user_id", int64(1))
		gateway.CancelOrder(c)
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

	gateway := gwHandler.New(gwHandler.Clients{Order: &conflictOrderClient{}})
	router := gin.New()
	router.PUT("/api/orders/:id/cancel", func(c *gin.Context) {
		c.Set("user_id", int64(1))
		gateway.CancelOrder(c)
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

	gateway := gwHandler.New(gwHandler.Clients{Order: &fakeOrderClient{}})
	router := gin.New()
	router.PUT("/api/orders/:id/ship", gwMiddleware.Auth(), gwMiddleware.RequireRole("merchant", "admin"), gateway.ShipOrder)

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
	gateway := gwHandler.New(gwHandler.Clients{Order: client})
	router := gin.New()
	router.PUT("/api/orders/:id/ship", gwMiddleware.Auth(), gwMiddleware.RequireRole("merchant", "admin"), gateway.ShipOrder)

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
