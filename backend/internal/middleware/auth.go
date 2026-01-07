package middleware

import (
	"strings"

	"tuyul/backend/internal/model"
	"tuyul/backend/internal/service"
	"tuyul/backend/internal/util"

	"github.com/gin-gonic/gin"
)

// AuthMiddleware creates authentication middleware
func AuthMiddleware(authService *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get token from Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			util.AbortWithCustomError(c, 401, util.ErrCodeUnauthorized, "Missing authorization header")
			return
		}

		// Check if Bearer token
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			util.AbortWithCustomError(c, 401, util.ErrCodeUnauthorized, "Invalid authorization header format")
			return
		}

		token := parts[1]

		// Validate token
		user, err := authService.ValidateToken(c.Request.Context(), token)
		if err != nil {
			util.AbortWithError(c, err)
			return
		}

		// Set user in context
		c.Set("user_id", user.ID)
		c.Set("username", user.Username)
		c.Set("user_role", user.Role)
		c.Set("user", user)

		c.Next()
	}
}

// RequireAdmin middleware requires admin role
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("user_role")
		if !exists {
			util.AbortWithCustomError(c, 401, util.ErrCodeUnauthorized, "Authentication required")
			return
		}

		if role != model.RoleAdmin {
			util.AbortWithCustomError(c, 403, util.ErrCodeForbidden, "Admin access required")
			return
		}

		c.Next()
	}
}

// OptionalAuth middleware extracts user info if token is present, but doesn't require it
func OptionalAuth(authService *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get token from Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.Next()
			return
		}

		// Check if Bearer token
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.Next()
			return
		}

		token := parts[1]

		// Validate token (don't abort if invalid)
		user, err := authService.ValidateToken(c.Request.Context(), token)
		if err == nil {
			// Set user in context if valid
			c.Set("user_id", user.ID)
			c.Set("username", user.Username)
			c.Set("user_role", user.Role)
			c.Set("user", user)
		}

		c.Next()
	}
}

