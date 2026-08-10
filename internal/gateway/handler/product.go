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
	minPriceCents, hasMinPrice := parseProductPriceCentsQuery(c.Query("min_price_cents"))
	maxPriceCents, hasMaxPrice := parseProductPriceCentsQuery(c.Query("max_price_cents"))
	if hasMinPrice && hasMaxPrice && minPriceCents > maxPriceCents {
		response.BadRequest(c, "min_price_cents must be less than or equal to max_price_cents")
		return
	}

	resp, err := h.productClient.ListProducts(middleware.GatewayContext(c), &pbProduct.ListProductsRequest{
		Page:          page,
		PageSize:      pageSize,
		Category:      strings.TrimSpace(c.Query("category")),
		Keyword:       strings.TrimSpace(c.Query("keyword")),
		SortBy:        sortBy,
		Order:         order,
		MinPriceCents: optionalProductPriceCents(minPriceCents, hasMinPrice),
		MaxPriceCents: optionalProductPriceCents(maxPriceCents, hasMaxPrice),
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
	case "created_at", "price_cents", "stock":
		sortBy = strings.TrimSpace(sortBy)
	case "price":
		sortBy = "price_cents"
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

func parseProductPriceCentsQuery(raw string) (int64, bool) {
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || value < 0 {
		return 0, false
	}
	return value, true
}

func optionalProductPriceCents(value int64, present bool) *int64 {
	if !present {
		return nil
	}
	return &value
}
