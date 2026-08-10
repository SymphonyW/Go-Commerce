package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	pbMerchant "go-commerce/api/merchant"
	pbOrder "go-commerce/api/order"
	"go-commerce/internal/gateway/middleware"
	"go-commerce/internal/gateway/response"
)

type createMerchantRequest struct {
	Name        string `json:"name" binding:"required"`
	ContactInfo string `json:"contact_info" binding:"required"`
}

func (h *Handler) CreateMerchant(c *gin.Context) {
	userID, ok := authenticatedUserID(c)
	if !ok {
		return
	}

	var req createMerchantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	resp, err := h.merchantClient.CreateMerchant(middleware.GatewayContext(c), &pbMerchant.CreateMerchantRequest{
		Name:        req.Name,
		ContactInfo: req.ContactInfo,
		OwnerUserId: userID,
	})
	if err != nil {
		response.WriteGRPCError(c, err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *Handler) GetMerchant(c *gin.Context) {
	merchantID, err := response.ParsePathID(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid merchant id")
		return
	}

	resp, err := h.merchantClient.GetMerchant(middleware.GatewayContext(c), &pbMerchant.GetMerchantRequest{Id: merchantID})
	if err != nil {
		response.WriteGRPCError(c, err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *Handler) ListMerchants(c *gin.Context) {
	page, err := response.ParsePage(c.Query("page"))
	if err != nil {
		response.BadRequest(c, "invalid page")
		return
	}
	pageSize, err := response.ParsePageSize(c.Query("page_size"))
	if err != nil {
		response.BadRequest(c, "invalid page_size")
		return
	}

	resp, err := h.merchantClient.ListMerchants(middleware.GatewayContext(c), &pbMerchant.ListMerchantsRequest{
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		response.WriteGRPCError(c, err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

type merchantProductRequest struct {
	MerchantID  int64  `json:"merchant_id" binding:"required,gt=0"`
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	PriceCents  int64  `json:"price_cents" binding:"gte=0"`
	Stock       int32  `json:"stock" binding:"gte=0"`
	Category    string `json:"category" binding:"required"`
	ImageURL    string `json:"image_url"`
}

func (h *Handler) MerchantAddProduct(c *gin.Context) {
	userID, ok := authenticatedUserID(c)
	if !ok {
		return
	}

	var req merchantProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	resp, err := h.merchantClient.AddProduct(middleware.GatewayContext(c), &pbMerchant.AddProductRequest{
		MerchantId:  req.MerchantID,
		Name:        req.Name,
		Description: req.Description,
		PriceCents:  req.PriceCents,
		Stock:       req.Stock,
		Category:    req.Category,
		ImageUrl:    req.ImageURL,
		ActorUserId: userID,
	})
	if err != nil {
		response.WriteGRPCError(c, err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

type deleteMerchantProductRequest struct {
	MerchantID int64 `json:"merchant_id" binding:"required,gt=0"`
	ProductID  int64 `json:"product_id" binding:"required,gt=0"`
}

func (h *Handler) MerchantDeleteProduct(c *gin.Context) {
	userID, ok := authenticatedUserID(c)
	if !ok {
		return
	}

	var req deleteMerchantProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	resp, err := h.merchantClient.DeleteProduct(middleware.GatewayContext(c), &pbMerchant.DeleteProductRequest{
		MerchantId:  req.MerchantID,
		ProductId:   req.ProductID,
		ActorUserId: userID,
	})
	if err != nil {
		response.WriteGRPCError(c, err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *Handler) CurrentMerchantProfile(c *gin.Context) {
	userID, ok := authenticatedUserID(c)
	if !ok {
		return
	}
	merchantID, err := response.ParseOptionalQueryID(c.Query("merchant_id"))
	if err != nil {
		response.BadRequest(c, "invalid merchant id")
		return
	}

	resp, err := h.merchantClient.GetCurrentMerchant(middleware.GatewayContext(c), &pbMerchant.CurrentMerchantRequest{
		ActorUserId: userID,
		MerchantId:  merchantID,
	})
	if err != nil {
		response.WriteGRPCError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) CurrentMerchantProducts(c *gin.Context) {
	userID, ok := authenticatedUserID(c)
	if !ok {
		return
	}
	merchantID, err := response.ParseOptionalQueryID(c.Query("merchant_id"))
	if err != nil {
		response.BadRequest(c, "invalid merchant id")
		return
	}
	page, err := response.ParsePage(c.Query("page"))
	if err != nil {
		response.BadRequest(c, "invalid page")
		return
	}
	pageSize, err := response.ParsePageSize(c.Query("page_size"))
	if err != nil {
		response.BadRequest(c, "invalid page_size")
		return
	}

	resp, err := h.merchantClient.ListMerchantProducts(middleware.GatewayContext(c), &pbMerchant.ListMerchantProductsRequest{
		ActorUserId: userID,
		MerchantId:  merchantID,
		Page:        page,
		PageSize:    pageSize,
	})
	if err != nil {
		response.WriteGRPCError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

type createCurrentMerchantProductRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	PriceCents  int64  `json:"price_cents" binding:"gte=0"`
	Stock       int32  `json:"stock" binding:"gte=0"`
	Category    string `json:"category" binding:"required"`
	ImageURL    string `json:"image_url"`
}

func (h *Handler) CreateCurrentMerchantProduct(c *gin.Context) {
	userID, ok := authenticatedUserID(c)
	if !ok {
		return
	}
	merchantID, err := response.ParseOptionalQueryID(c.Query("merchant_id"))
	if err != nil {
		response.BadRequest(c, "invalid merchant id")
		return
	}
	currentMerchant, err := h.merchantClient.GetCurrentMerchant(middleware.GatewayContext(c), &pbMerchant.CurrentMerchantRequest{
		ActorUserId: userID,
		MerchantId:  merchantID,
	})
	if err != nil {
		response.WriteGRPCError(c, err)
		return
	}

	var req createCurrentMerchantProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Category) == "" {
		response.BadRequest(c, "name and category are required")
		return
	}

	resp, err := h.merchantClient.AddProduct(middleware.GatewayContext(c), &pbMerchant.AddProductRequest{
		MerchantId:  currentMerchant.Merchant.Id,
		Name:        strings.TrimSpace(req.Name),
		Description: strings.TrimSpace(req.Description),
		PriceCents:  req.PriceCents,
		Stock:       req.Stock,
		Category:    strings.TrimSpace(req.Category),
		ImageUrl:    strings.TrimSpace(req.ImageURL),
		ActorUserId: userID,
	})
	if err != nil {
		response.WriteGRPCError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

type updateCurrentMerchantProductRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	PriceCents  *int64  `json:"price_cents"`
	Stock       *int32  `json:"stock"`
	Category    *string `json:"category"`
	ImageURL    *string `json:"image_url"`
}

func (h *Handler) UpdateCurrentMerchantProduct(c *gin.Context) {
	userID, ok := authenticatedUserID(c)
	if !ok {
		return
	}
	productID, err := response.ParsePathID(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid product id")
		return
	}
	merchantID, err := response.ParseOptionalQueryID(c.Query("merchant_id"))
	if err != nil {
		response.BadRequest(c, "invalid merchant id")
		return
	}

	var req updateCurrentMerchantProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.Name == nil && req.Description == nil && req.PriceCents == nil && req.Stock == nil && req.Category == nil && req.ImageURL == nil {
		response.BadRequest(c, "at least one field must be provided")
		return
	}
	if req.Name != nil && strings.TrimSpace(*req.Name) == "" {
		response.BadRequest(c, "name cannot be empty")
		return
	}
	if req.Category != nil && strings.TrimSpace(*req.Category) == "" {
		response.BadRequest(c, "category cannot be empty")
		return
	}
	if req.PriceCents != nil && *req.PriceCents < 0 {
		response.BadRequest(c, "price_cents must be non-negative")
		return
	}
	if req.Stock != nil && *req.Stock < 0 {
		response.BadRequest(c, "stock must be non-negative")
		return
	}
	trimOptionalString(req.Name)
	trimOptionalString(req.Description)
	trimOptionalString(req.Category)
	trimOptionalString(req.ImageURL)

	resp, err := h.merchantClient.UpdateMerchantProduct(middleware.GatewayContext(c), &pbMerchant.UpdateMerchantProductRequest{
		ProductId:   productID,
		ActorUserId: userID,
		MerchantId:  merchantID,
		Name:        req.Name,
		Description: req.Description,
		PriceCents:  req.PriceCents,
		Stock:       req.Stock,
		Category:    req.Category,
		ImageUrl:    req.ImageURL,
	})
	if err != nil {
		response.WriteGRPCError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) DeleteCurrentMerchantProduct(c *gin.Context) {
	userID, ok := authenticatedUserID(c)
	if !ok {
		return
	}
	productID, err := response.ParsePathID(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid product id")
		return
	}
	merchantID, err := response.ParseOptionalQueryID(c.Query("merchant_id"))
	if err != nil {
		response.BadRequest(c, "invalid merchant id")
		return
	}
	currentMerchant, err := h.merchantClient.GetCurrentMerchant(middleware.GatewayContext(c), &pbMerchant.CurrentMerchantRequest{
		ActorUserId: userID,
		MerchantId:  merchantID,
	})
	if err != nil {
		response.WriteGRPCError(c, err)
		return
	}

	resp, err := h.merchantClient.DeleteProduct(middleware.GatewayContext(c), &pbMerchant.DeleteProductRequest{
		MerchantId:  currentMerchant.Merchant.Id,
		ProductId:   productID,
		ActorUserId: userID,
	})
	if err != nil {
		response.WriteGRPCError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) CurrentMerchantOrders(c *gin.Context) {
	userID, ok := authenticatedUserID(c)
	if !ok {
		return
	}
	merchantID, err := response.ParseOptionalQueryID(c.Query("merchant_id"))
	if err != nil {
		response.BadRequest(c, "invalid merchant id")
		return
	}
	page, err := response.ParsePage(c.Query("page"))
	if err != nil {
		response.BadRequest(c, "invalid page")
		return
	}
	pageSize, err := response.ParsePageSize(c.Query("page_size"))
	if err != nil {
		response.BadRequest(c, "invalid page_size")
		return
	}
	resp, err := h.orderClient.ListMerchantOrders(middleware.GatewayContext(c), &pbOrder.ListMerchantOrdersRequest{
		ActorUserId: userID,
		MerchantId:  merchantID,
		Page:        page,
		PageSize:    pageSize,
	})
	if err != nil {
		response.WriteGRPCError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}
