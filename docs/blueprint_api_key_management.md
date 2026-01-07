# API Key Management

## Overview

TUYUL securely manages Indodax API keys for each user, providing encryption at rest, validation on creation, and secure decryption only during trading operations.

---

## Features

### 1. API Key Security
- **Encryption**: AES-256-GCM encryption for all API keys
- **Unique nonces**: Each encryption uses a unique nonce
- **In-memory only**: Decrypted keys never persisted
- **No logging**: Sensitive data never logged

### 2. API Key Validation
- **Connection test**: Verify API key on creation
- **Permission check**: Ensure key has trading permissions
- **Balance check**: Validate account access

### 3. User Restrictions
- **One key per user**: Each user can have only one active API key
- **Update capability**: Users can update their API key
- **Secure deletion**: Wipe keys from memory after deletion

---

## Data Models

### API Key Model
```go
type APIKey struct {
    ID               string    `json:"id"`                // UUID
    UserID           string    `json:"user_id"`           // Owner
    EncryptedKey     string    `json:"-"`                 // Never exposed
    EncryptedSecret  string    `json:"-"`                 // Never exposed
    Nonce            string    `json:"-"`                 // For decryption
    Label            string    `json:"label"`             // User-friendly name
    Permissions      []string  `json:"permissions"`       // ["trade", "info", "withdraw"]
    IsActive         bool      `json:"is_active"`         // Enable/disable
    LastUsedAt       time.Time `json:"last_used_at"`      // Last API call
    CreatedAt        time.Time `json:"created_at"`
    UpdatedAt        time.Time `json:"updated_at"`
}
```

### Decrypted Credentials (In-Memory Only)
```go
type APICredentials struct {
    Key    string // Decrypted API key
    Secret string // Decrypted API secret
}
```

---

## Redis Schema

### API Keys Storage
```
Key Pattern: apikey:{user_id}
Type: Hash
Fields:
  - id: string (UUID)
  - user_id: string
  - encrypted_key: string (base64)
  - encrypted_secret: string (base64)
  - nonce: string (base64)
  - label: string
  - permissions: JSON array
  - is_active: boolean
  - last_used_at: timestamp
  - created_at: timestamp
  - updated_at: timestamp

Index Keys:
  - apikey_by_id:{id} → {user_id}
```

### API Key Cache (Decrypted, Short-lived)
```
Key Pattern: apikey_cache:{user_id}
Type: String (JSON)
Value: {
  "key": "decrypted_key",
  "secret": "decrypted_secret"
}
TTL: 1 hour (or until user logs out)
```

---

## Encryption Implementation

### Encryption Algorithm
- **Algorithm**: AES-256-GCM
- **Key size**: 256 bits
- **Nonce size**: 12 bytes (96 bits)
- **Authentication tag**: 16 bytes (128 bits)

### Master Key Derivation
```go
// Derive master encryption key from environment variable
func deriveMasterKey(password string, salt []byte) []byte {
    return pbkdf2.Key([]byte(password), salt, 100000, 32, sha256.New)
}
```

### Encryption Process
```go
func encryptAPIKey(plaintext string, masterKey []byte) (ciphertext, nonce string, err error) {
    // 1. Generate random nonce (12 bytes)
    nonce := make([]byte, 12)
    rand.Read(nonce)
    
    // 2. Create AES-GCM cipher
    block, _ := aes.NewCipher(masterKey)
    gcm, _ := cipher.NewGCM(block)
    
    // 3. Encrypt plaintext
    encrypted := gcm.Seal(nil, nonce, []byte(plaintext), nil)
    
    // 4. Return base64-encoded ciphertext and nonce
    return base64.StdEncoding.EncodeToString(encrypted),
           base64.StdEncoding.EncodeToString(nonce), nil
}
```

### Decryption Process
```go
func decryptAPIKey(ciphertext, nonce string, masterKey []byte) (plaintext string, err error) {
    // 1. Decode base64
    encrypted, _ := base64.StdEncoding.DecodeString(ciphertext)
    nonceBytes, _ := base64.StdEncoding.DecodeString(nonce)
    
    // 2. Create AES-GCM cipher
    block, _ := aes.NewCipher(masterKey)
    gcm, _ := cipher.NewGCM(block)
    
    // 3. Decrypt ciphertext
    decrypted, err := gcm.Open(nil, nonceBytes, encrypted, nil)
    
    // 4. Return plaintext
    return string(decrypted), err
}
```

---

## API Key Validation Flow

### On Creation/Update
```
User submits API key + secret
        ↓
Validate format (not empty, min length)
        ↓
Test connection to Indodax API
        ↓
Call getInfo endpoint with provided credentials
        ↓
Check response for errors
        ↓
Verify trading permissions exist
        ↓
If valid → Encrypt and store
If invalid → Return error with details
```

