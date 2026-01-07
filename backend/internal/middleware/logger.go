package middleware

import (
	"time"

	"tuyul/backend/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequestID middleware adds a unique request ID to each request
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}
		c.Set("request_id", requestID)
		c.Header("X-Request-ID", requestID)
		c.Next()
	}
}

// Logger middleware logs HTTP requests
func Logger(log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method

		// Get request ID
		requestID, _ := c.Get("request_id")

		// Process request
		c.Next()

		// Calculate latency
		latency := time.Since(start)
		statusCode := c.Writer.Status()
		clientIP := c.ClientIP()

		// Log request
		logFields := map[string]interface{}{
			"request_id": requestID,
			"method":     method,
			"path":       path,
			"status":     statusCode,
			"latency_ms": latency.Milliseconds(),
			"ip":         clientIP,
			"user_agent": c.Request.UserAgent(),
		}

		// Add user ID if authenticated
		if userID, exists := c.Get("user_id"); exists {
			logFields["user_id"] = userID
		}

		// Log based on status code
		if statusCode >= 500 {
			log.WithFields(logFields).Error("Server error", nil)
		} else if statusCode >= 400 {
			log.WithFields(logFields).Warn("Client error")
		} else {
			log.WithFields(logFields).Info("Request completed")
		}
	}
}

