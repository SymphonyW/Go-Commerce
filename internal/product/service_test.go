package product

import (
	"context"
	"testing"
	"time"

	pb "go-commerce/api/product"
	"gorm.io/gorm"
)

func seedListProducts(t *testing.T) *Service {
	t.Helper()

	db := newStockTestDB(t)
	baseTime := time.Date(2026, 5, 16, 8, 0, 0, 0, time.UTC)
	products := []Product{
		{Name: "Go 入门", Description: "适合新手", PriceCents: 3900, Stock: 8, Category: "book", MerchantID: 1, Model: modelAt(baseTime.Add(1 * time.Minute))},
		{Name: "机械键盘", Description: "适合 Go 开发", PriceCents: 29900, Stock: 15, Category: "digital", MerchantID: 1, Model: modelAt(baseTime.Add(2 * time.Minute))},
		{Name: "咖啡豆", Description: "深烘焙风味", PriceCents: 5900, Stock: 30, Category: "food", MerchantID: 1, Model: modelAt(baseTime.Add(3 * time.Minute))},
		{Name: "Go 高级编程", Description: "覆盖并发与网络", PriceCents: 7900, Stock: 6, Category: "book", MerchantID: 1, Model: modelAt(baseTime.Add(4 * time.Minute))},
		{Name: "鼠标", Description: "轻量无线", PriceCents: 12900, Stock: 20, Category: "digital", MerchantID: 1, Model: modelAt(baseTime.Add(5 * time.Minute))},
		{Name: "显示器", Description: "4K 面板", PriceCents: 129900, Stock: 4, Category: "digital", MerchantID: 1, Model: modelAt(baseTime.Add(6 * time.Minute))},
		{Name: "算法书", Description: "包含 Go 语言示例", PriceCents: 8900, Stock: 9, Category: "book", MerchantID: 1, Model: modelAt(baseTime.Add(7 * time.Minute))},
		{Name: "茶具", Description: "陶瓷套装", PriceCents: 15900, Stock: 12, Category: "home", MerchantID: 1, Model: modelAt(baseTime.Add(8 * time.Minute))},
		{Name: "保温杯", Description: "304 不锈钢", PriceCents: 6900, Stock: 18, Category: "home", MerchantID: 1, Model: modelAt(baseTime.Add(9 * time.Minute))},
		{Name: "台灯", Description: "护眼阅读灯", PriceCents: 19900, Stock: 7, Category: "home", MerchantID: 1, Model: modelAt(baseTime.Add(10 * time.Minute))},
		{Name: "路由器", Description: "WiFi 6", PriceCents: 34900, Stock: 11, Category: "digital", MerchantID: 1, Model: modelAt(baseTime.Add(11 * time.Minute))},
		{Name: "Go 实战", Description: "项目驱动", PriceCents: 9900, Stock: 5, Category: "book", MerchantID: 1, Model: modelAt(baseTime.Add(12 * time.Minute))},
	}

	if err := db.Create(&products).Error; err != nil {
		t.Fatalf("failed to seed products: %v", err)
	}

	return NewService(db)
}

func modelAt(createdAt time.Time) gorm.Model {
	return gorm.Model{
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	}
}

func TestListProductsUsesDefaultPagination(t *testing.T) {
	service := seedListProducts(t)

	resp, err := service.ListProducts(context.Background(), &pb.ListProductsRequest{})
	if err != nil {
		t.Fatalf("ListProducts returned error: %v", err)
	}
	if got, want := len(resp.Products), 10; got != want {
		t.Fatalf("unexpected default page size: got %d want %d", got, want)
	}
	if got, want := resp.Total, int64(12); got != want {
		t.Fatalf("unexpected total: got %d want %d", got, want)
	}
}

func TestCreateProductPersistsProductSnapshot(t *testing.T) {
	service := seedListProducts(t)

	resp, err := service.CreateProduct(context.Background(), &pb.CreateProductRequest{
		Name:        "测试商品",
		Description: "来自 create-product 单测",
		PriceCents:  8850,
		Stock:       6,
		Category:    "book",
		ImageUrl:    "https://example.com/test.png",
		MerchantId:  9,
	})
	if err != nil {
		t.Fatalf("CreateProduct returned error: %v", err)
	}
	if resp.Product.Id == 0 {
		t.Fatal("expected created product id")
	}
	if got, want := resp.Product.MerchantId, int64(9); got != want {
		t.Fatalf("unexpected merchant id: got %d want %d", got, want)
	}

	got, err := service.GetProduct(context.Background(), &pb.GetProductRequest{Id: resp.Product.Id})
	if err != nil {
		t.Fatalf("GetProduct returned error: %v", err)
	}
	if got.Product.Name != "测试商品" || got.Product.PriceCents != 8850 || got.Product.Stock != 6 {
		t.Fatalf("unexpected persisted product: %+v", got.Product)
	}
}

func TestCreateProductAcceptsZeroAndOneCent(t *testing.T) {
	service := seedListProducts(t)

	for _, priceCents := range []int64{0, 1} {
		resp, err := service.CreateProduct(context.Background(), &pb.CreateProductRequest{
			Name:        "edge price",
			Description: "money edge case",
			PriceCents:  priceCents,
			Stock:       1,
			Category:    "book",
			MerchantId:  9,
		})
		if err != nil {
			t.Fatalf("CreateProduct(%d) returned error: %v", priceCents, err)
		}
		if got := resp.Product.PriceCents; got != priceCents {
			t.Fatalf("unexpected price_cents: got %d want %d", got, priceCents)
		}
	}
}

