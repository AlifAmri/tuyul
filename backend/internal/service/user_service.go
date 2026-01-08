package service

import (
	"context"
	"strconv"
	"time"

	"tuyul/backend/internal/model"
	"tuyul/backend/internal/repository"
	"tuyul/backend/internal/util"
	"tuyul/backend/pkg/crypto"

	"github.com/google/uuid"
)

// UserService handles user management operations
type UserService struct {
	userRepo   *repository.UserRepository
	botRepo    *repository.BotRepository
	tradeRepo  *repository.TradeRepository
	apiService *APIKeyService
}

// NewUserService creates a new user service
func NewUserService(userRepo *repository.UserRepository, botRepo *repository.BotRepository, tradeRepo *repository.TradeRepository, apiService *APIKeyService) *UserService {
	return &UserService{
		userRepo:   userRepo,
		botRepo:    botRepo,
		tradeRepo:  tradeRepo,
		apiService: apiService,
	}
}

// GetProfile gets the current user's profile
func (s *UserService) GetProfile(ctx context.Context, userID string) (*model.SafeUser, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, util.ErrNotFound("User not found")
	}

	return user.ToSafeUser(), nil
}

// UpdateProfile updates the current user's profile
func (s *UserService) UpdateProfile(ctx context.Context, userID string, email string) (*model.SafeUser, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, util.ErrNotFound("User not found")
	}

	// Update email if provided
	if email != "" && email != user.Email {
		// Check if email already exists
		existingUser, _ := s.userRepo.GetByEmail(ctx, email)
		if existingUser != nil && existingUser.ID != userID {
			return nil, util.ErrConflict("Email already exists")
		}
		user.Email = email
	}

	user.UpdatedAt = time.Now()

	if err := s.userRepo.Update(ctx, user); err != nil {
		return nil, util.ErrInternalServer("Failed to update profile")
	}

	return user.ToSafeUser(), nil
}

// ChangePassword changes the current user's password
func (s *UserService) ChangePassword(ctx context.Context, userID string, oldPassword, newPassword string) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return util.ErrNotFound("User not found")
	}

	// Verify old password
	if !crypto.CheckPassword(oldPassword, user.PasswordHash) {
		return util.ErrBadRequest("Invalid old password")
	}

	// Validate new password
	if !crypto.ValidatePasswordStrength(newPassword) {
		return util.ErrValidation("Password must be 8-100 characters")
	}

	// Hash new password
	passwordHash, err := crypto.HashPassword(newPassword)
	if err != nil {
		return util.ErrInternalServer("Failed to hash password")
	}

	user.PasswordHash = passwordHash
	user.UpdatedAt = time.Now()

	if err := s.userRepo.Update(ctx, user); err != nil {
		return util.ErrInternalServer("Failed to update password")
	}

	return nil
}

// ListUsers lists all users (admin only)
func (s *UserService) ListUsers(ctx context.Context) ([]*model.SafeUser, error) {
	users, err := s.userRepo.List(ctx)
	if err != nil {
		return nil, util.ErrInternalServer("Failed to list users")
	}

	safeUsers := make([]*model.SafeUser, len(users))
	for i, user := range users {
		safeUsers[i] = user.ToSafeUser()
	}

	return safeUsers, nil
}

// GetUser gets a user by ID (admin only)
func (s *UserService) GetUser(ctx context.Context, userID string) (*model.SafeUser, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, util.ErrNotFound("User not found")
	}

	return user.ToSafeUser(), nil
}

