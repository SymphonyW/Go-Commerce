package product

import (
	"context"
	"strings"

	pb "go-commerce/api/product"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

type Service struct {
	pb.UnimplementedProductServiceServer
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

func (s *Service) CreateProduct(ctx context.Context, req *pb.CreateProductRequest) (*pb.CreateProductResponse, error) {
	if req == nil {
		req = &pb.CreateProductRequest{}
	}
	if req.PriceCents < 0 {
		return nil, status.Error(codes.InvalidArgument, "price_cents must be non-negative")
	}

	product := Product{
		Name:        req.Name,
		Description: req.Description,
		PriceCents:  req.PriceCents,
		Stock:       req.Stock,
		Category:    req.Category,
		ImageURL:    req.ImageUrl,
		MerchantID:  uint(req.MerchantId),
	}
	if err := s.db.WithContext(ctx).Create(&product).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create product: %v", err)
	}
	return &pb.CreateProductResponse{Product: convertToPBProduct(&product)}, nil
}

func (s *Service) GetProduct(ctx context.Context, req *pb.GetProductRequest) (*pb.GetProductResponse, error) {
	var product Product
	if err := s.db.WithContext(ctx).First(&product, req.Id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, status.Error(codes.NotFound, "product not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to get product: %v", err)
	}
	return &pb.GetProductResponse{Product: convertToPBProduct(&product)}, nil
}

func (s *Service) ListProducts(ctx context.Context, req *pb.ListProductsRequest) (*pb.ListProductsResponse, error) {
	if req == nil {
		req = &pb.ListProductsRequest{}
	}
	page, pageSize := normalizeProductPagination(req.Page, req.PageSize)
	if req.MinPriceCents != nil && *req.MinPriceCents < 0 {
		return nil, status.Error(codes.InvalidArgument, "min_price_cents must be non-negative")
	}
	if req.MaxPriceCents != nil && *req.MaxPriceCents < 0 {
		return nil, status.Error(codes.InvalidArgument, "max_price_cents must be non-negative")
	}
	if req.MinPriceCents != nil && req.MaxPriceCents != nil && *req.MinPriceCents > *req.MaxPriceCents {
		return nil, status.Error(codes.InvalidArgument, "min_price_cents must be less than or equal to max_price_cents")
	}

	db := s.db.WithContext(ctx)
	filteredQuery := applyProductFilters(db.Model(&Product{}), req)

	var total int64
	if err := filteredQuery.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "failed to count products: %v", err)
	}

	sortBy, order := normalizeProductSort(req.SortBy, req.Order)
	offset := (page - 1) * pageSize
	var products []Product
	if err := filteredQuery.
		Session(&gorm.Session{}).
		Order(sortBy + " " + order).
		Order("id " + order).
		Offset(int(offset)).
		Limit(int(pageSize)).
		Find(&products).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list products: %v", err)
	}

	pbProducts := make([]*pb.Product, len(products))
	for i, product := range products {
		pbProducts[i] = convertToPBProduct(&product)
	}
	return &pb.ListProductsResponse{Products: pbProducts, Total: total}, nil
}

const (
	defaultProductPage     int32 = 1
	defaultProductPageSize int32 = 10
	maxProductPageSize     int32 = 100
)

func normalizeProductPagination(page, pageSize int32) (int32, int32) {
	if page <= 0 {
		page = defaultProductPage
	}
	if pageSize <= 0 {
		pageSize = defaultProductPageSize
	}
	if pageSize > maxProductPageSize {
		pageSize = maxProductPageSize
	}
	return page, pageSize
}

func applyProductFilters(query *gorm.DB, req *pb.ListProductsRequest) *gorm.DB {
	if category := strings.TrimSpace(req.Category); category != "" {
		query = query.Where("category = ?", category)
	}
	if keyword := strings.TrimSpace(req.Keyword); keyword != "" {
		pattern := "%" + keyword + "%"
		query = query.Where("(name LIKE ? OR description LIKE ?)", pattern, pattern)
	}
	if req.MinPriceCents != nil {
		query = query.Where("price_cents >= ?", *req.MinPriceCents)
	}
	if req.MaxPriceCents != nil {
		query = query.Where("price_cents <= ?", *req.MaxPriceCents)
	}
	return query
}

func normalizeProductSort(sortBy, order string) (string, string) {
	invalidSortBy := false
	switch strings.TrimSpace(sortBy) {
	case "price_cents", "stock", "created_at":
		sortBy = strings.TrimSpace(sortBy)
	case "price":
		sortBy = "price_cents"
	default:
		sortBy = "created_at"
		invalidSortBy = true
	}

	if invalidSortBy {
		return sortBy, "desc"
	}

	switch strings.ToLower(strings.TrimSpace(order)) {
	case "asc":
		order = "asc"
	case "desc":
		order = "desc"
	default:
		order = "desc"
	}

	return sortBy, order
}

func (s *Service) UpdateProduct(ctx context.Context, req *pb.UpdateProductRequest) (*pb.UpdateProductResponse, error) {
	var product Product
	if err := s.db.WithContext(ctx).First(&product, req.Id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, status.Error(codes.NotFound, "product not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to get product: %v", err)
	}

	if req.Name != nil {
		product.Name = *req.Name
	}
	if req.Description != nil {
		product.Description = *req.Description
	}
	if req.PriceCents != nil {
		if *req.PriceCents < 0 {
			return nil, status.Error(codes.InvalidArgument, "price_cents must be non-negative")
		}
		product.PriceCents = *req.PriceCents
	}
	if req.Stock != nil {
		product.Stock = *req.Stock
	}
	if req.Category != nil {
		product.Category = *req.Category
	}
	if req.ImageUrl != nil {
		product.ImageURL = *req.ImageUrl
	}

	if err := s.db.WithContext(ctx).Save(&product).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update product: %v", err)
	}
	return &pb.UpdateProductResponse{Product: convertToPBProduct(&product)}, nil
}

func (s *Service) DeleteProduct(ctx context.Context, req *pb.DeleteProductRequest) (*pb.DeleteProductResponse, error) {
	if err := s.db.WithContext(ctx).Delete(&Product{}, req.Id).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete product: %v", err)
	}
	return &pb.DeleteProductResponse{Success: true}, nil
}

func convertToPBProduct(product *Product) *pb.Product {
	return &pb.Product{
		Id:          int64(product.ID),
		Name:        product.Name,
		Description: product.Description,
		PriceCents:  product.PriceCents,
		Stock:       product.Stock,
		Category:    product.Category,
		ImageUrl:    product.ImageURL,
		MerchantId:  int64(product.MerchantID),
	}
}
