package service

import (
	"context"
	"time"

	"tuyul/backend/internal/model"
	"tuyul/backend/internal/repository"
	"tuyul/backend/internal/util"
	"tuyul/backend/pkg/crypto"
	"tuyul/backend/pkg/indodax"
)

// APIKeyService handles API key operations
type APIKeyService struct {
	apiKeyRepo          *repository.APIKeyRepository
	userRepo            *repository.UserRepository
	indodaxClient       *indodax.Client
	notificationService *NotificationService
	encryptionKey       string
}

// NewAPIKeyService creates a new API key service
func NewAPIKeyService(
	apiKeyRepo *repository.APIKeyRepository,
	userRepo *repository.UserRepository,
	indodaxClient *indodax.Client,
	notificationService *NotificationService,
	encryptionKey string,
) *APIKeyService {
	return &APIKeyService{
		apiKeyRepo:          apiKeyRepo,
		userRepo:            userRepo,
		indodaxClient:       indodaxClient,
		notificationService: notificationService,
		encryptionKey:       encryptionKey,
	}
}

// Create creates or updates an API key with validation
func (s *APIKeyService) Create(ctx context.Context, userID string, req *model.APIKeyRequest) (*model.APIKeyResponse, error) {
	// Validate API key with Indodax
	isValid, err := s.indodaxClient.ValidateAPIKey(req.Key, req.Secret)
	if err != nil {
		return nil, util.NewAppErrorWithDetails(400, util.ErrCodeIndodaxAPI,
			"Failed to validate API key", err.Error())
	}

	if !isValid {
		return nil, util.NewAppError(400, util.ErrCodeAPIKeyInvalid,
			"Invalid API key or secret. Please check your credentials from Indodax.")
	}

	// Encrypt API key and secret
	encryptedKey, err := crypto.Encrypt(req.Key, s.encryptionKey)
	if err != nil {
		return nil, util.ErrInternalServer("Failed to encrypt API key")
	}

	encryptedSecret, err := crypto.Encrypt(req.Secret, s.encryptionKey)
	if err != nil {
		return nil, util.ErrInternalServer("Failed to encrypt API secret")
	}

	// Create API key
	now := time.Now()
	apiKey := &model.APIKey{
		UserID:          userID,
		EncryptedKey:    encryptedKey,
		EncryptedSecret: encryptedSecret,
		IsValid:         true,
		LastValidatedAt: &now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	// Check if user already has an API key
	existingKey, _ := s.apiKeyRepo.Get(ctx, userID)
	if existingKey != nil {
		apiKey.CreatedAt = existingKey.CreatedAt // Keep original creation time
	}

	// Save API key
	if err := s.apiKeyRepo.Create(ctx, apiKey); err != nil {
		return nil, util.ErrInternalServer("Failed to save API key")
	}

	// Update user's API key status
	if err := s.userRepo.UpdateAPIKeyStatus(ctx, userID, true); err != nil {
		// Log error but don't fail
	}

	return apiKey.ToResponse(), nil
}

// Get gets the API key status (without exposing credentials)
func (s *APIKeyService) Get(ctx context.Context, userID string) (*model.APIKeyResponse, error) {
	apiKey, err := s.apiKeyRepo.Get(ctx, userID)
	if err != nil {
		return nil, util.ErrNotFound("API key not found")
	}

	return apiKey.ToResponse(), nil
}

// GetDecrypted gets decrypted API credentials (for internal use only)
func (s *APIKeyService) GetDecrypted(ctx context.Context, userID string) (*model.DecryptedAPIKey, error) {
	apiKey, err := s.apiKeyRepo.Get(ctx, userID)
	if err != nil {
		return nil, util.ErrNotFound("API key not found")
	}

	if !apiKey.IsValid {
		return nil, util.NewAppError(400, util.ErrCodeAPIKeyInvalid, "API key is invalid")
	}

	// Decrypt key
	key, err := crypto.Decrypt(apiKey.EncryptedKey, s.encryptionKey)
	if err != nil {
		return nil, util.ErrInternalServer("Failed to decrypt API key")
	}

	// Decrypt secret
	secret, err := crypto.Decrypt(apiKey.EncryptedSecret, s.encryptionKey)
	if err != nil {
		return nil, util.ErrInternalServer("Failed to decrypt API secret")
	}

	return &model.DecryptedAPIKey{
		Key:    key,
		Secret: secret,
	}, nil
}

// Delete deletes an API key
func (s *APIKeyService) Delete(ctx context.Context, userID string) error {
	// Check if API key exists
	_, err := s.apiKeyRepo.Get(ctx, userID)
	if err != nil {
		return util.ErrNotFound("API key not found")
	}

	// Delete API key
	if err := s.apiKeyRepo.Delete(ctx, userID); err != nil {
		return util.ErrInternalServer("Failed to delete API key")
	}

	// Update user's API key status
	if err := s.userRepo.UpdateAPIKeyStatus(ctx, userID, false); err != nil {
		// Log error but don't fail
	}

	return nil
}

// ValidateAndUpdate validates the existing API key with Indodax and updates status
func (s *APIKeyService) ValidateAndUpdate(ctx context.Context, userID string) (*model.APIKeyResponse, error) {
	// Get API key
	apiKey, err := s.apiKeyRepo.Get(ctx, userID)
	if err != nil {
		return nil, util.ErrNotFound("API key not found")
	}

	// Decrypt credentials
	key, err := crypto.Decrypt(apiKey.EncryptedKey, s.encryptionKey)
	if err != nil {
		return nil, util.ErrInternalServer("Failed to decrypt API key")
	}

	secret, err := crypto.Decrypt(apiKey.EncryptedSecret, s.encryptionKey)
	if err != nil {
		return nil, util.ErrInternalServer("Failed to decrypt API secret")
	}

	// Validate with Indodax
	isValid, err := s.indodaxClient.ValidateAPIKey(key, secret)
	if err != nil {
		return nil, util.NewAppErrorWithDetails(400, util.ErrCodeIndodaxAPI,
			"Failed to validate API key", err.Error())
	}

	// Update validation status
	now := time.Now()
	apiKey.IsValid = isValid
	apiKey.LastValidatedAt = &now
	apiKey.UpdatedAt = now

	if err := s.apiKeyRepo.Update(ctx, apiKey); err != nil {
		return nil, util.ErrInternalServer("Failed to update API key status")
	}

	return apiKey.ToResponse(), nil
}

// GetAccountInfo gets account information from Indodax
func (s *APIKeyService) GetAccountInfo(ctx context.Context, userID string) (*indodax.GetInfoReturn, error) {
	// Get decrypted credentials
	credentials, err := s.GetDecrypted(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Get account info from Indodax
	info, err := s.indodaxClient.GetInfo(ctx, credentials.Key, credentials.Secret)
	if err != nil {
		return nil, util.NewAppErrorWithDetails(400, util.ErrCodeIndodaxAPI,
			"Failed to get account info", err.Error())
	}

	// Notify via WebSocket
	s.notificationService.NotifyUser(ctx, userID, model.MessageTypeBalanceUpdate, info.Balance)

	return info, nil
}


