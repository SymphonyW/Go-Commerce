package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pbAuth "go-commerce/api/auth"
	pbCart "go-commerce/api/cart"
	pbMerchant "go-commerce/api/merchant"
	pbOrder "go-commerce/api/order"
	pbProduct "go-commerce/api/product"
	"go-commerce/pkg/jwt"
)

// getEnv 获取环境变量，如果不存在则返回默认值
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

type APIGateway struct {
	authClient     pbAuth.AuthServiceClient
	productClient  pbProduct.ProductServiceClient
	orderClient    pbOrder.OrderServiceClient
	merchantClient pbMerchant.MerchantServiceClient
	cartClient     pbCart.CartServiceClient
}

func main() {
	// 从环境变量获取服务地址
	authServiceAddr := getEnv("AUTH_SERVICE_ADDR", "localhost:50051")
	productServiceAddr := getEnv("PRODUCT_SERVICE_ADDR", "localhost:50052")
	orderServiceAddr := getEnv("ORDER_SERVICE_ADDR", "localhost:50053")
	merchantServiceAddr := getEnv("MERCHANT_SERVICE_ADDR", "localhost:50055")
	cartServiceAddr := getEnv("CART_SERVICE_ADDR", "localhost:50054")

	authConn, err := grpc.Dial(authServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect to auth service: %v", err)
	}
	defer authConn.Close()
	authClient := pbAuth.NewAuthServiceClient(authConn)

	productConn, err := grpc.Dial(productServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect to product service: %v", err)
	}
	defer productConn.Close()
	productClient := pbProduct.NewProductServiceClient(productConn)

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

	gateway := &APIGateway{
		authClient:     authClient,
		productClient:  productClient,
		orderClient:    orderClient,
		merchantClient: merchantClient,
		cartClient:     cartClient,
	}

	r := gin.Default()

	// 添加CORS中间件
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	public := r.Group("/api")
	{
		public.POST("/register", gateway.handleRegister)
		public.POST("/login", gateway.handleLogin)
		public.GET("/products", gateway.handleListProducts)
		public.GET("/products/:id", gateway.handleGetProduct)
		// 商家相关路由
		public.POST("/merchants", gateway.handleCreateMerchant)
		public.GET("/merchants/:id", gateway.handleGetMerchant)
		public.GET("/merchants", gateway.handleListMerchants)
		public.POST("/merchants/products", gateway.handleMerchantAddProduct)
		public.DELETE("/merchants/products", gateway.handleMerchantDeleteProduct)
	}

	private := r.Group("/api")
	private.Use(gateway.authMiddleware())
	{
		private.POST("/orders", gateway.handleCreateOrder)
		private.GET("/orders/:id", gateway.handleGetOrder)
		private.GET("/orders", gateway.handleListOrders)
		private.PUT("/orders/:id/cancel", gateway.handleCancelOrder)
		// 购物车相关路由
		private.POST("/cart/items", gateway.handleAddCartItem)
		private.GET("/cart", gateway.handleGetCart)
		private.PUT("/cart/items", gateway.handleUpdateCartItem)
		private.DELETE("/cart/items", gateway.handleDeleteCartItem)
		private.DELETE("/cart", gateway.handleClearCart)
	}

	log.Fatal(r.Run(":8081"))
}

func (g *APIGateway) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authorization header required"})
			c.Abort()
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization format"})
			c.Abort()
			return
		}

		token := parts[1]
		claims, err := jwt.ValidateToken(token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}

		c.Set("user_id", claims.UserID)
		c.Next()
	}
}

