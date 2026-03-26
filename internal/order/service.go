// order 包包含订单服务的模型和业务逻辑
// 负责处理订单的创建、查询、列表和取消
package order

import (
	"context"
	"log"
	"time"

	// RabbitMQ客户端：用于消息队列操作
	"github.com/streadway/amqp"
	// gRPC状态码：用于返回标准化的错误信息
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	// GORM：ORM框架，用于数据库操作
	"gorm.io/gorm"

	// 导入订单服务的protobuf生成代码
	pb "go-commerce/api/order"
	// 导入产品模型：用于库存检查和更新
	"go-commerce/internal/product"
)

// Service 订单服务结构体
// 实现了OrderServiceServer接口

type Service struct {
	pb.UnimplementedOrderServiceServer // 嵌入未实现的OrderServiceServer，以保持向后兼容性
	db *gorm.DB                       // 数据库连接
	ch *amqp.Channel                   // RabbitMQ通道，用于发布订单事件
}

// NewService 创建订单服务实例
// 参数：
//   db: 数据库连接
//   ch: RabbitMQ通道
// 返回值：
//   *Service: 订单服务实例
func NewService(db *gorm.DB, ch *amqp.Channel) *Service {
	return &Service{db: db, ch: ch}
}

// CreateOrder 创建订单：创建新订单并发送订单事件
// 功能：用户下单，检查库存并生成订单记录
// 参数：
//   ctx: 上下文，用于控制请求的生命周期
//   req: 订单创建请求，包含用户ID和订单商品列表
// 返回值：
//   *pb.CreateOrderResponse: 订单创建响应，包含创建的订单信息
//   error: 错误信息
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
		UserID:      uint(req.UserId),  // 用户ID
		TotalAmount: totalAmount,        // 订单总金额
		Status:      "pending",          // 订单状态，初始为待处理
		OrderDate:   time.Now(),         // 订单日期
	}
	if err := tx.Create(&order).Error; err != nil {
		tx.Rollback()
		return nil, status.Errorf(codes.Internal, "failed to create order: %v", err)
	}

	// 创建订单商品记录
	orderItems := make([]OrderItem, len(req.Items))
	for i, item := range req.Items {
		orderItems[i] = OrderItem{
			OrderID:     order.ID,         // 订单ID
			ProductID:   item.ProductId,   // 产品ID
			ProductName: item.ProductName, // 产品名称
			Price:       float64(item.Price), // 产品价格
			Quantity:    item.Quantity,    // 产品数量
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

	// 返回创建订单响应
	return &pb.CreateOrderResponse{
		Order: convertToPBOrder(&order, orderItems),
	}, nil
}

// convertToPBOrder 转换订单模型为proto对象
// 参数：
//   order: 订单模型对象
//   items: 订单项模型对象列表
// 返回值：
//   *pb.Order: proto订单对象
func convertToPBOrder(order *Order, items []OrderItem) *pb.Order {
	pbItems := make([]*pb.OrderItem, len(items))
	for i, item := range items {
		pbItems[i] = &pb.OrderItem{
			ProductId:   item.ProductID,   // 产品ID
			ProductName: item.ProductName, // 产品名称
			Price:       float32(item.Price), // 产品价格
			Quantity:    item.Quantity,    // 产品数量
		}
	}

	return &pb.Order{
		Id:           int64(order.ID),          // 订单ID
		UserId:       int64(order.UserID),       // 用户ID
		Items:        pbItems,                   // 订单商品列表
		TotalAmount:  float32(order.TotalAmount), // 订单总金额
		Status:       order.Status,              // 订单状态
		CreatedAt:    order.OrderDate.Format(time.RFC3339), // 订单创建时间
	}
}

// publishEvent 发布事件到RabbitMQ
// 参数：
//   ch: RabbitMQ通道
//   exchange: 交换机名称
//   event: 事件数据
// 返回值：
//   error: 错误信息
func publishEvent(ch *amqp.Channel, exchange string, event interface{}) error {
	// 实际实现会将事件发布到RabbitMQ
	return nil
}

// ListOrders 获取用户订单列表
// 功能：根据用户ID获取订单列表
// 参数：
//   ctx: 上下文，用于控制请求的生命周期
//   req: 订单列表请求，包含用户ID、页码和每页数量
// 返回值：
//   *pb.ListOrdersResponse: 订单列表响应，包含订单列表和总数
//   error: 错误信息
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

	// 返回订单列表响应
	return &pb.ListOrdersResponse{
		Orders: pbOrders,
		Total:  total,
	}, nil
}

// GetOrder 获取订单详情
// 功能：根据订单ID和用户ID获取订单详情
// 参数：
//   ctx: 上下文，用于控制请求的生命周期
//   req: 订单详情请求，包含订单ID和用户ID
// 返回值：
//   *pb.GetOrderResponse: 订单详情响应，包含订单详细信息
//   error: 错误信息
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

	// 返回订单详情响应
	return &pb.GetOrderResponse{
		Order: convertToPBOrder(&order, orderItems),
	}, nil
}

// CancelOrder 取消订单
// 功能：根据订单ID和用户ID取消订单
// 参数：
//   ctx: 上下文，用于控制请求的生命周期
//   req: 取消订单请求，包含订单ID和用户ID
// 返回值：
//   *pb.CancelOrderResponse: 取消订单响应，包含取消结果和消息
//   error: 错误信息
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
		"order_id":     order.ID,
		"user_id":      req.UserId,
		"status":       "cancelled",
		"cancelled_at": time.Now().Format(time.RFC3339),
	}
	if err := publishEvent(s.ch, "order.cancelled", orderEvent); err != nil {
		log.Printf("failed to publish order event: %v", err)
	}

	// 提交事务
	tx.Commit()

	// 返回取消订单响应
	return &pb.CancelOrderResponse{
		Success: true,
		Message: "订单取消成功",
	}, nil
}
