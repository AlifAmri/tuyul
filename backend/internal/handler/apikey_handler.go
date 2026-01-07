package handler

import (
	"tuyul/backend/internal/model"
	"tuyul/backend/internal/service"
	"tuyul/backend/internal/util"

	"github.com/gin-gonic/gin"
)

// APIKeyHandler handles API key endpoints
type APIKeyHandler struct {
	apiKeyService *service.APIKeyService
}

// NewAPIKeyHandler creates a new API key handler
func NewAPIKeyHandler(apiKeyService *service.APIKeyService) *APIKeyHandler {
	return &APIKeyHandler{
		apiKeyService: apiKeyService,
	}
}

// Create creates or updates an API key
// POST /api/v1/api-keys
func (h *APIKeyHandler) Create(c *gin.Context) {
	userID, _ := c.Get("user_id")
	
	var req model.APIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.SendValidationError(c, err.Error())
		return
	}

	apiKey, err := h.apiKeyService.Create(c.Request.Context(), userID.(string), &req)
	if err != nil {
		util.SendError(c, err)
		return
	}

	util.SendCreated(c, apiKey, "API key saved and validated successfully")
}

// Get gets the API key status
// GET /api/v1/api-keys
func (h *APIKeyHandler) Get(c *gin.Context) {
	userID, _ := c.Get("user_id")
	
	apiKey, err := h.apiKeyService.Get(c.Request.Context(), userID.(string))
	if err != nil {
		util.SendError(c, err)
		return
	}

	util.SendSuccess(c, apiKey)
}

// Delete deletes the API key
// DELETE /api/v1/api-keys
func (h *APIKeyHandler) Delete(c *gin.Context) {
	userID, _ := c.Get("user_id")
	
	if err := h.apiKeyService.Delete(c.Request.Context(), userID.(string)); err != nil {
		util.SendError(c, err)
		return
	}

	util.SendSuccessWithMessage(c, nil, "API key deleted successfully")
}

// Validate validates the API key with Indodax
// POST /api/v1/api-keys/validate
func (h *APIKeyHandler) Validate(c *gin.Context) {
	userID, _ := c.Get("user_id")
	
	apiKey, err := h.apiKeyService.ValidateAndUpdate(c.Request.Context(), userID.(string))
	if err != nil {
		util.SendError(c, err)
		return
	}

	util.SendSuccessWithMessage(c, apiKey, "API key validated successfully")
}

// GetAccountInfo gets account information from Indodax
// GET /api/v1/api-keys/account-info
func (h *APIKeyHandler) GetAccountInfo(c *gin.Context) {
	userID, _ := c.Get("user_id")
	
	info, err := h.apiKeyService.GetAccountInfo(c.Request.Context(), userID.(string))
	if err != nil {
		util.SendError(c, err)
		return
	}

	util.SendSuccess(c, info)
}

