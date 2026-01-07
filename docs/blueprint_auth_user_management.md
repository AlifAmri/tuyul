# Authentication & User Management

## Overview

TUYUL implements a secure multi-user authentication system with role-based access control. The system supports two user roles: **Admin** and **User**, with distinct permissions and capabilities.

---

## Features

### 1. Authentication
- JWT-based stateless authentication
- Refresh token mechanism for extended sessions
- Secure password hashing (bcrypt)
- Session management via Redis

### 2. User Roles
- **Admin**: Full system access, user management, view all data
- **User**: Limited to own data, trading, and market analysis

### 3. User Management (Admin Only)
- Create new users
- Update user information
- Delete/deactivate users
- View all users
- Reset user passwords

---

## Data Models

### User Model
```go
type User struct {
    ID           string    `json:"id"`           // UUID
    Username     string    `json:"username"`     // Unique, lowercase
    Email        string    `json:"email"`        // Unique, validated
    PasswordHash string    `json:"-"`            // Never exposed in JSON
    Role         string    `json:"role"`         // "admin" or "user"
    Status       string    `json:"status"`       // "active", "inactive", "suspended"
    CreatedAt    time.Time `json:"created_at"`
    UpdatedAt    time.Time `json:"updated_at"`
    LastLoginAt  time.Time `json:"last_login_at"`
}
```

### Session Model
```go
type Session struct {
    UserID       string    `json:"user_id"`
    AccessToken  string    `json:"access_token"`
    RefreshToken string    `json:"refresh_token"`
    ExpiresAt    time.Time `json:"expires_at"`
    CreatedAt    time.Time `json:"created_at"`
}
```

---

## Redis Schema

### Users Storage
```
Key Pattern: user:{user_id}
Type: Hash
Fields:
  - id: string
  - username: string
  - email: string
  - password_hash: string
  - role: string
  - status: string
  - created_at: timestamp
  - updated_at: timestamp
  - last_login_at: timestamp

Index Keys:
  - username_index:{username} → {user_id}
  - email_index:{email} → {user_id}
```

### Sessions Storage
```
Key Pattern: session:{user_id}
Type: String (JSON)
Value: {
  "user_id": "uuid",
  "access_token": "jwt",
  "refresh_token": "jwt",
  "expires_at": "timestamp"
}
TTL: 7 days (matches refresh token expiry)
```

### Active Tokens (Blacklist for logout)
```
Key Pattern: token_blacklist:{token_hash}
Type: String
Value: "1"
TTL: Same as token expiry
```

---

## Authentication Flow

### 1. Registration Flow (Admin Creates User)
```
Admin → POST /api/v1/admin/users
        ↓
    Validate input (username, email, password)
        ↓
    Check username/email uniqueness
        ↓
    Hash password (bcrypt, cost 12)
        ↓
    Generate user ID (UUID)
        ↓
    Store user in Redis
        ↓
    Create username/email indices
        ↓
    Return user object (without password)
```

### 2. Login Flow
```
User → POST /api/v1/auth/login
       ↓
   Validate credentials (username/email + password)
       ↓
   Verify password hash
       ↓
   Check user status (must be "active")
       ↓
   Generate access token (JWT, 15 min)
       ↓
   Generate refresh token (JWT, 7 days)
       ↓
   Store session in Redis
       ↓
   Update last_login_at
       ↓
   Return tokens + user info
```

### 3. Token Refresh Flow
```
User → POST /api/v1/auth/refresh
       ↓
   Validate refresh token (JWT signature + expiry)
       ↓
   Check token not in blacklist
       ↓
   Verify session exists in Redis
       ↓
   Generate new access token
       ↓
   Update session in Redis
       ↓
   Return new access token
```

### 4. Logout Flow
```
User → POST /api/v1/auth/logout
       ↓
   Extract access token from header
       ↓
   Add token to blacklist (Redis)
       ↓
   Delete session from Redis
       ↓
   Return success
```

