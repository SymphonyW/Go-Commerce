// order 包包含订单服务的模型和业务逻辑
// 负责处理订单的创建、查询、列表和取消
package order

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
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
	"go-commerce/internal/idempotency"
	"go-commerce/internal/merchant"
	"go-commerce/internal/outbox"
	// 导入产品模型：用于库存检查和更新
	"go-commerce/internal/product"
	"go-commerce/pkg/events"
	"go-commerce/pkg/mq"
	"go-commerce/pkg/observability"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Service 订单服务结构体
// 实现了OrderServiceServer接口

type Service struct {
	pb.UnimplementedOrderServiceServer          // 嵌入未实现的OrderServiceServer，以保持向后兼容性
	db                                 *gorm.DB // 数据库连接
	publisher                          mq.Publisher
	timeoutScheduler                   TimeoutScheduler
	paymentTimeout                     time.Duration
	idempotency                        *idempotency.Service
	outboxRepo                         outbox.EventRepository
	metrics                            *observability.Metrics
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
	return NewServiceWithTimeoutAndMetrics(db, publisher, scheduler, paymentTimeout, nil)
}

func NewServiceWithTimeoutAndMetrics(db *gorm.DB, publisher mq.Publisher, scheduler TimeoutScheduler, paymentTimeout time.Duration, metrics *observability.Metrics) *Service {
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
		idempotency:      idempotency.NewService(db, 24*time.Hour),
		outboxRepo:       outbox.NewRepository(db),
		metrics:          metrics,
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
	success := false
	defer func() {
		s.metrics.RecordOrderCreated(success)
	}()

	if req.IdempotencyKey == "" {
		return nil, status.Error(codes.InvalidArgument, "idempotency key required")
	}

	aggregatedItems, err := aggregateCreateOrderItems(req.Items)
	if err != nil {
		return nil, err
	}

	requestHash, err := idempotency.HashPayload(newCreateOrderFingerprint(req.UserId, aggregatedItems))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to hash create order request: %v", err)
	}

	idempotencyResult, err := s.idempotency.Begin(ctx, idempotency.BeginRequest{
		UserID:         uint(req.UserId),
		RequestPath:    createOrderRequestPath,
		IdempotencyKey: req.IdempotencyKey,
		RequestHash:    requestHash,
	})
	if err != nil {
		return nil, orderIdempotencyError(err)
	}
	if idempotencyResult.Action == idempotency.ActionReplay {
		var replay pb.CreateOrderResponse
		if err := idempotency.ReplayInto(idempotencyResult.Record, &replay); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to replay create order response: %v", err)
		}
		success = true
		return &replay, nil
	}

	ctx, txSpan := observability.StartSpan(ctx, "mysql.transaction", trace.WithAttributes(attribute.String("db.system", "mysql"), attribute.String("db.operation", "create_order")))
	var txErr error
	defer func() { observability.EndSpan(txSpan, txErr) }()

	tx := s.db.Begin()
	if tx.Error != nil {
		txErr = tx.Error
		_ = s.idempotency.Abort(ctx, idempotencyResult.Record.ID)
		return nil, status.Errorf(codes.Internal, "failed to start transaction: %v", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			_ = s.idempotency.Abort(ctx, idempotencyResult.Record.ID)
		}
	}()

	// ??????????????????????????????????????
	orderItems, totalAmountCents, err := buildOrderSnapshots(tx, aggregatedItems)
	if err != nil {
		txErr = err
		if strings.Contains(status.Convert(err).Message(), "insufficient stock") {
			s.metrics.RecordInsufficientStock()
		}
		tx.Rollback()
		_ = s.idempotency.Abort(ctx, idempotencyResult.Record.ID)
		return nil, err
	}

	order := Order{
		UserID:           uint(req.UserId),
		TotalAmountCents: totalAmountCents,
		Status:           OrderStatusPending,
		OrderDate:        time.Now(),
	}
	if err := tx.Create(&order).Error; err != nil {
		txErr = err
		tx.Rollback()
		_ = s.idempotency.Abort(ctx, idempotencyResult.Record.ID)
		return nil, status.Errorf(codes.Internal, "failed to create order: %v", err)
	}

	// ??????????????????????????????
	for i := range orderItems {
		orderItems[i].OrderID = order.ID
	}
	if err := tx.Create(&orderItems).Error; err != nil {
		txErr = err
		tx.Rollback()
		_ = s.idempotency.Abort(ctx, idempotencyResult.Record.ID)
		return nil, status.Errorf(codes.Internal, "failed to create order items: %v", err)
	}

	// 订单事件与订单、订单项、库存扣减同事务提交，避免业务成功后事件丢失。
	orderEvent := newOrderCreatedEvent(ctx, &order, req.UserId, totalAmountCents, orderItems)
	if _, err := s.outboxRepo.Create(ctx, tx, outbox.NewEventInput{
		AggregateType: "order",
		AggregateID:   strconv.FormatUint(uint64(order.ID), 10),
		EventType:     events.OrderCreatedType,
		Payload:       orderEvent,
	}); err != nil {
		txErr = err
		tx.Rollback()
		_ = s.idempotency.Abort(ctx, idempotencyResult.Record.ID)
		return nil, status.Errorf(codes.Internal, "failed to create order outbox event: %v", err)
	}

	if err := tx.Commit().Error; err != nil {
		txErr = err
		_ = s.idempotency.Abort(ctx, idempotencyResult.Record.ID)
		return nil, status.Errorf(codes.Internal, "failed to commit order transaction: %v", err)
	}

	// ???????????????????????????????
	timeoutEvent := newOrderTimeoutCheckEvent(ctx, &order, req.UserId, s.paymentTimeout)
	if err := s.timeoutScheduler.Schedule(ctx, timeoutEvent, s.paymentTimeout); err != nil {
		slog.ErrorContext(ctx, "timeout_schedule_failed",
			"event_type", timeoutEvent.EventType,
			"event_id", timeoutEvent.EventID,
			"order_id", order.ID,
			"user_id", req.UserId,
			"error", err,
		)
	}

	response := &pb.CreateOrderResponse{Order: convertToPBOrder(&order, orderItems)}

	if err := s.idempotency.Complete(ctx, idempotencyResult.Record.ID, http.StatusOK, response); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to finalize create order idempotency record: %v", err)
	}
	success = true
	return response, nil
}