### Indodax API Validation Call
```go
// Test API key by calling getInfo endpoint
func validateIndodaxAPIKey(key, secret string) (*ValidationResult, error) {
    // 1. Prepare request
    endpoint := "https://indodax.com/tapi"
    method := "getInfo"
    nonce := time.Now().UnixMilli()
    
    // 2. Create signature
    payload := fmt.Sprintf("method=%s&nonce=%d", method, nonce)
    signature := createHMAC512(payload, secret)
    
    // 3. Make HTTP request
    req := http.NewRequest("POST", endpoint, strings.NewReader(payload))
    req.Header.Set("Key", key)
    req.Header.Set("Sign", signature)
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
    
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return nil, err
    }
    
    // 4. Parse response
    var result struct {
        Success int    `json:"success"`
        Error   string `json:"error"`
        Return  struct {
            ServerTime int64              `json:"server_time"`
            Balance    map[string]float64 `json:"balance"`
        } `json:"return"`
    }
    json.NewDecoder(resp.Body).Decode(&result)
    
    // 5. Validate response
    if result.Success != 1 {
        return &ValidationResult{
            Valid: false,
            Error: result.Error,
        }, nil
    }
    
    return &ValidationResult{
        Valid:       true,
        Permissions: []string{"trade", "info"},
        HasBalance:  len(result.Return.Balance) > 0,
    }, nil
}
```

---

## API Endpoints

### POST /api/v1/user/apikey
Create or update user's API key

**Headers:**
```
Authorization: Bearer {access_token}
```

**Request:**
```json
{
  "key": "ABCDEF123456",
  "secret": "secret123456789abcdef",
  "label": "My Trading Key"  // optional
}
```

**Validation Steps:**
1. Check if key/secret are not empty
2. Test connection to Indodax
3. Verify trading permissions
4. Encrypt key and secret
5. Store in Redis

