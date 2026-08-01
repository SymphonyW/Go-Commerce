package merchant

import (
	"errors"

	"go-commerce/internal/auth"
	"go-commerce/internal/product"

	"gorm.io/gorm"
)

var (
	ErrUserNotFound              = errors.New("user not found")
	ErrMerchantNotFound          = errors.New("merchant not found")
	ErrMerchantSelectionRequired = errors.New("merchant id is required for admin")
	ErrProductNotFound           = errors.New("product not found or not belong to this merchant")
	ErrPermissionDenied          = errors.New("permission denied")
)

// Service 商家领域服务，集中处理资源归属与权限校验。
type Service struct {
	DB *gorm.DB
}

// NewService 创建商家服务实例。
func NewService(db *gorm.DB) *Service {
	return &Service{DB: db}
}

// CreateMerchantForUser 为具备商家权限的用户创建商家，并绑定资源归属。
func (s *Service) CreateMerchantForUser(ownerUserID uint, merchantData Merchant) (Merchant, error) {
	actor, err := s.loadActor(ownerUserID)
	if err != nil {
		return Merchant{}, err
	}
	if !canManageMerchantWrites(actor.Role) {
		return Merchant{}, ErrPermissionDenied
	}

	merchantData.OwnerUserID = &ownerUserID
	if err := s.DB.Create(&merchantData).Error; err != nil {
		return Merchant{}, err
	}

	return merchantData, nil
}

// AddProductForUser 仅允许商家本人或管理员向目标商家新增商品。
func (s *Service) AddProductForUser(actorUserID uint, merchantID uint, productData product.Product) (uint, error) {
	if _, err := s.authorizeMerchantWrite(actorUserID, merchantID); err != nil {
		return 0, err
	}

	productData.MerchantID = merchantID
	if err := s.DB.Create(&productData).Error; err != nil {
		return 0, err
	}

	return productData.ID, nil
}

// DeleteProductForUser 仅允许商家本人或管理员删除目标商家下的商品。
func (s *Service) DeleteProductForUser(actorUserID uint, merchantID uint, productID uint) error {
	if _, err := s.authorizeMerchantWrite(actorUserID, merchantID); err != nil {
		return err
	}

	var productInfo product.Product
	if err := s.DB.Where("id = ? AND merchant_id = ?", productID, merchantID).First(&productInfo).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrProductNotFound
		}
		return err
	}

	if err := s.DB.Delete(&productInfo).Error; err != nil {
		return err
	}

	return nil
}

// CurrentMerchantForUser 返回当前控制台上下文下可操作的商家。
// 对普通商家而言，默认取其最早创建的店铺；管理员则必须显式指定 merchant_id，
// 这样可以避免后台操作在“多个店铺”场景下出现含糊目标。
func (s *Service) CurrentMerchantForUser(actorUserID uint, requestedMerchantID *uint) (Merchant, error) {
	actor, err := s.loadActor(actorUserID)
	if err != nil {
		return Merchant{}, err
	}
	if !canManageMerchantWrites(actor.Role) {
		return Merchant{}, ErrPermissionDenied
	}

	if actor.Role == auth.RoleAdmin {
		if requestedMerchantID == nil {
			return Merchant{}, ErrMerchantSelectionRequired
		}
		var merchant Merchant
		if err := s.DB.First(&merchant, *requestedMerchantID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return Merchant{}, ErrMerchantNotFound
			}
			return Merchant{}, err
		}
		return merchant, nil
	}

	query := s.DB.Where("owner_user_id = ?", actorUserID)
	if requestedMerchantID != nil {
		query = query.Where("id = ?", *requestedMerchantID)
	}

	var merchant Merchant
	if err := query.Order("created_at ASC").Order("id ASC").First(&merchant).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if requestedMerchantID != nil {
				return Merchant{}, ErrPermissionDenied
			}
			return Merchant{}, ErrMerchantNotFound
		}
		return Merchant{}, err
	}
	return merchant, nil
}

