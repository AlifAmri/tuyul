package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"tuyul/backend/internal/util"
	"tuyul/backend/pkg/redis"

	"github.com/gin-gonic/gin"
)

// RateLimiter middleware limits requests per minute
type RateLimiter struct {
	redis       *redis.Client
	limit       int
	window      time.Duration
	keyPrefix   string
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(redisClient *redis.Client, limit int, window time.Duration, keyPrefix string) *RateLimiter {
	return &RateLimiter{
		redis:     redisClient,
		limit:     limit,
		window:    window,
		keyPrefix: keyPrefix,
	}
}

// Limit returns a middleware that limits requests
func (rl *RateLimiter) Limit() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get identifier (IP address or user ID)
		identifier := c.ClientIP()
		if userID, exists := c.Get("user_id"); exists {
			identifier = fmt.Sprintf("user:%v", userID)
		}

		// Create rate limit key
		key := redis.RateLimitKey(identifier, rl.keyPrefix)

		// Check rate limit
		allowed, err := rl.checkRateLimit(c.Request.Context(), key)
		if err != nil {
			// Log error but don't block request
			c.Next()
			return
		}

		if !allowed {
			util.AbortWithCustomError(c, http.StatusTooManyRequests,
				util.ErrCodeRateLimit, "Rate limit exceeded. Please try again later.")
			return
		}

		c.Next()
	}
}

// checkRateLimit checks if request is within rate limit
func (rl *RateLimiter) checkRateLimit(ctx context.Context, key string) (bool, error) {
	// Increment counter
	count, err := rl.redis.Incr(ctx, key)
	if err != nil {
		return false, err
	}

	// Set expiration on first request
	if count == 1 {
		err = rl.redis.Expire(ctx, key, rl.window)
		if err != nil {
			return false, err
		}
	}

	// Check if limit exceeded
	return count <= int64(rl.limit), nil
}

// RateLimit creates a rate limiting middleware with default settings (per IP)
func RateLimit(redisClient *redis.Client, limit int) gin.HandlerFunc {
	limiter := NewRateLimiter(redisClient, limit, time.Minute, "general")
	return limiter.Limit()
}

// AuthRateLimit creates a rate limiting middleware for auth endpoints
func AuthRateLimit(redisClient *redis.Client, limit int) gin.HandlerFunc {
	limiter := NewRateLimiter(redisClient, limit, time.Minute, "auth")
	return limiter.Limit()
}

