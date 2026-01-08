package model

import "time"

// User represents a user in the system
type User struct {
	ID           string     `json:"id"`
	Username     string     `json:"username"`
	Email        string     `json:"email"`
	PasswordHash string     `json:"password_hash"` // Stored in Redis, but excluded from SafeUser responses
	Role         string     `json:"role"`          // "admin" or "user"
	Status       string     `json:"status"`        // "active" or "inactive"
	HasAPIKey    bool       `json:"has_api_key"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
}

// UserRole constants
const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

// UserStatus constants
const (
	StatusActive   = "active"
	StatusInactive = "inactive"
)

// IsAdmin checks if user has admin role
func (u *User) IsAdmin() bool {
	return u.Role == RoleAdmin
}

// IsActive checks if user is active
func (u *User) IsActive() bool {
	return u.Status == StatusActive
}

// SafeUser returns user data safe for API response (no sensitive fields)
type SafeUser struct {
	ID          string     `json:"id"`
	Username    string     `json:"username"`
	Email       string     `json:"email"`
	Role        string     `json:"role"`
	Status      string     `json:"status"`
	HasAPIKey   bool       `json:"has_api_key"`
	CreatedAt   time.Time  `json:"created_at"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
}

// ToSafeUser converts User to SafeUser
func (u *User) ToSafeUser() *SafeUser {
	return &SafeUser{
		ID:          u.ID,
		Username:    u.Username,
		Email:       u.Email,
		Role:        u.Role,
		Status:      u.Status,
		HasAPIKey:   u.HasAPIKey,
		CreatedAt:   u.CreatedAt,
		LastLoginAt: u.LastLoginAt,
	}
}

// RegisterRequest represents user registration request
type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8,max=100"`
}

// LoginRequest represents user login request
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// UpdateUserRequest represents user update request
type UpdateUserRequest struct {
	Email  string `json:"email" binding:"omitempty,email"`
	Role   string `json:"role" binding:"omitempty,oneof=admin user"`
	Status string `json:"status" binding:"omitempty,oneof=active inactive"`
}

// ChangePasswordRequest represents password change request
type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=8,max=100"`
}

// AuthResponse represents authentication response
type AuthResponse struct {
	User         *SafeUser `json:"user"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresIn    int64     `json:"expires_in"` // seconds
}

// RefreshTokenRequest represents refresh token request
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// Session represents a user session
type Session struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	CreatedAt    time.Time `json:"created_at"`
	LastUsedAt   time.Time `json:"last_used_at"`
	UserAgent    string    `json:"user_agent"`
	IP           string    `json:"ip"`
}

// UserStats represents aggregated user statistics
type UserStats struct {
	ActiveBots        int     `json:"active_bots"`
	TotalProfitIDR    float64 `json:"total_profit_idr"`
	AvgWinRate        float64 `json:"avg_win_rate"`
	TotalTrades       int     `json:"total_trades"`
	RealIDRBalance    float64 `json:"real_idr_balance"`
	TotalAllocatedIDR float64 `json:"total_allocated_idr"`
	TotalPaperBalance float64 `json:"total_paper_balance"`
}