---

## JWT Token Structure

### Access Token (15 minutes)
```json
{
  "sub": "user_id",
  "username": "john_doe",
  "email": "john@example.com",
  "role": "user",
  "type": "access",
  "iat": 1704672000,
  "exp": 1704672900
}
```

### Refresh Token (7 days)
```json
{
  "sub": "user_id",
  "type": "refresh",
  "iat": 1704672000,
  "exp": 1705276800
}
```

---

## API Endpoints

### Public Endpoints

#### POST /api/v1/auth/login
Login with credentials

**Request:**
```json
{
  "username": "john_doe",  // or email
  "password": "secure_password123"
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "user": {
      "id": "uuid",
      "username": "john_doe",
      "email": "john@example.com",
      "role": "user",
      "status": "active"
    },
    "access_token": "eyJhbGc...",
    "refresh_token": "eyJhbGc...",
    "expires_in": 900
  }
}
```

#### POST /api/v1/auth/refresh
Refresh access token

**Request:**
```json
{
  "refresh_token": "eyJhbGc..."
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "access_token": "eyJhbGc...",
    "expires_in": 900
  }
}
```

---

### Protected Endpoints

#### POST /api/v1/auth/logout
Logout current session

**Headers:**
```
Authorization: Bearer {access_token}
```

**Response:**
```json
{
  "success": true,
  "message": "Logged out successfully"
}
```

#### GET /api/v1/auth/me
Get current user info

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
    "username": "john_doe",
    "email": "john@example.com",
    "role": "user",
    "status": "active",
    "created_at": "2024-01-07T10:00:00Z",
    "last_login_at": "2024-01-07T15:30:00Z"
  }
}
```

---

### Admin-Only Endpoints

#### GET /api/v1/admin/users
List all users

**Query Parameters:**
- `page`: int (default: 1)
- `limit`: int (default: 20, max: 100)
- `role`: string (filter by role)
- `status`: string (filter by status)
- `search`: string (search username/email)

**Response:**
```json
{
  "success": true,
  "data": {
    "users": [
      {
        "id": "uuid",
        "username": "john_doe",
        "email": "john@example.com",
        "role": "user",
        "status": "active",
        "created_at": "2024-01-07T10:00:00Z",
        "last_login_at": "2024-01-07T15:30:00Z"
      }
    ],
    "pagination": {
      "page": 1,
      "limit": 20,
      "total": 45
    }
  }
}
```

#### POST /api/v1/admin/users
Create new user

**Request:**
```json
{
  "username": "new_user",
  "email": "newuser@example.com",
  "password": "secure_password123",
  "role": "user"  // "admin" or "user"
}
```

**Validation:**
- Username: 3-30 chars, alphanumeric + underscore, lowercase
- Email: Valid email format
- Password: Min 8 chars, must contain uppercase, lowercase, number
- Role: Must be "admin" or "user"

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "username": "new_user",
    "email": "newuser@example.com",
    "role": "user",
    "status": "active",
    "created_at": "2024-01-07T16:00:00Z"
  }
}
```

