// 产品服务：处理产品的创建、查询和列表
package product

import (
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"

	pb "go-commerce/api/product"
)

// Service 产品服务结构体

type Service struct {
	pb.UnimplementedProductServiceServer
	db *gorm.DB // 数据库连接
}

// NewService 创建产品服务实例
func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// CreateProduct 创建产品：创建新的产品记录
// 功能：创建新产品，设置商品归属商家
// 参数：req (*pb.CreateProductRequest) - 产品创建请求
// 返回：(*pb.CreateProductResponse, error) - 产品创建响应和错误
func (s *Service) CreateProduct(ctx context.Context, req *pb.CreateProductRequest) (*pb.CreateProductResponse, error) {
	// 构建产品对象
	product := Product{
		Name:        req.Name,
		Description: req.Description,
		Price:       float64(req.Price),
		Stock:       req.Stock,
		Category:    req.Category,
		ImageURL:    req.ImageUrl,
		MerchantID:  uint(req.MerchantId), // 设置商家ID
	}

	// 保存到数据库
	if err := s.db.Create(&product).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create product: %v", err)
	}

	return &pb.CreateProductResponse{
		Product: convertToPBProduct(&product),
	}, nil
}

// GetProduct 获取产品：根据ID获取产品详情
func (s *Service) GetProduct(ctx context.Context, req *pb.GetProductRequest) (*pb.GetProductResponse, error) {
	// 查找产品
	var product Product
	if err := s.db.First(&product, req.Id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, status.Errorf(codes.NotFound, "product not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to get product: %v", err)
	}

	return &pb.GetProductResponse{
		Product: convertToPBProduct(&product),
	}, nil
}

// ListProducts 列出产品：获取产品列表，支持分页和分类筛选
func (s *Service) ListProducts(ctx context.Context, req *pb.ListProductsRequest) (*pb.ListProductsResponse, error) {
	var products []Product
	var total int64

	// 构建查询
	query := s.db.Model(&Product{})
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

	return &pb.ListProductsResponse{
		Products: pbProducts,
		Total:    total,
	}, nil
}

// convertToPBProduct 转换产品模型为proto对象
func convertToPBProduct(product *Product) *pb.Product {
	return &pb.Product{
		Id:          int64(product.ID),
		Name:        product.Name,
		Description: product.Description,
		Price:       float32(product.Price),
		Stock:       int32(product.Stock),
		Category:    product.Category,
		ImageUrl:    product.ImageURL,
		MerchantId:  int64(product.MerchantID), // 添加商家ID
	}
}
