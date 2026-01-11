package util

import (
	"errors"
	"net/http"
	"strings"
)

// AppError represents an application error with HTTP status code
type AppError struct {
	StatusCode int    `json:"-"`
	Code       string `json:"code"`
	Message    string `json:"message"`
	Details    string `json:"details,omitempty"`
	Err        error  `json:"-"`
}

// Error implements the error interface
func (e *AppError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return e.Message
}

// Common error codes
const (
	ErrCodeInternal         = "INTERNAL_ERROR"
	ErrCodeBadRequest       = "BAD_REQUEST"
	ErrCodeUnauthorized     = "UNAUTHORIZED"
	ErrCodeForbidden        = "FORBIDDEN"
	ErrCodeNotFound         = "NOT_FOUND"
	ErrCodeConflict         = "CONFLICT"
	ErrCodeValidation       = "VALIDATION_ERROR"
	ErrCodeRateLimit        = "RATE_LIMIT_EXCEEDED"
	ErrCodeInsufficientBalance = "INSUFFICIENT_BALANCE"
	ErrCodeInvalidCredentials  = "INVALID_CREDENTIALS"
	ErrCodeTokenExpired     = "TOKEN_EXPIRED"
	ErrCodeTokenInvalid     = "TOKEN_INVALID"
	ErrCodeAPIKeyInvalid    = "API_KEY_INVALID"
	ErrCodeOrderNotFound    = "ORDER_NOT_FOUND"
	ErrCodeBotNotFound      = "BOT_NOT_FOUND"
	ErrCodeIndodaxAPI       = "INDODAX_API_ERROR"
)

// NewAppError creates a new application error
func NewAppError(statusCode int, code, message string) *AppError {
	return &AppError{
		StatusCode: statusCode,
		Code:       code,
		Message:    message,
	}
}

// NewAppErrorWithDetails creates a new application error with details
func NewAppErrorWithDetails(statusCode int, code, message, details string) *AppError {
	return &AppError{
		StatusCode: statusCode,
		Code:       code,
		Message:    message,
		Details:    details,
	}
}

// WrapError wraps an existing error
func WrapError(statusCode int, code, message string, err error) *AppError {
	return &AppError{
		StatusCode: statusCode,
		Code:       code,
		Message:    message,
		Err:        err,
	}
}

// Common error constructors

func ErrBadRequest(message string) *AppError {
	return NewAppError(http.StatusBadRequest, ErrCodeBadRequest, message)
}

func ErrUnauthorized(message string) *AppError {
	return NewAppError(http.StatusUnauthorized, ErrCodeUnauthorized, message)
}

func ErrForbidden(message string) *AppError {
	return NewAppError(http.StatusForbidden, ErrCodeForbidden, message)
}

func ErrNotFound(message string) *AppError {
	return NewAppError(http.StatusNotFound, ErrCodeNotFound, message)
}

func ErrConflict(message string) *AppError {
	return NewAppError(http.StatusConflict, ErrCodeConflict, message)
}

func ErrValidation(message string) *AppError {
	return NewAppError(http.StatusBadRequest, ErrCodeValidation, message)
}

func ErrInternalServer(message string) *AppError {
	return NewAppError(http.StatusInternalServerError, ErrCodeInternal, message)
}

func ErrRateLimit(message string) *AppError {
	return NewAppError(http.StatusTooManyRequests, ErrCodeRateLimit, message)
}

// IsAppError checks if an error is an AppError
func IsAppError(err error) bool {
	var appErr *AppError
	return errors.As(err, &appErr)
}

// GetAppError extracts AppError from error
func GetAppError(err error) *AppError {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr
	}
	return nil
}

// IsAPIKeyError checks if an error is related to API key issues
func IsAPIKeyError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "api key") ||
		strings.Contains(errStr, "invalid credentials") ||
		strings.Contains(errStr, "bad sign") ||
		strings.Contains(errStr, ErrCodeAPIKeyInvalid) ||
		strings.Contains(errStr, "api_key_invalid")
}

// IsCriticalTradingError checks if an error is critical and should stop the bot
func IsCriticalTradingError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	// API key errors
	if IsAPIKeyError(err) {
		return true
	}
	// Invalid pair - configuration issue that prevents trading
	if strings.Contains(errStr, "invalid pair") {
		return true
	}
	return false
}

// IsOrderNotFoundError checks if an error is "Order not found" (non-critical, order already filled/cancelled)
func IsOrderNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "order not found")
}
