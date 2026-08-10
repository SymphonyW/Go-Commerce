package merchant

import (
	"context"
	"fmt"
	"strings"
	"testing"

	pb "go-commerce/api/merchant"
	"go-commerce/internal/auth"
	"go-commerce/internal/product"

	"github.com/glebarez/sqlite"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

func newMerchantTestService(t *testing.T) (*GRPCService, *gorm.DB) {
	t.Helper()

	dsn := fmt.Sprintf(
		"file:%s?mode=memory&cache=shared",
		strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()),
	)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	if err := db.AutoMigrate(&auth.User{}, &Merchant{}, &product.Product{}); err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}

	return NewGRPCService(db), db
}

func createMerchantTestUser(t *testing.T, db *gorm.DB, username, role string) auth.User {
	t.Helper()

	user := auth.User{
		Username: username,
		Password: "hashed-password",
		Email:    username + "@example.com",
		Role:     role,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	return user
}

func createOwnedMerchant(t *testing.T, db *gorm.DB, name string, ownerID uint) Merchant {
	t.Helper()

	merchant := Merchant{
		Name:        name,
		ContactInfo: name + "@example.com",
		OwnerUserID: &ownerID,
	}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}

	return merchant
}

func TestCreateMerchantBindsCurrentMerchantUser(t *testing.T) {
	service, db := newMerchantTestService(t)
	merchantUser := createMerchantTestUser(t, db, "merchant-owner", "merchant")

	resp, err := service.CreateMerchant(context.Background(), &pb.CreateMerchantRequest{
		Name:        "Owner Shop",
		ContactInfo: "owner@example.com",
		OwnerUserId: int64(merchantUser.ID),
	})
	if err != nil {
		t.Fatalf("CreateMerchant returned error: %v", err)
	}
	if resp.Merchant.OwnerUserId != int64(merchantUser.ID) {
		t.Fatalf("unexpected owner user id: got %d want %d", resp.Merchant.OwnerUserId, merchantUser.ID)
	}
}

func TestMerchantCanManageOwnProducts(t *testing.T) {
	service, db := newMerchantTestService(t)
	merchantUser := createMerchantTestUser(t, db, "merchant-self", "merchant")
	merchant := createOwnedMerchant(t, db, "Self Shop", merchantUser.ID)

	addResp, err := service.AddProduct(context.Background(), &pb.AddProductRequest{
		MerchantId:  int64(merchant.ID),
		Name:        "Owned Product",
		Description: "desc",
		PriceCents:  1000,
		Stock:       3,
		Category:    "demo",
		ImageUrl:    "https://example.com/image.jpg",
		ActorUserId: int64(merchantUser.ID),
	})
	if err != nil {
		t.Fatalf("AddProduct returned error: %v", err)
	}

	deleteResp, err := service.DeleteProduct(context.Background(), &pb.DeleteProductRequest{
		MerchantId:  int64(merchant.ID),
		ProductId:   addResp.ProductId,
		ActorUserId: int64(merchantUser.ID),
	})
	if err != nil {
		t.Fatalf("DeleteProduct returned error: %v", err)
	}
	if !deleteResp.Success {
		t.Fatal("expected delete to succeed")
	}
}

