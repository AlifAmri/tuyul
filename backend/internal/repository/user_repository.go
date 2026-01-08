// Package repository provides data access for the application and interacts with Redis.
package repository

import (
	"context"
	"errors"
	"time"

	"tuyul/backend/internal/model"
	"tuyul/backend/pkg/redis"

	redislib "github.com/redis/go-redis/v9"
)

// UserRepository handles user data operations
type UserRepository struct {
	redis *redis.Client
}

// NewUserRepository creates a new user repository
func NewUserRepository(redisClient *redis.Client) *UserRepository {
	return &UserRepository{
		redis: redisClient,
	}
}

// Create creates a new user
func (r *UserRepository) Create(ctx context.Context, user *model.User) error {
	// Check if username already exists
	usernameKey := redis.UserByUsernameKey(user.Username)
	exists, err := r.redis.Exists(ctx, usernameKey)
	if err != nil {
		return err
	}
	if exists {
		return errors.New("username already exists")
	}

	// Check if email already exists
	emailKey := redis.UserByEmailKey(user.Email)
	exists, err = r.redis.Exists(ctx, emailKey)
	if err != nil {
		return err
	}
	if exists {
		return errors.New("email already exists")
	}

	// Store user
	userKey := redis.UserKey(user.ID)
	if err := r.redis.SetJSON(ctx, userKey, user, 0); err != nil {
		return err
	}

	// Store username and email indices
	if err := r.redis.Set(ctx, usernameKey, user.ID, 0); err != nil {
		return err
	}
	if err := r.redis.Set(ctx, emailKey, user.ID, 0); err != nil {
		return err
	}

	return nil
}

// GetByID gets a user by ID
func (r *UserRepository) GetByID(ctx context.Context, userID string) (*model.User, error) {
	userKey := redis.UserKey(userID)

	var user model.User
	if err := r.redis.GetJSON(ctx, userKey, &user); err != nil {
		if err == redislib.Nil {
			return nil, errors.New("user not found")
		}
		return nil, err
	}

	return &user, nil
}

// GetByUsername gets a user by username
func (r *UserRepository) GetByUsername(ctx context.Context, username string) (*model.User, error) {
	// Get user ID from username index
	usernameKey := redis.UserByUsernameKey(username)
	userID, err := r.redis.Get(ctx, usernameKey)
	if err != nil {
		if err == redislib.Nil {
			return nil, errors.New("user not found")
		}
		return nil, err
	}

	return r.GetByID(ctx, userID)
}

// GetByEmail gets a user by email
func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	// Get user ID from email index
	emailKey := redis.UserByEmailKey(email)
	userID, err := r.redis.Get(ctx, emailKey)
	if err != nil {
		if err == redislib.Nil {
			return nil, errors.New("user not found")
		}
		return nil, err
	}

	return r.GetByID(ctx, userID)
}

// Update updates a user
func (r *UserRepository) Update(ctx context.Context, user *model.User) error {
	user.UpdatedAt = time.Now()

	userKey := redis.UserKey(user.ID)
	return r.redis.SetJSON(ctx, userKey, user, 0)
}

// UpdateLastLogin updates user's last login time
func (r *UserRepository) UpdateLastLogin(ctx context.Context, userID string) error {
	user, err := r.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	now := time.Now()
	user.LastLoginAt = &now
	user.UpdatedAt = now

	return r.Update(ctx, user)
}

// Delete deletes a user
func (r *UserRepository) Delete(ctx context.Context, userID string) error {
	// Get user first to remove indices
	user, err := r.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	// Delete user
	userKey := redis.UserKey(userID)
	if err := r.redis.Del(ctx, userKey); err != nil {
		return err
	}

	// Delete username and email indices
	usernameKey := redis.UserByUsernameKey(user.Username)
	emailKey := redis.UserByEmailKey(user.Email)
	if err := r.redis.Del(ctx, usernameKey, emailKey); err != nil {
		return err
	}

	return nil
}

// List lists all users (admin only)
func (r *UserRepository) List(ctx context.Context) ([]*model.User, error) {
	// Scan for all user keys
	pattern := "user:*"
	keys, err := r.redis.Keys(ctx, pattern)
	if err != nil {
		return nil, err
	}

	users := make([]*model.User, 0, len(keys))
	for _, key := range keys {
		// Skip index keys
		if key == pattern {
			continue
		}

		var user model.User
		if err := r.redis.GetJSON(ctx, key, &user); err != nil {
			continue // Skip invalid entries
		}
		users = append(users, &user)
	}

	return users, nil
}

// CreateSession creates a new session
func (r *UserRepository) CreateSession(ctx context.Context, session *model.Session) error {
	sessionKey := redis.SessionKey(session.ID)

	// Store session
	if err := r.redis.SetJSON(ctx, sessionKey, session, time.Until(session.ExpiresAt)); err != nil {
		return err
	}

	// Add to user's sessions
	userSessionsKey := redis.UserSessionsKey(session.UserID)
	if err := r.redis.SAdd(ctx, userSessionsKey, session.ID); err != nil {
		return err
	}

	return nil
}

// GetSession gets a session by ID
func (r *UserRepository) GetSession(ctx context.Context, sessionID string) (*model.Session, error) {
	sessionKey := redis.SessionKey(sessionID)

	var session model.Session
	if err := r.redis.GetJSON(ctx, sessionKey, &session); err != nil {
		if err == redislib.Nil {
			return nil, errors.New("session not found")
		}
		return nil, err
	}

	return &session, nil
}

// DeleteSession deletes a session
func (r *UserRepository) DeleteSession(ctx context.Context, sessionID string) error {
	// Get session to find user ID
	session, err := r.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}

	// Delete session
	sessionKey := redis.SessionKey(sessionID)
	if err := r.redis.Del(ctx, sessionKey); err != nil {
		return err
	}

	// Remove from user's sessions
	userSessionsKey := redis.UserSessionsKey(session.UserID)
	if err := r.redis.SRem(ctx, userSessionsKey, sessionID); err != nil {
		return err
	}

	return nil
}

// DeleteUserSessions deletes all sessions for a user
func (r *UserRepository) DeleteUserSessions(ctx context.Context, userID string) error {
	userSessionsKey := redis.UserSessionsKey(userID)

	// Get all session IDs
	sessionIDs, err := r.redis.SMembers(ctx, userSessionsKey)
	if err != nil {
		return err
	}

	// Delete each session
	for _, sessionID := range sessionIDs {
		sessionKey := redis.SessionKey(sessionID)
		if err := r.redis.Del(ctx, sessionKey); err != nil {
			return err
		}
	}

	// Clear user sessions set
	return r.redis.Del(ctx, userSessionsKey)
}

// BlacklistToken adds a token to blacklist
func (r *UserRepository) BlacklistToken(ctx context.Context, token string, expiration time.Duration) error {
	key := redis.TokenBlacklistKey(token)
	return r.redis.Set(ctx, key, "blacklisted", expiration)
}

// IsTokenBlacklisted checks if a token is blacklisted
func (r *UserRepository) IsTokenBlacklisted(ctx context.Context, token string) (bool, error) {
	key := redis.TokenBlacklistKey(token)
	return r.redis.Exists(ctx, key)
}

// UpdateAPIKeyStatus updates user's API key status
func (r *UserRepository) UpdateAPIKeyStatus(ctx context.Context, userID string, hasAPIKey bool) error {
	user, err := r.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	user.HasAPIKey = hasAPIKey
	return r.Update(ctx, user)
}
