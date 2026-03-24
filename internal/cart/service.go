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
)

// Service 购物车服务结构体

type Service struct {
	pb.UnimplementedCartServiceServer
	redisClient *redis.Client // Redis客户端
}

// NewService 创建购物车服务实例
func NewService(redisClient *redis.Client) *Service {
	return &Service{redisClient: redisClient}
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
		// 商品不存在，创建新商品
		existingItem = pb.CartItem{
			ProductId:   req.ProductId,
			ProductName: "Sample Product",
			Price:       99.99,
			Quantity:    req.Quantity,
			ImageUrl:    "https://example.com/image.jpg",
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
	var items []*pb.CartItem
	var totalAmount float32
	for _, itemJSON := range itemsJSON {
		var item pb.CartItem
		json.Unmarshal([]byte(itemJSON), &item)
		items = append(items, &item)
		totalAmount += item.Price * float32(item.Quantity)
	}

	return &pb.GetCartResponse{
		Items:        items,
		TotalAmount:  totalAmount,
	}, nil
}