#### GET /api/v1/admin/users/:id
Get user by ID

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "username": "john_doe",
    "email": "john@example.com",
    "role": "user",
    "status": "active",
    "created_at": "2024-01-07T10:00:00Z",
    "updated_at": "2024-01-07T10:00:00Z",
    "last_login_at": "2024-01-07T15:30:00Z",
    "has_api_key": true
  }
}
```

#### PUT /api/v1/admin/users/:id
Update user

**Request:**
```json
{
  "email": "newemail@example.com",  // optional
  "role": "admin",                   // optional
  "status": "active"                 // optional: "active", "inactive", "suspended"
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "username": "john_doe",
    "email": "newemail@example.com",
    "role": "admin",
    "status": "active",
    "updated_at": "2024-01-07T16:30:00Z"
  }
}
```

#### DELETE /api/v1/admin/users/:id
Delete user (soft delete - sets status to "inactive")

**Response:**
```json
{
  "success": true,
  "message": "User deleted successfully"
}
```

#### POST /api/v1/admin/users/:id/reset-password
Reset user password

**Request:**
```json
{
  "new_password": "new_secure_password123"
}
```

**Response:**
```json
{
  "success": true,
  "message": "Password reset successfully"
}
```

---

## Middleware

### Authentication Middleware
```go
// Verifies JWT token and adds user info to context
func AuthMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        // Extract token from Authorization header
        // Verify token signature and expiry
        // Check token not in blacklist
        // Decode claims and add to context
        // Call next handler or return 401
    }
}
```

### Authorization Middleware
```go
// Checks user role for admin-only endpoints
func AdminOnlyMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        // Get user from context
        // Check if role == "admin"
        // Call next handler or return 403
    }
}
```

### Rate Limiting Middleware
```go
// Prevents brute force attacks on login
func RateLimitMiddleware(limit int, window time.Duration) gin.HandlerFunc {
    return func(c *gin.Context) {
        // Get client IP or user ID
        // Check request count in Redis
        // Increment counter with expiry
        // Call next handler or return 429
    }
}
```

---

## Security Best Practices

### Password Security
- **Hashing**: bcrypt with cost factor 12
- **Complexity**: Min 8 chars, mixed case, numbers
- **Never store plaintext**: Always hash before storage
- **Never log passwords**: Sanitize logs

### Token Security
- **Short-lived access tokens**: 15 minutes
- **Secure refresh tokens**: 7 days, one-time use
- **Token rotation**: Generate new refresh token on each use
- **Blacklist on logout**: Prevent token reuse

### Session Security
- **Redis TTL**: Auto-expire old sessions
- **Session fingerprinting**: Bind to IP (optional)
- **Concurrent sessions**: Allow or restrict (configurable)

### API Security
- **HTTPS only**: All traffic over TLS in production
- **CORS**: Restrict to known origins
- **Rate limiting**: Prevent abuse
- **Input validation**: Validate all user input
- **SQL injection**: N/A (using Redis)

---

## Error Handling

### Common Error Responses

**401 Unauthorized:**
```json
{
  "success": false,
  "error": {
    "code": "UNAUTHORIZED",
    "message": "Invalid or expired token"
  }
}
```

**403 Forbidden:**
```json
{
  "success": false,
  "error": {
    "code": "FORBIDDEN",
    "message": "Admin access required"
  }
}
```

**409 Conflict:**
```json
{
  "success": false,
  "error": {
    "code": "USER_EXISTS",
    "message": "Username or email already exists"
  }
}
```

**422 Validation Error:**
```json
{
  "success": false,
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Invalid input",
    "details": {
      "username": ["Must be 3-30 characters"],
      "password": ["Must contain at least one uppercase letter"]
    }
  }
}
```

---

## Testing Considerations

### Unit Tests
- Password hashing/verification
- JWT generation/validation
- Input validation
- Role-based access logic

### Integration Tests
- Full login/logout flow
- Token refresh flow
- User CRUD operations (admin)
- Authentication middleware
- Authorization middleware

### Security Tests
- Brute force protection
- Token expiry enforcement
- Role escalation prevention
- SQL injection attempts (N/A)
- XSS prevention in user input

---

## Initial Setup

### Default Admin User
On first run, create a default admin user:

```
Username: admin
Password: [from environment variable]
Email: admin@tuyul.local
Role: admin
Status: active
```

Admin should change password on first login.

---

## Future Enhancements

- [ ] Two-factor authentication (2FA)
- [ ] OAuth2 integration (Google, GitHub)
- [ ] API key authentication (for programmatic access)
- [ ] Session management dashboard
- [ ] Password reset via email
- [ ] Account recovery mechanisms
- [ ] Audit log for admin actions
- [ ] IP whitelisting/blacklisting

