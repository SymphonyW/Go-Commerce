// api-gateway 鏈嶅姟鍏ュ彛鏂囦欢
// 璐熻矗澶勭悊HTTP璇锋眰锛屼綔涓哄墠绔拰鍚庣寰湇鍔′箣闂寸殑妗ユ
// 浣跨敤Gin妗嗘灦鎻愪緵RESTful API鎺ュ彛
package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"

	// Gin妗嗘灦锛氱敤浜庡鐞咹TTP璇锋眰鍜岃矾鐢?	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	// gRPC瀹㈡埛绔細鐢ㄤ簬涓庡悗绔井鏈嶅姟閫氫俊
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	// 瀵煎叆鍚勪釜鏈嶅姟鐨刾rotobuf鐢熸垚浠ｇ爜
	pbAuth "go-commerce/api/auth"
	pbCart "go-commerce/api/cart"
	pbMerchant "go-commerce/api/merchant"
	pbOrder "go-commerce/api/order"
	pbPayment "go-commerce/api/payment"
	pbProduct "go-commerce/api/product"

	// JWT宸ュ叿锛氱敤浜庨獙璇佺敤鎴蜂护鐗?	"go-commerce/pkg/jwt"
	"go-commerce/pkg/observability"
)

// getEnv 鑾峰彇鐜鍙橀噺锛屽鏋滀笉瀛樺湪鍒欒繑鍥為粯璁ゅ€?// 鍙傛暟锛?//
//	key: 鐜鍙橀噺鍚嶇О
//	defaultValue: 榛樿鍊?//
// 杩斿洖鍊硷細
//
//	鐜鍙橀噺鍊兼垨榛樿鍊?func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// APIGateway API缃戝叧缁撴瀯浣?// 鍖呭惈鎵€鏈夊井鏈嶅姟鐨刧RPC瀹㈡埛绔?// 鐢ㄤ簬杞彂HTTP璇锋眰鍒板搴旂殑寰湇鍔?
type APIGateway struct {
	authClient     pbAuth.AuthServiceClient         // 璁よ瘉鏈嶅姟瀹㈡埛绔?	productClient  pbProduct.ProductServiceClient   // 浜у搧鏈嶅姟瀹㈡埛绔?	orderClient    pbOrder.OrderServiceClient       // 璁㈠崟鏈嶅姟瀹㈡埛绔?	paymentClient  pbPayment.PaymentServiceClient   // 鏀粯鏈嶅姟瀹㈡埛绔?	merchantClient pbMerchant.MerchantServiceClient // 鍟嗗鏈嶅姟瀹㈡埛绔?	cartClient     pbCart.CartServiceClient         // 璐墿杞︽湇鍔″鎴风
}

