package util

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Response represents a standard API response
type Response struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *ErrorInfo  `json:"error,omitempty"`
	Message string      `json:"message,omitempty"`
}

// ErrorInfo represents error information in response
type ErrorInfo struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

// PaginationResponse represents a paginated response
type PaginationResponse struct {
	Success    bool        `json:"success"`
	Data       interface{} `json:"data"`
	Pagination Pagination  `json:"pagination"`
}

// Pagination represents pagination metadata
type Pagination struct {
	Limit  int   `json:"limit"`
	Offset int   `json:"offset"`
	Total  int64 `json:"total"`
}

// SendSuccess sends a success response
func SendSuccess(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Success: true,
		Data:    data,
	})
}

// SendSuccessWithMessage sends a success response with a message
func SendSuccessWithMessage(c *gin.Context, data interface{}, message string) {
	c.JSON(http.StatusOK, Response{
		Success: true,
		Data:    data,
		Message: message,
	})
}

// SendCreated sends a 201 Created response
func SendCreated(c *gin.Context, data interface{}, message string) {
	c.JSON(http.StatusCreated, Response{
		Success: true,
		Data:    data,
		Message: message,
	})
}

// SendPaginated sends a paginated response
func SendPaginated(c *gin.Context, data interface{}, pagination Pagination) {
	c.JSON(http.StatusOK, PaginationResponse{
		Success:    true,
		Data:       data,
		Pagination: pagination,
	})
}

// SendError sends an error response
func SendError(c *gin.Context, err error) {
	appErr := GetAppError(err)
	if appErr == nil {
		// Not an AppError, return generic internal server error
		c.JSON(http.StatusInternalServerError, Response{
			Success: false,
			Error: &ErrorInfo{
				Code:    ErrCodeInternal,
				Message: "Internal server error",
			},
		})
		return
	}

	c.JSON(appErr.StatusCode, Response{
		Success: false,
		Error: &ErrorInfo{
			Code:    appErr.Code,
			Message: appErr.Message,
			Details: appErr.Details,
		},
	})
}

// SendCustomError sends a custom error response
func SendCustomError(c *gin.Context, statusCode int, code, message string) {
	c.JSON(statusCode, Response{
		Success: false,
		Error: &ErrorInfo{
			Code:    code,
			Message: message,
		},
	})
}

// SendCustomErrorWithDetails sends a custom error response with details
func SendCustomErrorWithDetails(c *gin.Context, statusCode int, code, message string, details interface{}) {
	c.JSON(statusCode, Response{
		Success: false,
		Error: &ErrorInfo{
			Code:    code,
			Message: message,
			Details: details,
		},
	})
}

// SendValidationError sends a validation error response
func SendValidationError(c *gin.Context, details interface{}) {
	c.JSON(http.StatusBadRequest, Response{
		Success: false,
		Error: &ErrorInfo{
			Code:    ErrCodeValidation,
			Message: "Validation failed",
			Details: details,
		},
	})
}

// AbortWithError aborts the request with an error
func AbortWithError(c *gin.Context, err error) {
	SendError(c, err)
	c.Abort()
}

// AbortWithCustomError aborts the request with a custom error
func AbortWithCustomError(c *gin.Context, statusCode int, code, message string) {
	SendCustomError(c, statusCode, code, message)
	c.Abort()
}