const createOrderRequestPath = "/api/orders"

type createOrderFingerprint struct {
	UserID int64                        `json:"user_id"`
	Items  []createOrderFingerprintItem `json:"items"`
}

type createOrderFingerprintItem struct {
	ProductID int64 `json:"product_id"`
	Quantity  int32 `json:"quantity"`
}

func newCreateOrderFingerprint(userID int64, items []aggregatedCreateOrderItem) createOrderFingerprint {
	fingerprintItems := make([]createOrderFingerprintItem, len(items))
	for i, item := range items {
		fingerprintItems[i] = createOrderFingerprintItem(item)
	}
	sort.Slice(fingerprintItems, func(i, j int) bool {
		return fingerprintItems[i].ProductID < fingerprintItems[j].ProductID
	})
	return createOrderFingerprint{UserID: userID, Items: fingerprintItems}
}

func newOrderCreatedEvent(ctx context.Context, order *Order, userID int64, totalAmountCents int64, items []OrderItem) events.OrderCreatedEvent {
	eventItems := make([]events.OrderItemSnapshot, len(items))
	for i, item := range items {
		eventItems[i] = events.OrderItemSnapshot{
			ProductID:   item.ProductID,
			MerchantID:  int64(item.MerchantID),
			ProductName: item.ProductName,
			PriceCents:  item.PriceCents,
			Quantity:    item.Quantity,
		}
	}

	return events.OrderCreatedEvent{
		BaseEvent:        events.NewBaseEventWithContext(ctx, events.OrderCreatedType, time.Now()),
		OrderID:          int64(order.ID),
		UserID:           userID,
		TotalAmountCents: totalAmountCents,
		Items:            eventItems,
	}
}

