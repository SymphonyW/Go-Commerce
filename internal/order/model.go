package order

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Order stores the order header. CreatedAt is indexed because order list
// queries page by the newest rows first.
type Order struct {
	gorm.Model
	UserID       uint      `gorm:"not null;index"`
	TotalAmount  float64   `gorm:"not null"`
	Status       string    `gorm:"not null;default:'pending'"`
	CancelReason string    `gorm:"size:64"`
	OrderDate    time.Time `gorm:"not null"`
}

// OrderItem stores immutable product snapshots captured at order creation.
// MerchantID is a snapshot; historical rows can be zero when created before
// merchant attribution existed.
type OrderItem struct {
	gorm.Model
	OrderID     uint    `gorm:"not null;index"`
	ProductID   int64   `gorm:"not null"`
	MerchantID  uint    `gorm:"index"`
	ProductName string  `gorm:"not null"`
	Price       float64 `gorm:"not null"`
	Quantity    int32   `gorm:"not null"`
}

// EnsureOrderIndexes creates indexes that cannot be declared directly on
// embedded gorm.Model fields while preserving Order's public struct shape.
func EnsureOrderIndexes(db *gorm.DB) error {
	return ensureIndex(db, &Order{}, "idx_orders_created_at", "orders", "created_at")
}

func ensureIndex(db *gorm.DB, model interface{}, indexName, tableName string, columns ...string) error {
	if db.Migrator().HasIndex(model, indexName) {
		return nil
	}

	quotedColumns := make([]string, len(columns))
	for i, column := range columns {
		quotedColumns[i] = quoteIdentifier(db, column)
	}
	sql := fmt.Sprintf(
		"CREATE INDEX %s ON %s (%s)",
		quoteIdentifier(db, indexName),
		quoteIdentifier(db, tableName),
		strings.Join(quotedColumns, ", "),
	)
	return db.Exec(sql).Error
}

func quoteIdentifier(db *gorm.DB, name string) string {
	if db != nil && db.Dialector != nil && db.Dialector.Name() == "mysql" {
		return "`" + strings.ReplaceAll(name, "`", "``") + "`"
	}
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
