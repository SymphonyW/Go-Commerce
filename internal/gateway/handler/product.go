package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	pbProduct "go-commerce/api/product"
	"go-commerce/internal/gateway/middleware"
	"go-commerce/internal/gateway/response"
)

func (h *Handler) ListProducts(c *gin.Context) {
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
	sortBy, order := normalizeProductSortQuery(c.Query("sort_by"), c.Query("order"))
	minPrice, hasMinPrice := parseProductPriceQuery(c.Query("min_price"))
	maxPrice, hasMaxPrice := parseProductPriceQuery(c.Query("max_price"))
	if hasMinPrice && hasMaxPrice && minPrice > maxPrice {
		response.BadRequest(c, "min_price must be less than or equal to max_price")
		return
	}

	resp, err := h.productClient.ListProducts(middleware.GatewayContext(c), &pbProduct.ListProductsRequest{
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
		response.WriteGRPCError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) GetProduct(c *gin.Context) {
	productID, err := response.ParsePathID(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid product id")
		return
	}

	resp, err := h.productClient.GetProduct(middleware.GatewayContext(c), &pbProduct.GetProductRequest{Id: productID})
	if err != nil {
		response.WriteGRPCError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
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
