// order 包包含订单服务的模型和业务逻辑
// 负责处理订单的创建、查询、列表和取消
package order

import (
	"context"
	"errors"
	"log"
	"math"
	"time"

	// gRPC状态码：用于返回标准化的错误信息
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	// GORM：ORM框架，用于数据库操作
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	// 导入订单服务的protobuf生成代码
	pb "go-commerce/api/order"
	"go-commerce/internal/auth"
	"go-commerce/internal/merchant"
	// 导入产品模型：用于库存检查和更新
	"go-commerce/internal/product"
	"go-commerce/pkg/events"
	"go-commerce/pkg/mq"
)

// Service 订单服务结构体
// 实现了OrderServiceServer接口

type Service struct {
	pb.UnimplementedOrderServiceServer          // 嵌入未实现的OrderServiceServer，以保持向后兼容性
	db                                 *gorm.DB // 数据库连接
	publisher                          mq.Publisher
	timeoutScheduler                   TimeoutScheduler
	paymentTimeout                     time.Duration
}

// NewService 创建订单服务实例
// 参数：
//
//	db: 数据库连接
//	publisher: 订单事件发布器
//
// 返回值：
//
//	*Service: 订单服务实例
func NewService(db *gorm.DB, publisher mq.Publisher) *Service {
	return NewServiceWithTimeout(db, publisher, nil, DefaultOrderPaymentTimeout)
}

// NewServiceWithTimeout 允许在测试和启动阶段注入超时调度器及支付窗口。
func NewServiceWithTimeout(db *gorm.DB, publisher mq.Publisher, scheduler TimeoutScheduler, paymentTimeout time.Duration) *Service {
	if publisher == nil {
		publisher = mq.NopPublisher{}
	}
	if scheduler == nil {
		scheduler = NopTimeoutScheduler{}
	}
	if paymentTimeout <= 0 {
		paymentTimeout = DefaultOrderPaymentTimeout
	}
	return &Service{
		db:               db,
		publisher:        publisher,
		timeoutScheduler: scheduler,
		paymentTimeout:   paymentTimeout,
	}
}

