package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"tuyul/backend/internal/model"
	"tuyul/backend/internal/repository"
	"tuyul/backend/internal/util"
	"tuyul/backend/pkg/crypto"
	"tuyul/backend/pkg/indodax"
	"tuyul/backend/pkg/logger"
)

// APIKeyService handles API key operations
type APIKeyService struct {
	apiKeyRepo          *repository.APIKeyRepository
	userRepo            *repository.UserRepository
	indodaxClient       *indodax.Client
	notificationService *NotificationService
	orderMonitor        *OrderMonitor // For subscribing to order updates when API key is created/updated
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
		orderMonitor:        nil, // Will be set via SetOrderMonitor
		encryptionKey:       encryptionKey,
	}
}

// SetOrderMonitor sets the order monitor (called after OrderMonitor is created to avoid circular dependency)
func (s *APIKeyService) SetOrderMonitor(orderMonitor *OrderMonitor) {
	s.orderMonitor = orderMonitor
}

// Create creates or updates an API key with validation
func (s *APIKeyService) Create(ctx context.Context, userID string, req *model.APIKeyRequest) (*model.APIKeyResponse, error) {
	// Trim whitespace from key and secret (common issue from copy-paste)
	key := strings.TrimSpace(req.Key)
	secret := strings.TrimSpace(req.Secret)

	// Validate that key and secret are not empty after trimming
	if key == "" {
		return nil, util.NewAppError(400, util.ErrCodeValidation, "API key cannot be empty")
	}
	if secret == "" {
		return nil, util.NewAppError(400, util.ErrCodeValidation, "API secret cannot be empty")
	}

	// Log key/secret lengths (masked for security)
	log := logger.GetLogger()
	maskString := func(s string) string {
		if len(s) == 0 {
			return "<empty>"
		}
		if len(s) <= 4 {
			return "****"
		}
		return s[:2] + "****" + s[len(s)-2:]
	}
	log.Infof("Validating API key for user %s: key=%s (len=%d), secret=%s (len=%d)", userID, maskString(key), len(key), maskString(secret), len(secret))

	// Validate API key with Indodax
	isValid, err := s.indodaxClient.ValidateAPIKey(key, secret)
	if err != nil {
		log.Errorf("API key validation failed for user %s: %v", userID, err)
		return nil, util.NewAppErrorWithDetails(400, util.ErrCodeIndodaxAPI,
			"Failed to validate API key", err.Error())
	}

	if !isValid {
		return nil, util.NewAppError(400, util.ErrCodeAPIKeyInvalid,
			"Invalid API key or secret. Please check your credentials from Indodax.")
	}

	// Encrypt API key and secret (use trimmed values)
	encryptedKey, err := crypto.Encrypt(key, s.encryptionKey)
	if err != nil {
		return nil, util.ErrInternalServer("Failed to encrypt API key")
	}

	encryptedSecret, err := crypto.Encrypt(secret, s.encryptionKey)
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

	// Subscribe to order updates for this user (if orderMonitor is available)
	if s.orderMonitor != nil {
		go func() {
			// Subscribe in background to avoid blocking the API response
			subscribeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			
			if err := s.orderMonitor.SubscribeUserOrders(subscribeCtx, userID); err != nil {
				log := logger.GetLogger()
				log.Warnf("Failed to subscribe user %s to order updates after API key creation: %v", userID, err)
			} else {
				log := logger.GetLogger()
				log.Infof("Subscribed user %s to order updates after API key creation", userID)
			}
		}()
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

	// Validate encryption key length
	if len(s.encryptionKey) != 32 {
		return nil, util.NewAppErrorWithDetails(500, util.ErrCodeInternal,
			"Encryption key is invalid", fmt.Sprintf("Encryption key must be 32 bytes, got %d bytes", len(s.encryptionKey)))
	}

	// Debug: Check if encrypted fields are populated
	log := logger.GetLogger()
	if apiKey.EncryptedKey == "" {
		log.Errorf("API key encrypted_key field is empty for user %s. This means the API key was created with old code. Please delete and recreate it.", userID)
		return nil, util.NewAppErrorWithDetails(500, util.ErrCodeInternal,
			"API key data is missing", "The encrypted API key data is missing. This usually happens when the API key was created with an older version of the code. Please delete and recreate your API key.")
	}
	if apiKey.EncryptedSecret == "" {
		log.Errorf("API key encrypted_secret field is empty for user %s. This means the API key was created with old code. Please delete and recreate it.", userID)
		return nil, util.NewAppErrorWithDetails(500, util.ErrCodeInternal,
			"API key data is missing", "The encrypted API secret data is missing. This usually happens when the API key was created with an older version of the code. Please delete and recreate your API key.")
	}

	// Log encrypted data length (for debugging, but not the actual data)
	log.Infof("Decrypting API key for user %s: encrypted_key length=%d, encrypted_secret length=%d", userID, len(apiKey.EncryptedKey), len(apiKey.EncryptedSecret))

	// Decrypt key
	key, err := crypto.Decrypt(apiKey.EncryptedKey, s.encryptionKey)
	if err != nil {
		log.Errorf("Failed to decrypt API key for user %s: %v (encrypted_key length: %d)", userID, err, len(apiKey.EncryptedKey))
		return nil, util.NewAppErrorWithDetails(500, util.ErrCodeInternal,
			"Failed to decrypt API key", fmt.Sprintf("Decryption error: %v. This usually means the encryption key has changed or the encrypted data is corrupted. Please delete and recreate your API key.", err))
	}

	// Decrypt secret
	secret, err := crypto.Decrypt(apiKey.EncryptedSecret, s.encryptionKey)
	if err != nil {
		log.Errorf("Failed to decrypt API secret for user %s: %v (encrypted_secret length: %d)", userID, err, len(apiKey.EncryptedSecret))
		return nil, util.NewAppErrorWithDetails(500, util.ErrCodeInternal,
			"Failed to decrypt API secret", fmt.Sprintf("Decryption error: %v. This usually means the encryption key has changed or the encrypted data is corrupted. Please delete and recreate your API key.", err))
	}

	// Log decrypted key/secret lengths (masked for security)
	maskString := func(s string) string {
		if len(s) == 0 {
			return "<empty>"
		}
		if len(s) <= 4 {
			return "****"
		}
		return s[:2] + "****" + s[len(s)-2:]
	}
	log.Infof("Successfully decrypted API key for user %s: key=%s (len=%d), secret=%s (len=%d)", userID, maskString(key), len(key), maskString(secret), len(secret))

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

	// Unsubscribe from order updates for this user (if orderMonitor is available)
	if s.orderMonitor != nil {
		s.orderMonitor.UnsubscribeUserOrders(userID)
		log := logger.GetLogger()
		log.Infof("Unsubscribed user %s from order updates after API key deletion", userID)
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

	// Validate encryption key length
	if len(s.encryptionKey) != 32 {
		return nil, util.NewAppErrorWithDetails(500, util.ErrCodeInternal,
			"Encryption key is invalid", fmt.Sprintf("Encryption key must be 32 bytes, got %d bytes", len(s.encryptionKey)))
	}

	// Decrypt credentials
	key, err := crypto.Decrypt(apiKey.EncryptedKey, s.encryptionKey)
	if err != nil {
		return nil, util.NewAppErrorWithDetails(500, util.ErrCodeInternal,
			"Failed to decrypt API key", fmt.Sprintf("Decryption error: %v. This usually means the encryption key has changed or the encrypted data is corrupted. Please delete and recreate your API key.", err))
	}

	secret, err := crypto.Decrypt(apiKey.EncryptedSecret, s.encryptionKey)
	if err != nil {
		return nil, util.NewAppErrorWithDetails(500, util.ErrCodeInternal,
			"Failed to decrypt API secret", fmt.Sprintf("Decryption error: %v. This usually means the encryption key has changed or the encrypted data is corrupted. Please delete and recreate your API key.", err))
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

	// Subscribe to order updates for this user (if orderMonitor is available and API key is valid)
	if s.orderMonitor != nil && apiKey.IsValid {
		go func() {
			// Subscribe in background to avoid blocking the API response
			subscribeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			
			if err := s.orderMonitor.SubscribeUserOrders(subscribeCtx, userID); err != nil {
				log := logger.GetLogger()
				log.Warnf("Failed to subscribe user %s to order updates after API key update: %v", userID, err)
			} else {
				log := logger.GetLogger()
				log.Infof("Subscribed user %s to order updates after API key update", userID)
			}
		}()
	}

	return apiKey.ToResponse(), nil
}

// filterNonZeroBalances filters out zero balances from a map
func filterNonZeroBalances(balances map[string]indodax.BalanceValue) map[string]indodax.BalanceValue {
	filtered := make(map[string]indodax.BalanceValue)
	for k, v := range balances {
		// Parse the balance value to check if it's non-zero
		balanceStr := v.String()
		var balance float64
		if _, err := fmt.Sscanf(balanceStr, "%f", &balance); err == nil {
			if balance > 0 {
				filtered[k] = v
			}
		} else {
			// If parsing fails, include it (might be a special format)
			filtered[k] = v
		}
	}
	return filtered
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

	// Filter out zero balances
	info.Balance = filterNonZeroBalances(info.Balance)
	info.BalanceHold = filterNonZeroBalances(info.BalanceHold)

	// Notify via WebSocket - convert BalanceValue map to string map (only non-zero)
	balanceMap := make(map[string]string)
	for k, v := range info.Balance {
		balanceMap[k] = v.String()
	}
	s.notificationService.NotifyUser(ctx, userID, model.MessageTypeBalanceUpdate, balanceMap)

	return info, nil
}


