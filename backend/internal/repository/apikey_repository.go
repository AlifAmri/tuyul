// Package repository provides data access for the application and interacts with Redis.
package repository

import (
	"context"
	"errors"

	"tuyul/backend/internal/model"
	"tuyul/backend/pkg/redis"

	redislib "github.com/redis/go-redis/v9"
)

// APIKeyRepository handles API key data operations
type APIKeyRepository struct {
	redis *redis.Client
}

// NewAPIKeyRepository creates a new API key repository
func NewAPIKeyRepository(redisClient *redis.Client) *APIKeyRepository {
	return &APIKeyRepository{
		redis: redisClient,
	}
}

// Create creates or updates an API key
func (r *APIKeyRepository) Create(ctx context.Context, apiKey *model.APIKey) error {
	key := redis.APIKeyKey(apiKey.UserID)
	return r.redis.SetJSON(ctx, key, apiKey, 0)
}

// Get gets an API key by user ID
func (r *APIKeyRepository) Get(ctx context.Context, userID string) (*model.APIKey, error) {
	key := redis.APIKeyKey(userID)

	var apiKey model.APIKey
	if err := r.redis.GetJSON(ctx, key, &apiKey); err != nil {
		if err == redislib.Nil {
			return nil, errors.New("API key not found")
		}
		return nil, err
	}

	return &apiKey, nil
}

// Update updates an API key
func (r *APIKeyRepository) Update(ctx context.Context, apiKey *model.APIKey) error {
	return r.Create(ctx, apiKey) // Same as create in Redis
}

// Delete deletes an API key
func (r *APIKeyRepository) Delete(ctx context.Context, userID string) error {
	key := redis.APIKeyKey(userID)
	return r.redis.Del(ctx, key)
}

// Exists checks if a user has an API key
func (r *APIKeyRepository) Exists(ctx context.Context, userID string) (bool, error) {
	key := redis.APIKeyKey(userID)
	return r.redis.Exists(ctx, key)
}

// GetAllUserIDs returns all user IDs that have API keys
func (r *APIKeyRepository) GetAllUserIDs(ctx context.Context) ([]string, error) {
	// Get the pattern for API key keys (e.g., "tuyul:api_key:*")
	pattern := redis.APIKeyKey("*")
	keys, err := r.redis.Keys(ctx, pattern)
	if err != nil {
		return nil, err
	}

	// Extract user IDs from keys
	// Key format: "tuyul:api_key:{userID}"
	userIDs := make([]string, 0, len(keys))
	prefix := redis.APIKeyKey("")
	
	for _, key := range keys {
		// Remove prefix to get userID
		if len(key) > len(prefix) {
			userID := key[len(prefix):]
			userIDs = append(userIDs, userID)
		}
	}

	return userIDs, nil
}