// CreateOrder 创建订单：创建新订单并发送订单事件
// 功能：用户下单，检查库存并生成订单记录
// 参数：
//
//	ctx: 上下文，用于控制请求的生命周期
//	req: 订单创建请求，包含用户ID和订单商品列表
//
// 返回值：
//
//	*pb.CreateOrderResponse: 订单创建响应，包含创建的订单信息
//	error: 错误信息
func (s *Service) CreateOrder(ctx context.Context, req *pb.CreateOrderRequest) (*pb.CreateOrderResponse, error) {
	aggregatedItems, err := aggregateCreateOrderItems(req.Items)
	if err != nil {
		return nil, err
	}

	// 开启事务，确保商品快照、库存扣减和订单落库要么全部成功，要么全部回滚。
	tx := s.db.Begin()
	if tx.Error != nil {
		return nil, status.Errorf(codes.Internal, "failed to start transaction: %v", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 订单金额和订单项快照必须以后端读取到的真实商品信息为准，不能信任客户端输入。
	orderItems, totalAmount, err := buildOrderSnapshots(tx, aggregatedItems)
	if err != nil {
		tx.Rollback()
		return nil, err
	}

	// 创建订单记录
	order := Order{
		UserID:      uint(req.UserId), // 用户ID
		TotalAmount: totalAmount,      // 订单总金额
		Status:      OrderStatusPending,
		OrderDate:   time.Now(), // 订单日期
	}
	if err := tx.Create(&order).Error; err != nil {
		tx.Rollback()
		return nil, status.Errorf(codes.Internal, "failed to create order: %v", err)
	}

	// 订单项保存“下单时快照”，后续商品改价不会影响历史订单展示。
	for i := range orderItems {
		orderItems[i].OrderID = order.ID
	}
	if err := tx.Create(&orderItems).Error; err != nil {
		tx.Rollback()
		return nil, status.Errorf(codes.Internal, "failed to create order items: %v", err)
	}

	// 提交事务
	if err := tx.Commit().Error; err != nil {
		return nil, status.Errorf(codes.Internal, "failed to commit order transaction: %v", err)
	}

	// 先提交交易事务，再发布事件；当前阶段采用弱一致，避免消息先于数据库提交对外可见。
	orderEvent := newOrderCreatedEvent(&order, req.UserId, totalAmount, orderItems)
	if err := s.publisher.Publish(ctx, events.OrderCreatedType, orderEvent); err != nil {
		log.Printf(
			"event_publish_failed event_type=%s event_id=%s order_id=%d user_id=%d error=%v",
			orderEvent.EventType,
			orderEvent.EventID,
			order.ID,
			req.UserId,
			err,
		)
	}

	// 超时消息同样在事务提交后再发，避免订单未真正落库时被提前消费。
	timeoutEvent := newOrderTimeoutCheckEvent(&order, req.UserId, s.paymentTimeout)
	if err := s.timeoutScheduler.Schedule(ctx, timeoutEvent, s.paymentTimeout); err != nil {
		log.Printf(
			"timeout_schedule_failed event_type=%s event_id=%s order_id=%d user_id=%d error=%v",
			timeoutEvent.EventType,
			timeoutEvent.EventID,
			order.ID,
			req.UserId,
			err,
		)
	}

	// 返回创建订单响应
	return &pb.CreateOrderResponse{
		Order: convertToPBOrder(&order, orderItems),
	}, nil
}

func newOrderCreatedEvent(order *Order, userID int64, totalAmount float64, items []OrderItem) events.OrderCreatedEvent {
	eventItems := make([]events.OrderItemSnapshot, len(items))
	for i, item := range items {
		eventItems[i] = events.OrderItemSnapshot{
			ProductID:   item.ProductID,
			MerchantID:  int64(item.MerchantID),
			ProductName: item.ProductName,
			Price:       item.Price,
			Quantity:    item.Quantity,
		}
	}

	return events.OrderCreatedEvent{
		BaseEvent:   events.NewBaseEvent(events.OrderCreatedType, time.Now()),
		OrderID:     int64(order.ID),
		UserID:      userID,
		TotalAmount: totalAmount,
		Items:       eventItems,
	}
}

func newOrderTimeoutCheckEvent(order *Order, userID int64, timeout time.Duration) events.OrderTimeoutCheckEvent {
	createdAt := order.OrderDate.UTC()
	return events.OrderTimeoutCheckEvent{
		BaseEvent:      events.NewBaseEvent(events.OrderTimeoutCheckType, time.Now()),
		OrderID:        int64(order.ID),
		UserID:         userID,
		CreatedAt:      createdAt.Format(time.RFC3339Nano),
		ExpireAt:       createdAt.Add(timeout).Format(time.RFC3339Nano),
		TimeoutMinutes: timeout.Minutes(),
	}
}

// aggregatedCreateOrderItem 表示同一商品在一次下单请求中的合并结果。
type aggregatedCreateOrderItem struct {
	ProductID int64
	Quantity  int32
}

// aggregateCreateOrderItems 只接受客户端可信输入，并将重复商品先合并后再参与库存校验。
func aggregateCreateOrderItems(items []*pb.CreateOrderItem) ([]aggregatedCreateOrderItem, error) {
	if len(items) == 0 {
		return nil, status.Error(codes.InvalidArgument, "order items cannot be empty")
	}

	quantities := make(map[int64]int32, len(items))
	orderedProductIDs := make([]int64, 0, len(items))

	for _, item := range items {
		if item == nil || item.ProductId <= 0 {
			return nil, status.Error(codes.InvalidArgument, "invalid product id")
		}
		if item.Quantity <= 0 {
			return nil, status.Error(codes.InvalidArgument, "quantity must be greater than zero")
		}

		currentQuantity, exists := quantities[item.ProductId]
		if !exists {
			orderedProductIDs = append(orderedProductIDs, item.ProductId)
		}
		if item.Quantity > math.MaxInt32-currentQuantity {
			return nil, status.Error(codes.InvalidArgument, "quantity is too large")
		}
		quantities[item.ProductId] = currentQuantity + item.Quantity
	}

	aggregatedItems := make([]aggregatedCreateOrderItem, 0, len(orderedProductIDs))
	for _, productID := range orderedProductIDs {
		aggregatedItems = append(aggregatedItems, aggregatedCreateOrderItem{
			ProductID: productID,
			Quantity:  quantities[productID],
		})
	}

	return aggregatedItems, nil
}

// buildOrderSnapshots 基于数据库中的真实商品信息生成订单项快照，并在同一事务内完成库存扣减。
func buildOrderSnapshots(tx *gorm.DB, items []aggregatedCreateOrderItem) ([]OrderItem, float64, error) {
	orderItems := make([]OrderItem, 0, len(items))
	var totalAmount float64

	for _, item := range items {
		var productInfo product.Product
		if err := tx.First(&productInfo, item.ProductID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil, 0, status.Errorf(codes.NotFound, "product not found: %d", item.ProductID)
			}
			return nil, 0, status.Errorf(codes.Internal, "failed to fetch product %d: %v", item.ProductID, err)
		}

		// 订单项必须保存下单瞬间的名称与价格，保证历史订单展示稳定。
		orderItems = append(orderItems, OrderItem{
			ProductID:   int64(productInfo.ID),
			MerchantID:  productInfo.MerchantID,
			ProductName: productInfo.Name,
			Price:       productInfo.Price,
			Quantity:    item.Quantity,
		})
		totalAmount += productInfo.Price * float64(item.Quantity)

		// 库存扣减使用数据库条件更新，确保并发下 stock 不会被扣成负数。
		if err := product.DeductStock(tx, item.ProductID, item.Quantity); err != nil {
			switch {
			case errors.Is(err, product.ErrProductNotFound):
				return nil, 0, status.Errorf(codes.NotFound, "product not found: %d", item.ProductID)
			case errors.Is(err, product.ErrInsufficientStock):
				return nil, 0, status.Errorf(codes.InvalidArgument, "insufficient stock for product %s", productInfo.Name)
			case errors.Is(err, product.ErrInvalidQuantity):
				return nil, 0, status.Error(codes.InvalidArgument, "quantity must be greater than zero")
			default:
				return nil, 0, status.Errorf(codes.Internal, "failed to deduct stock for product %d: %v", item.ProductID, err)
			}
		}
	}

	return orderItems, totalAmount, nil
}

// convertToPBOrder 转换订单模型为proto对象
// 参数：
//
//	order: 订单模型对象
//	items: 订单项模型对象列表
//
// 返回值：
//
//	*pb.Order: proto订单对象
func convertToPBOrder(order *Order, items []OrderItem) *pb.Order {
	pbItems := make([]*pb.OrderItem, len(items))
	for i, item := range items {
		pbItems[i] = &pb.OrderItem{
			ProductId:   item.ProductID,      // 产品ID
			ProductName: item.ProductName,    // 产品名称
			Price:       float32(item.Price), // 产品价格
			Quantity:    item.Quantity,       // 产品数量
		}
	}

	return &pb.Order{
		Id:           int64(order.ID),                      // 订单ID
		UserId:       int64(order.UserID),                  // 用户ID
		Items:        pbItems,                              // 订单商品列表
		TotalAmount:  float32(order.TotalAmount),           // 订单总金额
		Status:       order.Status,                         // 订单状态
		CreatedAt:    order.OrderDate.Format(time.RFC3339), // 订单创建时间
		CancelReason: order.CancelReason,                   // 取消原因
	}
}

// ListOrders 获取用户订单列表
// 功能：根据用户ID获取订单列表
// 参数：
//
//	ctx: 上下文，用于控制请求的生命周期
//	req: 订单列表请求，包含用户ID、页码和每页数量
//
// 返回值：
//
//	*pb.ListOrdersResponse: 订单列表响应，包含订单列表和总数
//	error: 错误信息
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
//
//	ctx: 上下文，用于控制请求的生命周期
//	req: 订单详情请求，包含订单ID和用户ID
//
// 返回值：
//
//	*pb.GetOrderResponse: 订单详情响应，包含订单详细信息
//	error: 错误信息
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
//
//	ctx: 上下文，用于控制请求的生命周期
//	req: 取消订单请求，包含订单ID和用户ID
//
// 返回值：
//
//	*pb.CancelOrderResponse: 取消订单响应，包含取消结果和消息
//	error: 错误信息
func (s *Service) CancelOrder(ctx context.Context, req *pb.CancelOrderRequest) (*pb.CancelOrderResponse, error) {
	order, changed, err := cancelOrderWithReason(s.db, req.Id, req.UserId, OrderCancelReasonUserCancelled)
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return &pb.CancelOrderResponse{
				Success: false,
				Message: "订单不存在",
			}, nil
		case errors.Is(err, ErrInvalidOrderTransition):
			return &pb.CancelOrderResponse{
				Success: false,
				Message: err.Error(),
			}, nil
		default:
			return &pb.CancelOrderResponse{
				Success: false,
				Message: "取消订单失败",
			}, nil
		}
	}
	if !changed {
		return &pb.CancelOrderResponse{
			Success: false,
			Message: "订单已取消",
		}, nil
	}

	// 取消成功后再发布领域事件；发布失败不回滚订单，只记录日志等待后续可靠消息机制补强。
	orderEvent := events.OrderCancelledEvent{
		BaseEvent: events.NewBaseEvent(events.OrderCancelledType, time.Now()),
		OrderID:   int64(order.ID),
		UserID:    req.UserId,
		Reason:    OrderCancelReasonUserCancelled,
	}
	if err := s.publisher.Publish(ctx, events.OrderCancelledType, orderEvent); err != nil {
		log.Printf(
			"event_publish_failed event_type=%s event_id=%s order_id=%d user_id=%d error=%v",
			orderEvent.EventType,
			orderEvent.EventID,
			order.ID,
			req.UserId,
			err,
		)
	}

	// 返回取消订单响应
	return &pb.CancelOrderResponse{
		Success: true,
		Message: "订单取消成功",
	}, nil
}

