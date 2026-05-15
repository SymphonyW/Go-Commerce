package merchant

import (
	"errors"

	"go-commerce/internal/auth"
	"go-commerce/internal/product"

	"gorm.io/gorm"
)

var (
	ErrUserNotFound     = errors.New("user not found")
	ErrMerchantNotFound = errors.New("merchant not found")
	ErrProductNotFound  = errors.New("product not found or not belong to this merchant")
	ErrPermissionDenied = errors.New("permission denied")
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