func TestCreateProductRejectsNegativePriceCents(t *testing.T) {
	service := seedListProducts(t)

	_, err := service.CreateProduct(context.Background(), &pb.CreateProductRequest{
		Name:       "invalid price",
		PriceCents: -1,
		Stock:      1,
		Category:   "book",
		MerchantId: 9,
	})
	if err == nil {
		t.Fatal("expected negative price_cents to be rejected")
	}
}

func TestListProductsAppliesPageAndPageSize(t *testing.T) {
	service := seedListProducts(t)

	resp, err := service.ListProducts(context.Background(), &pb.ListProductsRequest{Page: 2, PageSize: 3})
	if err != nil {
		t.Fatalf("ListProducts returned error: %v", err)
	}
	if got, want := len(resp.Products), 3; got != want {
		t.Fatalf("unexpected page size: got %d want %d", got, want)
	}
	if got, want := resp.Products[0].Name, "保温杯"; got != want {
		t.Fatalf("unexpected first item on second page: got %q want %q", got, want)
	}
}

func TestListProductsFiltersByCategory(t *testing.T) {
	service := seedListProducts(t)

	resp, err := service.ListProducts(context.Background(), &pb.ListProductsRequest{Category: "book"})
	if err != nil {
		t.Fatalf("ListProducts returned error: %v", err)
	}
	if got, want := resp.Total, int64(4); got != want {
		t.Fatalf("unexpected total: got %d want %d", got, want)
	}
	for _, product := range resp.Products {
		if got, want := product.Category, "book"; got != want {
			t.Fatalf("unexpected category: got %q want %q", got, want)
		}
	}
}

func TestListProductsSearchesKeywordByName(t *testing.T) {
	service := seedListProducts(t)

	resp, err := service.ListProducts(context.Background(), &pb.ListProductsRequest{Keyword: "键盘"})
	if err != nil {
		t.Fatalf("ListProducts returned error: %v", err)
	}
	if got, want := resp.Total, int64(1); got != want {
		t.Fatalf("unexpected total: got %d want %d", got, want)
	}
	if got, want := resp.Products[0].Name, "机械键盘"; got != want {
		t.Fatalf("unexpected matched product: got %q want %q", got, want)
	}
}

func TestListProductsSearchesKeywordByDescription(t *testing.T) {
	service := seedListProducts(t)

	resp, err := service.ListProducts(context.Background(), &pb.ListProductsRequest{Keyword: "并发"})
	if err != nil {
		t.Fatalf("ListProducts returned error: %v", err)
	}
	if got, want := resp.Total, int64(1); got != want {
		t.Fatalf("unexpected total: got %d want %d", got, want)
	}
	if got, want := resp.Products[0].Name, "Go 高级编程"; got != want {
		t.Fatalf("unexpected matched product: got %q want %q", got, want)
	}
}

func TestListProductsFiltersByPriceRange(t *testing.T) {
	service := seedListProducts(t)
	minPriceCents, maxPriceCents := int64(6000), int64(10000)

	resp, err := service.ListProducts(context.Background(), &pb.ListProductsRequest{
		MinPriceCents: &minPriceCents,
		MaxPriceCents: &maxPriceCents,
	})
	if err != nil {
		t.Fatalf("ListProducts returned error: %v", err)
	}
	if got, want := resp.Total, int64(4); got != want {
		t.Fatalf("unexpected total: got %d want %d", got, want)
	}
	for _, product := range resp.Products {
		if product.PriceCents < minPriceCents || product.PriceCents > maxPriceCents {
			t.Fatalf("price_cents out of range: got %d want between %d and %d", product.PriceCents, minPriceCents, maxPriceCents)
		}
	}
}

func TestListProductsSortsByPriceAscending(t *testing.T) {
	service := seedListProducts(t)

	resp, err := service.ListProducts(context.Background(), &pb.ListProductsRequest{
		PageSize: 3,
		SortBy:   "price",
		Order:    "asc",
	})
	if err != nil {
		t.Fatalf("ListProducts returned error: %v", err)
	}
	if got, want := []string{resp.Products[0].Name, resp.Products[1].Name, resp.Products[2].Name}, []string{"Go 入门", "咖啡豆", "保温杯"}; !equalStrings(got, want) {
		t.Fatalf("unexpected sorted products: got %v want %v", got, want)
	}
}

func TestListProductsFallsBackToDefaultSortForInvalidSortBy(t *testing.T) {
	service := seedListProducts(t)

	resp, err := service.ListProducts(context.Background(), &pb.ListProductsRequest{
		PageSize: 3,
		SortBy:   "name",
		Order:    "asc",
	})
	if err != nil {
		t.Fatalf("ListProducts returned error: %v", err)
	}
	if got, want := []string{resp.Products[0].Name, resp.Products[1].Name, resp.Products[2].Name}, []string{"Go 实战", "路由器", "台灯"}; !equalStrings(got, want) {
		t.Fatalf("unexpected fallback order: got %v want %v", got, want)
	}
}

func TestListProductsCountsOnlyFilteredRows(t *testing.T) {
	service := seedListProducts(t)
	minPriceCents, maxPriceCents := int64(7000), int64(10000)

	resp, err := service.ListProducts(context.Background(), &pb.ListProductsRequest{
		Category:      "book",
		Keyword:       "Go",
		MinPriceCents: &minPriceCents,
		MaxPriceCents: &maxPriceCents,
	})
	if err != nil {
		t.Fatalf("ListProducts returned error: %v", err)
	}
	if got, want := resp.Total, int64(3); got != want {
		t.Fatalf("unexpected filtered total: got %d want %d", got, want)
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