const (
	OrderCancelReasonUserCancelled  = "user_cancelled"
	OrderCancelReasonPaymentTimeout = "payment_timeout"
)

// cancelOrderWithReason 是人工取消与超时取消共用的核心路径。
// 只有首次把 pending 推进到 cancelled 时才会回补库存，因此天然支持重复消息幂等。
func cancelOrderWithReason(db *gorm.DB, orderID, userID int64, reason string) (*Order, bool, error) {
	var order Order
	changed := false

	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ?", orderID, userID).
			First(&order).Error; err != nil {
			return err
		}

		if order.Status == OrderStatusCancelled {
			return nil
		}
		if err := ValidateTransition(order.Status, OrderStatusCancelled); err != nil {
			return err
		}

		var orderItems []OrderItem
		if err := tx.Where("order_id = ?", order.ID).Find(&orderItems).Error; err != nil {
			return err
		}

		for _, item := range orderItems {
			if err := product.RestoreStock(tx, item.ProductID, item.Quantity); err != nil {
				return err
			}
		}

		if err := TransitionTo(&order, OrderStatusCancelled); err != nil {
			return err
		}
		order.CancelReason = reason
		if err := tx.Save(&order).Error; err != nil {
			return err
		}
		changed = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return &order, changed, nil
}

// ShipOrder 允许具备权限的商家或管理员把已支付订单推进到已发货。
func (s *Service) ShipOrder(ctx context.Context, req *pb.ShipOrderRequest) (*pb.ShipOrderResponse, error) {
	var order Order
	if err := s.db.First(&order, req.Id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Error(codes.NotFound, "order not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to fetch order: %v", err)
	}

	var orderItems []OrderItem
	if err := s.db.Where("order_id = ?", order.ID).Find(&orderItems).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch order items: %v", err)
	}
	if err := s.authorizeShipment(uint(req.ActorUserId), orderItems); err != nil {
		return nil, orderStatusError(err)
	}

	fromStatus := order.Status
	if err := TransitionTo(&order, OrderStatusShipped); err != nil {
		return nil, orderStatusError(err)
	}
	if err := s.db.Save(&order).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update order status: %v", err)
	}

	s.publishOrderStatusChanged(ctx, events.OrderShippedType, &order, fromStatus, OrderStatusShipped)
	return &pb.ShipOrderResponse{Success: true, Message: "订单已发货"}, nil
}

