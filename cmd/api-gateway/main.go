// api-gateway 服务入口文件
// 负责处理HTTP请求，作为前端和后端微服务之间的桥梁
// 使用Gin框架提供RESTful API接口
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	// Gin框架：用于处理HTTP请求和路由
	"github.com/gin-gonic/gin"
	// gRPC客户端：用于与后端微服务通信
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	// 导入各个服务的protobuf生成代码
	pbAuth "go-commerce/api/auth"
	pbCart "go-commerce/api/cart"
	pbMerchant "go-commerce/api/merchant"
	pbOrder "go-commerce/api/order"
	pbProduct "go-commerce/api/product"

	// JWT工具：用于验证用户令牌
	"go-commerce/pkg/jwt"
)

// getEnv 获取环境变量，如果不存在则返回默认值
// 参数：
//
//	key: 环境变量名称
//	defaultValue: 默认值
//
// 返回值：
//
//	环境变量值或默认值
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// APIGateway API网关结构体
// 包含所有微服务的gRPC客户端
// 用于转发HTTP请求到对应的微服务

type APIGateway struct {
	authClient     pbAuth.AuthServiceClient         // 认证服务客户端
	productClient  pbProduct.ProductServiceClient   // 产品服务客户端
	orderClient    pbOrder.OrderServiceClient       // 订单服务客户端
	merchantClient pbMerchant.MerchantServiceClient // 商家服务客户端
	cartClient     pbCart.CartServiceClient         // 购物车服务客户端
}

// main 函数是api-gateway服务的入口点
// 负责初始化各个微服务客户端、设置路由和启动HTTP服务器
func main() {
	// 从环境变量获取各个微服务的地址
	// 如果环境变量不存在，则使用默认地址
	authServiceAddr := getEnv("AUTH_SERVICE_ADDR", "localhost:50051")
	productServiceAddr := getEnv("PRODUCT_SERVICE_ADDR", "localhost:50052")
	orderServiceAddr := getEnv("ORDER_SERVICE_ADDR", "localhost:50053")
	merchantServiceAddr := getEnv("MERCHANT_SERVICE_ADDR", "localhost:50055")
	cartServiceAddr := getEnv("CART_SERVICE_ADDR", "localhost:50054")

	// 连接认证服务
	authConn, err := grpc.Dial(authServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect to auth service: %v", err)
	}
	defer authConn.Close()
	authClient := pbAuth.NewAuthServiceClient(authConn)

	// 连接产品服务
	productConn, err := grpc.Dial(productServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect to product service: %v", err)
	}
	defer productConn.Close()
	productClient := pbProduct.NewProductServiceClient(productConn)

	// 连接订单服务
	orderConn, err := grpc.Dial(orderServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect to order service: %v", err)
	}
	defer orderConn.Close()
	orderClient := pbOrder.NewOrderServiceClient(orderConn)

	// 连接商家服务
	merchantConn, err := grpc.Dial(merchantServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect to merchant service: %v", err)
	}
	defer merchantConn.Close()
	merchantClient := pbMerchant.NewMerchantServiceClient(merchantConn)

	// 连接购物车服务
	cartConn, err := grpc.Dial(cartServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect to cart service: %v", err)
	}
	defer cartConn.Close()
	cartClient := pbCart.NewCartServiceClient(cartConn)

	// 初始化API网关实例
	gateway := &APIGateway{
		authClient:     authClient,
		productClient:  productClient,
		orderClient:    orderClient,
		merchantClient: merchantClient,
		cartClient:     cartClient,
	}

	// 创建Gin默认路由引擎
	r := gin.Default()

	// 添加CORS中间件
	// 允许跨域请求，设置允许的HTTP方法和头信息
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		// 处理OPTIONS预检请求
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	// 公共路由组：不需要认证的接口
	public := r.Group("/api")
	{
		// 用户认证相关路由
		public.POST("/register", gateway.handleRegister) // 用户注册
		public.POST("/login", gateway.handleLogin)       // 用户登录
		// 产品相关路由
		public.GET("/products", gateway.handleListProducts)   // 获取产品列表
		public.GET("/products/:id", gateway.handleGetProduct) // 获取单个产品详情
		// 商家相关路由
		public.POST("/merchants", gateway.handleCreateMerchant)                   // 创建商家
		public.GET("/merchants/:id", gateway.handleGetMerchant)                   // 获取商家详情
		public.GET("/merchants", gateway.handleListMerchants)                     // 获取商家列表
		public.POST("/merchants/products", gateway.handleMerchantAddProduct)      // 商家添加产品
		public.DELETE("/merchants/products", gateway.handleMerchantDeleteProduct) // 商家删除产品
	}

	// 私有路由组：需要认证的接口
	private := r.Group("/api")
	private.Use(gateway.authMiddleware()) // 添加认证中间件
	{
		// 订单相关路由
		private.POST("/orders", gateway.handleCreateOrder)           // 创建订单
		private.GET("/orders/:id", gateway.handleGetOrder)           // 获取订单详情
		private.GET("/orders", gateway.handleListOrders)             // 获取订单列表
		private.PUT("/orders/:id/cancel", gateway.handleCancelOrder) // 取消订单
		// 购物车相关路由
		private.POST("/cart/items", gateway.handleAddCartItem)      // 添加购物车商品
		private.GET("/cart", gateway.handleGetCart)                 // 获取购物车
		private.PUT("/cart/items", gateway.handleUpdateCartItem)    // 更新购物车商品
		private.DELETE("/cart/items", gateway.handleDeleteCartItem) // 删除购物车商品
		private.DELETE("/cart", gateway.handleClearCart)            // 清空购物车
	}

	// 启动HTTP服务器，监听8080端口
	log.Fatal(r.Run(":8080"))
}

