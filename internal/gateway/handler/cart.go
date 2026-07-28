package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	pbCart "go-commerce/api/cart"
	"go-commerce/internal/gateway/middleware"
	"go-commerce/internal/gateway/response"
)

type cartItemRequest struct {
	ProductID int64 `json:"product_id" binding:"required,gt=0"`
	Quantity  int32 `json:"quantity" binding:"required,gt=0"`
}

func (h *Handler) AddCartItem(c *gin.Context) {
	userID, ok := authenticatedUserID(c)
	if !ok {
		return
	}

	var req cartItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	resp, err := h.cartClient.AddCartItem(middleware.GatewayContext(c), &pbCart.AddCartItemRequest{
		UserId:    userID,
		ProductId: req.ProductID,
		Quantity:  req.Quantity,
	})
	if err != nil {
		response.WriteGRPCError(c, err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *Handler) GetCart(c *gin.Context) {
	userID, ok := authenticatedUserID(c)
	if !ok {
		return
	}

	resp, err := h.cartClient.GetCart(middleware.GatewayContext(c), &pbCart.GetCartRequest{UserId: userID})
	if err != nil {
		response.WriteGRPCError(c, err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *Handler) UpdateCartItem(c *gin.Context) {
	userID, ok := authenticatedUserID(c)
	if !ok {
		return
	}

	var req cartItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	resp, err := h.cartClient.UpdateCartItem(middleware.GatewayContext(c), &pbCart.UpdateCartItemRequest{
		UserId:    userID,
		ProductId: req.ProductID,
		Quantity:  req.Quantity,
	})
	if err != nil {
		response.WriteGRPCError(c, err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

type deleteCartItemRequest struct {
	ProductID int64 `json:"product_id" binding:"required,gt=0"`
}

func (h *Handler) DeleteCartItem(c *gin.Context) {
	userID, ok := authenticatedUserID(c)
	if !ok {
		return
	}

	var req deleteCartItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	resp, err := h.cartClient.RemoveCartItem(middleware.GatewayContext(c), &pbCart.RemoveCartItemRequest{
		UserId:    userID,
		ProductId: req.ProductID,
	})
	if err != nil {
		response.WriteGRPCError(c, err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *Handler) ClearCart(c *gin.Context) {
	userID, ok := authenticatedUserID(c)
	if !ok {
		return
	}

	resp, err := h.cartClient.ClearCart(middleware.GatewayContext(c), &pbCart.ClearCartRequest{UserId: userID})
	if err != nil {
		response.WriteGRPCError(c, err)
		return
	}

	c.JSON(http.StatusOK, resp)
}