// CompleteOrder 仅允许订单所属用户确认收货，把已发货订单推进到已完成。
func (s *Service) CompleteOrder(ctx context.Context, req *pb.CompleteOrderRequest) (*pb.CompleteOrderResponse, error) {
	var order Order
	if err := s.db.Where("id = ? AND user_id = ?", req.Id, req.UserId).First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Error(codes.NotFound, "order not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to fetch order: %v", err)
	}

	fromStatus := order.Status
	if err := TransitionTo(&order, OrderStatusCompleted); err != nil {
		return nil, orderStatusError(err)
	}
	if err := s.db.Save(&order).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update order status: %v", err)
	}

	s.publishOrderStatusChanged(ctx, events.OrderCompletedType, &order, fromStatus, OrderStatusCompleted)
	return &pb.CompleteOrderResponse{Success: true, Message: "订单已完成"}, nil
}

func (s *Service) authorizeShipment(actorUserID uint, orderItems []OrderItem) error {
	var actor auth.User
	if err := s.db.First(&actor, actorUserID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return merchant.ErrUserNotFound
		}
		return err
	}

	switch actor.Role {
	case auth.RoleAdmin:
		return nil
	case auth.RoleMerchant:
		// 订单项保存商家快照后，商家只能为自己名下商品构成的订单发货。
		merchantIDs := uniqueMerchantIDs(orderItems)
		if len(merchantIDs) == 0 {
			return merchant.ErrPermissionDenied
		}

		var merchants []merchant.Merchant
		if err := s.db.Where("id IN ?", merchantIDs).Find(&merchants).Error; err != nil {
			return err
		}
		if len(merchants) != len(merchantIDs) {
			return merchant.ErrPermissionDenied
		}
		for _, shop := range merchants {
			if shop.OwnerUserID == nil || *shop.OwnerUserID != actorUserID {
				return merchant.ErrPermissionDenied
			}
		}
		return nil
	default:
		return merchant.ErrPermissionDenied
	}
}