// authMiddleware 认证中间件
// 用于验证用户的JWT令牌
// 返回值：
//
//	gin.HandlerFunc: Gin中间件函数
func (g *APIGateway) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从请求头获取Authorization
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authorization header required"})
			c.Abort()
			return
		}

		// 检查Authorization格式是否正确
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization format"})
			c.Abort()
			return
		}

		// 提取令牌并验证
		token := parts[1]
		claims, err := jwt.ValidateToken(token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}

		// 将用户ID存储到上下文中
		c.Set("user_id", claims.UserID)
		c.Next()
	}
}

// handleRegister 处理用户注册请求
// 参数：
//
//	c: Gin上下文，包含请求和响应信息
func (g *APIGateway) handleRegister(c *gin.Context) {
	// 定义请求结构体
	var req struct {
		Username string `json:"username"` // 用户名
		Password string `json:"password"` // 密码
		Email    string `json:"email"`    // 邮箱
	}

	// 绑定JSON请求体
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 调用认证服务的Register方法
	resp, err := g.authClient.Register(context.Background(), &pbAuth.RegisterRequest{
		Username: req.Username,
		Password: req.Password,
		Email:    req.Email,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 返回注册成功响应
	c.JSON(http.StatusOK, gin.H{
		"user_id": resp.UserId,
		"token":   resp.Token,
	})
}

// handleLogin 处理用户登录请求
// 参数：
//
//	c: Gin上下文，包含请求和响应信息
func (g *APIGateway) handleLogin(c *gin.Context) {
	// 定义请求结构体
	var req struct {
		Username string `json:"username"` // 用户名
		Password string `json:"password"` // 密码
	}

	// 绑定JSON请求体
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 调用认证服务的Login方法
	resp, err := g.authClient.Login(context.Background(), &pbAuth.LoginRequest{
		Username: req.Username,
		Password: req.Password,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 返回登录成功响应
	c.JSON(http.StatusOK, gin.H{
		"user_id": resp.UserId,
		"token":   resp.Token,
	})
}

// handleListProducts 处理获取产品列表请求
// 参数：
//
//	c: Gin上下文，包含请求和响应信息
func (g *APIGateway) handleListProducts(c *gin.Context) {
	// 调用产品服务的ListProducts方法
	resp, err := g.productClient.ListProducts(context.Background(), &pbProduct.ListProductsRequest{
		Page:     1,  // 默认页码
		PageSize: 10, // 默认每页数量
	})
	if err != nil {
		log.Printf("Error calling product service: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// 返回产品列表
	c.JSON(http.StatusOK, resp)
}

// handleGetProduct 处理获取单个产品详情请求
// 参数：
//
//	c: Gin上下文，包含请求和响应信息
func (g *APIGateway) handleGetProduct(c *gin.Context) {
	// 从路径参数获取产品ID
	id := c.Param("id")
	productId := int64(0)
	// 解析产品ID
	_, err := fmt.Sscanf(id, "%d", &productId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid product id"})
		return
	}
	// 调用产品服务的GetProduct方法
	resp, err := g.productClient.GetProduct(context.Background(), &pbProduct.GetProductRequest{
		Id: productId,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// 返回产品详情
	c.JSON(http.StatusOK, resp)
}

// handleCreateOrder 处理创建订单请求
// 参数：
//
//	c: Gin上下文，包含请求和响应信息
func (g *APIGateway) handleCreateOrder(c *gin.Context) {
	// 从上下文中获取用户ID
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	// 定义请求结构体
	var req struct {
		Items []struct {
			ProductId   int64   `json:"product_id"`   // 产品ID
			ProductName string  `json:"product_name"` // 产品名称
			Price       float32 `json:"price"`        // 产品价格
			Quantity    int32   `json:"quantity"`     // 产品数量
		} `json:"items"` // 订单商品列表
	}

	// 绑定JSON请求体
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 转换订单商品格式
	orderItems := make([]*pbOrder.OrderItem, len(req.Items))
	for i, item := range req.Items {
		orderItems[i] = &pbOrder.OrderItem{
			ProductId:   item.ProductId,
			ProductName: item.ProductName,
			Price:       item.Price,
			Quantity:    item.Quantity,
		}
	}

	// 调用订单服务的CreateOrder方法
	resp, err := g.orderClient.CreateOrder(context.Background(), &pbOrder.CreateOrderRequest{
		UserId: userID.(int64),
		Items:  orderItems,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 返回创建订单响应
	c.JSON(http.StatusOK, resp)
}

// handleGetOrder 处理获取订单详情请求
// 参数：
//
//	c: Gin上下文，包含请求和响应信息
func (g *APIGateway) handleGetOrder(c *gin.Context) {
	// 从上下文中获取用户ID
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	// 从路径参数获取订单ID
	id := c.Param("id")
	orderId := int64(0)
	// 解析订单ID
	_, err := fmt.Sscanf(id, "%d", &orderId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid order id"})
		return
	}

	// 调用订单服务的GetOrder方法
	resp, err := g.orderClient.GetOrder(context.Background(), &pbOrder.GetOrderRequest{
		Id:     orderId,
		UserId: userID.(int64),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 返回订单详情
	c.JSON(http.StatusOK, resp)
}

// handleListOrders 处理获取订单列表请求
// 参数：
//
//	c: Gin上下文，包含请求和响应信息
func (g *APIGateway) handleListOrders(c *gin.Context) {
	// 从上下文中获取用户ID
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	// 调用订单服务的ListOrders方法
	resp, err := g.orderClient.ListOrders(context.Background(), &pbOrder.ListOrdersRequest{
		UserId: userID.(int64),
	})
	if err != nil {
		log.Printf("Error calling order service: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 返回订单列表
	c.JSON(http.StatusOK, resp)
}

// handleCreateMerchant 处理创建商家请求
// 参数：
//
//	c: Gin上下文，包含请求和响应信息
func (g *APIGateway) handleCreateMerchant(c *gin.Context) {
	// 定义请求结构体
	var req struct {
		Name        string `json:"name"`         // 商家名称
		ContactInfo string `json:"contact_info"` // 联系信息
	}

	// 绑定JSON请求体
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 调用商家服务的CreateMerchant方法
	resp, err := g.merchantClient.CreateMerchant(context.Background(), &pbMerchant.CreateMerchantRequest{
		Name:        req.Name,
		ContactInfo: req.ContactInfo,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 返回创建商家响应
	c.JSON(http.StatusOK, resp)
}

// handleGetMerchant 处理获取商家详情请求
// 参数：
//
//	c: Gin上下文，包含请求和响应信息
func (g *APIGateway) handleGetMerchant(c *gin.Context) {
	// 从路径参数获取商家ID
	id := c.Param("id")
	merchantId := int64(0)
	// 解析商家ID
	_, err := fmt.Sscanf(id, "%d", &merchantId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid merchant id"})
		return
	}

	// 调用商家服务的GetMerchant方法
	resp, err := g.merchantClient.GetMerchant(context.Background(), &pbMerchant.GetMerchantRequest{
		Id: merchantId,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 返回商家详情
	c.JSON(http.StatusOK, resp)
}

// handleListMerchants 处理获取商家列表请求
// 参数：
//
//	c: Gin上下文，包含请求和响应信息
func (g *APIGateway) handleListMerchants(c *gin.Context) {
	// 调用商家服务的ListMerchants方法
	resp, err := g.merchantClient.ListMerchants(context.Background(), &pbMerchant.ListMerchantsRequest{
		Page:     1,  // 默认页码
		PageSize: 10, // 默认每页数量
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 返回商家列表
	c.JSON(http.StatusOK, resp)
}

// handleMerchantAddProduct 处理商家添加产品请求
// 参数：
//
//	c: Gin上下文，包含请求和响应信息
func (g *APIGateway) handleMerchantAddProduct(c *gin.Context) {
	// 定义请求结构体
	var req struct {
		MerchantId  int64   `json:"merchant_id"` // 商家ID
		Name        string  `json:"name"`        // 产品名称
		Description string  `json:"description"` // 产品描述
		Price       float32 `json:"price"`       // 产品价格
		Stock       int32   `json:"stock"`       // 产品库存
		Category    string  `json:"category"`    // 产品分类
		ImageUrl    string  `json:"image_url"`   // 产品图片URL
	}

	// 绑定JSON请求体
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 调用商家服务的AddProduct方法
	resp, err := g.merchantClient.AddProduct(context.Background(), &pbMerchant.AddProductRequest{
		MerchantId:  req.MerchantId,
		Name:        req.Name,
		Description: req.Description,
		Price:       req.Price,
		Stock:       req.Stock,
		Category:    req.Category,
		ImageUrl:    req.ImageUrl,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 返回添加产品响应
	c.JSON(http.StatusOK, resp)
}

// handleMerchantDeleteProduct 处理商家删除产品请求
// 参数：
//
//	c: Gin上下文，包含请求和响应信息
func (g *APIGateway) handleMerchantDeleteProduct(c *gin.Context) {
	// 定义请求结构体
	var req struct {
		MerchantId int64 `json:"merchant_id"` // 商家ID
		ProductId  int64 `json:"product_id"`  // 产品ID
	}

	// 绑定JSON请求体
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 调用商家服务的DeleteProduct方法
	resp, err := g.merchantClient.DeleteProduct(context.Background(), &pbMerchant.DeleteProductRequest{
		MerchantId: req.MerchantId,
		ProductId:  req.ProductId,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 返回删除产品响应
	c.JSON(http.StatusOK, resp)
}

// handleAddCartItem 处理添加购物车商品请求
// 参数：
//
//	c: Gin上下文，包含请求和响应信息
func (g *APIGateway) handleAddCartItem(c *gin.Context) {
	// 从上下文中获取用户ID
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	// 定义请求结构体
	var req struct {
		ProductId int64 `json:"product_id"` // 产品ID
		Quantity  int32 `json:"quantity"`   // 产品数量
	}

	// 绑定JSON请求体
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 调用购物车服务的AddCartItem方法
	resp, err := g.cartClient.AddCartItem(context.Background(), &pbCart.AddCartItemRequest{
		UserId:    userID.(int64),
		ProductId: req.ProductId,
		Quantity:  req.Quantity,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 返回添加购物车商品响应
	c.JSON(http.StatusOK, resp)
}

// handleGetCart 处理获取购物车请求
// 参数：
//
//	c: Gin上下文，包含请求和响应信息
func (g *APIGateway) handleGetCart(c *gin.Context) {
	// 从上下文中获取用户ID
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	// 调用购物车服务的GetCart方法
	resp, err := g.cartClient.GetCart(context.Background(), &pbCart.GetCartRequest{
		UserId: userID.(int64),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 返回购物车信息
	c.JSON(http.StatusOK, resp)
}

// handleUpdateCartItem 处理更新购物车商品请求
// 参数：
//
//	c: Gin上下文，包含请求和响应信息
func (g *APIGateway) handleUpdateCartItem(c *gin.Context) {
	// 从上下文中获取用户ID
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	// 定义请求结构体
	var req struct {
		ProductId int64 `json:"product_id"` // 产品ID
		Quantity  int32 `json:"quantity"`   // 产品数量
	}

	// 绑定JSON请求体
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 调用购物车服务的UpdateCartItem方法
	resp, err := g.cartClient.UpdateCartItem(context.Background(), &pbCart.UpdateCartItemRequest{
		UserId:    userID.(int64),
		ProductId: req.ProductId,
		Quantity:  req.Quantity,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 返回更新购物车商品响应
	c.JSON(http.StatusOK, resp)
}

// handleDeleteCartItem 处理删除购物车商品请求
// 参数：
//
//	c: Gin上下文，包含请求和响应信息
func (g *APIGateway) handleDeleteCartItem(c *gin.Context) {
	// 从上下文中获取用户ID
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	// 定义请求结构体
	var req struct {
		ProductId int64 `json:"product_id"` // 产品ID
	}

	// 绑定JSON请求体
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 调用购物车服务的RemoveCartItem方法
	resp, err := g.cartClient.RemoveCartItem(context.Background(), &pbCart.RemoveCartItemRequest{
		UserId:    userID.(int64),
		ProductId: req.ProductId,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 返回删除购物车商品响应
	c.JSON(http.StatusOK, resp)
}

// handleClearCart 处理清空购物车请求
// 参数：
//
//	c: Gin上下文，包含请求和响应信息
func (g *APIGateway) handleClearCart(c *gin.Context) {
	// 从上下文中获取用户ID
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	// 调用购物车服务的ClearCart方法
	resp, err := g.cartClient.ClearCart(context.Background(), &pbCart.ClearCartRequest{
		UserId: userID.(int64),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 返回清空购物车响应
	c.JSON(http.StatusOK, resp)
}

// handleCancelOrder 处理取消订单请求
// 参数：
//
//	c: Gin上下文，包含请求和响应信息
func (g *APIGateway) handleCancelOrder(c *gin.Context) {
	// 从上下文中获取用户ID
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	// 从路径参数获取订单ID
	id := c.Param("id")
	orderId := int64(0)
	// 解析订单ID
	_, err := fmt.Sscanf(id, "%d", &orderId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid order id"})
		return
	}

	// 调用订单服务的CancelOrder方法
	resp, err := g.orderClient.CancelOrder(context.Background(), &pbOrder.CancelOrderRequest{
		Id:     orderId,
		UserId: userID.(int64),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 返回取消订单响应
	c.JSON(http.StatusOK, resp)
}
