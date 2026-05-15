package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"

	pbOrder "go-commerce/api/order"
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
