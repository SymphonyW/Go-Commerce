package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pbAuth "go-commerce/api/auth"
	pbProduct "go-commerce/api/product"
	pbOrder "go-commerce/api/order"
	"go-commerce/pkg/jwt"
)

type APIGateway struct {
	authClient    pbAuth.AuthServiceClient
	productClient pbProduct.ProductServiceClient
	orderClient   pbOrder.OrderServiceClient
}

func main() {
	authConn, err := grpc.Dial("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect to auth service: %v", err)
	}
	defer authConn.Close()
	authClient := pbAuth.NewAuthServiceClient(authConn)

	productConn, err := grpc.Dial("localhost:50052", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect to product service: %v", err)
	}
	defer productConn.Close()
	productClient := pbProduct.NewProductServiceClient(productConn)

	orderConn, err := grpc.Dial("localhost:50053", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect to order service: %v", err)
	}
	defer orderConn.Close()
	orderClient := pbOrder.NewOrderServiceClient(orderConn)

	gateway := &APIGateway{
		authClient:    authClient,
		productClient: productClient,
		orderClient:   orderClient,
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
	}

	private := r.Group("/api")
	private.Use(gateway.authMiddleware())
	{
		private.POST("/orders", gateway.handleCreateOrder)
		private.GET("/orders/:id", gateway.handleGetOrder)
		private.GET("/orders", gateway.handleListOrders)
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
	_, exists := c.Get("user_id")
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

	// 这里需要实现 GetOrder 方法，暂时返回模拟数据
	c.JSON(http.StatusOK, gin.H{
		"order": gin.H{
			"id": orderId,
			"status": "pending",
			"total_amount": 0,
			"items": []gin.H{},
			"created_at": "2026-03-24T00:00:00Z",
		},
	})
}

func (g *APIGateway) handleListOrders(c *gin.Context) {
	_, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	// 这里需要实现 ListOrders 方法，暂时返回模拟数据
	c.JSON(http.StatusOK, gin.H{
		"orders": []gin.H{
			{
				"id": 1,
				"status": "pending",
				"total_amount": 99.99,
				"created_at": "2026-03-24T00:00:00Z",
			},
			{
				"id": 2,
				"status": "completed",
				"total_amount": 199.98,
				"created_at": "2026-03-23T00:00:00Z",
			},
		},
	})
}