func newOrderTimeoutCheckEvent(ctx context.Context, order *Order, userID int64, timeout time.Duration) events.OrderTimeoutCheckEvent {
	createdAt := order.OrderDate.UTC()
	return events.OrderTimeoutCheckEvent{
		BaseEvent:      events.NewBaseEventWithContext(ctx, events.OrderTimeoutCheckType, time.Now()),
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
func buildOrderSnapshots(tx *gorm.DB, items []aggregatedCreateOrderItem) ([]OrderItem, int64, error) {
	orderItems := make([]OrderItem, 0, len(items))
	var totalAmountCents int64

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
			PriceCents:  productInfo.PriceCents,
			Quantity:    item.Quantity,
		})
		lineAmountCents, err := checkedLineAmountCents(productInfo.PriceCents, item.Quantity)
		if err != nil {
			return nil, 0, err
		}
		totalAmountCents, err = checkedAddAmountCents(totalAmountCents, lineAmountCents)
		if err != nil {
			return nil, 0, err
		}

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

	return orderItems, totalAmountCents, nil
}

func checkedLineAmountCents(priceCents int64, quantity int32) (int64, error) {
	if priceCents < 0 {
		return 0, status.Error(codes.InvalidArgument, "price_cents must be non-negative")
	}
	if quantity <= 0 {
		return 0, status.Error(codes.InvalidArgument, "quantity must be greater than zero")
	}
	if priceCents > math.MaxInt64/int64(quantity) {
		return 0, status.Error(codes.InvalidArgument, "order amount is too large")
	}
	return priceCents * int64(quantity), nil
}

func checkedAddAmountCents(total, add int64) (int64, error) {
	if add < 0 || total > math.MaxInt64-add {
		return 0, status.Error(codes.InvalidArgument, "order amount is too large")
	}
	return total + add, nil
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
			ProductId:   item.ProductID,   // 产品ID
			ProductName: item.ProductName, // 产品名称
			PriceCents:  item.PriceCents,  // 产品价格
			Quantity:    item.Quantity,    // 产品数量
		}
	}

	return &pb.Order{
		Id:               int64(order.ID),                      // 订单ID
		UserId:           int64(order.UserID),                  // 用户ID
		Items:            pbItems,                              // 订单商品列表
		TotalAmountCents: order.TotalAmountCents,               // 订单总金额
		Status:           order.Status,                         // 订单状态
		CreatedAt:        order.OrderDate.Format(time.RFC3339), // 订单创建时间
		CancelReason:     order.CancelReason,                   // 取消原因
	}
}

func collectOrderIDs(orders []Order) []uint {
	if len(orders) == 0 {
		return nil
	}
	orderIDs := make([]uint, len(orders))
	for i, order := range orders {
		orderIDs[i] = order.ID
	}
	return orderIDs
}

