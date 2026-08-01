//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"testing"

	"google.golang.org/grpc"

	pbCart "go-commerce/api/cart"
	pbProduct "go-commerce/api/product"
	"go-commerce/internal/cart"
)

type integrationProductClient struct {
	product *pbProduct.Product
}

func (c *integrationProductClient) CreateProduct(context.Context, *pbProduct.CreateProductRequest, ...grpc.CallOption) (*pbProduct.CreateProductResponse, error) {
	return nil, nil
}

func (c *integrationProductClient) GetProduct(context.Context, *pbProduct.GetProductRequest, ...grpc.CallOption) (*pbProduct.GetProductResponse, error) {
	return &pbProduct.GetProductResponse{Product: c.product}, nil
}

func (c *integrationProductClient) ListProducts(context.Context, *pbProduct.ListProductsRequest, ...grpc.CallOption) (*pbProduct.ListProductsResponse, error) {
	return nil, nil
}

func (c *integrationProductClient) UpdateProduct(context.Context, *pbProduct.UpdateProductRequest, ...grpc.CallOption) (*pbProduct.UpdateProductResponse, error) {
	return nil, nil
}

func (c *integrationProductClient) DeleteProduct(context.Context, *pbProduct.DeleteProductRequest, ...grpc.CallOption) (*pbProduct.DeleteProductResponse, error) {
	return nil, nil
}

func TestRedisCartRoundTrip(t *testing.T) {
	ctx := context.Background()
	redisClient := openIntegrationRedis(t)
	userID := int64(90_000_000 + len(t.Name()))
	key := fmt.Sprintf("cart:%d", userID)
	t.Cleanup(func() { _ = redisClient.Del(ctx, key).Err() })

	service := cart.NewService(redisClient, &integrationProductClient{
		product: &pbProduct.Product{
			Id:         101,
			Name:       "Integration Keyboard",
			PriceCents: 29900,
			ImageUrl:   "https://example.com/keyboard.png",
		},
	})

	// Arrange / Act：真实 Redis 中写入购物车，再读取回来。
	if _, err := service.AddCartItem(ctx, &pbCart.AddCartItemRequest{
		UserId:    userID,
		ProductId: 101,
		Quantity:  2,
	}); err != nil {
		t.Fatalf("AddCartItem returned error: %v", err)
	}

	resp, err := service.GetCart(ctx, &pbCart.GetCartRequest{UserId: userID})
	if err != nil {
		t.Fatalf("GetCart returned error: %v", err)
	}

	// Assert：验证真实 Redis 读写和金额汇总都正确。
	if got, want := len(resp.Items), 1; got != want {
		t.Fatalf("unexpected item count: got %d want %d", got, want)
	}
	if got, want := resp.Items[0].Quantity, int32(2); got != want {
		t.Fatalf("unexpected quantity: got %d want %d", got, want)
	}
	if got, want := resp.TotalAmountCents, int64(59800); got != want {
		t.Fatalf("unexpected total AmountCents: got %d want %d", got, want)
	}
}
