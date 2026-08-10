package gateway

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"go-commerce/internal/gateway/handler"
	"go-commerce/internal/gateway/middleware"
	"go-commerce/internal/gateway/response"
	"go-commerce/pkg/healthcheck"
	"go-commerce/pkg/observability"
)

type RouterConfig struct {
	Clients            handler.Clients
	Logger             *slog.Logger
	Registry           *prometheus.Registry
	Metrics            *observability.Metrics
	CORSAllowedOrigins []string
	HealthDependencies []healthcheck.Dependency
}

func NewRouter(config RouterConfig) *gin.Engine {
	registry := config.Registry
	if registry == nil {
		registry = prometheus.NewRegistry()
	}

	r := gin.New()
	r.Use(
		gin.Recovery(),
		middleware.RequestContext(),
		middleware.Tracing("api-gateway"),
		middleware.CORS(config.CORSAllowedOrigins),
		middleware.Metrics(config.Metrics),
		middleware.Logging(config.Logger),
	)
	r.NoRoute(func(c *gin.Context) {
		if c.Request.Method == http.MethodOptions {
			c.Status(http.StatusNoContent)
			return
		}
		response.WriteError(c, http.StatusNotFound, "route not found")
	})

	r.GET("/metrics", gin.WrapH(promhttp.HandlerFor(registry, promhttp.HandlerOpts{})))
	r.GET("/healthz", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	r.GET("/readyz", func(c *gin.Context) {
		healthcheck.Handler(config.HealthDependencies...).ServeHTTP(c.Writer, c.Request)
	})

	h := handler.New(config.Clients)
	public := r.Group("/api")
	{
		public.POST("/register", h.Register)
		public.POST("/login", h.Login)
		public.GET("/products", h.ListProducts)
		public.GET("/products/:id", h.GetProduct)
		public.GET("/merchants/:id", h.GetMerchant)
		public.GET("/merchants", h.ListMerchants)
	}

	private := r.Group("/api")
	private.Use(middleware.Auth())
	{
		private.POST("/orders", h.CreateOrder)
		private.GET("/orders/:id", h.GetOrder)
		private.GET("/orders", h.ListOrders)
		private.PUT("/orders/:id/cancel", h.CancelOrder)
		private.PUT("/orders/:id/ship", middleware.RequireRole("merchant", "admin"), h.ShipOrder)
		private.PUT("/orders/:id/complete", h.CompleteOrder)

		private.POST("/payments", h.CreatePayment)
		private.GET("/payments/:id", h.GetPayment)
		private.POST("/payments/:id/success", h.MarkPaymentSucceeded)
		private.POST("/payments/:id/fail", h.MarkPaymentFailed)

		private.POST("/cart/items", h.AddCartItem)
		private.GET("/cart", h.GetCart)
		private.PUT("/cart/items", h.UpdateCartItem)
		private.DELETE("/cart/items", h.DeleteCartItem)
		private.DELETE("/cart", h.ClearCart)

		private.POST("/merchants", middleware.RequireRole("merchant", "admin"), h.CreateMerchant)
		private.POST("/merchants/products", middleware.RequireRole("merchant", "admin"), h.MerchantAddProduct)
		private.DELETE("/merchants/products", middleware.RequireRole("merchant", "admin"), h.MerchantDeleteProduct)

		merchantConsole := private.Group("/merchant")
		merchantConsole.Use(middleware.RequireRole("merchant", "admin"))
		{
			merchantConsole.GET("/profile", h.CurrentMerchantProfile)
			merchantConsole.GET("/products", h.CurrentMerchantProducts)
			merchantConsole.POST("/products", h.CreateCurrentMerchantProduct)
			merchantConsole.PUT("/products/:id", h.UpdateCurrentMerchantProduct)
			merchantConsole.DELETE("/products/:id", h.DeleteCurrentMerchantProduct)
			merchantConsole.GET("/orders", h.CurrentMerchantOrders)
		}
	}

	return r
}
