// merchant 包包含商家服务的模型和业务逻辑
// 负责处理商家的创建、查询、列表以及商品的添加和删除
package merchant

import (
	"context"
	"errors"
	"time"

	// gRPC状态码：用于返回标准化的错误信息
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	// GORM：ORM框架，用于数据库操作
	"gorm.io/gorm"

	// 导入商家服务的protobuf生成代码
	pb "go-commerce/api/merchant"
	// 导入产品模型：用于创建和管理商品
	"go-commerce/internal/product"
)

// GRPCService 商家服务gRPC实现
// 实现了MerchantServiceServer接口

type GRPCService struct {
	pb.UnimplementedMerchantServiceServer          // 嵌入未实现的MerchantServiceServer，以保持向后兼容性
	db                                    *gorm.DB // 数据库连接
	core                                  *Service // 领域服务，集中处理权限与归属校验
}

// NewGRPCService 创建商家服务gRPC实例
// 参数：
//
//	db: 数据库连接
//
// 返回值：
//
//	*GRPCService: 商家服务gRPC实例
func NewGRPCService(db *gorm.DB) *GRPCService {
	return &GRPCService{db: db, core: NewService(db)}
}

// CreateMerchant 创建商家
// 参数：
//
//	ctx: 上下文，用于控制请求的生命周期
//	req: 创建商家请求，包含商家名称和联系信息
//
// 返回值：
//
//	*pb.CreateMerchantResponse: 创建商家响应，包含创建的商家信息
//	error: 错误信息
func (s *GRPCService) CreateMerchant(ctx context.Context, req *pb.CreateMerchantRequest) (*pb.CreateMerchantResponse, error) {
	merchant, err := s.core.CreateMerchantForUser(uint(req.OwnerUserId), Merchant{
		Name:        req.Name,        // 商家名称
		ContactInfo: req.ContactInfo, // 联系信息
	})
	if err != nil {
		return nil, merchantStatusError(err, "failed to create merchant")
	}

	// 返回创建商家响应
	return &pb.CreateMerchantResponse{
		Merchant: &pb.Merchant{
			Id:          int64(merchant.ID),                      // 商家ID
			Name:        merchant.Name,                           // 商家名称
			ContactInfo: merchant.ContactInfo,                    // 联系信息
			CreatedAt:   merchant.CreatedAt.Format(time.RFC3339), // 创建时间
			OwnerUserId: ownerUserIDValue(merchant.OwnerUserID),
		},
	}, nil
}

// GetMerchant 获取商家信息
// 参数：
//
//	ctx: 上下文，用于控制请求的生命周期
//	req: 获取商家请求，包含商家ID
//
// 返回值：
//
//	*pb.GetMerchantResponse: 获取商家响应，包含商家详细信息
//	error: 错误信息
func (s *GRPCService) GetMerchant(ctx context.Context, req *pb.GetMerchantRequest) (*pb.GetMerchantResponse, error) {
	// 查找商家
	var merchant Merchant
	if err := s.db.First(&merchant, req.Id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, status.Errorf(codes.NotFound, "merchant not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to get merchant: %v", err)
	}

	// 返回获取商家响应
	return &pb.GetMerchantResponse{
		Merchant: &pb.Merchant{
			Id:          int64(merchant.ID),                      // 商家ID
			Name:        merchant.Name,                           // 商家名称
			ContactInfo: merchant.ContactInfo,                    // 联系信息
			CreatedAt:   merchant.CreatedAt.Format(time.RFC3339), // 创建时间
			OwnerUserId: ownerUserIDValue(merchant.OwnerUserID),
		},
	}, nil
}

// ListMerchants 列出商家
// 参数：
//
//	ctx: 上下文，用于控制请求的生命周期
//	req: 列出商家请求，包含页码和每页数量
//
// 返回值：
//
//	*pb.ListMerchantsResponse: 列出商家响应，包含商家列表和总数
//	error: 错误信息
func (s *GRPCService) ListMerchants(ctx context.Context, req *pb.ListMerchantsRequest) (*pb.ListMerchantsResponse, error) {
	var merchants []Merchant
	var total int64

	// 构建查询
	query := s.db.Model(&Merchant{})
	// 获取总数
	query.Count(&total)

	// 分页查询
	offset := (req.Page - 1) * req.PageSize
	query.Offset(int(offset)).Limit(int(req.PageSize)).Find(&merchants)

	// 转换为proto对象
	pbMerchants := make([]*pb.Merchant, len(merchants))
	for i, merchant := range merchants {
		pbMerchants[i] = &pb.Merchant{
			Id:          int64(merchant.ID),                      // 商家ID
			Name:        merchant.Name,                           // 商家名称
			ContactInfo: merchant.ContactInfo,                    // 联系信息
			CreatedAt:   merchant.CreatedAt.Format(time.RFC3339), // 创建时间
			OwnerUserId: ownerUserIDValue(merchant.OwnerUserID),
		}
	}

	// 返回列出商家响应
	return &pb.ListMerchantsResponse{
		Merchants: pbMerchants,
		Total:     total,
	}, nil
}

