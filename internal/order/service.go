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
	"go-commerce/internal/product"
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
// 功能：用户下单，检查库存并生成订单记录
// 参数：req (*pb.CreateOrderRequest) - 订单请求
// 返回：(*pb.CreateOrderResponse, error) - 订单响应和错误
func (s *Service) CreateOrder(ctx context.Context, req *pb.CreateOrderRequest) (*pb.CreateOrderResponse, error) {
	// 开始数据库事务
	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 检查库存
	for _, item := range req.Items {
		var product product.Product
		if err := tx.First(&product, item.ProductId).Error; err != nil {
			tx.Rollback()
			return nil, status.Errorf(codes.NotFound, "product not found: %v", err)
		}

		// 检查库存是否充足
		if product.Stock < item.Quantity {
			tx.Rollback()
			return nil, status.Errorf(codes.InvalidArgument, "insufficient stock for product %s", product.Name)
		}

		// 扣减库存
		product.Stock -= item.Quantity
		if err := tx.Save(&product).Error; err != nil {
			tx.Rollback()
			return nil, status.Errorf(codes.Internal, "failed to update stock: %v", err)
		}
	}

	// 计算订单总金额
	var totalAmount float64
	for _, item := range req.Items {
		totalAmount += float64(item.Price) * float64(item.Quantity)
	}

	// 创建订单记录
	order := Order{
		UserID:      uint(req.UserId),
		TotalAmount: totalAmount,
		Status:      "pending",
		OrderDate:   time.Now(),
	}
	if err := tx.Create(&order).Error; err != nil {
		tx.Rollback()
		return nil, status.Errorf(codes.Internal, "failed to create order: %v", err)
	}

	// 创建订单商品记录
	orderItems := make([]OrderItem, len(req.Items))
	for i, item := range req.Items {
		orderItems[i] = OrderItem{
			OrderID:     order.ID,
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
		UserId:       int64(order.UserID),
		Items:        pbItems,
		TotalAmount:  float32(order.TotalAmount),
		Status:       order.Status,
		CreatedAt:    order.OrderDate.Format(time.RFC3339),
	}
}

// publishEvent 发布事件到RabbitMQ
func publishEvent(ch *amqp.Channel, exchange string, event interface{}) error {
	// 实际实现会将事件发布到RabbitMQ
	return nil
}

// ListOrders 获取用户订单列表
// 功能：根据用户ID获取订单列表
// 参数：req (*pb.ListOrdersRequest) - 订单列表请求
// 返回：(*pb.ListOrdersResponse, error) - 订单列表响应和错误
func (s *Service) ListOrders(ctx context.Context, req *pb.ListOrdersRequest) (*pb.ListOrdersResponse, error) {
	// 从数据库查询用户订单
	var orders []Order
	var total int64

	// 计算偏移量
	offset := (req.Page - 1) * req.PageSize
	if req.Page <= 0 {
		req.Page = 1
		offset = 0
	}
	if req.PageSize <= 0 {
		req.PageSize = 10
	}

	// 查询订单总数
	if err := s.db.Model(&Order{}).Where("user_id = ?", req.UserId).Count(&total).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "failed to count orders: %v", err)
	}

	// 查询订单列表
	if err := s.db.Where("user_id = ?", req.UserId).Order("created_at DESC").Offset(int(offset)).Limit(int(req.PageSize)).Find(&orders).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch orders: %v", err)
	}

	// 转换为proto对象
	pbOrders := make([]*pb.Order, len(orders))
	for i, order := range orders {
		// 查询订单项
		var orderItems []OrderItem
		if err := s.db.Where("order_id = ?", order.ID).Find(&orderItems).Error; err != nil {
			return nil, status.Errorf(codes.Internal, "failed to fetch order items: %v", err)
		}
		pbOrders[i] = convertToPBOrder(&order, orderItems)
	}

	return &pb.ListOrdersResponse{
		Orders: pbOrders,
		Total:  total,
	}, nil
}

// GetOrder 获取订单详情
// 功能：根据订单ID和用户ID获取订单详情
// 参数：req (*pb.GetOrderRequest) - 订单详情请求
// 返回：(*pb.GetOrderResponse, error) - 订单详情响应和错误
func (s *Service) GetOrder(ctx context.Context, req *pb.GetOrderRequest) (*pb.GetOrderResponse, error) {
	// 查询订单
	var order Order
	if err := s.db.Where("id = ? AND user_id = ?", req.Id, req.UserId).First(&order).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, status.Errorf(codes.NotFound, "order not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to fetch order: %v", err)
	}

	// 查询订单项
	var orderItems []OrderItem
	if err := s.db.Where("order_id = ?", order.ID).Find(&orderItems).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch order items: %v", err)
	}

	return &pb.GetOrderResponse{
		Order: convertToPBOrder(&order, orderItems),
	}, nil
}

// CancelOrder 取消订单
// 功能：根据订单ID和用户ID取消订单
// 参数：req (*pb.CancelOrderRequest) - 取消订单请求
// 返回：(*pb.CancelOrderResponse, error) - 取消订单响应和错误
func (s *Service) CancelOrder(ctx context.Context, req *pb.CancelOrderRequest) (*pb.CancelOrderResponse, error) {
	// 查询订单
	var order Order
	if err := s.db.Where("id = ? AND user_id = ?", req.Id, req.UserId).First(&order).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &pb.CancelOrderResponse{
				Success: false,
				Message: "订单不存在",
			}, nil
		}
		return &pb.CancelOrderResponse{
			Success: false,
			Message: "获取订单失败",
		}, nil
	}

	// 检查订单状态
	if order.Status != "pending" {
		return &pb.CancelOrderResponse{
			Success: false,
			Message: "只能取消待处理状态的订单",
		}, nil
	}

	// 开始数据库事务
	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 查询订单项
	var orderItems []OrderItem
	if err := tx.Where("order_id = ?", order.ID).Find(&orderItems).Error; err != nil {
		tx.Rollback()
		return &pb.CancelOrderResponse{
			Success: false,
			Message: "获取订单项失败",
		}, nil
	}

	// 恢复库存
	for _, item := range orderItems {
		var product product.Product
		if err := tx.First(&product, item.ProductID).Error; err != nil {
			tx.Rollback()
			return &pb.CancelOrderResponse{
				Success: false,
				Message: "获取商品信息失败",
			}, nil
		}

		// 恢复库存
		product.Stock += item.Quantity
		if err := tx.Save(&product).Error; err != nil {
			tx.Rollback()
			return &pb.CancelOrderResponse{
				Success: false,
				Message: "恢复库存失败",
			}, nil
		}
	}

	// 更新订单状态
	order.Status = "cancelled"
	if err := tx.Save(&order).Error; err != nil {
		tx.Rollback()
		return &pb.CancelOrderResponse{
			Success: false,
			Message: "更新订单状态失败",
		}, nil
	}

	// 发布订单取消事件
	orderEvent := map[string]interface{}{
		"order_id":   order.ID,
		"user_id":    req.UserId,
		"status":     "cancelled",
		"cancelled_at": time.Now().Format(time.RFC3339),
	}
	if err := publishEvent(s.ch, "order.cancelled", orderEvent); err != nil {
		log.Printf("failed to publish order event: %v", err)
	}

	// 提交事务
	tx.Commit()

	return &pb.CancelOrderResponse{
		Success: true,
		Message: "订单取消成功",
	}, nil
}
