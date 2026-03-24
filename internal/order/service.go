// 订单服务：处理订单的创建和管理
package order

import (
	"context"
	"log"
	"time"

	"github.com/streadway/amqp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"

	pb "go-commerce/api/order"
)

// Service 订单服务结构体

type Service struct {
	pb.UnimplementedOrderServiceServer
	db *gorm.DB         // 数据库连接
	ch *amqp.Channel     // RabbitMQ通道
}

// NewService 创建订单服务实例
func NewService(db *gorm.DB, ch *amqp.Channel) *Service {
	return &Service{db: db, ch: ch}
}

// CreateOrder 创建订单：创建新订单并发送订单事件
func (s *Service) CreateOrder(ctx context.Context, req *pb.CreateOrderRequest) (*pb.CreateOrderResponse, error) {
	// 开始数据库事务
	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 计算订单总金额
	var totalAmount float64
	for _, item := range req.Items {
		totalAmount += float64(item.Price) * float64(item.Quantity)
	}

	// 创建订单记录
	order := Order{
		UserID:      req.UserId,
		TotalAmount: totalAmount,
		Status:      "pending",
	}
	if err := tx.Create(&order).Error; err != nil {
		tx.Rollback()
		return nil, status.Errorf(codes.Internal, "failed to create order: %v", err)
	}

	// 创建订单商品记录
	orderItems := make([]OrderItem, len(req.Items))
	for i, item := range req.Items {
		orderItems[i] = OrderItem{
			OrderID:     int64(order.ID),
			ProductID:   item.ProductId,
			ProductName: item.ProductName,
			Price:       float64(item.Price),
			Quantity:    item.Quantity,
		}
	}
	if err := tx.Create(&orderItems).Error; err != nil {
		tx.Rollback()
		return nil, status.Errorf(codes.Internal, "failed to create order items: %v", err)
	}

	// 发布订单创建事件
	orderEvent := map[string]interface{}{
		"order_id":   order.ID,
		"user_id":    req.UserId,
		"total":      totalAmount,
		"created_at": time.Now().Format(time.RFC3339),
	}
	if err := publishEvent(s.ch, "order.created", orderEvent); err != nil {
		log.Printf("failed to publish order event: %v", err)
	}

	// 提交事务
	tx.Commit()

	return &pb.CreateOrderResponse{
		Order: convertToPBOrder(&order, orderItems),
	}, nil
}

// convertToPBOrder 转换订单模型为proto对象
func convertToPBOrder(order *Order, items []OrderItem) *pb.Order {
	pbItems := make([]*pb.OrderItem, len(items))
	for i, item := range items {
		pbItems[i] = &pb.OrderItem{
			ProductId:   item.ProductID,
			ProductName: item.ProductName,
			Price:       float32(item.Price),
			Quantity:    item.Quantity,
		}
	}

	return &pb.Order{
		Id:           int64(order.ID),
		UserId:       order.UserID,
		Items:        pbItems,
		TotalAmount:  float32(order.TotalAmount),
		Status:       order.Status,
		CreatedAt:    order.CreatedAt.Format(time.RFC3339),
	}
}

// publishEvent 发布事件到RabbitMQ
func publishEvent(ch *amqp.Channel, exchange string, event interface{}) error {
	// 实际实现会将事件发布到RabbitMQ
	return nil
}