// AddProduct 商家新增商品
// 参数：
//
//	ctx: 上下文，用于控制请求的生命周期
//	req: 添加商品请求，包含商家ID和商品详细信息
//
// 返回值：
//
//	*pb.AddProductResponse: 添加商品响应，包含创建的商品ID
//	error: 错误信息
func (s *GRPCService) AddProduct(ctx context.Context, req *pb.AddProductRequest) (*pb.AddProductResponse, error) {
	productID, err := s.core.AddProductForUser(uint(req.ActorUserId), uint(req.MerchantId), product.Product{
		Name:        req.Name,        // 商品名称
		Description: req.Description, // 商品描述
		PriceCents:  req.PriceCents,  // 商品价格
		Stock:       req.Stock,       // 商品库存
		Category:    req.Category,    // 商品分类
		ImageURL:    req.ImageUrl,    // 商品图片URL
	})
	if err != nil {
		return nil, merchantStatusError(err, "failed to create product")
	}

	// 返回添加商品响应
	return &pb.AddProductResponse{
		ProductId: int64(productID),
	}, nil
}

// DeleteProduct 商家删除自有商品
// 参数：
//
//	ctx: 上下文，用于控制请求的生命周期
//	req: 删除商品请求，包含商家ID和商品ID
//
// 返回值：
//
//	*pb.DeleteProductResponse: 删除商品响应，包含删除是否成功
//	error: 错误信息
func (s *GRPCService) DeleteProduct(ctx context.Context, req *pb.DeleteProductRequest) (*pb.DeleteProductResponse, error) {
	if err := s.core.DeleteProductForUser(uint(req.ActorUserId), uint(req.MerchantId), uint(req.ProductId)); err != nil {
		return nil, merchantStatusError(err, "failed to delete product")
	}

	// 返回删除商品响应
	return &pb.DeleteProductResponse{
		Success: true,
	}, nil
}

// GetCurrentMerchant 获取当前登录商家的控制台店铺信息。
func (s *GRPCService) GetCurrentMerchant(ctx context.Context, req *pb.CurrentMerchantRequest) (*pb.CurrentMerchantResponse, error) {
	merchant, err := s.core.CurrentMerchantForUser(uint(req.ActorUserId), optionalMerchantID(req.MerchantId))
	if err != nil {
		return nil, merchantStatusError(err, "failed to get current merchant")
	}
	return &pb.CurrentMerchantResponse{Merchant: convertToPBMerchant(&merchant)}, nil
}

// ListMerchantProducts 列出当前控制台店铺下的商品。
func (s *GRPCService) ListMerchantProducts(ctx context.Context, req *pb.ListMerchantProductsRequest) (*pb.ListMerchantProductsResponse, error) {
	products, total, err := s.core.ListProductsForUser(
		uint(req.ActorUserId),
		optionalMerchantID(req.MerchantId),
		req.Page,
		req.PageSize,
	)
	if err != nil {
		return nil, merchantStatusError(err, "failed to list merchant products")
	}

	pbProducts := make([]*pb.MerchantProduct, len(products))
	for i := range products {
		pbProducts[i] = convertToPBMerchantProduct(&products[i])
	}
	return &pb.ListMerchantProductsResponse{Products: pbProducts, Total: total}, nil
}

// UpdateMerchantProduct 更新当前控制台店铺下的商品资料。
func (s *GRPCService) UpdateMerchantProduct(ctx context.Context, req *pb.UpdateMerchantProductRequest) (*pb.UpdateMerchantProductResponse, error) {
	productInfo, err := s.core.UpdateProductForUser(
		uint(req.ActorUserId),
		optionalMerchantID(req.MerchantId),
		uint(req.ProductId),
		ProductUpdate{
			Name:        req.Name,
			Description: req.Description,
			PriceCents:  req.PriceCents,
			Stock:       req.Stock,
			Category:    req.Category,
			ImageURL:    req.ImageUrl,
		},
	)
	if err != nil {
		return nil, merchantStatusError(err, "failed to update merchant product")
	}
	return &pb.UpdateMerchantProductResponse{Product: convertToPBMerchantProduct(&productInfo)}, nil
}

func ownerUserIDValue(ownerUserID *uint) int64 {
	if ownerUserID == nil {
		return 0
	}
	return int64(*ownerUserID)
}

func convertToPBMerchant(merchant *Merchant) *pb.Merchant {
	return &pb.Merchant{
		Id:          int64(merchant.ID),
		Name:        merchant.Name,
		ContactInfo: merchant.ContactInfo,
		CreatedAt:   merchant.CreatedAt.Format(time.RFC3339),
		OwnerUserId: ownerUserIDValue(merchant.OwnerUserID),
	}
}

func convertToPBMerchantProduct(productInfo *product.Product) *pb.MerchantProduct {
	return &pb.MerchantProduct{
		Id:          int64(productInfo.ID),
		Name:        productInfo.Name,
		Description: productInfo.Description,
		PriceCents:  productInfo.PriceCents,
		Stock:       productInfo.Stock,
		Category:    productInfo.Category,
		ImageUrl:    productInfo.ImageURL,
		MerchantId:  int64(productInfo.MerchantID),
	}
}

func optionalMerchantID(value *int64) *uint {
	if value == nil {
		return nil
	}
	merchantID := uint(*value)
	return &merchantID
}

func merchantStatusError(err error, fallback string) error {
	switch {
	case errors.Is(err, ErrPermissionDenied):
		return status.Error(codes.PermissionDenied, "merchant operation is not allowed")
	case errors.Is(err, ErrUserNotFound):
		return status.Error(codes.NotFound, "user not found")
	case errors.Is(err, ErrMerchantNotFound):
		return status.Error(codes.NotFound, "merchant not found")
	case errors.Is(err, ErrMerchantSelectionRequired):
		return status.Error(codes.InvalidArgument, "merchant id is required for admin")
	case errors.Is(err, ErrProductNotFound):
		return status.Error(codes.NotFound, "product not found or not belong to this merchant")
	default:
		return status.Errorf(codes.Internal, "%s: %v", fallback, err)
	}
}