func fetchOrderItemsForOrders(db *gorm.DB, orderIDs []uint, merchantIDs []uint) (map[uint][]OrderItem, error) {
	itemsByOrderID := make(map[uint][]OrderItem, len(orderIDs))
	if len(orderIDs) == 0 {
		return itemsByOrderID, nil
	}

	var orderItems []OrderItem
	query := db.Where("order_id IN ?", orderIDs)
	if len(merchantIDs) > 0 {
		query = query.Where("merchant_id IN ?", merchantIDs)
	}
	if err := query.Order("order_id ASC").Order("id ASC").Find(&orderItems).Error; err != nil {
		return nil, err
	}

	for _, item := range orderItems {
		itemsByOrderID[item.OrderID] = append(itemsByOrderID[item.OrderID], item)
	}
	return itemsByOrderID, nil
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
	if req == nil {
		req = &pb.ListOrdersRequest{}
	}
	page, pageSize := normalizeOrderPagination(req.Page, req.PageSize)

	// 从数据库查询用户订单
	var orders []Order
	var total int64
	db := s.db.WithContext(ctx)

	// 查询订单总数
	if err := db.Model(&Order{}).Where("user_id = ?", req.UserId).Count(&total).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "failed to count orders: %v", err)
	}

	// 查询订单列表
	offset := (page - 1) * pageSize
	if err := db.Where("user_id = ?", req.UserId).Order("created_at DESC").Offset(int(offset)).Limit(int(pageSize)).Find(&orders).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch orders: %v", err)
	}

	itemsByOrderID, err := fetchOrderItemsForOrders(db, collectOrderIDs(orders), nil)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch order items: %v", err)
	}

	// 转换为proto对象
	pbOrders := make([]*pb.Order, len(orders))
	for i, order := range orders {
		pbOrders[i] = convertToPBOrder(&order, itemsByOrderID[order.ID])
	}

	// 返回订单列表响应
	return &pb.ListOrdersResponse{
		Orders: pbOrders,
		Total:  total,
	}, nil
}

// ListMerchantOrders 返回与当前商家相关的订单。
// 订单归属直接基于 order_items.merchant_id 快照筛选，因此商品后续改名、改价或换归属都不会污染历史订单。
func (s *Service) ListMerchantOrders(ctx context.Context, req *pb.ListMerchantOrdersRequest) (*pb.ListMerchantOrdersResponse, error) {
	if req == nil {
		req = &pb.ListMerchantOrdersRequest{}
	}
	page, pageSize := normalizeOrderPagination(req.Page, req.PageSize)

	merchantIDs, restrictedItems, err := s.resolveMerchantOrderScope(uint(req.ActorUserId), req.MerchantId)
	if err != nil {
		return nil, orderStatusError(err)
	}

	var orders []Order
	var total int64
	db := s.db.WithContext(ctx)

	if len(merchantIDs) == 0 && restrictedItems {
		return &pb.ListMerchantOrdersResponse{}, nil
	}

	orderQuery := db.Model(&Order{})
	if len(merchantIDs) > 0 {
		orderIDSubQuery := db.Model(&OrderItem{}).
			Select("DISTINCT order_id").
			Where("merchant_id IN ?", merchantIDs)
		orderQuery = orderQuery.Where("id IN (?)", orderIDSubQuery)
	}
	if err := orderQuery.Count(&total).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "failed to count merchant orders: %v", err)
	}
	offset := (page - 1) * pageSize
	if err := orderQuery.
		Order("created_at DESC").
		Order("id DESC").
		Offset(int(offset)).
		Limit(int(pageSize)).
		Find(&orders).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch merchant orders: %v", err)
	}

	var itemMerchantIDs []uint
	if restrictedItems {
		itemMerchantIDs = merchantIDs
	}
	itemsByOrderID, err := fetchOrderItemsForOrders(db, collectOrderIDs(orders), itemMerchantIDs)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch merchant order items: %v", err)
	}

	pbOrders := make([]*pb.Order, len(orders))
	for i, orderInfo := range orders {
		orderItems := itemsByOrderID[orderInfo.ID]

		visibleOrder := orderInfo
		if restrictedItems {
			visibleOrder.TotalAmountCents = sumOrderItems(orderItems)
		}
		pbOrders[i] = convertToPBOrder(&visibleOrder, orderItems)
	}

	return &pb.ListMerchantOrdersResponse{Orders: pbOrders, Total: total}, nil
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
	if req.IdempotencyKey == "" {
		return nil, status.Error(codes.InvalidArgument, "idempotency key required")
	}

	requestHash, err := idempotency.HashPayload(newCancelOrderFingerprint(req.UserId, req.Id))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to hash cancel order request: %v", err)
	}

	idempotencyResult, err := s.idempotency.Begin(ctx, idempotency.BeginRequest{
		UserID:         uint(req.UserId),
		RequestPath:    cancelOrderRequestPath,
		IdempotencyKey: req.IdempotencyKey,
		RequestHash:    requestHash,
	})
	if err != nil {
		return nil, orderIdempotencyError(err)
	}
	if idempotencyResult.Action == idempotency.ActionReplay {
		var replay pb.CancelOrderResponse
		if err := idempotency.ReplayInto(idempotencyResult.Record, &replay); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to replay cancel order response: %v", err)
		}
		return &replay, nil
	}

	_, changed, err := cancelOrderWithReason(s.db, req.Id, req.UserId, OrderCancelReasonUserCancelled, func(tx *gorm.DB, order *Order) error {
		event := events.OrderCancelledEvent{
			BaseEvent: events.NewBaseEventWithContext(ctx, events.OrderCancelledType, time.Now()),
			OrderID:   int64(order.ID),
			UserID:    req.UserId,
			Reason:    OrderCancelReasonUserCancelled,
		}
		_, err := s.outboxRepo.Create(ctx, tx, outbox.NewEventInput{
			AggregateType: "order",
			AggregateID:   strconv.FormatUint(uint64(order.ID), 10),
			EventType:     events.OrderCancelledType,
			Payload:       event,
		})
		return err
	})
	var response *pb.CancelOrderResponse
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			response = &pb.CancelOrderResponse{
				Success: false,
				Message: "订单不存在",
			}
		case errors.Is(err, ErrInvalidOrderTransition):
			response = &pb.CancelOrderResponse{
				Success: false,
				Message: err.Error(),
			}
		default:
			_ = s.idempotency.Abort(ctx, idempotencyResult.Record.ID)
			return &pb.CancelOrderResponse{
				Success: false,
				Message: "取消订单失败",
			}, nil
		}
	} else if !changed {
		response = &pb.CancelOrderResponse{
			Success: false,
			Message: "订单已取消",
		}
	} else {
		// 返回取消订单响应
		response = &pb.CancelOrderResponse{
			Success: true,
			Message: "订单取消成功",
		}
	}

	s.metrics.RecordOrderCancelled(response.GetSuccess(), OrderCancelReasonUserCancelled)

	if err := s.idempotency.Complete(ctx, idempotencyResult.Record.ID, http.StatusOK, response); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to finalize cancel order idempotency record: %v", err)
	}
	return response, nil
}