// main 鍑芥暟鏄痑pi-gateway鏈嶅姟鐨勫叆鍙ｇ偣
// 璐熻矗鍒濆鍖栧悇涓井鏈嶅姟瀹㈡埛绔€佽缃矾鐢卞拰鍚姩HTTP鏈嶅姟鍣?func main() {
	logger := observability.NewLogger("api-gateway")
	slog.SetDefault(logger)
	registry := prometheus.NewRegistry()
	metrics := observability.NewMetrics("api-gateway", registry)
	// 浠庣幆澧冨彉閲忚幏鍙栧悇涓井鏈嶅姟鐨勫湴鍧€
	// 濡傛灉鐜鍙橀噺涓嶅瓨鍦紝鍒欎娇鐢ㄩ粯璁ゅ湴鍧€
	authServiceAddr := getEnv("AUTH_SERVICE_ADDR", "localhost:50051")
	productServiceAddr := getEnv("PRODUCT_SERVICE_ADDR", "localhost:50052")
	orderServiceAddr := getEnv("ORDER_SERVICE_ADDR", "localhost:50053")
	cartServiceAddr := getEnv("CART_SERVICE_ADDR", "localhost:50054")
	merchantServiceAddr := getEnv("MERCHANT_SERVICE_ADDR", "localhost:50055")
	paymentServiceAddr := getEnv("PAYMENT_SERVICE_ADDR", "localhost:50056")

	// 杩炴帴璁よ瘉鏈嶅姟
	authConn, err := grpc.Dial(authServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithUnaryInterceptor(observability.UnaryClientInterceptor()))
	if err != nil {
		logger.Error("grpc_dial_failed", "target", "auth-service", "error", err)
		os.Exit(1)
	}
	defer authConn.Close()
	authClient := pbAuth.NewAuthServiceClient(authConn)

	// 杩炴帴浜у搧鏈嶅姟
	productConn, err := grpc.Dial(productServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithUnaryInterceptor(observability.UnaryClientInterceptor()))
	if err != nil {
		logger.Error("grpc_dial_failed", "target", "product-service", "error", err)
		os.Exit(1)
	}
	defer productConn.Close()
	productClient := pbProduct.NewProductServiceClient(productConn)

	// 杩炴帴璁㈠崟鏈嶅姟
	orderConn, err := grpc.Dial(orderServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithUnaryInterceptor(observability.UnaryClientInterceptor()))
	if err != nil {
		logger.Error("grpc_dial_failed", "target", "order-service", "error", err)
		os.Exit(1)
	}
	defer orderConn.Close()
	orderClient := pbOrder.NewOrderServiceClient(orderConn)

	// 杩炴帴鏀粯鏈嶅姟
	paymentConn, err := grpc.Dial(paymentServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithUnaryInterceptor(observability.UnaryClientInterceptor()))
	if err != nil {
		logger.Error("grpc_dial_failed", "target", "payment-service", "error", err)
		os.Exit(1)
	}
	defer paymentConn.Close()
	paymentClient := pbPayment.NewPaymentServiceClient(paymentConn)

	// 杩炴帴鍟嗗鏈嶅姟
	merchantConn, err := grpc.Dial(merchantServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithUnaryInterceptor(observability.UnaryClientInterceptor()))
	if err != nil {
		logger.Error("grpc_dial_failed", "target", "merchant-service", "error", err)
		os.Exit(1)
	}
	defer merchantConn.Close()
	merchantClient := pbMerchant.NewMerchantServiceClient(merchantConn)

	// 杩炴帴璐墿杞︽湇鍔?	cartConn, err := grpc.Dial(cartServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithUnaryInterceptor(observability.UnaryClientInterceptor()))
	if err != nil {
		logger.Error("grpc_dial_failed", "target", "cart-service", "error", err)
		os.Exit(1)
	}
	defer cartConn.Close()
	cartClient := pbCart.NewCartServiceClient(cartConn)

	// 鍒濆鍖朅PI缃戝叧瀹炰緥
	gateway := &APIGateway{
		authClient:     authClient,
		productClient:  productClient,
		orderClient:    orderClient,
		paymentClient:  paymentClient,
		merchantClient: merchantClient,
		cartClient:     cartClient,
	}

	// 鍒涘缓Gin榛樿璺敱寮曟搸
	r := gin.New()
	r.Use(
		gin.Recovery(),
		requestContextMiddleware(),
		httpMetricsMiddleware(metrics),
		httpLoggingMiddleware(logger),
	)

	r.GET("/metrics", gin.WrapH(promhttp.HandlerFor(registry, promhttp.HandlerOpts{})))
	r.GET("/healthz", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	r.GET("/readyz", func(c *gin.Context) {
		connections := map[string]*grpc.ClientConn{
			"auth-service":     authConn,
			"product-service":  productConn,
			"order-service":    orderConn,
			"payment-service":  paymentConn,
			"merchant-service": merchantConn,
			"cart-service":     cartConn,
		}
		for name, conn := range connections {
			state := conn.GetState()
			if state == connectivity.TransientFailure || state == connectivity.Shutdown {
				c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready", "dependency": name, "state": state.String()})
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})

	// 娣诲姞CORS涓棿浠?	// 鍏佽璺ㄥ煙璇锋眰锛岃缃厑璁哥殑HTTP鏂规硶鍜屽ご淇℃伅
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, Idempotency-Key, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		// 澶勭悊OPTIONS棰勬璇锋眰
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	// 鍏叡璺敱缁勶細涓嶉渶瑕佽璇佺殑鎺ュ彛
	public := r.Group("/api")
	{
		// 鐢ㄦ埛璁よ瘉鐩稿叧璺敱
		public.POST("/register", gateway.handleRegister) // 鐢ㄦ埛娉ㄥ唽
		public.POST("/login", gateway.handleLogin)       // 鐢ㄦ埛鐧诲綍
		// 浜у搧鐩稿叧璺敱
		public.GET("/products", gateway.handleListProducts)   // 鑾峰彇浜у搧鍒楄〃
		public.GET("/products/:id", gateway.handleGetProduct) // 鑾峰彇鍗曚釜浜у搧璇︽儏
		// 鍟嗗鐩稿叧璺敱
		public.GET("/merchants/:id", gateway.handleGetMerchant) // 鑾峰彇鍟嗗璇︽儏
		public.GET("/merchants", gateway.handleListMerchants)   // 鑾峰彇鍟嗗鍒楄〃
	}

	// 绉佹湁璺敱缁勶細闇€瑕佽璇佺殑鎺ュ彛
	private := r.Group("/api")
	private.Use(gateway.authMiddleware()) // 娣诲姞璁よ瘉涓棿浠?	{
		// 璁㈠崟鐩稿叧璺敱
		private.POST("/orders", gateway.handleCreateOrder)           // 鍒涘缓璁㈠崟
		private.GET("/orders/:id", gateway.handleGetOrder)           // 鑾峰彇璁㈠崟璇︽儏
		private.GET("/orders", gateway.handleListOrders)             // 鑾峰彇璁㈠崟鍒楄〃
		private.PUT("/orders/:id/cancel", gateway.handleCancelOrder) // 鍙栨秷璁㈠崟
		private.PUT("/orders/:id/ship", gateway.requireRole("merchant", "admin"), gateway.handleShipOrder)
		private.PUT("/orders/:id/complete", gateway.handleCompleteOrder)
		// 鏀粯鐩稿叧璺敱
		private.POST("/payments", gateway.handleCreatePayment)
		private.GET("/payments/:id", gateway.handleGetPayment)
		private.POST("/payments/:id/success", gateway.handleMarkPaymentSucceeded)
		private.POST("/payments/:id/fail", gateway.handleMarkPaymentFailed)
		// 璐墿杞︾浉鍏宠矾鐢?		private.POST("/cart/items", gateway.handleAddCartItem)      // 娣诲姞璐墿杞﹀晢鍝?		private.GET("/cart", gateway.handleGetCart)                 // 鑾峰彇璐墿杞?		private.PUT("/cart/items", gateway.handleUpdateCartItem)    // 鏇存柊璐墿杞﹀晢鍝?		private.DELETE("/cart/items", gateway.handleDeleteCartItem) // 鍒犻櫎璐墿杞﹀晢鍝?		private.DELETE("/cart", gateway.handleClearCart)            // 娓呯┖璐墿杞?		// 鍟嗗鍐欐搷浣滈渶瑕佺櫥褰曪紝骞惰姹?merchant 鎴?admin 瑙掕壊
		private.POST("/merchants", gateway.requireRole("merchant", "admin"), gateway.handleCreateMerchant)
		private.POST("/merchants/products", gateway.requireRole("merchant", "admin"), gateway.handleMerchantAddProduct)
		private.DELETE("/merchants/products", gateway.requireRole("merchant", "admin"), gateway.handleMerchantDeleteProduct)
	}

	// 鍚姩HTTP鏈嶅姟鍣紝鐩戝惉8080绔彛
	if err := r.Run(":8080"); err != nil {
			logger.Error("http_server_failed", "addr", ":8080", "error", err)
			os.Exit(1)
	}
}

func (g *APIGateway) handleCreatePayment(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	var req struct {
		OrderID       int64  `json:"order_id"`
		PaymentMethod string `json:"payment_method"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := g.paymentClient.CreatePayment(gatewayContext(c), &pbPayment.CreatePaymentRequest{
		OrderId:       req.OrderID,
		UserId:        userID.(int64),
		PaymentMethod: req.PaymentMethod,
	})
	if err != nil {
		writeGRPCError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (g *APIGateway) handleGetPayment(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}
	paymentID, err := parsePathID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payment id"})
		return
	}
	resp, err := g.paymentClient.GetPayment(gatewayContext(c), &pbPayment.GetPaymentRequest{
		Id:     paymentID,
		UserId: userID.(int64),
	})
	if err != nil {
		writeGRPCError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (g *APIGateway) handleMarkPaymentSucceeded(c *gin.Context) {
	g.handlePaymentAction(c, true)
}

func (g *APIGateway) handleMarkPaymentFailed(c *gin.Context) {
	g.handlePaymentAction(c, false)
}

func (g *APIGateway) handlePaymentAction(c *gin.Context, succeed bool) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}
	paymentID, err := parsePathID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payment id"})
		return
	}

	req := &pbPayment.PaymentActionRequest{Id: paymentID, UserId: userID.(int64)}
	if succeed {
		idempotencyKey, ok := readRequiredIdempotencyKey(c)
		if !ok {
			return
		}
		req.IdempotencyKey = idempotencyKey
	}
	var resp *pbPayment.PaymentActionResponse
	if succeed {
		resp, err = g.paymentClient.MarkPaymentSucceeded(gatewayContext(c), req)
	} else {
		resp, err = g.paymentClient.MarkPaymentFailed(gatewayContext(c), req)
	}
	if err != nil {
		writeGRPCError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

// readRequiredIdempotencyKey 缁熶竴鏍￠獙鍐欐帴鍙ｇ殑骞傜瓑閿紝閬垮厤鍚勮矾鐢卞垎鏁ｅ疄鐜般€?func readRequiredIdempotencyKey(c *gin.Context) (string, bool) {
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Idempotency-Key header required"})
		return "", false
	}
	return key, true
}

func parsePathID(raw string) (int64, error) {
	var id int64
	_, err := fmt.Sscanf(raw, "%d", &id)
	return id, err
}

// authMiddleware 璁よ瘉涓棿浠?// 鐢ㄤ簬楠岃瘉鐢ㄦ埛鐨凧WT浠ょ墝
// 杩斿洖鍊硷細
//
//	gin.HandlerFunc: Gin涓棿浠跺嚱鏁?func (g *APIGateway) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 浠庤姹傚ご鑾峰彇Authorization
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authorization header required"})
			c.Abort()
			return
		}

		// 妫€鏌uthorization鏍煎紡鏄惁姝ｇ‘
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization format"})
			c.Abort()
			return
		}

		// 鎻愬彇浠ょ墝骞堕獙璇?		token := parts[1]
		claims, err := jwt.ValidateToken(token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}

		// 灏嗙敤鎴稩D瀛樺偍鍒颁笂涓嬫枃涓?		c.Set("user_id", claims.UserID)
		c.Set("role", claims.Role)
		c.Next()
	}
}

// requireRole 鏍￠獙褰撳墠鐢ㄦ埛鏄惁鍏峰鎸囧畾瑙掕壊涔嬩竴銆?func (g *APIGateway) requireRole(allowedRoles ...string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(allowedRoles))
	for _, role := range allowedRoles {
		allowed[role] = struct{}{}
	}

	return func(c *gin.Context) {
		roleValue, exists := c.Get("role")
		if !exists {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			c.Abort()
			return
		}

		role, ok := roleValue.(string)
		if !ok {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			c.Abort()
			return
		}

		if _, ok := allowed[role]; !ok {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// handleRegister 澶勭悊鐢ㄦ埛娉ㄥ唽璇锋眰
// 鍙傛暟锛?//
//	c: Gin涓婁笅鏂囷紝鍖呭惈璇锋眰鍜屽搷搴斾俊鎭?func (g *APIGateway) handleRegister(c *gin.Context) {
	// 瀹氫箟璇锋眰缁撴瀯浣?	var req struct {
		Username string `json:"username"` // 鐢ㄦ埛鍚?		Password string `json:"password"` // 瀵嗙爜
		Email    string `json:"email"`    // 閭
		Role     string `json:"role"`     // 鐢ㄦ埛瑙掕壊
	}

	// 缁戝畾JSON璇锋眰浣?	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 璋冪敤璁よ瘉鏈嶅姟鐨凴egister鏂规硶
	resp, err := g.authClient.Register(gatewayContext(c), &pbAuth.RegisterRequest{
		Username: req.Username,
		Password: req.Password,
		Email:    req.Email,
		Role:     req.Role,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 杩斿洖娉ㄥ唽鎴愬姛鍝嶅簲
	c.JSON(http.StatusOK, gin.H{
		"user_id": resp.UserId,
		"token":   resp.Token,
		"role":    resp.Role,
	})
}

// handleLogin 澶勭悊鐢ㄦ埛鐧诲綍璇锋眰
// 鍙傛暟锛?//
//	c: Gin涓婁笅鏂囷紝鍖呭惈璇锋眰鍜屽搷搴斾俊鎭?func (g *APIGateway) handleLogin(c *gin.Context) {
	// 瀹氫箟璇锋眰缁撴瀯浣?	var req struct {
		Username string `json:"username"` // 鐢ㄦ埛鍚?		Password string `json:"password"` // 瀵嗙爜
	}

	// 缁戝畾JSON璇锋眰浣?	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 璋冪敤璁よ瘉鏈嶅姟鐨凩ogin鏂规硶
	resp, err := g.authClient.Login(gatewayContext(c), &pbAuth.LoginRequest{
		Username: req.Username,
		Password: req.Password,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 杩斿洖鐧诲綍鎴愬姛鍝嶅簲
	c.JSON(http.StatusOK, gin.H{
		"user_id": resp.UserId,
		"token":   resp.Token,
		"role":    resp.Role,
	})
}

// handleListProducts 澶勭悊鑾峰彇浜у搧鍒楄〃璇锋眰
// 鍙傛暟锛?//
//	c: Gin涓婁笅鏂囷紝鍖呭惈璇锋眰鍜屽搷搴斾俊鎭?func (g *APIGateway) handleListProducts(c *gin.Context) {
	page := parseProductPage(c.Query("page"))
	pageSize := parseProductPageSize(c.Query("page_size"))
	sortBy, order := normalizeProductSortQuery(c.Query("sort_by"), c.Query("order"))
	minPrice, hasMinPrice := parseProductPriceQuery(c.Query("min_price"))
	maxPrice, hasMaxPrice := parseProductPriceQuery(c.Query("max_price"))
	if hasMinPrice && hasMaxPrice && minPrice > maxPrice {
		c.JSON(http.StatusBadRequest, gin.H{"error": "min_price must be less than or equal to max_price"})
		return
	}

	// 缃戝叧鍏堝仛 HTTP 灞傝緭鍏ユ竻娲楋紝鍐嶆妸鍙敤鏉′欢瀹屾暣閫忎紶缁欏晢鍝佹湇鍔°€?	resp, err := g.productClient.ListProducts(gatewayContext(c), &pbProduct.ListProductsRequest{
		Page:     page,
		PageSize: pageSize,
		Category: strings.TrimSpace(c.Query("category")),
		Keyword:  strings.TrimSpace(c.Query("keyword")),
		SortBy:   sortBy,
		Order:    order,
		MinPrice: optionalProductPrice(minPrice, hasMinPrice),
		MaxPrice: optionalProductPrice(maxPrice, hasMaxPrice),
	})
	if err != nil {
		slog.ErrorContext(gatewayContext(c), "grpc_call_failed", "target", "product-service", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// 杩斿洖浜у搧鍒楄〃
	c.JSON(http.StatusOK, resp)
}

func parseProductPage(raw string) int32 {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return 1
	}
	return int32(value)
}

func parseProductPageSize(raw string) int32 {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return 10
	}
	if value > 100 {
		return 100
	}
	return int32(value)
}

func normalizeProductSortQuery(sortBy, order string) (string, string) {
	invalidSortBy := false
	switch strings.TrimSpace(sortBy) {
	case "created_at", "price", "stock":
		sortBy = strings.TrimSpace(sortBy)
	default:
		sortBy = "created_at"
		invalidSortBy = true
	}

	if invalidSortBy {
		return sortBy, "desc"
	}

	switch strings.ToLower(strings.TrimSpace(order)) {
	case "asc":
		order = "asc"
	case "desc":
		order = "desc"
	default:
		order = "desc"
	}

	return sortBy, order
}

func parseProductPriceQuery(raw string) (float32, bool) {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 32)
	if err != nil || value < 0 {
		return 0, false
	}
	return float32(value), true
}

func optionalProductPrice(value float32, present bool) *float32 {
	if !present {
		return nil
	}
	return &value
}

// handleGetProduct 澶勭悊鑾峰彇鍗曚釜浜у搧璇︽儏璇锋眰
// 鍙傛暟锛?//
//	c: Gin涓婁笅鏂囷紝鍖呭惈璇锋眰鍜屽搷搴斾俊鎭?func (g *APIGateway) handleGetProduct(c *gin.Context) {
	// 浠庤矾寰勫弬鏁拌幏鍙栦骇鍝両D
	id := c.Param("id")
	productId := int64(0)
	// 瑙ｆ瀽浜у搧ID
	_, err := fmt.Sscanf(id, "%d", &productId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid product id"})
		return
	}
	// 璋冪敤浜у搧鏈嶅姟鐨凣etProduct鏂规硶
	resp, err := g.productClient.GetProduct(gatewayContext(c), &pbProduct.GetProductRequest{
		Id: productId,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// 杩斿洖浜у搧璇︽儏
	c.JSON(http.StatusOK, resp)
}

// handleCreateOrder 澶勭悊鍒涘缓璁㈠崟璇锋眰
// 鍙傛暟锛?//
//	c: Gin涓婁笅鏂囷紝鍖呭惈璇锋眰鍜屽搷搴斾俊鎭?func (g *APIGateway) handleCreateOrder(c *gin.Context) {
	// 浠庝笂涓嬫枃涓幏鍙栫敤鎴稩D
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	idempotencyKey, ok := readRequiredIdempotencyKey(c)
	if !ok {
		return
	}

	// 瀹氫箟璇锋眰缁撴瀯浣?	var req struct {
		Items []struct {
			ProductId int64 `json:"product_id"` // 浜у搧ID
			Quantity  int32 `json:"quantity"`   // 浜у搧鏁伴噺
		} `json:"items"` // 璁㈠崟鍟嗗搧鍒楄〃
	}

	// 缁戝畾JSON璇锋眰浣?	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 杞崲璁㈠崟鍟嗗搧鏍煎紡
	orderItems := make([]*pbOrder.CreateOrderItem, len(req.Items))
	for i, item := range req.Items {
		orderItems[i] = &pbOrder.CreateOrderItem{
			ProductId: item.ProductId,
			Quantity:  item.Quantity,
		}
	}

	// 璋冪敤璁㈠崟鏈嶅姟鐨凜reateOrder鏂规硶
	resp, err := g.orderClient.CreateOrder(gatewayContext(c), &pbOrder.CreateOrderRequest{
		UserId:         userID.(int64),
		Items:          orderItems,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		writeGRPCError(c, err)
		return
	}

	// 杩斿洖鍒涘缓璁㈠崟鍝嶅簲
	c.JSON(http.StatusOK, resp)
}

// handleGetOrder 澶勭悊鑾峰彇璁㈠崟璇︽儏璇锋眰
// 鍙傛暟锛?//
//	c: Gin涓婁笅鏂囷紝鍖呭惈璇锋眰鍜屽搷搴斾俊鎭?func (g *APIGateway) handleGetOrder(c *gin.Context) {
	// 浠庝笂涓嬫枃涓幏鍙栫敤鎴稩D
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	// 浠庤矾寰勫弬鏁拌幏鍙栬鍗旾D
	id := c.Param("id")
	orderId := int64(0)
	// 瑙ｆ瀽璁㈠崟ID
	_, err := fmt.Sscanf(id, "%d", &orderId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid order id"})
		return
	}

	// 璋冪敤璁㈠崟鏈嶅姟鐨凣etOrder鏂规硶
	resp, err := g.orderClient.GetOrder(gatewayContext(c), &pbOrder.GetOrderRequest{
		Id:     orderId,
		UserId: userID.(int64),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 杩斿洖璁㈠崟璇︽儏
	c.JSON(http.StatusOK, resp)
}

// handleListOrders 澶勭悊鑾峰彇璁㈠崟鍒楄〃璇锋眰
// 鍙傛暟锛?//
//	c: Gin涓婁笅鏂囷紝鍖呭惈璇锋眰鍜屽搷搴斾俊鎭?func (g *APIGateway) handleListOrders(c *gin.Context) {
	// 浠庝笂涓嬫枃涓幏鍙栫敤鎴稩D
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	// 璋冪敤璁㈠崟鏈嶅姟鐨凩istOrders鏂规硶
	resp, err := g.orderClient.ListOrders(gatewayContext(c), &pbOrder.ListOrdersRequest{
		UserId: userID.(int64),
	})
	if err != nil {
		slog.ErrorContext(gatewayContext(c), "grpc_call_failed", "target", "order-service", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 杩斿洖璁㈠崟鍒楄〃
	c.JSON(http.StatusOK, resp)
}

// handleCreateMerchant 澶勭悊鍒涘缓鍟嗗璇锋眰
// 鍙傛暟锛?//
//	c: Gin涓婁笅鏂囷紝鍖呭惈璇锋眰鍜屽搷搴斾俊鎭?func (g *APIGateway) handleCreateMerchant(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	// 瀹氫箟璇锋眰缁撴瀯浣?	var req struct {
		Name        string `json:"name"`         // 鍟嗗鍚嶇О
		ContactInfo string `json:"contact_info"` // 鑱旂郴淇℃伅
	}

	// 缁戝畾JSON璇锋眰浣?	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 璋冪敤鍟嗗鏈嶅姟鐨凜reateMerchant鏂规硶
	resp, err := g.merchantClient.CreateMerchant(gatewayContext(c), &pbMerchant.CreateMerchantRequest{
		Name:        req.Name,
		ContactInfo: req.ContactInfo,
		OwnerUserId: userID.(int64),
	})
	if err != nil {
		writeGRPCError(c, err)
		return
	}

	// 杩斿洖鍒涘缓鍟嗗鍝嶅簲
	c.JSON(http.StatusOK, resp)
}

// handleGetMerchant 澶勭悊鑾峰彇鍟嗗璇︽儏璇锋眰
// 鍙傛暟锛?//
//	c: Gin涓婁笅鏂囷紝鍖呭惈璇锋眰鍜屽搷搴斾俊鎭?func (g *APIGateway) handleGetMerchant(c *gin.Context) {
	// 浠庤矾寰勫弬鏁拌幏鍙栧晢瀹禝D
	id := c.Param("id")
	merchantId := int64(0)
	// 瑙ｆ瀽鍟嗗ID
	_, err := fmt.Sscanf(id, "%d", &merchantId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid merchant id"})
		return
	}

	// 璋冪敤鍟嗗鏈嶅姟鐨凣etMerchant鏂规硶
	resp, err := g.merchantClient.GetMerchant(gatewayContext(c), &pbMerchant.GetMerchantRequest{
		Id: merchantId,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 杩斿洖鍟嗗璇︽儏
	c.JSON(http.StatusOK, resp)
}

// handleListMerchants 澶勭悊鑾峰彇鍟嗗鍒楄〃璇锋眰
// 鍙傛暟锛?//
//	c: Gin涓婁笅鏂囷紝鍖呭惈璇锋眰鍜屽搷搴斾俊鎭?func (g *APIGateway) handleListMerchants(c *gin.Context) {
	// 璋冪敤鍟嗗鏈嶅姟鐨凩istMerchants鏂规硶
	resp, err := g.merchantClient.ListMerchants(gatewayContext(c), &pbMerchant.ListMerchantsRequest{
		Page:     1,  // 榛樿椤电爜
		PageSize: 10, // 榛樿姣忛〉鏁伴噺
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 杩斿洖鍟嗗鍒楄〃
	c.JSON(http.StatusOK, resp)
}

// handleMerchantAddProduct 澶勭悊鍟嗗娣诲姞浜у搧璇锋眰
// 鍙傛暟锛?//
//	c: Gin涓婁笅鏂囷紝鍖呭惈璇锋眰鍜屽搷搴斾俊鎭?func (g *APIGateway) handleMerchantAddProduct(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	// 瀹氫箟璇锋眰缁撴瀯浣?	var req struct {
		MerchantId  int64   `json:"merchant_id"` // 鍟嗗ID
		Name        string  `json:"name"`        // 浜у搧鍚嶇О
		Description string  `json:"description"` // 浜у搧鎻忚堪
		Price       float32 `json:"price"`       // 浜у搧浠锋牸
		Stock       int32   `json:"stock"`       // 浜у搧搴撳瓨
		Category    string  `json:"category"`    // 浜у搧鍒嗙被
		ImageUrl    string  `json:"image_url"`   // 浜у搧鍥剧墖URL
	}

	// 缁戝畾JSON璇锋眰浣?	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 璋冪敤鍟嗗鏈嶅姟鐨凙ddProduct鏂规硶
	resp, err := g.merchantClient.AddProduct(gatewayContext(c), &pbMerchant.AddProductRequest{
		MerchantId:  req.MerchantId,
		Name:        req.Name,
		Description: req.Description,
		Price:       req.Price,
		Stock:       req.Stock,
		Category:    req.Category,
		ImageUrl:    req.ImageUrl,
		ActorUserId: userID.(int64),
	})
	if err != nil {
		writeGRPCError(c, err)
		return
	}

	// 杩斿洖娣诲姞浜у搧鍝嶅簲
	c.JSON(http.StatusOK, resp)
}

// handleMerchantDeleteProduct 澶勭悊鍟嗗鍒犻櫎浜у搧璇锋眰
// 鍙傛暟锛?//
//	c: Gin涓婁笅鏂囷紝鍖呭惈璇锋眰鍜屽搷搴斾俊鎭?func (g *APIGateway) handleMerchantDeleteProduct(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	// 瀹氫箟璇锋眰缁撴瀯浣?	var req struct {
		MerchantId int64 `json:"merchant_id"` // 鍟嗗ID
		ProductId  int64 `json:"product_id"`  // 浜у搧ID
	}

	// 缁戝畾JSON璇锋眰浣?	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 璋冪敤鍟嗗鏈嶅姟鐨凞eleteProduct鏂规硶
	resp, err := g.merchantClient.DeleteProduct(gatewayContext(c), &pbMerchant.DeleteProductRequest{
		MerchantId:  req.MerchantId,
		ProductId:   req.ProductId,
		ActorUserId: userID.(int64),
	})
	if err != nil {
		writeGRPCError(c, err)
		return
	}

	// 杩斿洖鍒犻櫎浜у搧鍝嶅簲
	c.JSON(http.StatusOK, resp)
}

func writeGRPCError(c *gin.Context, err error) {
	grpcStatus := status.Convert(err)
	switch grpcStatus.Code() {
	case codes.InvalidArgument:
		c.JSON(http.StatusBadRequest, gin.H{"error": grpcStatus.Message()})
	case codes.Unauthenticated:
		c.JSON(http.StatusUnauthorized, gin.H{"error": grpcStatus.Message()})
	case codes.PermissionDenied:
		c.JSON(http.StatusForbidden, gin.H{"error": grpcStatus.Message()})
	case codes.NotFound:
		c.JSON(http.StatusNotFound, gin.H{"error": grpcStatus.Message()})
	case codes.FailedPrecondition:
		c.JSON(http.StatusConflict, gin.H{"error": grpcStatus.Message()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": grpcStatus.Message()})
	}
}

// handleAddCartItem 澶勭悊娣诲姞璐墿杞﹀晢鍝佽姹?// 鍙傛暟锛?//
//	c: Gin涓婁笅鏂囷紝鍖呭惈璇锋眰鍜屽搷搴斾俊鎭?func (g *APIGateway) handleAddCartItem(c *gin.Context) {
	// 浠庝笂涓嬫枃涓幏鍙栫敤鎴稩D
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	// 瀹氫箟璇锋眰缁撴瀯浣?	var req struct {
		ProductId int64 `json:"product_id"` // 浜у搧ID
		Quantity  int32 `json:"quantity"`   // 浜у搧鏁伴噺
	}

	// 缁戝畾JSON璇锋眰浣?	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 璋冪敤璐墿杞︽湇鍔＄殑AddCartItem鏂规硶
	resp, err := g.cartClient.AddCartItem(gatewayContext(c), &pbCart.AddCartItemRequest{
		UserId:    userID.(int64),
		ProductId: req.ProductId,
		Quantity:  req.Quantity,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 杩斿洖娣诲姞璐墿杞﹀晢鍝佸搷搴?	c.JSON(http.StatusOK, resp)
}

// handleGetCart 澶勭悊鑾峰彇璐墿杞﹁姹?// 鍙傛暟锛?//
//	c: Gin涓婁笅鏂囷紝鍖呭惈璇锋眰鍜屽搷搴斾俊鎭?func (g *APIGateway) handleGetCart(c *gin.Context) {
	// 浠庝笂涓嬫枃涓幏鍙栫敤鎴稩D
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	// 璋冪敤璐墿杞︽湇鍔＄殑GetCart鏂规硶
	resp, err := g.cartClient.GetCart(gatewayContext(c), &pbCart.GetCartRequest{
		UserId: userID.(int64),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 杩斿洖璐墿杞︿俊鎭?	c.JSON(http.StatusOK, resp)
}

// handleUpdateCartItem 澶勭悊鏇存柊璐墿杞﹀晢鍝佽姹?// 鍙傛暟锛?//
//	c: Gin涓婁笅鏂囷紝鍖呭惈璇锋眰鍜屽搷搴斾俊鎭?func (g *APIGateway) handleUpdateCartItem(c *gin.Context) {
	// 浠庝笂涓嬫枃涓幏鍙栫敤鎴稩D
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	// 瀹氫箟璇锋眰缁撴瀯浣?	var req struct {
		ProductId int64 `json:"product_id"` // 浜у搧ID
		Quantity  int32 `json:"quantity"`   // 浜у搧鏁伴噺
	}

	// 缁戝畾JSON璇锋眰浣?	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 璋冪敤璐墿杞︽湇鍔＄殑UpdateCartItem鏂规硶
	resp, err := g.cartClient.UpdateCartItem(gatewayContext(c), &pbCart.UpdateCartItemRequest{
		UserId:    userID.(int64),
		ProductId: req.ProductId,
		Quantity:  req.Quantity,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 杩斿洖鏇存柊璐墿杞﹀晢鍝佸搷搴?	c.JSON(http.StatusOK, resp)
}

// handleDeleteCartItem 澶勭悊鍒犻櫎璐墿杞﹀晢鍝佽姹?// 鍙傛暟锛?//
//	c: Gin涓婁笅鏂囷紝鍖呭惈璇锋眰鍜屽搷搴斾俊鎭?func (g *APIGateway) handleDeleteCartItem(c *gin.Context) {
	// 浠庝笂涓嬫枃涓幏鍙栫敤鎴稩D
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	// 瀹氫箟璇锋眰缁撴瀯浣?	var req struct {
		ProductId int64 `json:"product_id"` // 浜у搧ID
	}

	// 缁戝畾JSON璇锋眰浣?	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 璋冪敤璐墿杞︽湇鍔＄殑RemoveCartItem鏂规硶
	resp, err := g.cartClient.RemoveCartItem(gatewayContext(c), &pbCart.RemoveCartItemRequest{
		UserId:    userID.(int64),
		ProductId: req.ProductId,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 杩斿洖鍒犻櫎璐墿杞﹀晢鍝佸搷搴?	c.JSON(http.StatusOK, resp)
}

// handleClearCart 澶勭悊娓呯┖璐墿杞﹁姹?// 鍙傛暟锛?//
//	c: Gin涓婁笅鏂囷紝鍖呭惈璇锋眰鍜屽搷搴斾俊鎭?func (g *APIGateway) handleClearCart(c *gin.Context) {
	// 浠庝笂涓嬫枃涓幏鍙栫敤鎴稩D
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	// 璋冪敤璐墿杞︽湇鍔＄殑ClearCart鏂规硶
	resp, err := g.cartClient.ClearCart(gatewayContext(c), &pbCart.ClearCartRequest{
		UserId: userID.(int64),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 杩斿洖娓呯┖璐墿杞﹀搷搴?	c.JSON(http.StatusOK, resp)
}

// handleCancelOrder 澶勭悊鍙栨秷璁㈠崟璇锋眰
// 鍙傛暟锛?//
//	c: Gin涓婁笅鏂囷紝鍖呭惈璇锋眰鍜屽搷搴斾俊鎭?func (g *APIGateway) handleCancelOrder(c *gin.Context) {
	// 浠庝笂涓嬫枃涓幏鍙栫敤鎴稩D
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	idempotencyKey, ok := readRequiredIdempotencyKey(c)
	if !ok {
		return
	}

	// 浠庤矾寰勫弬鏁拌幏鍙栬鍗旾D
	id := c.Param("id")
	orderId := int64(0)
	// 瑙ｆ瀽璁㈠崟ID
	_, err := fmt.Sscanf(id, "%d", &orderId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid order id"})
		return
	}

	// 璋冪敤璁㈠崟鏈嶅姟鐨凜ancelOrder鏂规硶
	resp, err := g.orderClient.CancelOrder(gatewayContext(c), &pbOrder.CancelOrderRequest{
		Id:             orderId,
		UserId:         userID.(int64),
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		writeGRPCError(c, err)
		return
	}

	// 杩斿洖鍙栨秷璁㈠崟鍝嶅簲
	c.JSON(http.StatusOK, resp)
}

// handleShipOrder 澶勭悊鍟嗗/绠＄悊鍛樺彂璐ц姹傘€?func (g *APIGateway) handleShipOrder(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	orderID, err := parsePathID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid order id"})
		return
	}

	resp, err := g.orderClient.ShipOrder(gatewayContext(c), &pbOrder.ShipOrderRequest{
		Id:          orderID,
		ActorUserId: userID.(int64),
	})
	if err != nil {
		writeGRPCError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

// handleCompleteOrder 澶勭悊鐢ㄦ埛纭鏀惰揣璇锋眰銆?func (g *APIGateway) handleCompleteOrder(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	orderID, err := parsePathID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid order id"})
		return
	}

	resp, err := g.orderClient.CompleteOrder(gatewayContext(c), &pbOrder.CompleteOrderRequest{
		Id:     orderID,
		UserId: userID.(int64),
	})
	if err != nil {
		writeGRPCError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}








