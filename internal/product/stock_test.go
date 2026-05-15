package product

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newStockTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf(
		"file:%s?mode=memory&cache=shared",
		strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()),
	)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	if err := db.AutoMigrate(&Product{}); err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}

	return db
}

func TestDeductStockUsesAtomicCondition(t *testing.T) {
	db := newStockTestDB(t)
	item := Product{Name: "库存商品", Price: 10, Stock: 3, MerchantID: 1}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("failed to create product: %v", err)
	}

	if err := DeductStock(db, int64(item.ID), 2); err != nil {
		t.Fatalf("DeductStock returned error: %v", err)
	}

	var latest Product
	if err := db.First(&latest, item.ID).Error; err != nil {
		t.Fatalf("failed to query latest product: %v", err)
	}
	if got, want := latest.Stock, int32(1); got != want {
		t.Fatalf("unexpected remaining stock: got %d want %d", got, want)
	}

	if err := DeductStock(db, int64(item.ID), 2); !errors.Is(err, ErrInsufficientStock) {
		t.Fatalf("unexpected error: got %v want %v", err, ErrInsufficientStock)
	}
}

func TestDeductStockRejectsMissingProductAndInvalidQuantity(t *testing.T) {
	db := newStockTestDB(t)

	if err := DeductStock(db, 999, 1); !errors.Is(err, ErrProductNotFound) {
		t.Fatalf("unexpected missing-product error: got %v want %v", err, ErrProductNotFound)
	}
	if err := DeductStock(db, 1, 0); !errors.Is(err, ErrInvalidQuantity) {
		t.Fatalf("unexpected invalid-quantity error: got %v want %v", err, ErrInvalidQuantity)
	}
}

func TestRestoreStockAddsStockAtomically(t *testing.T) {
	db := newStockTestDB(t)
	item := Product{Name: "回补商品", Price: 10, Stock: 1, MerchantID: 1}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("failed to create product: %v", err)
	}

	if err := RestoreStock(db, int64(item.ID), 2); err != nil {
		t.Fatalf("RestoreStock returned error: %v", err)
	}

	var latest Product
	if err := db.First(&latest, item.ID).Error; err != nil {
		t.Fatalf("failed to query latest product: %v", err)
	}
	if got, want := latest.Stock, int32(3); got != want {
		t.Fatalf("unexpected restored stock: got %d want %d", got, want)
	}
}
