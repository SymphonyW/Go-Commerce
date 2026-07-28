package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	pbOrder "go-commerce/api/order"
	"go-commerce/internal/gateway/middleware"
	"go-commerce/internal/gateway/response"
)

type createOrderItemRequest struct {
	ProductID int64 `json:"product_id" binding:"required,gt=0"`
	Quantity  int32 `json:"quantity" binding:"required,gt=0"`
}

type createOrderRequest struct {
	Items []createOrderItemRequest `json:"items" binding:"required,min=1,dive"`
}

func (h *Handler) CreateOrder(c *gin.Context) {
	userID, ok := authenticatedUserID(c)
	if !ok {
		return
	}

	idempotencyKey, ok := readRequiredIdempotencyKey(c)
	if !ok {
		return
	}

	var req createOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	orderItems := make([]*pbOrder.CreateOrderItem, len(req.Items))
	for i, item := range req.Items {
		orderItems[i] = &pbOrder.CreateOrderItem{
			ProductId: item.ProductID,
			Quantity:  item.Quantity,
		}
	}

	resp, err := h.orderClient.CreateOrder(middleware.GatewayContext(c), &pbOrder.CreateOrderRequest{
		UserId:         userID,
		Items:          orderItems,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		response.WriteGRPCError(c, err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *Handler) GetOrder(c *gin.Context) {
	userID, ok := authenticatedUserID(c)
	if !ok {
		return
	}

	orderID, err := response.ParsePathID(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid order id")
		return
	}

	resp, err := h.orderClient.GetOrder(middleware.GatewayContext(c), &pbOrder.GetOrderRequest{
		Id:     orderID,
		UserId: userID,
	})
	if err != nil {
		response.WriteGRPCError(c, err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *Handler) ListOrders(c *gin.Context) {
	userID, ok := authenticatedUserID(c)
	if !ok {
		return
	}

	resp, err := h.orderClient.ListOrders(middleware.GatewayContext(c), &pbOrder.ListOrdersRequest{UserId: userID})
	if err != nil {
		response.WriteGRPCError(c, err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *Handler) CancelOrder(c *gin.Context) {
	userID, ok := authenticatedUserID(c)
	if !ok {
		return
	}

	idempotencyKey, ok := readRequiredIdempotencyKey(c)
	if !ok {
		return
	}

	orderID, err := response.ParsePathID(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid order id")
		return
	}

	resp, err := h.orderClient.CancelOrder(middleware.GatewayContext(c), &pbOrder.CancelOrderRequest{
		Id:             orderID,
		UserId:         userID,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		response.WriteGRPCError(c, err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *Handler) ShipOrder(c *gin.Context) {
	userID, ok := authenticatedUserID(c)
	if !ok {
		return
	}

	orderID, err := response.ParsePathID(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid order id")
		return
	}

	resp, err := h.orderClient.ShipOrder(middleware.GatewayContext(c), &pbOrder.ShipOrderRequest{
		Id:          orderID,
		ActorUserId: userID,
	})
	if err != nil {
		response.WriteGRPCError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) CompleteOrder(c *gin.Context) {
	userID, ok := authenticatedUserID(c)
	if !ok {
		return
	}

	orderID, err := response.ParsePathID(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid order id")
		return
	}

	resp, err := h.orderClient.CompleteOrder(middleware.GatewayContext(c), &pbOrder.CompleteOrderRequest{
		Id:     orderID,
		UserId: userID,
	})
	if err != nil {
		response.WriteGRPCError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}
