// merchant 包包含商家服务的模型和业务逻辑
// 负责处理商家的创建、查询、列表以及商品的添加和删除
package merchant

import (
	"context"
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
	pb.UnimplementedMerchantServiceServer // 嵌入未实现的MerchantServiceServer，以保持向后兼容性
	db *gorm.DB                         // 数据库连接
}

// NewGRPCService 创建商家服务gRPC实例
// 参数：
//   db: 数据库连接
// 返回值：
//   *GRPCService: 商家服务gRPC实例
func NewGRPCService(db *gorm.DB) *GRPCService {
	return &GRPCService{db: db}
}

// CreateMerchant 创建商家
// 参数：
//   ctx: 上下文，用于控制请求的生命周期
//   req: 创建商家请求，包含商家名称和联系信息
// 返回值：
//   *pb.CreateMerchantResponse: 创建商家响应，包含创建的商家信息
//   error: 错误信息
func (s *GRPCService) CreateMerchant(ctx context.Context, req *pb.CreateMerchantRequest) (*pb.CreateMerchantResponse, error) {
	// 构建商家对象
	merchant := Merchant{
		Name:        req.Name,        // 商家名称
		ContactInfo: req.ContactInfo, // 联系信息
	}

	// 保存到数据库
	if err := s.db.Create(&merchant).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create merchant: %v", err)
	}

	// 返回创建商家响应
	return &pb.CreateMerchantResponse{
		Merchant: &pb.Merchant{
			Id:          int64(merchant.ID),          // 商家ID
			Name:        merchant.Name,               // 商家名称
			ContactInfo: merchant.ContactInfo,        // 联系信息
			CreatedAt:   merchant.CreatedAt.Format(time.RFC3339), // 创建时间
		},
	}, nil
}

// GetMerchant 获取商家信息
// 参数：
//   ctx: 上下文，用于控制请求的生命周期
//   req: 获取商家请求，包含商家ID
// 返回值：
//   *pb.GetMerchantResponse: 获取商家响应，包含商家详细信息
//   error: 错误信息
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
			Id:          int64(merchant.ID),          // 商家ID
			Name:        merchant.Name,               // 商家名称
			ContactInfo: merchant.ContactInfo,        // 联系信息
			CreatedAt:   merchant.CreatedAt.Format(time.RFC3339), // 创建时间
		},
	}, nil
}

// ListMerchants 列出商家
// 参数：
//   ctx: 上下文，用于控制请求的生命周期
//   req: 列出商家请求，包含页码和每页数量
// 返回值：
//   *pb.ListMerchantsResponse: 列出商家响应，包含商家列表和总数
//   error: 错误信息
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
			Id:          int64(merchant.ID),          // 商家ID
			Name:        merchant.Name,               // 商家名称
			ContactInfo: merchant.ContactInfo,        // 联系信息
			CreatedAt:   merchant.CreatedAt.Format(time.RFC3339), // 创建时间
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
//   ctx: 上下文，用于控制请求的生命周期
//   req: 添加商品请求，包含商家ID和商品详细信息
// 返回值：
//   *pb.AddProductResponse: 添加商品响应，包含创建的商品ID
//   error: 错误信息
func (s *GRPCService) AddProduct(ctx context.Context, req *pb.AddProductRequest) (*pb.AddProductResponse, error) {
	// 验证商家是否存在
	var merchant Merchant
	if err := s.db.First(&merchant, req.MerchantId).Error; err != nil {
		return nil, status.Errorf(codes.NotFound, "merchant not found: %v", err)
	}

	// 创建商品
	product := product.Product{
		Name:        req.Name,        // 商品名称
		Description: req.Description, // 商品描述
		Price:       float64(req.Price), // 商品价格
		Stock:       req.Stock,       // 商品库存
		Category:    req.Category,    // 商品分类
		ImageURL:    req.ImageUrl,    // 商品图片URL
		MerchantID:  uint(req.MerchantId), // 商家ID
	}

	// 保存到数据库
	if err := s.db.Create(&product).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create product: %v", err)
	}

	// 返回添加商品响应
	return &pb.AddProductResponse{
		ProductId: int64(product.ID),
	}, nil
}

// DeleteProduct 商家删除自有商品
// 参数：
//   ctx: 上下文，用于控制请求的生命周期
//   req: 删除商品请求，包含商家ID和商品ID
// 返回值：
//   *pb.DeleteProductResponse: 删除商品响应，包含删除是否成功
//   error: 错误信息
func (s *GRPCService) DeleteProduct(ctx context.Context, req *pb.DeleteProductRequest) (*pb.DeleteProductResponse, error) {
	// 验证商家是否存在
	var merchant Merchant
	if err := s.db.First(&merchant, req.MerchantId).Error; err != nil {
		return nil, status.Errorf(codes.NotFound, "merchant not found: %v", err)
	}

	// 验证商品是否存在且属于该商家
	var product product.Product
	if err := s.db.Where("id = ? AND merchant_id = ?", req.ProductId, req.MerchantId).First(&product).Error; err != nil {
		return nil, status.Errorf(codes.NotFound, "product not found or not belong to this merchant")
	}

	// 删除商品
	if err := s.db.Delete(&product).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete product: %v", err)
	}

	// 返回删除商品响应
	return &pb.DeleteProductResponse{
		Success: true,
	}, nil
}