func uniqueMerchantIDs(orderItems []OrderItem) []uint {
	seen := make(map[uint]struct{}, len(orderItems))
	merchantIDs := make([]uint, 0, len(orderItems))
	for _, item := range orderItems {
		if item.MerchantID == 0 {
			return nil
		}
		if _, ok := seen[item.MerchantID]; ok {
			continue
		}
		seen[item.MerchantID] = struct{}{}
		merchantIDs = append(merchantIDs, item.MerchantID)
	}
	return merchantIDs
}

func (s *Service) publishOrderStatusChanged(ctx context.Context, eventType string, order *Order, fromStatus, toStatus string) {
	event := newOrderStatusChangedEvent(eventType, order, fromStatus, toStatus)
	if err := s.publisher.Publish(ctx, eventType, event); err != nil {
		log.Printf(
			"event_publish_failed event_type=%s event_id=%s order_id=%d user_id=%d error=%v",
			event.EventType,
			event.EventID,
			order.ID,
			order.UserID,
			err,
		)
	}
}

func newOrderStatusChangedEvent(eventType string, order *Order, fromStatus, toStatus string) events.OrderStatusChangedEvent {
	return events.OrderStatusChangedEvent{
		BaseEvent:  events.NewBaseEvent(eventType, time.Now()),
		OrderID:    int64(order.ID),
		UserID:     int64(order.UserID),
		FromStatus: fromStatus,
		ToStatus:   toStatus,
	}
}

func orderStatusError(err error) error {
	switch {
	case errors.Is(err, ErrInvalidOrderTransition):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, merchant.ErrPermissionDenied):
		return status.Error(codes.PermissionDenied, "order operation is not allowed")
	case errors.Is(err, merchant.ErrUserNotFound):
		return status.Error(codes.NotFound, "user not found")
	default:
		return status.Errorf(codes.Internal, "order operation failed: %v", err)
	}
}
