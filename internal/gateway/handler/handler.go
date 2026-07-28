package handler

import (
	"strings"

	"github.com/gin-gonic/gin"

	pbAuth "go-commerce/api/auth"
	pbCart "go-commerce/api/cart"
	pbMerchant "go-commerce/api/merchant"
	pbOrder "go-commerce/api/order"
	pbPayment "go-commerce/api/payment"
	pbProduct "go-commerce/api/product"
	"go-commerce/internal/gateway/middleware"
	"go-commerce/internal/gateway/response"
)

type Clients struct {
	Auth     pbAuth.AuthServiceClient
	Product  pbProduct.ProductServiceClient
	Order    pbOrder.OrderServiceClient
	Payment  pbPayment.PaymentServiceClient
	Merchant pbMerchant.MerchantServiceClient
	Cart     pbCart.CartServiceClient
}

type Handler struct {
	authClient     pbAuth.AuthServiceClient
	productClient  pbProduct.ProductServiceClient
	orderClient    pbOrder.OrderServiceClient
	paymentClient  pbPayment.PaymentServiceClient
	merchantClient pbMerchant.MerchantServiceClient
	cartClient     pbCart.CartServiceClient
}

func New(clients Clients) *Handler {
	return &Handler{
		authClient:     clients.Auth,
		productClient:  clients.Product,
		orderClient:    clients.Order,
		paymentClient:  clients.Payment,
		merchantClient: clients.Merchant,
		cartClient:     clients.Cart,
	}
}

func authenticatedUserID(c *gin.Context) (int64, bool) {
	userID, ok := middleware.CurrentUserID(c)
	if !ok {
		response.Unauthorized(c, "user not authenticated")
		return 0, false
	}
	return userID, true
}

func readRequiredIdempotencyKey(c *gin.Context) (string, bool) {
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		response.BadRequest(c, "Idempotency-Key header required")
		return "", false
	}
	return key, true
}

func trimOptionalString(value *string) {
	if value == nil {
		return
	}
	*value = strings.TrimSpace(*value)
}
