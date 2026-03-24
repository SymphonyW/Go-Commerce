// 购物车服务：处理用户购物车的添加、查询等操作
package cart

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "go-commerce/api/cart"
	pbProduct "go-commerce/api/product"
)

// Service 购物车服务结构体

type Service struct {
	pb.UnimplementedCartServiceServer
	redisClient    *redis.Client           // Redis客户端
	productClient  pbProduct.ProductServiceClient // 产品服务客户端
}

// NewService 创建购物车服务实例
func NewService(redisClient *redis.Client, productClient pbProduct.ProductServiceClient) *Service {
	return &Service{redisClient: redisClient, productClient: productClient}
}

// AddCartItem 添加购物车商品：向用户购物车添加商品
func (s *Service) AddCartItem(ctx context.Context, req *pb.AddCartItemRequest) (*pb.AddCartItemResponse, error) {
	// 构建购物车键
	key := fmt.Sprintf("cart:%d", req.UserId)
	
	// 构建商品键
	itemKey := fmt.Sprintf("product:%d", req.ProductId)
	// 尝试获取已存在的商品
	itemJSON, err := s.redisClient.HGet(ctx, key, itemKey).Result()
	
	var existingItem pb.CartItem
	if err == nil {
		// 商品已存在，更新数量
		json.Unmarshal([]byte(itemJSON), &existingItem)
		existingItem.Quantity += req.Quantity
	} else {
		// 商品不存在，从产品服务获取商品信息
		productResp, err := s.productClient.GetProduct(ctx, &pbProduct.GetProductRequest{
			Id: req.ProductId,
		})
		if err != nil {
			return nil, status.Errorf(codes.NotFound, "product not found: %v", err)
		}
		
		// 使用实际商品信息创建购物车商品
		existingItem = pb.CartItem{
			ProductId:   productResp.Product.Id,
			ProductName: productResp.Product.Name,
			Price:       productResp.Product.Price,
			Quantity:    req.Quantity,
			ImageUrl:    productResp.Product.ImageUrl,
		}
	}

	// 序列化商品并保存到Redis
	itemJSONBytes, _ := json.Marshal(existingItem)
	if err := s.redisClient.HSet(ctx, key, itemKey, string(itemJSONBytes)).Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to add cart item: %v", err)
	}

	// 设置购物车过期时间（7天）
	s.redisClient.Expire(ctx, key, 7*24*time.Hour)

	return &pb.AddCartItemResponse{Item: &existingItem}, nil
}

// GetCart 获取购物车：获取用户的完整购物车信息
func (s *Service) GetCart(ctx context.Context, req *pb.GetCartRequest) (*pb.GetCartResponse, error) {
	// 构建购物车键
	key := fmt.Sprintf("cart:%d", req.UserId)
	
	// 获取购物车所有商品
	itemsJSON, err := s.redisClient.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get cart: %v", err)
	}

	// 解析商品并计算总金额
	items := make([]*pb.CartItem, 0)
	var totalAmount float32
	for _, itemJSON := range itemsJSON {
		var item pb.CartItem
		json.Unmarshal([]byte(itemJSON), &item)
		items = append(items, &item)
		totalAmount += item.Price * float32(item.Quantity)
	}

	// 确保返回的响应包含空数组而不是nil
	response := &pb.GetCartResponse{
		Items:        items,
		TotalAmount:  totalAmount,
	}

	return response, nil
}

// UpdateCartItem 更新购物车商品：更新购物车中商品的数量
func (s *Service) UpdateCartItem(ctx context.Context, req *pb.UpdateCartItemRequest) (*pb.UpdateCartItemResponse, error) {
	// 构建购物车键
	key := fmt.Sprintf("cart:%d", req.UserId)
	
	// 构建商品键
	itemKey := fmt.Sprintf("product:%d", req.ProductId)
	
	// 检查商品是否存在
	itemJSON, err := s.redisClient.HGet(ctx, key, itemKey).Result()
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "cart item not found")
	}
	
	// 解析商品
	var item pb.CartItem
	json.Unmarshal([]byte(itemJSON), &item)
	
	// 更新数量
	item.Quantity = req.Quantity
	
	// 序列化商品并保存到Redis
	itemJSONBytes, _ := json.Marshal(item)
	if err := s.redisClient.HSet(ctx, key, itemKey, string(itemJSONBytes)).Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update cart item: %v", err)
	}
	
	// 设置购物车过期时间（7天）
	s.redisClient.Expire(ctx, key, 7*24*time.Hour)
	
	return &pb.UpdateCartItemResponse{Item: &item}, nil
}

// RemoveCartItem 删除购物车商品：从购物车中删除商品
func (s *Service) RemoveCartItem(ctx context.Context, req *pb.RemoveCartItemRequest) (*pb.RemoveCartItemResponse, error) {
	// 构建购物车键
	key := fmt.Sprintf("cart:%d", req.UserId)
	
	// 构建商品键
	itemKey := fmt.Sprintf("product:%d", req.ProductId)
	
	// 从Redis中删除商品
	if err := s.redisClient.HDel(ctx, key, itemKey).Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete cart item: %v", err)
	}
	
	return &pb.RemoveCartItemResponse{Success: true}, nil
}

// ClearCart 清空购物车：清空用户的购物车
func (s *Service) ClearCart(ctx context.Context, req *pb.ClearCartRequest) (*pb.ClearCartResponse, error) {
	// 构建购物车键
	key := fmt.Sprintf("cart:%d", req.UserId)
	
	// 从Redis中删除购物车
	if err := s.redisClient.Del(ctx, key).Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to clear cart: %v", err)
	}
	
	return &pb.ClearCartResponse{Success: true}, nil
}