// CreateUser creates a new user (admin only)
func (s *UserService) CreateUser(ctx context.Context, req *model.RegisterRequest, role string) (*model.SafeUser, error) {
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
		Role:         role,
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

// UpdateUser updates a user (admin only)
func (s *UserService) UpdateUser(ctx context.Context, userID string, req *model.UpdateUserRequest) (*model.SafeUser, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, util.ErrNotFound("User not found")
	}

	// Update fields if provided
	if req.Email != "" && req.Email != user.Email {
		// Check if email already exists
		existingUser, _ := s.userRepo.GetByEmail(ctx, req.Email)
		if existingUser != nil && existingUser.ID != userID {
			return nil, util.ErrConflict("Email already exists")
		}
		user.Email = req.Email
	}

	if req.Role != "" {
		user.Role = req.Role
	}

	if req.Status != "" {
		user.Status = req.Status
	}

	user.UpdatedAt = time.Now()

	if err := s.userRepo.Update(ctx, user); err != nil {
		return nil, util.ErrInternalServer("Failed to update user")
	}

	return user.ToSafeUser(), nil
}

// DeleteUser deletes a user (admin only)
func (s *UserService) DeleteUser(ctx context.Context, userID string) error {
	// Check if user exists
	_, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return util.ErrNotFound("User not found")
	}

	// Delete all user sessions
	if err := s.userRepo.DeleteUserSessions(ctx, userID); err != nil {
		// Log error but continue with deletion
	}

	// Delete user
	if err := s.userRepo.Delete(ctx, userID); err != nil {
		return util.ErrInternalServer("Failed to delete user")
	}

	return nil
}

// ResetPassword resets a user's password (admin only)
func (s *UserService) ResetPassword(ctx context.Context, userID, newPassword string) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return util.ErrNotFound("User not found")
	}

	// Validate new password
	if !crypto.ValidatePasswordStrength(newPassword) {
		return util.ErrValidation("Password must be 8-100 characters")
	}

	// Hash new password
	passwordHash, err := crypto.HashPassword(newPassword)
	if err != nil {
		return util.ErrInternalServer("Failed to hash password")
	}

	user.PasswordHash = passwordHash
	user.UpdatedAt = time.Now()

	if err := s.userRepo.Update(ctx, user); err != nil {
		return util.ErrInternalServer("Failed to reset password")
	}

	// Delete all user sessions to force re-login
	if err := s.userRepo.DeleteUserSessions(ctx, userID); err != nil {
		// Log error but don't fail
	}

	return nil
}

// GetStats returns aggregated user statistics
func (s *UserService) GetStats(ctx context.Context, userID string) (*model.UserStats, error) {
	// 1. Get all bots for user
	bots, err := s.botRepo.ListByUser(ctx, userID)
	if err != nil {
		return nil, util.ErrInternalServer("Failed to fetch user bots")
	}

	stats := &model.UserStats{}
	var totalWinningTrades int

	for _, bot := range bots {
		if bot.Status == model.BotStatusRunning {
			stats.ActiveBots++
		}
		stats.TotalProfitIDR += bot.TotalProfitIDR
		stats.TotalTrades += bot.TotalTrades
		totalWinningTrades += bot.WinningTrades

		// Aggregate virtual balances
		if bot.IsPaperTrading {
			stats.TotalPaperBalance += bot.Balances["idr"]
		} else {
			stats.TotalAllocatedIDR += bot.InitialBalanceIDR
		}
	}

	// 2. Get all copilot trades
	trades, _, err := s.tradeRepo.ListByUser(ctx, userID, 0, 1000)
	if err == nil {
		for _, trade := range trades {
			if trade.Status == model.TradeStatusCompleted || trade.Status == model.TradeStatusStopped {
				stats.TotalProfitIDR += trade.ProfitIDR
				stats.TotalTrades++
				if trade.ProfitIDR > 0 {
					totalWinningTrades++
				}
			}
		}
	}

	// 3. Get Real IDR Balance if API Key exists
	if s.apiService != nil {
		info, err := s.apiService.GetAccountInfo(ctx, userID)
		if err == nil && info != nil {
			stats.RealIDRBalance, _ = strconv.ParseFloat(info.Balance["idr"], 64)
		}
	}

	// Calculate average win rate
	if stats.TotalTrades > 0 {
		stats.AvgWinRate = float64(totalWinningTrades) / float64(stats.TotalTrades) * 100
	}

	return stats, nil
}
