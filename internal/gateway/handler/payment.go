package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	pbPayment "go-commerce/api/payment"
	"go-commerce/internal/gateway/middleware"
	"go-commerce/internal/gateway/response"
)

type createPaymentRequest struct {
	OrderID       int64  `json:"order_id" binding:"required,gt=0"`
	PaymentMethod string `json:"payment_method" binding:"required"`
}

func (h *Handler) CreatePayment(c *gin.Context) {
	userID, ok := authenticatedUserID(c)
	if !ok {
		return
	}

	var req createPaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	resp, err := h.paymentClient.CreatePayment(middleware.GatewayContext(c), &pbPayment.CreatePaymentRequest{
		OrderId:       req.OrderID,
		UserId:        userID,
		PaymentMethod: req.PaymentMethod,
	})
	if err != nil {
		response.WriteGRPCError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) GetPayment(c *gin.Context) {
	userID, ok := authenticatedUserID(c)
	if !ok {
		return
	}
	paymentID, err := response.ParsePathID(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid payment id")
		return
	}
	resp, err := h.paymentClient.GetPayment(middleware.GatewayContext(c), &pbPayment.GetPaymentRequest{
		Id:     paymentID,
		UserId: userID,
	})
	if err != nil {
		response.WriteGRPCError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) MarkPaymentSucceeded(c *gin.Context) {
	h.paymentAction(c, true)
}

func (h *Handler) MarkPaymentFailed(c *gin.Context) {
	h.paymentAction(c, false)
}

func (h *Handler) paymentAction(c *gin.Context, succeed bool) {
	userID, ok := authenticatedUserID(c)
	if !ok {
		return
	}
	paymentID, err := response.ParsePathID(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid payment id")
		return
	}

	req := &pbPayment.PaymentActionRequest{Id: paymentID, UserId: userID}
	if succeed {
		idempotencyKey, ok := readRequiredIdempotencyKey(c)
		if !ok {
			return
		}
		req.IdempotencyKey = idempotencyKey
	}
	var resp *pbPayment.PaymentActionResponse
	if succeed {
		resp, err = h.paymentClient.MarkPaymentSucceeded(middleware.GatewayContext(c), req)
	} else {
		resp, err = h.paymentClient.MarkPaymentFailed(middleware.GatewayContext(c), req)
	}
	if err != nil {
		response.WriteGRPCError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}
