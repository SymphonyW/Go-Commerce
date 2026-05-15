package product

import (
	"errors"

	"gorm.io/gorm"
)

var (
	ErrInvalidQuantity   = errors.New("quantity must be greater than zero")
	ErrProductNotFound   = errors.New("product not found")
	ErrInsufficientStock = errors.New("insufficient stock")
)

// DeductStock 通过数据库条件更新完成原子扣减，避免“先查后改”在并发下发生超卖。
func DeductStock(tx *gorm.DB, productID int64, quantity int32) error {
	if quantity <= 0 {
		return ErrInvalidQuantity
	}

	result := tx.Model(&Product{}).
		Where("id = ? AND stock >= ?", productID, quantity).
		UpdateColumn("stock", gorm.Expr("stock - ?", quantity))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		return nil
	}

	exists, err := productExists(tx, productID)
	if err != nil {
		return err
	}
	if !exists {
		return ErrProductNotFound
	}

	return ErrInsufficientStock
}

// RestoreStock 使用原子自增恢复库存，让取消订单的回补路径同样保持清晰。
func RestoreStock(tx *gorm.DB, productID int64, quantity int32) error {
	if quantity <= 0 {
		return ErrInvalidQuantity
	}

	result := tx.Model(&Product{}).
		Where("id = ?", productID).
		UpdateColumn("stock", gorm.Expr("stock + ?", quantity))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		return nil
	}

	return ErrProductNotFound
}

func productExists(tx *gorm.DB, productID int64) (bool, error) {
	var count int64
	if err := tx.Model(&Product{}).Where("id = ?", productID).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
