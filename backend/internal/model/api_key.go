package model

import "time"

// APIKey represents an Indodax API key
type APIKey struct {
	UserID          string    `json:"user_id"`
	EncryptedKey    string    `json:"-"` // Never expose encrypted key
	EncryptedSecret string    `json:"-"` // Never expose encrypted secret
	IsValid         bool      `json:"is_valid"`
	LastValidatedAt *time.Time `json:"last_validated_at,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// APIKeyRequest represents API key creation/update request
type APIKeyRequest struct {
	Key    string `json:"key" binding:"required"`
	Secret string `json:"secret" binding:"required"`
}

// APIKeyResponse represents API key response (without secret)
type APIKeyResponse struct {
	UserID          string     `json:"user_id"`
	IsValid         bool       `json:"is_valid"`
	LastValidatedAt *time.Time `json:"last_validated_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// ToResponse converts APIKey to APIKeyResponse
func (k *APIKey) ToResponse() *APIKeyResponse {
	return &APIKeyResponse{
		UserID:          k.UserID,
		IsValid:         k.IsValid,
		LastValidatedAt: k.LastValidatedAt,
		CreatedAt:       k.CreatedAt,
		UpdatedAt:       k.UpdatedAt,
	}
}

// DecryptedAPIKey holds decrypted API credentials (in-memory only)
type DecryptedAPIKey struct {
	Key    string
	Secret string
}



