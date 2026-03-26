// product 包包含产品服务的模型和业务逻辑
// 负责处理产品的创建、查询和列表
package product

import (
	"context"
	// gRPC状态码：用于返回标准化的错误信息
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	// GORM：ORM框架，用于数据库操作
	"gorm.io/gorm"

	// 导入产品服务的protobuf生成代码
	pb "go-commerce/api/product"
)

// Service 产品服务结构体
// 实现了ProductServiceServer接口

type Service struct {
	pb.UnimplementedProductServiceServer // 嵌入未实现的ProductServiceServer，以保持向后兼容性
	db *gorm.DB                         // 数据库连接
}

// NewService 创建产品服务实例
// 参数：
//   db: 数据库连接
// 返回值：
//   *Service: 产品服务实例
func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// CreateProduct 创建产品：创建新的产品记录
// 功能：创建新产品，设置商品归属商家
// 参数：
//   ctx: 上下文，用于控制请求的生命周期
//   req: 产品创建请求，包含产品的详细信息
// 返回值：
//   *pb.CreateProductResponse: 产品创建响应，包含创建的产品信息
//   error: 错误信息
func (s *Service) CreateProduct(ctx context.Context, req *pb.CreateProductRequest) (*pb.CreateProductResponse, error) {
	// 构建产品对象
	product := Product{
		Name:        req.Name,        // 产品名称
		Description: req.Description, // 产品描述
		Price:       float64(req.Price), // 产品价格
		Stock:       req.Stock,       // 产品库存
		Category:    req.Category,    // 产品分类
		ImageURL:    req.ImageUrl,    // 产品图片URL
		MerchantID:  uint(req.MerchantId), // 设置商家ID
	}

	// 保存到数据库
	if err := s.db.Create(&product).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create product: %v", err)
	}

	// 返回创建产品响应
	return &pb.CreateProductResponse{
		Product: convertToPBProduct(&product),
	}, nil
}

// GetProduct 获取产品：根据ID获取产品详情
// 参数：
//   ctx: 上下文，用于控制请求的生命周期
//   req: 获取产品请求，包含产品ID
// 返回值：
//   *pb.GetProductResponse: 获取产品响应，包含产品详情
//   error: 错误信息
func (s *Service) GetProduct(ctx context.Context, req *pb.GetProductRequest) (*pb.GetProductResponse, error) {
	// 查找产品
	var product Product
	if err := s.db.First(&product, req.Id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, status.Errorf(codes.NotFound, "product not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to get product: %v", err)
	}

	// 返回获取产品响应
	return &pb.GetProductResponse{
		Product: convertToPBProduct(&product),
	}, nil
}

// ListProducts 列出产品：获取产品列表，支持分页和分类筛选
// 参数：
//   ctx: 上下文，用于控制请求的生命周期
//   req: 列出产品请求，包含页码、每页数量和可选的分类筛选
// 返回值：
//   *pb.ListProductsResponse: 列出产品响应，包含产品列表和总数
//   error: 错误信息
func (s *Service) ListProducts(ctx context.Context, req *pb.ListProductsRequest) (*pb.ListProductsResponse, error) {
	var products []Product
	var total int64

	// 构建查询
	query := s.db.Model(&Product{})
	// 如果指定了分类，则按分类筛选
	if req.Category != "" {
		query = query.Where("category = ?", req.Category)
	}

	// 获取总数
	query.Count(&total)
	// 分页查询
	offset := (req.Page - 1) * req.PageSize
	query.Offset(int(offset)).Limit(int(req.PageSize)).Find(&products)

	// 转换为proto对象
	pbProducts := make([]*pb.Product, len(products))
	for i, product := range products {
		pbProducts[i] = convertToPBProduct(&product)
	}

	// 返回列出产品响应
	return &pb.ListProductsResponse{
		Products: pbProducts,
		Total:    total,
	}, nil
}

// convertToPBProduct 转换产品模型为proto对象
// 参数：
//   product: 产品模型对象
// 返回值：
//   *pb.Product: proto产品对象
func convertToPBProduct(product *Product) *pb.Product {
	return &pb.Product{
		Id:          int64(product.ID),          // 产品ID
		Name:        product.Name,               // 产品名称
		Description: product.Description,        // 产品描述
		Price:       float32(product.Price),     // 产品价格
		Stock:       int32(product.Stock),       // 产品库存
		Category:    product.Category,           // 产品分类
		ImageUrl:    product.ImageURL,           // 产品图片URL
		MerchantId:  int64(product.MerchantID),  // 商家ID
	}
}