const (
	cancelOrderRequestPath = "/api/orders/:id/cancel"
	cancelOrderOperation   = "cancel_order"
)

type cancelOrderFingerprint struct {
	Operation string `json:"operation"`
	UserID    int64  `json:"user_id"`
	OrderID   int64  `json:"order_id"`
}

func newCancelOrderFingerprint(userID, orderID int64) cancelOrderFingerprint {
	return cancelOrderFingerprint{
		Operation: cancelOrderOperation,
		UserID:    userID,
		OrderID:   orderID,
	}
}

const (
	OrderCancelReasonUserCancelled  = "user_cancelled"
	OrderCancelReasonPaymentTimeout = "payment_timeout"
)

// cancelOrderWithReason 是人工取消与超时取消共用的核心路径。
// 只有首次把 pending 推进到 cancelled 时才会回补库存，因此天然支持重复消息幂等。
func cancelOrderWithReason(db *gorm.DB, orderID, userID int64, reason string, afterChange func(tx *gorm.DB, order *Order) error) (*Order, bool, error) {
	var order *Order
	changed := false

	err := db.Transaction(func(tx *gorm.DB) error {
		updated, didChange, err := cancelOrderWithReasonInTx(tx, orderID, userID, reason, afterChange)
		order = updated
		changed = didChange
		return err
	})
	if err != nil {
		return nil, false, err
	}
	return order, changed, nil
}

