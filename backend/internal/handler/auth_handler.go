package handler

import (
	"tuyul/backend/internal/model"
	"tuyul/backend/internal/service"
	"tuyul/backend/internal/util"

	"github.com/gin-gonic/gin"
)

// AuthHandler handles authentication endpoints
type AuthHandler struct {
	authService *service.AuthService
}

// NewAuthHandler creates a new auth handler
func NewAuthHandler(authService *service.AuthService) *AuthHandler {
	return &AuthHandler{
		authService: authService,
	}
}

// Register handles user registration
// POST /api/v1/auth/register
func (h *AuthHandler) Register(c *gin.Context) {
	var req model.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.SendValidationError(c, err.Error())
		return
	}

	user, err := h.authService.Register(c.Request.Context(), &req)
	if err != nil {
		util.SendError(c, err)
		return
	}

	util.SendCreated(c, user, "User registered successfully")
}

// Login handles user login
// POST /api/v1/auth/login
func (h *AuthHandler) Login(c *gin.Context) {
	var req model.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.SendValidationError(c, err.Error())
		return
	}

	// Get user agent and IP
	userAgent := c.Request.UserAgent()
	ip := c.ClientIP()

	authResp, err := h.authService.Login(c.Request.Context(), &req, userAgent, ip)
	if err != nil {
		util.SendError(c, err)
		return
	}

	util.SendSuccess(c, authResp)
}

// RefreshToken handles token refresh
// POST /api/v1/auth/refresh
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	var req model.RefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.SendValidationError(c, err.Error())
		return
	}

	authResp, err := h.authService.RefreshToken(c.Request.Context(), req.RefreshToken)
	if err != nil {
		util.SendError(c, err)
		return
	}

	util.SendSuccess(c, authResp)
}

// Logout handles user logout
// POST /api/v1/auth/logout
func (h *AuthHandler) Logout(c *gin.Context) {
	// Get tokens from request
	accessToken := c.GetHeader("Authorization")
	if len(accessToken) > 7 {
		accessToken = accessToken[7:] // Remove "Bearer "
	}

	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.SendValidationError(c, err.Error())
		return
	}

	if err := h.authService.Logout(c.Request.Context(), accessToken, req.RefreshToken); err != nil {
		util.SendError(c, err)
		return
	}

	util.SendSuccessWithMessage(c, nil, "Logged out successfully")
}

// GetMe returns current user info
// GET /api/v1/auth/me
func (h *AuthHandler) GetMe(c *gin.Context) {
	userID, _ := c.Get("user_id")
	
	user, err := h.authService.GetUserByID(c.Request.Context(), userID.(string))
	if err != nil {
		util.SendError(c, err)
		return
	}

	util.SendSuccess(c, user)
}