// ListProductsForUser 只返回当前商家控制台可见的商品，并在服务端完成分页。
func (s *Service) ListProductsForUser(actorUserID uint, requestedMerchantID *uint, page, pageSize int32) ([]product.Product, int64, error) {
	merchant, err := s.CurrentMerchantForUser(actorUserID, requestedMerchantID)
	if err != nil {
		return nil, 0, err
	}
	page, pageSize = normalizeMerchantPagination(page, pageSize)

	var total int64
	query := s.DB.Model(&product.Product{}).Where("merchant_id = ?", merchant.ID)
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var products []product.Product
	offset := (page - 1) * pageSize
	if err := query.
		Session(&gorm.Session{}).
		Order("created_at DESC").
		Order("id DESC").
		Offset(int(offset)).
		Limit(int(pageSize)).
		Find(&products).Error; err != nil {
		return nil, 0, err
	}

	return products, total, nil
}

// ProductUpdate 只携带本次需要变更的字段，避免把“未传值”和“清空值”混在一起。
type ProductUpdate struct {
	Name        *string
	Description *string
	PriceCents  *int64
	Stock       *int32
	Category    *string
	ImageURL    *string
}

// UpdateProductForUser 在更新前先校验当前操作者对目标商家的归属权限。
func (s *Service) UpdateProductForUser(actorUserID uint, requestedMerchantID *uint, productID uint, update ProductUpdate) (product.Product, error) {
	var productInfo product.Product
	if err := s.DB.First(&productInfo, productID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return product.Product{}, ErrProductNotFound
		}
		return product.Product{}, err
	}
	if requestedMerchantID != nil && productInfo.MerchantID != *requestedMerchantID {
		return product.Product{}, ErrProductNotFound
	}
	if _, err := s.authorizeMerchantWrite(actorUserID, productInfo.MerchantID); err != nil {
		return product.Product{}, err
	}

	updates := map[string]any{}
	if update.Name != nil {
		updates["name"] = *update.Name
	}
	if update.Description != nil {
		updates["description"] = *update.Description
	}
	if update.PriceCents != nil {
		updates["price_cents"] = *update.PriceCents
	}
	if update.Stock != nil {
		updates["stock"] = *update.Stock
	}
	if update.Category != nil {
		updates["category"] = *update.Category
	}
	if update.ImageURL != nil {
		updates["image_url"] = *update.ImageURL
	}
	if len(updates) == 0 {
		return productInfo, nil
	}

	if err := s.DB.Model(&productInfo).Updates(updates).Error; err != nil {
		return product.Product{}, err
	}
	if err := s.DB.First(&productInfo, productInfo.ID).Error; err != nil {
		return product.Product{}, err
	}
	return productInfo, nil
}

// authorizeMerchantWrite 以数据库中的真实角色与资源归属为准，避免只信任网关参数。
func (s *Service) authorizeMerchantWrite(actorUserID uint, merchantID uint) (Merchant, error) {
	actor, err := s.loadActor(actorUserID)
	if err != nil {
		return Merchant{}, err
	}
	if !canManageMerchantWrites(actor.Role) {
		return Merchant{}, ErrPermissionDenied
	}

	var merchant Merchant
	if err := s.DB.First(&merchant, merchantID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Merchant{}, ErrMerchantNotFound
		}
		return Merchant{}, err
	}

	if actor.Role != auth.RoleAdmin {
		// 历史商家记录若尚未回填 owner_user_id，默认不授予普通商家写权限。
		if merchant.OwnerUserID == nil || *merchant.OwnerUserID != actorUserID {
			return Merchant{}, ErrPermissionDenied
		}
	}

	return merchant, nil
}

func (s *Service) loadActor(userID uint) (auth.User, error) {
	var actor auth.User
	if err := s.DB.First(&actor, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return auth.User{}, ErrUserNotFound
		}
		return auth.User{}, err
	}

	actor.Role = normalizeActorRole(actor.Role)
	return actor, nil
}

func canManageMerchantWrites(role string) bool {
	return role == auth.RoleMerchant || role == auth.RoleAdmin
}

func normalizeActorRole(role string) string {
	switch role {
	case auth.RoleMerchant, auth.RoleAdmin:
		return role
	default:
		return auth.RoleCustomer
	}
}

func normalizeMerchantPagination(page, pageSize int32) (int32, int32) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}
