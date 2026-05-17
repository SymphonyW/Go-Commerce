package main

import (
	"testing"

	"go-commerce/internal/auth"
	"go-commerce/internal/merchant"
	"go-commerce/internal/product"

	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func TestSeedDemoDataIsIdempotent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:seed-demo?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite database: %v", err)
	}
	if err := db.AutoMigrate(&auth.User{}, &merchant.Merchant{}, &product.Product{}); err != nil {
		t.Fatalf("failed to migrate tables: %v", err)
	}

	firstRun, err := seedDemoData(db)
	if err != nil {
		t.Fatalf("first seed run failed: %v", err)
	}
	if firstRun.MerchantsCreated != len(demoMerchants) {
		t.Fatalf("unexpected merchants created: got %d want %d", firstRun.MerchantsCreated, len(demoMerchants))
	}
	if firstRun.ProductsCreated != len(demoProducts) {
		t.Fatalf("unexpected products created: got %d want %d", firstRun.ProductsCreated, len(demoProducts))
	}
	if firstRun.UsersCreated != 1 {
		t.Fatalf("unexpected users created: got %d want %d", firstRun.UsersCreated, 1)
	}
	if firstRun.MerchantsBound != 0 {
		t.Fatalf("unexpected merchants bound on first run: got %d want %d", firstRun.MerchantsBound, 0)
	}

	secondRun, err := seedDemoData(db)
	if err != nil {
		t.Fatalf("second seed run failed: %v", err)
	}
	if secondRun.MerchantsCreated != 0 || secondRun.ProductsCreated != 0 {
		t.Fatalf("expected second seed run to create nothing, got merchants=%d products=%d", secondRun.MerchantsCreated, secondRun.ProductsCreated)
	}
	if secondRun.ProductsUpdated != 0 {
		t.Fatalf("expected second seed run to update nothing, got products_updated=%d", secondRun.ProductsUpdated)
	}
	if secondRun.UsersSkipped != 1 {
		t.Fatalf("unexpected users skipped: got %d want %d", secondRun.UsersSkipped, 1)
	}
	if secondRun.MerchantsSkipped != len(demoMerchants) {
		t.Fatalf("unexpected merchants skipped: got %d want %d", secondRun.MerchantsSkipped, len(demoMerchants))
	}
	if secondRun.ProductsSkipped != len(demoProducts) {
		t.Fatalf("unexpected products skipped: got %d want %d", secondRun.ProductsSkipped, len(demoProducts))
	}

	var merchantCount int64
	if err := db.Model(&merchant.Merchant{}).Count(&merchantCount).Error; err != nil {
		t.Fatalf("failed to count merchants: %v", err)
	}
	if merchantCount != int64(len(demoMerchants)) {
		t.Fatalf("unexpected merchant count: got %d want %d", merchantCount, len(demoMerchants))
	}

	var productCount int64
	if err := db.Model(&product.Product{}).Count(&productCount).Error; err != nil {
		t.Fatalf("failed to count products: %v", err)
	}
	if productCount != int64(len(demoProducts)) {
		t.Fatalf("unexpected product count: got %d want %d", productCount, len(demoProducts))
	}

	var seededUser auth.User
	if err := db.Where("username = ?", demoMerchantUser.Username).First(&seededUser).Error; err != nil {
		t.Fatalf("failed to find seeded merchant user: %v", err)
	}
	if seededUser.Role != auth.RoleMerchant {
		t.Fatalf("unexpected seeded merchant role: got %q want %q", seededUser.Role, auth.RoleMerchant)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(seededUser.Password), []byte(demoMerchantUser.Password)); err != nil {
		t.Fatalf("seeded merchant password does not match demo password: %v", err)
	}

	var merchants []merchant.Merchant
	if err := db.Order("id ASC").Find(&merchants).Error; err != nil {
		t.Fatalf("failed to load merchants: %v", err)
	}
	for _, item := range merchants {
		if item.OwnerUserID == nil || *item.OwnerUserID != seededUser.ID {
			t.Fatalf("merchant %q owner mismatch: got %v want %d", item.Name, item.OwnerUserID, seededUser.ID)
		}
	}
}

func TestSeedDemoDataRefreshesExistingProductImageURLs(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:seed-demo-refresh?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite database: %v", err)
	}
	if err := db.AutoMigrate(&auth.User{}, &merchant.Merchant{}, &product.Product{}); err != nil {
		t.Fatalf("failed to migrate tables: %v", err)
	}

	if _, err := seedDemoData(db); err != nil {
		t.Fatalf("initial seed run failed: %v", err)
	}

	var existing product.Product
	if err := db.Where("name = ?", demoProducts[0].Name).First(&existing).Error; err != nil {
		t.Fatalf("failed to find demo product: %v", err)
	}
	if err := db.Model(&existing).Update("image_url", "/demo-products/keyboard.svg").Error; err != nil {
		t.Fatalf("failed to overwrite demo product image: %v", err)
	}

	report, err := seedDemoData(db)
	if err != nil {
		t.Fatalf("refresh seed run failed: %v", err)
	}
	if report.ProductsUpdated != 1 {
		t.Fatalf("unexpected products updated: got %d want %d", report.ProductsUpdated, 1)
	}

	var refreshed product.Product
	if err := db.First(&refreshed, existing.ID).Error; err != nil {
		t.Fatalf("failed to reload demo product: %v", err)
	}
	if refreshed.ImageURL != demoProducts[0].ImageURL {
		t.Fatalf("unexpected refreshed image URL: got %q want %q", refreshed.ImageURL, demoProducts[0].ImageURL)
	}
}

func TestSeedDemoDataRefreshesMerchantBinding(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:seed-demo-binding?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite database: %v", err)
	}
	if err := db.AutoMigrate(&auth.User{}, &merchant.Merchant{}, &product.Product{}); err != nil {
		t.Fatalf("failed to migrate tables: %v", err)
	}

	if _, err := seedDemoData(db); err != nil {
		t.Fatalf("initial seed run failed: %v", err)
	}

	if err := db.Model(&merchant.Merchant{}).Where("name = ?", demoMerchants[0].Name).Update("owner_user_id", nil).Error; err != nil {
		t.Fatalf("failed to clear merchant owner: %v", err)
	}

	report, err := seedDemoData(db)
	if err != nil {
		t.Fatalf("refresh seed run failed: %v", err)
	}
	if report.MerchantsBound != 1 {
		t.Fatalf("unexpected merchants bound: got %d want %d", report.MerchantsBound, 1)
	}
}
