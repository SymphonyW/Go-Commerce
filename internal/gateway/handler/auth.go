package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	pbAuth "go-commerce/api/auth"
	"go-commerce/internal/gateway/middleware"
	"go-commerce/internal/gateway/response"
)

type registerRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Email    string `json:"email" binding:"required"`
	Role     string `json:"role" binding:"required"`
}

func (h *Handler) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	resp, err := h.authClient.Register(middleware.GatewayContext(c), &pbAuth.RegisterRequest{
		Username: req.Username,
		Password: req.Password,
		Email:    req.Email,
		Role:     req.Role,
	})
	if err != nil {
		response.WriteGRPCError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user_id": resp.UserId,
		"token":   resp.Token,
		"role":    resp.Role,
	})
}

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (h *Handler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	resp, err := h.authClient.Login(middleware.GatewayContext(c), &pbAuth.LoginRequest{
		Username: req.Username,
		Password: req.Password,
	})
	if err != nil {
		response.WriteGRPCError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user_id": resp.UserId,
		"token":   resp.Token,
		"role":    resp.Role,
	})
}
