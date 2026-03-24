package merchant

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"

	pb "go-commerce/api/merchant"
	"go-commerce/internal/product"
)

// GRPCService 商家服务gRPC实现
type GRPCService struct {
	pb.UnimplementedMerchantServiceServer
	db *gorm.DB
}

// NewGRPCService 创建商家服务gRPC实例
func NewGRPCService(db *gorm.DB) *GRPCService {
	return &GRPCService{db: db}
}

// CreateMerchant 创建商家
func (s *GRPCService) CreateMerchant(ctx context.Context, req *pb.CreateMerchantRequest) (*pb.CreateMerchantResponse, error) {
	merchant := Merchant{
		Name:        req.Name,
		ContactInfo: req.ContactInfo,
	}

	if err := s.db.Create(&merchant).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create merchant: %v", err)
	}

	return &pb.CreateMerchantResponse{
		Merchant: &pb.Merchant{
			Id:          int64(merchant.ID),
			Name:        merchant.Name,
			ContactInfo: merchant.ContactInfo,
			CreatedAt:   merchant.CreatedAt.Format(time.RFC3339),
		},
	}, nil
}

// GetMerchant 获取商家信息
func (s *GRPCService) GetMerchant(ctx context.Context, req *pb.GetMerchantRequest) (*pb.GetMerchantResponse, error) {
	var merchant Merchant
	if err := s.db.First(&merchant, req.Id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, status.Errorf(codes.NotFound, "merchant not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to get merchant: %v", err)
	}

	return &pb.GetMerchantResponse{
		Merchant: &pb.Merchant{
			Id:          int64(merchant.ID),
			Name:        merchant.Name,
			ContactInfo: merchant.ContactInfo,
			CreatedAt:   merchant.CreatedAt.Format(time.RFC3339),
		},
	}, nil
}

// ListMerchants 列出商家
func (s *GRPCService) ListMerchants(ctx context.Context, req *pb.ListMerchantsRequest) (*pb.ListMerchantsResponse, error) {
	var merchants []Merchant
	var total int64

	query := s.db.Model(&Merchant{})
	query.Count(&total)

	offset := (req.Page - 1) * req.PageSize
	query.Offset(int(offset)).Limit(int(req.PageSize)).Find(&merchants)

	pbMerchants := make([]*pb.Merchant, len(merchants))
	for i, merchant := range merchants {
		pbMerchants[i] = &pb.Merchant{
			Id:          int64(merchant.ID),
			Name:        merchant.Name,
			ContactInfo: merchant.ContactInfo,
			CreatedAt:   merchant.CreatedAt.Format(time.RFC3339),
		}
	}

	return &pb.ListMerchantsResponse{
		Merchants: pbMerchants,
		Total:     total,
	}, nil
}

// AddProduct 商家新增商品
func (s *GRPCService) AddProduct(ctx context.Context, req *pb.AddProductRequest) (*pb.AddProductResponse, error) {
	// 验证商家是否存在
	var merchant Merchant
	if err := s.db.First(&merchant, req.MerchantId).Error; err != nil {
		return nil, status.Errorf(codes.NotFound, "merchant not found: %v", err)
	}

	// 创建商品
	product := product.Product{
		Name:        req.Name,
		Description: req.Description,
		Price:       float64(req.Price),
		Stock:       req.Stock,
		Category:    req.Category,
		ImageURL:    req.ImageUrl,
		MerchantID:  uint(req.MerchantId),
	}

	if err := s.db.Create(&product).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create product: %v", err)
	}

	return &pb.AddProductResponse{
		ProductId: int64(product.ID),
	}, nil
}

// DeleteProduct 商家删除自有商品
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

	return &pb.DeleteProductResponse{
		Success: true,
	}, nil
}