func (g *APIGateway) handleRegister(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Email    string `json:"email"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := g.authClient.Register(context.Background(), &pbAuth.RegisterRequest{
		Username: req.Username,
		Password: req.Password,
		Email:    req.Email,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user_id": resp.UserId,
		"token":   resp.Token,
	})
}

func (g *APIGateway) handleLogin(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := g.authClient.Login(context.Background(), &pbAuth.LoginRequest{
		Username: req.Username,
		Password: req.Password,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user_id": resp.UserId,
		"token":   resp.Token,
	})
}

func (g *APIGateway) handleListProducts(c *gin.Context) {
	resp, err := g.productClient.ListProducts(context.Background(), &pbProduct.ListProductsRequest{
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		log.Printf("Error calling product service: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (g *APIGateway) handleGetProduct(c *gin.Context) {
	id := c.Param("id")
	productId := int64(0)
	_, err := fmt.Sscanf(id, "%d", &productId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid product id"})
		return
	}
	resp, err := g.productClient.GetProduct(context.Background(), &pbProduct.GetProductRequest{
		Id: productId,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (g *APIGateway) handleCreateOrder(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	var req struct {
		Items []struct {
			ProductId   int64   `json:"product_id"`
			ProductName string  `json:"product_name"`
			Price       float32 `json:"price"`
			Quantity    int32   `json:"quantity"`
		} `json:"items"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	orderItems := make([]*pbOrder.OrderItem, len(req.Items))
	for i, item := range req.Items {
		orderItems[i] = &pbOrder.OrderItem{
			ProductId:   item.ProductId,
			ProductName: item.ProductName,
			Price:       item.Price,
			Quantity:    item.Quantity,
		}
	}

	resp, err := g.orderClient.CreateOrder(context.Background(), &pbOrder.CreateOrderRequest{
		UserId: userID.(int64),
		Items:  orderItems,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (g *APIGateway) handleGetOrder(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	id := c.Param("id")
	orderId := int64(0)
	_, err := fmt.Sscanf(id, "%d", &orderId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid order id"})
		return
	}

	resp, err := g.orderClient.GetOrder(context.Background(), &pbOrder.GetOrderRequest{
		Id:     orderId,
		UserId: userID.(int64),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (g *APIGateway) handleListOrders(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	resp, err := g.orderClient.ListOrders(context.Background(), &pbOrder.ListOrdersRequest{
		UserId: userID.(int64),
	})
	if err != nil {
		log.Printf("Error calling order service: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// 商家相关处理函数
func (g *APIGateway) handleCreateMerchant(c *gin.Context) {
	var req struct {
		Name        string `json:"name"`
		ContactInfo string `json:"contact_info"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := g.merchantClient.CreateMerchant(context.Background(), &pbMerchant.CreateMerchantRequest{
		Name:        req.Name,
		ContactInfo: req.ContactInfo,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (g *APIGateway) handleGetMerchant(c *gin.Context) {
	id := c.Param("id")
	merchantId := int64(0)
	_, err := fmt.Sscanf(id, "%d", &merchantId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid merchant id"})
		return
	}

	resp, err := g.merchantClient.GetMerchant(context.Background(), &pbMerchant.GetMerchantRequest{
		Id: merchantId,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (g *APIGateway) handleListMerchants(c *gin.Context) {
	resp, err := g.merchantClient.ListMerchants(context.Background(), &pbMerchant.ListMerchantsRequest{
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (g *APIGateway) handleMerchantAddProduct(c *gin.Context) {
	var req struct {
		MerchantId  int64   `json:"merchant_id"`
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Price       float32 `json:"price"`
		Stock       int32   `json:"stock"`
		Category    string  `json:"category"`
		ImageUrl    string  `json:"image_url"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

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

	c.JSON(http.StatusOK, resp)
}

func (g *APIGateway) handleMerchantDeleteProduct(c *gin.Context) {
	var req struct {
		MerchantId int64 `json:"merchant_id"`
		ProductId  int64 `json:"product_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := g.merchantClient.DeleteProduct(context.Background(), &pbMerchant.DeleteProductRequest{
		MerchantId: req.MerchantId,
		ProductId:  req.ProductId,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// 购物车相关处理函数
func (g *APIGateway) handleAddCartItem(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	var req struct {
		ProductId int64 `json:"product_id"`
		Quantity  int32 `json:"quantity"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := g.cartClient.AddCartItem(context.Background(), &pbCart.AddCartItemRequest{
		UserId:    userID.(int64),
		ProductId: req.ProductId,
		Quantity:  req.Quantity,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (g *APIGateway) handleGetCart(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	resp, err := g.cartClient.GetCart(context.Background(), &pbCart.GetCartRequest{
		UserId: userID.(int64),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (g *APIGateway) handleUpdateCartItem(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	var req struct {
		ProductId int64 `json:"product_id"`
		Quantity  int32 `json:"quantity"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := g.cartClient.UpdateCartItem(context.Background(), &pbCart.UpdateCartItemRequest{
		UserId:    userID.(int64),
		ProductId: req.ProductId,
		Quantity:  req.Quantity,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (g *APIGateway) handleDeleteCartItem(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	var req struct {
		ProductId int64 `json:"product_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := g.cartClient.RemoveCartItem(context.Background(), &pbCart.RemoveCartItemRequest{
		UserId:    userID.(int64),
		ProductId: req.ProductId,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (g *APIGateway) handleClearCart(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	resp, err := g.cartClient.ClearCart(context.Background(), &pbCart.ClearCartRequest{
		UserId: userID.(int64),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (g *APIGateway) handleCancelOrder(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	id := c.Param("id")
	orderId := int64(0)
	_, err := fmt.Sscanf(id, "%d", &orderId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid order id"})
		return
	}

	resp, err := g.orderClient.CancelOrder(context.Background(), &pbOrder.CancelOrderRequest{
		Id:     orderId,
		UserId: userID.(int64),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}
