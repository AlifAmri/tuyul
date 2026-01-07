package handler

import (
	"tuyul/backend/internal/model"
	"tuyul/backend/internal/service"
	"tuyul/backend/internal/util"

	"github.com/gin-gonic/gin"
)

// UserHandler handles user management endpoints
type UserHandler struct {
	userService *service.UserService
}

// NewUserHandler creates a new user handler
func NewUserHandler(userService *service.UserService) *UserHandler {
	return &UserHandler{
		userService: userService,
	}
}

// GetProfile gets current user's profile
// GET /api/v1/users/profile
func (h *UserHandler) GetProfile(c *gin.Context) {
	userID, _ := c.Get("user_id")
	
	user, err := h.userService.GetProfile(c.Request.Context(), userID.(string))
	if err != nil {
		util.SendError(c, err)
		return
	}

	util.SendSuccess(c, user)
}

// UpdateProfile updates current user's profile
// PUT /api/v1/users/profile
func (h *UserHandler) UpdateProfile(c *gin.Context) {
	userID, _ := c.Get("user_id")
	
	var req struct {
		Email string `json:"email" binding:"omitempty,email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.SendValidationError(c, err.Error())
		return
	}

	user, err := h.userService.UpdateProfile(c.Request.Context(), userID.(string), req.Email)
	if err != nil {
		util.SendError(c, err)
		return
	}

	util.SendSuccessWithMessage(c, user, "Profile updated successfully")
}

// ChangePassword changes current user's password
// POST /api/v1/users/password
func (h *UserHandler) ChangePassword(c *gin.Context) {
	userID, _ := c.Get("user_id")
	
	var req model.ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.SendValidationError(c, err.Error())
		return
	}

	if err := h.userService.ChangePassword(c.Request.Context(), userID.(string), req.OldPassword, req.NewPassword); err != nil {
		util.SendError(c, err)
		return
	}

	util.SendSuccessWithMessage(c, nil, "Password changed successfully")
}

// ListUsers lists all users (admin only)
// GET /api/v1/users
func (h *UserHandler) ListUsers(c *gin.Context) {
	users, err := h.userService.ListUsers(c.Request.Context())
	if err != nil {
		util.SendError(c, err)
		return
	}

	util.SendSuccess(c, gin.H{
		"users": users,
		"total": len(users),
	})
}

// GetUser gets a user by ID (admin only)
// GET /api/v1/users/:id
func (h *UserHandler) GetUser(c *gin.Context) {
	userID := c.Param("id")
	
	user, err := h.userService.GetUser(c.Request.Context(), userID)
	if err != nil {
		util.SendError(c, err)
		return
	}

	util.SendSuccess(c, user)
}

// CreateUser creates a new user (admin only)
// POST /api/v1/users
func (h *UserHandler) CreateUser(c *gin.Context) {
	var req model.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.SendValidationError(c, err.Error())
		return
	}

	// Get role from request body, default to "user"
	role := model.RoleUser
	if roleParam := c.Query("role"); roleParam == model.RoleAdmin {
		role = model.RoleAdmin
	}

	user, err := h.userService.CreateUser(c.Request.Context(), &req, role)
	if err != nil {
		util.SendError(c, err)
		return
	}

	util.SendCreated(c, user, "User created successfully")
}

// UpdateUser updates a user (admin only)
// PUT /api/v1/users/:id
func (h *UserHandler) UpdateUser(c *gin.Context) {
	userID := c.Param("id")
	
	var req model.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.SendValidationError(c, err.Error())
		return
	}

	user, err := h.userService.UpdateUser(c.Request.Context(), userID, &req)
	if err != nil {
		util.SendError(c, err)
		return
	}

	util.SendSuccessWithMessage(c, user, "User updated successfully")
}

// DeleteUser deletes a user (admin only)
// DELETE /api/v1/users/:id
func (h *UserHandler) DeleteUser(c *gin.Context) {
	userID := c.Param("id")
	
	// Prevent self-deletion
	currentUserID, _ := c.Get("user_id")
	if userID == currentUserID.(string) {
		util.SendCustomError(c, 400, util.ErrCodeBadRequest, "Cannot delete your own account")
		return
	}

	if err := h.userService.DeleteUser(c.Request.Context(), userID); err != nil {
		util.SendError(c, err)
		return
	}

	util.SendSuccessWithMessage(c, nil, "User deleted successfully")
}

// ResetPassword resets a user's password (admin only)
// POST /api/v1/users/:id/reset-password
func (h *UserHandler) ResetPassword(c *gin.Context) {
	userID := c.Param("id")
	
	var req struct {
		NewPassword string `json:"new_password" binding:"required,min=8,max=100"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.SendValidationError(c, err.Error())
		return
	}

	if err := h.userService.ResetPassword(c.Request.Context(), userID, req.NewPassword); err != nil {
		util.SendError(c, err)
		return
	}

	util.SendSuccessWithMessage(c, nil, "Password reset successfully")
}

