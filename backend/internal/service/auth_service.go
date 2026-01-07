package service

import (
	"context"
	"time"

	"tuyul/backend/internal/model"
	"tuyul/backend/internal/repository"
	"tuyul/backend/internal/util"
	"tuyul/backend/pkg/crypto"
	"tuyul/backend/pkg/jwt"

	"github.com/google/uuid"
)

// AuthService handles authentication logic
type AuthService struct {
	userRepo   *repository.UserRepository
	jwtManager *jwt.JWTManager
}

// NewAuthService creates a new auth service
func NewAuthService(userRepo *repository.UserRepository, jwtManager *jwt.JWTManager) *AuthService {
	return &AuthService{
		userRepo:   userRepo,
		jwtManager: jwtManager,
	}
}

// Register registers a new user
func (s *AuthService) Register(ctx context.Context, req *model.RegisterRequest) (*model.SafeUser, error) {
	// Validate password strength
	if !crypto.ValidatePasswordStrength(req.Password) {
		return nil, util.ErrValidation("Password must be 8-100 characters")
	}

	// Hash password
	passwordHash, err := crypto.HashPassword(req.Password)
	if err != nil {
		return nil, util.ErrInternalServer("Failed to hash password")
	}

	// Create user
	user := &model.User{
		ID:           uuid.New().String(),
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: passwordHash,
		Role:         model.RoleUser,
		Status:       model.StatusActive,
		HasAPIKey:    false,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	// Save user
	if err := s.userRepo.Create(ctx, user); err != nil {
		if err.Error() == "username already exists" {
			return nil, util.ErrConflict("Username already exists")
		}
		if err.Error() == "email already exists" {
			return nil, util.ErrConflict("Email already exists")
		}
		return nil, util.ErrInternalServer("Failed to create user")
	}

	return user.ToSafeUser(), nil
}

// Login authenticates a user and returns tokens
func (s *AuthService) Login(ctx context.Context, req *model.LoginRequest, userAgent, ip string) (*model.AuthResponse, error) {
	// Get user by username
	user, err := s.userRepo.GetByUsername(ctx, req.Username)
	if err != nil {
		return nil, util.NewAppError(401, util.ErrCodeInvalidCredentials, "Invalid username or password")
	}

	// Check if user is active
	if !user.IsActive() {
		return nil, util.ErrForbidden("User account is inactive")
	}

	// Verify password
	if !crypto.CheckPassword(req.Password, user.PasswordHash) {
		return nil, util.NewAppError(401, util.ErrCodeInvalidCredentials, "Invalid username or password")
	}

	// Generate tokens
	accessToken, err := s.jwtManager.GenerateAccessToken(user.ID, user.Username, user.Role)
	if err != nil {
		return nil, util.ErrInternalServer("Failed to generate access token")
	}

	refreshToken, err := s.jwtManager.GenerateRefreshToken(user.ID)
	if err != nil {
		return nil, util.ErrInternalServer("Failed to generate refresh token")
	}

	// Create session
	session := &model.Session{
		ID:           uuid.New().String(),
		UserID:       user.ID,
		RefreshToken: refreshToken,
		ExpiresAt:    time.Now().Add(7 * 24 * time.Hour), // 7 days
		CreatedAt:    time.Now(),
		LastUsedAt:   time.Now(),
		UserAgent:    userAgent,
		IP:           ip,
	}

	if err := s.userRepo.CreateSession(ctx, session); err != nil {
		return nil, util.ErrInternalServer("Failed to create session")
	}

	// Update last login
	if err := s.userRepo.UpdateLastLogin(ctx, user.ID); err != nil {
		// Log error but don't fail login
	}

	return &model.AuthResponse{
		User:         user.ToSafeUser(),
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(15 * 60), // 15 minutes in seconds
	}, nil
}

// RefreshToken refreshes access token using refresh token
func (s *AuthService) RefreshToken(ctx context.Context, refreshToken string) (*model.AuthResponse, error) {
	// Validate refresh token
	claims, err := s.jwtManager.ValidateToken(refreshToken)
	if err != nil {
		return nil, util.NewAppError(401, util.ErrCodeTokenInvalid, "Invalid refresh token")
	}

	// Check if token is blacklisted
	blacklisted, err := s.userRepo.IsTokenBlacklisted(ctx, refreshToken)
	if err != nil {
		return nil, util.ErrInternalServer("Failed to check token status")
	}
	if blacklisted {
		return nil, util.NewAppError(401, util.ErrCodeTokenInvalid, "Token has been revoked")
	}

	// Get user
	user, err := s.userRepo.GetByID(ctx, claims.UserID)
	if err != nil {
		return nil, util.ErrNotFound("User not found")
	}

	// Check if user is active
	if !user.IsActive() {
		return nil, util.ErrForbidden("User account is inactive")
	}

	// Generate new access token
	accessToken, err := s.jwtManager.GenerateAccessToken(user.ID, user.Username, user.Role)
	if err != nil {
		return nil, util.ErrInternalServer("Failed to generate access token")
	}

	return &model.AuthResponse{
		User:         user.ToSafeUser(),
		AccessToken:  accessToken,
		RefreshToken: refreshToken, // Return same refresh token
		ExpiresIn:    int64(15 * 60), // 15 minutes in seconds
	}, nil
}

// Logout logs out a user and blacklists tokens
func (s *AuthService) Logout(ctx context.Context, accessToken, refreshToken string) error {
	// Validate access token to get expiration
	claims, err := s.jwtManager.ValidateToken(accessToken)
	if err != nil {
		// Token might be expired, but we still want to blacklist it
	}

	// Blacklist access token
	if claims != nil {
		expiration := time.Until(claims.ExpiresAt.Time)
		if expiration > 0 {
			if err := s.userRepo.BlacklistToken(ctx, accessToken, expiration); err != nil {
				return util.ErrInternalServer("Failed to blacklist access token")
			}
		}
	}

	// Validate refresh token
	refreshClaims, err := s.jwtManager.ValidateToken(refreshToken)
	if err != nil {
		return nil // Already invalid, no need to blacklist
	}

	// Blacklist refresh token
	expiration := time.Until(refreshClaims.ExpiresAt.Time)
	if expiration > 0 {
		if err := s.userRepo.BlacklistToken(ctx, refreshToken, expiration); err != nil {
			return util.ErrInternalServer("Failed to blacklist refresh token")
		}
	}

	// TODO: Delete specific session if we track session IDs with tokens

	return nil
}

// GetUserByID gets a user by ID
func (s *AuthService) GetUserByID(ctx context.Context, userID string) (*model.SafeUser, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, util.ErrNotFound("User not found")
	}

	return user.ToSafeUser(), nil
}

// ValidateToken validates an access token and returns user info
func (s *AuthService) ValidateToken(ctx context.Context, token string) (*model.User, error) {
	// Validate token
	claims, err := s.jwtManager.ValidateToken(token)
	if err != nil {
		return nil, util.NewAppError(401, util.ErrCodeTokenInvalid, "Invalid token")
	}

	// Check if token is blacklisted
	blacklisted, err := s.userRepo.IsTokenBlacklisted(ctx, token)
	if err != nil {
		return nil, util.ErrInternalServer("Failed to check token status")
	}
	if blacklisted {
		return nil, util.NewAppError(401, util.ErrCodeTokenInvalid, "Token has been revoked")
	}

	// Get user
	user, err := s.userRepo.GetByID(ctx, claims.UserID)
	if err != nil {
		return nil, util.ErrNotFound("User not found")
	}

	// Check if user is active
	if !user.IsActive() {
		return nil, util.ErrForbidden("User account is inactive")
	}

	return user, nil
}

