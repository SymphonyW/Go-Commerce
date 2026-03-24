package merchant

import (
	"errors"
	"go-commerce/internal/product"
	"gorm.io/gorm"
)

// Service 商家服务
type Service struct {
	DB *gorm.DB
}

// NewService 创建商家服务实例
func NewService(db *gorm.DB) *Service {
	return &Service{DB: db}
}

// AddProduct 商家新增商品
// 参数：merchantID (uint) - 商家ID
// 参数：productData (product.Product) - 商品信息
// 返回：productID (uint) - 新增商品的ID
// 异常：error - 商家权限验证失败时抛出
func (s *Service) AddProduct(merchantID uint, productData product.Product) (uint, error) {
	// 验证商家是否存在
	var merchant Merchant
	if err := s.DB.First(&merchant, merchantID).Error; err != nil {
		return 0, errors.New("merchant not found")
	}

	// 设置商品归属商家
	productData.MerchantID = merchantID

	// 保存商品
	if err := s.DB.Create(&productData).Error; err != nil {
		return 0, err
	}

	return productData.ID, nil
}

// DeleteProduct 商家删除自有商品
// 参数：merchantID (uint) - 商家ID
// 参数：productID (uint) - 商品ID
// 返回：error - 操作错误
// 异常：error - 商家权限验证失败时抛出
func (s *Service) DeleteProduct(merchantID uint, productID uint) error {
	// 验证商家是否存在
	var merchant Merchant
	if err := s.DB.First(&merchant, merchantID).Error; err != nil {
		return errors.New("merchant not found")
	}

	// 验证商品是否存在且属于该商家
	var product product.Product
	if err := s.DB.Where("id = ? AND merchant_id = ?", productID, merchantID).First(&product).Error; err != nil {
		return errors.New("product not found or not belong to this merchant")
	}

	// 删除商品
	if err := s.DB.Delete(&product).Error; err != nil {
		return err
	}

	return nil
}