**Response (Success):**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "user_id": "user_uuid",
    "label": "My Trading Key",
    "permissions": ["trade", "info"],
    "is_active": true,
    "created_at": "2024-01-07T16:00:00Z"
  },
  "message": "API key validated and saved successfully"
}
```

**Response (Validation Failed):**
```json
{
  "success": false,
  "error": {
    "code": "INVALID_API_KEY",
    "message": "Invalid API credentials",
    "details": "Invalid key or secret"  // From Indodax
  }
}
```

**Response (Network Error):**
```json
{
  "success": false,
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Unable to validate API key",
    "details": "Connection to Indodax failed. Please try again."
  }
}
```

### GET /api/v1/user/apikey
Get user's API key info (without revealing key/secret)

**Headers:**
```
Authorization: Bearer {access_token}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "user_id": "user_uuid",
    "label": "My Trading Key",
    "permissions": ["trade", "info"],
    "is_active": true,
    "last_used_at": "2024-01-07T17:30:00Z",
    "created_at": "2024-01-07T16:00:00Z",
    "updated_at": "2024-01-07T16:00:00Z",
    "masked_key": "ABC***456"  // Show first 3 and last 3 chars
  }
}
```

**Response (No API Key):**
```json
{
  "success": true,
  "data": null,
  "message": "No API key configured"
}
```

### PUT /api/v1/user/apikey/status
Toggle API key active status

**Headers:**
```
Authorization: Bearer {access_token}
```

**Request:**
```json
{
  "is_active": false
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "is_active": false,
    "updated_at": "2024-01-07T18:00:00Z"
  },
  "message": "API key status updated"
}
```

### DELETE /api/v1/user/apikey
Delete user's API key

**Headers:**
```
Authorization: Bearer {access_token}
```

**Response:**
```json
{
  "success": true,
  "message": "API key deleted successfully"
}
```

### POST /api/v1/user/apikey/test
Test current API key connection

**Headers:**
```
Authorization: Bearer {access_token}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "status": "connected",
    "permissions": ["trade", "info"],
    "server_time": 1704643200000
  }
}
```

---

## Internal Service Methods

### Get Decrypted Credentials (Internal Use Only)
```go
func (s *APIKeyService) GetDecryptedCredentials(userID string) (*APICredentials, error) {
    // 1. Check cache first
    cached, err := s.redis.Get(ctx, fmt.Sprintf("apikey_cache:%s", userID)).Result()
    if err == nil {
        var creds APICredentials
        json.Unmarshal([]byte(cached), &creds)
        return &creds, nil
    }
    
    // 2. Load encrypted key from Redis
    apiKey, err := s.repo.GetByUserID(userID)
    if err != nil {
        return nil, err
    }
    
    // 3. Check if active
    if !apiKey.IsActive {
        return nil, errors.New("API key is not active")
    }
    
    // 4. Decrypt key and secret
    key, err := s.crypto.Decrypt(apiKey.EncryptedKey, apiKey.Nonce)
    if err != nil {
        return nil, err
    }
    
    secret, err := s.crypto.Decrypt(apiKey.EncryptedSecret, apiKey.Nonce)
    if err != nil {
        return nil, err
    }
    
    // 5. Cache for 1 hour
    creds := &APICredentials{
        Key:    key,
        Secret: secret,
    }
    credsJSON, _ := json.Marshal(creds)
    s.redis.Set(ctx, fmt.Sprintf("apikey_cache:%s", userID), credsJSON, time.Hour)
    
    // 6. Update last_used_at
    s.repo.UpdateLastUsed(apiKey.ID)
    
    return creds, nil
}
```

### Clear Cache on Logout
```go
func (s *APIKeyService) ClearCache(userID string) error {
    return s.redis.Del(ctx, fmt.Sprintf("apikey_cache:%s", userID)).Err()
}
```

---

## Security Best Practices

### Encryption
- **Master key**: Store in environment variable, never in code
- **Key rotation**: Support key rotation without re-encrypting (use key versioning)
- **Secure random**: Use crypto/rand for nonce generation
- **Authenticated encryption**: GCM mode provides both confidentiality and integrity

### Storage
- **Never log**: Never log plaintext keys or secrets
- **Never expose**: API responses never include plaintext keys
- **Secure deletion**: Overwrite memory before releasing
- **Access control**: Only owner can access their keys

### Validation
- **Always test**: Validate API key on creation/update
- **Rate limiting**: Limit validation attempts to prevent abuse
- **Error handling**: Don't expose internal details in errors
- **Timeout**: Set reasonable timeout for validation calls

### Usage
- **Cache wisely**: Short-lived cache (1 hour max)
- **Clear on logout**: Remove from cache when user logs out
- **Update last_used**: Track when key was last used
- **Deactivate option**: Allow users to disable without deleting

---

## Error Handling

### Common Errors

**API Key Not Found:**
```json
{
  "success": false,
  "error": {
    "code": "API_KEY_NOT_FOUND",
    "message": "No API key configured for this user"
  }
}
```

**API Key Inactive:**
```json
{
  "success": false,
  "error": {
    "code": "API_KEY_INACTIVE",
    "message": "API key is disabled. Enable it to trade."
  }
}
```

**Decryption Failed:**
```json
{
  "success": false,
  "error": {
    "code": "DECRYPTION_ERROR",
    "message": "Unable to decrypt API key. Please update your API key."
  }
}
```

**Indodax API Error:**
```json
{
  "success": false,
  "error": {
    "code": "INDODAX_API_ERROR",
    "message": "Error from Indodax API",
    "details": "Invalid key or secret"
  }
}
```

---

## Admin Features

### View User's API Key Status (Admin Only)
```
GET /api/v1/admin/users/:id/apikey
```

**Response:**
```json
{
  "success": true,
  "data": {
    "has_api_key": true,
    "is_active": true,
    "label": "My Trading Key",
    "masked_key": "ABC***456",
    "last_used_at": "2024-01-07T17:30:00Z",
    "created_at": "2024-01-07T16:00:00Z"
  }
}
```

Note: Admin can see if a user has an API key but cannot see the actual key/secret.

---

## Testing Strategy

### Unit Tests
- Encryption/decryption functions
- Nonce generation uniqueness
- Master key derivation
- Error handling

### Integration Tests
- Full create/update/delete flow
- API key validation with mock Indodax API
- Cache behavior (set/get/clear)
- Concurrent access handling

### Security Tests
- Attempt to access other user's keys
- Test with tampered ciphertext
- Test with invalid nonces
- Test encryption with weak keys (should fail)

---

## Environment Variables

```env
# Master encryption key (32 bytes, base64 encoded)
MASTER_ENCRYPTION_KEY=base64encodedkey...

# Salt for key derivation (16 bytes, base64 encoded)
ENCRYPTION_SALT=base64encodedsalt...

# Cache TTL for decrypted keys (in seconds)
API_KEY_CACHE_TTL=3600
```

---

## Future Enhancements

- [ ] Multiple API keys per user (with labels)
- [ ] API key permission scopes (read-only, trade-only, etc.)
- [ ] API key expiry dates
- [ ] API key usage statistics
- [ ] Alert when API key is about to expire
- [ ] Key rotation automation
- [ ] Hardware security module (HSM) integration
- [ ] API key auditing and logging (without exposing keys)

