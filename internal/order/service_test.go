package order

import (
	"context"
	"fmt"
	"strings"
	"testing"

	pb "go-commerce/api/order"
	"go-commerce/internal/product"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newTestService 为订单服务创建独立的内存数据库，避免测试之间互相污染。
func newTestService(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()

	dsn := fmt.Sprintf(
		"file:%s?mode=memory&cache=shared",
		strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()),
	)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	if err := db.AutoMigrate(&product.Product{}, &Order{}, &OrderItem{}); err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}

	return NewService(db, nil), db
}

// createTestProduct 写入真实商品数据，供订单服务生成快照时读取。
func createTestProduct(t *testing.T, db *gorm.DB, name string, price float64, stock int32) product.Product {
	t.Helper()

	item := product.Product{
		Name:       name,
		Price:      price,
		Stock:      stock,
		MerchantID: 1,
	}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("failed to create product: %v", err)
	}

	return item
}

func TestCreateOrderUsesDatabaseSnapshot(t *testing.T) {
	service, db := newTestService(t)
	item := createTestProduct(t, db, "真实商品", 88.5, 10)

	resp, err := service.CreateOrder(context.Background(), &pb.CreateOrderRequest{
		UserId: 1,
		Items: []*pb.CreateOrderItem{
			{ProductId: int64(item.ID), Quantity: 2},
		},
	})
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}

	if got, want := resp.Order.TotalAmount, float32(177); got != want {
		t.Fatalf("unexpected total amount: got %.2f want %.2f", got, want)
	}
	if len(resp.Order.Items) != 1 {
		t.Fatalf("unexpected order item count: got %d want 1", len(resp.Order.Items))
	}
	if got, want := resp.Order.Items[0].ProductName, "真实商品"; got != want {
		t.Fatalf("unexpected snapshot product name: got %q want %q", got, want)
	}
	if got, want := resp.Order.Items[0].Price, float32(88.5); got != want {
		t.Fatalf("unexpected snapshot price: got %.2f want %.2f", got, want)
	}

	var saved OrderItem
	if err := db.First(&saved).Error; err != nil {
		t.Fatalf("failed to query saved order item: %v", err)
	}
	if got, want := saved.ProductName, "真实商品"; got != want {
		t.Fatalf("unexpected saved snapshot name: got %q want %q", got, want)
	}
	if got, want := saved.Price, 88.5; got != want {
		t.Fatalf("unexpected saved snapshot price: got %.2f want %.2f", got, want)
	}
}

func TestCreateOrderRejectsMissingProduct(t *testing.T) {
	service, _ := newTestService(t)

	_, err := service.CreateOrder(context.Background(), &pb.CreateOrderRequest{
		UserId: 1,
		Items: []*pb.CreateOrderItem{
			{ProductId: 999, Quantity: 1},
		},
	})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("unexpected error code: got %v want %v", status.Code(err), codes.NotFound)
	}
}

func TestCreateOrderRejectsInsufficientStock(t *testing.T) {
	service, db := newTestService(t)
	item := createTestProduct(t, db, "库存商品", 12, 1)

	_, err := service.CreateOrder(context.Background(), &pb.CreateOrderRequest{
		UserId: 1,
		Items: []*pb.CreateOrderItem{
			{ProductId: int64(item.ID), Quantity: 2},
		},
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("unexpected error code: got %v want %v", status.Code(err), codes.InvalidArgument)
	}
}

func TestCreateOrderRejectsInvalidQuantity(t *testing.T) {
	tests := []struct {
		name     string
		quantity int32
	}{
		{name: "zero", quantity: 0},
		{name: "negative", quantity: -1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service, db := newTestService(t)
			item := createTestProduct(t, db, "数量商品", 10, 5)

			_, err := service.CreateOrder(context.Background(), &pb.CreateOrderRequest{
				UserId: 1,
				Items: []*pb.CreateOrderItem{
					{ProductId: int64(item.ID), Quantity: tc.quantity},
				},
			})
			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("unexpected error code: got %v want %v", status.Code(err), codes.InvalidArgument)
			}
		})
	}
}

func TestCreateOrderRejectsEmptyItems(t *testing.T) {
	service, _ := newTestService(t)

	_, err := service.CreateOrder(context.Background(), &pb.CreateOrderRequest{
		UserId: 1,
		Items:  nil,
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("unexpected error code: got %v want %v", status.Code(err), codes.InvalidArgument)
	}
}

func TestCreateOrderCalculatesTotalAcrossProducts(t *testing.T) {
	service, db := newTestService(t)
	first := createTestProduct(t, db, "商品A", 10, 10)
	second := createTestProduct(t, db, "商品B", 20.5, 10)

	resp, err := service.CreateOrder(context.Background(), &pb.CreateOrderRequest{
		UserId: 1,
		Items: []*pb.CreateOrderItem{
			{ProductId: int64(first.ID), Quantity: 2},
			{ProductId: int64(second.ID), Quantity: 3},
		},
	})
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}

	if got, want := resp.Order.TotalAmount, float32(81.5); got != want {
		t.Fatalf("unexpected total amount: got %.2f want %.2f", got, want)
	}
}

func TestCreateOrderMergesDuplicateProducts(t *testing.T) {
	service, db := newTestService(t)
	item := createTestProduct(t, db, "重复商品", 15, 10)

	resp, err := service.CreateOrder(context.Background(), &pb.CreateOrderRequest{
		UserId: 1,
		Items: []*pb.CreateOrderItem{
			{ProductId: int64(item.ID), Quantity: 1},
			{ProductId: int64(item.ID), Quantity: 2},
		},
	})
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}

	if len(resp.Order.Items) != 1 {
		t.Fatalf("unexpected order item count: got %d want 1", len(resp.Order.Items))
	}
	if got, want := resp.Order.Items[0].Quantity, int32(3); got != want {
		t.Fatalf("unexpected merged quantity: got %d want %d", got, want)
	}
	if got, want := resp.Order.TotalAmount, float32(45); got != want {
		t.Fatalf("unexpected total amount: got %.2f want %.2f", got, want)
	}

	var latest product.Product
	if err := db.First(&latest, item.ID).Error; err != nil {
		t.Fatalf("failed to query updated product: %v", err)
	}
	if got, want := latest.Stock, int32(7); got != want {
		t.Fatalf("unexpected remaining stock: got %d want %d", got, want)
	}
}