func cancelOrderWithReasonInTx(tx *gorm.DB, orderID, userID int64, reason string, afterChange func(tx *gorm.DB, order *Order) error) (*Order, bool, error) {
	var order Order
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND user_id = ?", orderID, userID).
		First(&order).Error; err != nil {
		return nil, false, err
	}

	if order.Status == OrderStatusCancelled {
		return &order, false, nil
	}
	if err := ValidateTransition(order.Status, OrderStatusCancelled); err != nil {
		return nil, false, err
	}

	var orderItems []OrderItem
	if err := tx.Where("order_id = ?", order.ID).Find(&orderItems).Error; err != nil {
		return nil, false, err
	}

	for _, item := range orderItems {
		if err := product.RestoreStock(tx, item.ProductID, item.Quantity); err != nil {
			return nil, false, err
		}
	}

	if err := TransitionTo(&order, OrderStatusCancelled); err != nil {
		return nil, false, err
	}
	order.CancelReason = reason
	if err := tx.Save(&order).Error; err != nil {
		return nil, false, err
	}
	if afterChange != nil {
		if err := afterChange(tx, &order); err != nil {
			return nil, false, err
		}
	}
	return &order, true, nil
}

// ShipOrder 允许具备权限的商家或管理员把已支付订单推进到已发货。
func (s *Service) ShipOrder(ctx context.Context, req *pb.ShipOrderRequest) (*pb.ShipOrderResponse, error) {
	var order Order
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&order, req.Id).Error; err != nil {
			return err
		}

		var orderItems []OrderItem
		if err := tx.Where("order_id = ?", order.ID).Find(&orderItems).Error; err != nil {
			return err
		}
		if err := s.authorizeShipmentWithDB(tx, uint(req.ActorUserId), orderItems); err != nil {
			return err
		}

		fromStatus := order.Status
		if err := TransitionTo(&order, OrderStatusShipped); err != nil {
			return err
		}
		if err := tx.Save(&order).Error; err != nil {
			return err
		}
		statusEvent := newOrderStatusChangedEvent(ctx, events.OrderShippedType, &order, fromStatus, OrderStatusShipped)
		_, err := s.outboxRepo.Create(ctx, tx, outbox.NewEventInput{
			AggregateType: "order",
			AggregateID:   strconv.FormatUint(uint64(order.ID), 10),
			EventType:     events.OrderShippedType,
			Payload:       statusEvent,
		})
		return err
	}); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Error(codes.NotFound, "order not found")
		}
		if isOrderStatusError(err) {
			return nil, orderStatusError(err)
		}
		return nil, status.Errorf(codes.Internal, "failed to update order status: %v", err)
	}

	return &pb.ShipOrderResponse{Success: true, Message: "订单已发货"}, nil
}

// CompleteOrder 仅允许订单所属用户确认收货，把已发货订单推进到已完成。
func (s *Service) CompleteOrder(ctx context.Context, req *pb.CompleteOrderRequest) (*pb.CompleteOrderResponse, error) {
	var order Order
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ?", req.Id, req.UserId).
			First(&order).Error; err != nil {
			return err
		}

		fromStatus := order.Status
		if err := TransitionTo(&order, OrderStatusCompleted); err != nil {
			return err
		}
		if err := tx.Save(&order).Error; err != nil {
			return err
		}
		statusEvent := newOrderStatusChangedEvent(ctx, events.OrderCompletedType, &order, fromStatus, OrderStatusCompleted)
		_, err := s.outboxRepo.Create(ctx, tx, outbox.NewEventInput{
			AggregateType: "order",
			AggregateID:   strconv.FormatUint(uint64(order.ID), 10),
			EventType:     events.OrderCompletedType,
			Payload:       statusEvent,
		})
		return err
	}); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Error(codes.NotFound, "order not found")
		}
		if isOrderStatusError(err) {
			return nil, orderStatusError(err)
		}
		return nil, status.Errorf(codes.Internal, "failed to update order status: %v", err)
	}

	return &pb.CompleteOrderResponse{Success: true, Message: "订单已完成"}, nil
}