func TestMerchantCannotManageOtherMerchantProducts(t *testing.T) {
	service, db := newMerchantTestService(t)
	owner := createMerchantTestUser(t, db, "owner-user", "merchant")
	otherMerchant := createMerchantTestUser(t, db, "other-merchant", "merchant")
	merchant := createOwnedMerchant(t, db, "Owner Shop", owner.ID)

	_, err := service.AddProduct(context.Background(), &pb.AddProductRequest{
		MerchantId:  int64(merchant.ID),
		Name:        "Forbidden Product",
		Description: "desc",
		PriceCents:  1000,
		Stock:       3,
		Category:    "demo",
		ImageUrl:    "https://example.com/image.jpg",
		ActorUserId: int64(otherMerchant.ID),
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("unexpected error code: got %v want %v", status.Code(err), codes.PermissionDenied)
	}
}

func TestAdminCanManageAnyMerchant(t *testing.T) {
	service, db := newMerchantTestService(t)
	owner := createMerchantTestUser(t, db, "shop-owner", "merchant")
	admin := createMerchantTestUser(t, db, "admin-user", "admin")
	merchant := createOwnedMerchant(t, db, "Admin Shop", owner.ID)

	if _, err := service.AddProduct(context.Background(), &pb.AddProductRequest{
		MerchantId:  int64(merchant.ID),
		Name:        "Admin Product",
		Description: "desc",
		PriceCents:  1000,
		Stock:       3,
		Category:    "demo",
		ImageUrl:    "https://example.com/image.jpg",
		ActorUserId: int64(admin.ID),
	}); err != nil {
		t.Fatalf("AddProduct returned error: %v", err)
	}
}

func TestCustomerCannotUseMerchantWriteOperations(t *testing.T) {
	service, db := newMerchantTestService(t)
	customer := createMerchantTestUser(t, db, "customer-user", "customer")

	_, err := service.CreateMerchant(context.Background(), &pb.CreateMerchantRequest{
		Name:        "Denied Shop",
		ContactInfo: "denied@example.com",
		OwnerUserId: int64(customer.ID),
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("unexpected error code: got %v want %v", status.Code(err), codes.PermissionDenied)
	}
}

func TestMerchantOnlyListsOwnProducts(t *testing.T) {
	service, db := newMerchantTestService(t)
	owner := createMerchantTestUser(t, db, "product-owner", "merchant")
	other := createMerchantTestUser(t, db, "product-other", "merchant")
	ownShop := createOwnedMerchant(t, db, "Own Shop", owner.ID)
	otherShop := createOwnedMerchant(t, db, "Other Shop", other.ID)

	if err := db.Create(&product.Product{Name: "Own Product", PriceCents: 1000, Stock: 3, MerchantID: ownShop.ID}).Error; err != nil {
		t.Fatalf("failed to create own product: %v", err)
	}
	if err := db.Create(&product.Product{Name: "Other Product", PriceCents: 2000, Stock: 5, MerchantID: otherShop.ID}).Error; err != nil {
		t.Fatalf("failed to create other product: %v", err)
	}

	resp, err := service.ListMerchantProducts(context.Background(), &pb.ListMerchantProductsRequest{
		ActorUserId: int64(owner.ID),
		Page:        1,
		PageSize:    10,
	})
	if err != nil {
		t.Fatalf("ListMerchantProducts returned error: %v", err)
	}
	if got, want := resp.Total, int64(1); got != want {
		t.Fatalf("unexpected product total: got %d want %d", got, want)
	}
	if got, want := len(resp.Products), 1; got != want {
		t.Fatalf("unexpected product count: got %d want %d", got, want)
	}
	if got, want := resp.Products[0].Name, "Own Product"; got != want {
		t.Fatalf("unexpected product name: got %q want %q", got, want)
	}
}

func TestMerchantCannotUpdateForeignProduct(t *testing.T) {
	service, db := newMerchantTestService(t)
	owner := createMerchantTestUser(t, db, "foreign-owner", "merchant")
	other := createMerchantTestUser(t, db, "foreign-actor", "merchant")
	shop := createOwnedMerchant(t, db, "Foreign Shop", owner.ID)

	target := product.Product{Name: "Protected Product", PriceCents: 1000, Stock: 3, MerchantID: shop.ID}
	if err := db.Create(&target).Error; err != nil {
		t.Fatalf("failed to create product: %v", err)
	}

	newPrice := int64(9900)
	_, err := service.UpdateMerchantProduct(context.Background(), &pb.UpdateMerchantProductRequest{
		ProductId:   int64(target.ID),
		ActorUserId: int64(other.ID),
		PriceCents:  &newPrice,
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("unexpected error code: got %v want %v", status.Code(err), codes.PermissionDenied)
	}
}