func (s *Service) authorizeShipment(actorUserID uint, orderItems []OrderItem) error {
	return s.authorizeShipmentWithDB(s.db, actorUserID, orderItems)
}

func (s *Service) authorizeShipmentWithDB(db *gorm.DB, actorUserID uint, orderItems []OrderItem) error {
	var actor auth.User
	if err := db.First(&actor, actorUserID).Error; err != nil {
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
		if err := db.Where("id IN ?", merchantIDs).Find(&merchants).Error; err != nil {
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

func normalizeOrderPagination(page, pageSize int32) (int32, int32) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func (s *Service) resolveMerchantOrderScope(actorUserID uint, requestedMerchantID *int64) ([]uint, bool, error) {
	var actor auth.User
	if err := s.db.First(&actor, actorUserID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, merchant.ErrUserNotFound
		}
		return nil, false, err
	}

	switch actor.Role {
	case auth.RoleAdmin:
		if requestedMerchantID == nil {
			return nil, false, nil
		}
		var shop merchant.Merchant
		if err := s.db.First(&shop, *requestedMerchantID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, false, merchant.ErrMerchantNotFound
			}
			return nil, false, err
		}
		return []uint{shop.ID}, false, nil
	case auth.RoleMerchant:
		query := s.db.Where("owner_user_id = ?", actorUserID)
		if requestedMerchantID != nil {
			query = query.Where("id = ?", *requestedMerchantID)
		}
		var shops []merchant.Merchant
		if err := query.Find(&shops).Error; err != nil {
			return nil, false, err
		}
		if requestedMerchantID != nil && len(shops) == 0 {
			return nil, false, merchant.ErrPermissionDenied
		}
		merchantIDs := make([]uint, len(shops))
		for i, shop := range shops {
			merchantIDs[i] = shop.ID
		}
		return merchantIDs, true, nil
	default:
		return nil, false, merchant.ErrPermissionDenied
	}
}

func sumOrderItems(items []OrderItem) int64 {
	var total int64
	for _, item := range items {
		lineAmountCents, err := checkedLineAmountCents(item.PriceCents, item.Quantity)
		if err != nil {
			return 0
		}
		total, err = checkedAddAmountCents(total, lineAmountCents)
		if err != nil {
			return 0
		}
	}
	return total
}

func newOrderStatusChangedEvent(ctx context.Context, eventType string, order *Order, fromStatus, toStatus string) events.OrderStatusChangedEvent {
	return events.OrderStatusChangedEvent{
		BaseEvent:  events.NewBaseEventWithContext(ctx, eventType, time.Now()),
		OrderID:    int64(order.ID),
		UserID:     int64(order.UserID),
		FromStatus: fromStatus,
		ToStatus:   toStatus,
	}
}

func orderIdempotencyError(err error) error {
	switch {
	case errors.Is(err, idempotency.ErrConflict):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, idempotency.ErrInProgress):
		return status.Error(codes.FailedPrecondition, err.Error())
	default:
		return status.Errorf(codes.Internal, "idempotency operation failed: %v", err)
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
	case errors.Is(err, merchant.ErrMerchantNotFound):
		return status.Error(codes.NotFound, "merchant not found")
	default:
		return status.Errorf(codes.Internal, "order operation failed: %v", err)
	}
}

func isOrderStatusError(err error) bool {
	return errors.Is(err, ErrInvalidOrderTransition) ||
		errors.Is(err, merchant.ErrPermissionDenied) ||
		errors.Is(err, merchant.ErrUserNotFound) ||
		errors.Is(err, merchant.ErrMerchantNotFound)
}
