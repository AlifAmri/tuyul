# API Endpoints Documentation

## Base URL

```
Development: http://localhost:8080
Production: https://api.tuyul.com
```

## API Version

Current version: `v1`

All endpoints are prefixed with `/api/v1`

---

## Authentication

Most endpoints require JWT authentication.

### Headers
```
Authorization: Bearer {access_token}
Content-Type: application/json
```

### Rate Limits
- **Public endpoints**: 60 requests/minute per IP
- **Authenticated endpoints**: 180 requests/minute per user
- **Trading endpoints**: 20 requests/second per user
- **Admin endpoints**: 300 requests/minute

---

## Response Format

### Success Response
```json
{
  "success": true,
  "data": {
    // Response data
  },
  "message": "Optional success message"
}
```

### Error Response
```json
{
  "success": false,
  "error": {
    "code": "ERROR_CODE",
    "message": "Human-readable error message",
    "details": "Additional error details (optional)"
  }
}
```

### Pagination Format
```json
{
  "success": true,
  "data": {
    "items": [...],
    "pagination": {
      "page": 1,
      "limit": 20,
      "total": 100,
      "total_pages": 5
    }
  }
}
```

---

## Error Codes

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `UNAUTHORIZED` | 401 | Invalid or missing authentication token |
| `FORBIDDEN` | 403 | Insufficient permissions |
| `NOT_FOUND` | 404 | Resource not found |
| `VALIDATION_ERROR` | 422 | Invalid input data |
| `USER_EXISTS` | 409 | Username or email already exists |
| `INVALID_CREDENTIALS` | 401 | Wrong username/password |
| `API_KEY_NOT_FOUND` | 404 | No API key configured |
| `API_KEY_INACTIVE` | 403 | API key is disabled |
| `INVALID_API_KEY` | 422 | Invalid Indodax API credentials |
| `INSUFFICIENT_BALANCE` | 422 | Not enough balance for trade |
| `ORDER_NOT_FOUND` | 404 | Order does not exist |
| `CANNOT_CANCEL` | 422 | Order cannot be cancelled |
| `INDODAX_API_ERROR` | 502 | Error from Indodax API |
| `RATE_LIMIT_EXCEEDED` | 429 | Too many requests |
| `INTERNAL_ERROR` | 500 | Internal server error |

---

## Endpoints

## 1. Authentication

### POST /api/v1/auth/login
Login with credentials

**Public endpoint**

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

**Errors:**
- `INVALID_CREDENTIALS`: Wrong username or password
- `VALIDATION_ERROR`: Missing required fields
- `RATE_LIMIT_EXCEEDED`: Too many login attempts

---

### POST /api/v1/auth/refresh
Refresh access token

**Public endpoint**

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

**Errors:**
- `UNAUTHORIZED`: Invalid or expired refresh token

---

### POST /api/v1/auth/logout
Logout current session

**Requires authentication**

**Response:**
```json
{
  "success": true,
  "message": "Logged out successfully"
}
```

---

### GET /api/v1/auth/me
Get current user info

**Requires authentication**

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
    "last_login_at": "2024-01-07T15:30:00Z",
    "has_api_key": true
  }
}
```

---

## 2. User Management (Admin Only)

### GET /api/v1/admin/users
List all users

**Requires admin role**

**Query Parameters:**
- `page`: int (default: 1)
- `limit`: int (default: 20, max: 100)
- `role`: string (filter: "admin", "user")
- `status`: string (filter: "active", "inactive", "suspended")
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
        "has_api_key": true,
        "created_at": "2024-01-07T10:00:00Z",
        "last_login_at": "2024-01-07T15:30:00Z"
      }
    ],
    "pagination": {
      "page": 1,
      "limit": 20,
      "total": 45,
      "total_pages": 3
    }
  }
}
```

---

### POST /api/v1/admin/users
Create new user

**Requires admin role**

**Request:**
```json
{
  "username": "new_user",
  "email": "newuser@example.com",
  "password": "secure_password123",
  "role": "user"  // "admin" or "user"
}
```

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

**Errors:**
- `USER_EXISTS`: Username or email already exists
- `VALIDATION_ERROR`: Invalid input data

---

### GET /api/v1/admin/users/:id
Get user by ID

**Requires admin role**

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
    "has_api_key": true,
    "created_at": "2024-01-07T10:00:00Z",
    "updated_at": "2024-01-07T10:00:00Z",
    "last_login_at": "2024-01-07T15:30:00Z",
    "total_orders": 45,
    "active_orders": 3
  }
}
```

---

### PUT /api/v1/admin/users/:id
Update user

**Requires admin role**

**Request:**
```json
{
  "email": "newemail@example.com",  // optional
  "role": "admin",                   // optional
  "status": "active"                 // optional
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

---

### DELETE /api/v1/admin/users/:id
Delete user (soft delete)

**Requires admin role**

**Response:**
```json
{
  "success": true,
  "message": "User deleted successfully"
}
```

---

### POST /api/v1/admin/users/:id/reset-password
Reset user password

**Requires admin role**

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

### GET /api/v1/admin/users/:id/apikey
View user's API key status

**Requires admin role**

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

---

## 3. API Key Management

### POST /api/v1/user/apikey
Create or update user's API key

**Requires authentication**

**Request:**
```json
{
  "key": "ABCDEF123456",
  "secret": "secret123456789abcdef",
  "label": "My Trading Key"  // optional
}
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
    "created_at": "2024-01-07T16:00:00Z"
  },
  "message": "API key validated and saved successfully"
}
```

**Errors:**
- `INVALID_API_KEY`: Invalid Indodax credentials
- `VALIDATION_ERROR`: Connection to Indodax failed

---

### GET /api/v1/user/apikey
Get user's API key info

**Requires authentication**

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
    "masked_key": "ABC***456",
    "last_used_at": "2024-01-07T17:30:00Z",
    "created_at": "2024-01-07T16:00:00Z",
    "updated_at": "2024-01-07T16:00:00Z"
  }
}
```

---

### PUT /api/v1/user/apikey/status
Toggle API key active status

**Requires authentication**

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

---

### DELETE /api/v1/user/apikey
Delete user's API key

**Requires authentication**

**Response:**
```json
{
  "success": true,
  "message": "API key deleted successfully"
}
```

---

### POST /api/v1/user/apikey/test
Test current API key connection

**Requires authentication**

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

## 4. Market Analysis

### GET /api/v1/market/summary
Get all market summaries

**Requires authentication**

**Query Parameters:**
- `limit`: int (default: 50, max: 200)
- `min_volume`: float (minimum volume in IDR, default: 0)

**Response:**
```json
{
  "success": true,
  "data": {
    "markets": [
      {
        "pair": "btcidr",
        "base_currency": "btc",
        "quote_currency": "idr",
        "last_price": 650000000,
        "high_24h": 670000000,
        "low_24h": 640000000,
        "volume_24h": 12.5,
        "volume_idr": 8125000000,
        "best_bid": 649500000,
        "best_ask": 650500000,
        "change_24h": 1.5,
        "timestamp": "2024-01-07T18:00:00Z"
      }
    ],
    "count": 50,
    "last_update": "2024-01-07T18:00:00Z"
  }
}
```

---

### GET /api/v1/market/pump-scores
Get markets sorted by pump score

**Requires authentication**

**Query Parameters:**
- `limit`: int (default: 20, max: 100)
- `min_score`: float (minimum pump score, default: 0)
- `min_volume`: float (minimum volume in IDR, default: 1000000)

**Response:**
```json
{
  "success": true,
  "data": {
    "scores": [
      {
        "pair": "shibidr",
        "symbol": "shib",
        "score": 87.5,
        "last_price": 0.00085,
        "change_24h": 25.3,
        "volume_24h": 50000000000,
        "volume_idr": 42500000,
        "volume_ratio": 3.2,
        "volatility": 28.5,
        "timestamp": "2024-01-07T18:00:00Z"
      }
    ],
    "count": 20
  }
}
```

---

### GET /api/v1/market/gaps
Get markets sorted by bid-ask gap

**Requires authentication**

**Query Parameters:**
- `limit`: int (default: 20, max: 100)
- `min_gap`: float (minimum gap percentage, default: 0)
- `min_volume`: float (minimum volume in IDR, default: 10000000)

**Response:**
```json
{
  "success": true,
  "data": {
    "gaps": [
      {
        "pair": "ethidr",
        "symbol": "eth",
        "gap_percentage": 0.85,
        "best_bid": 35000000,
        "best_ask": 35300000,
        "spread": 300000,
        "volume_24h": 350.2,
        "volume_idr": 12257000000,
        "timestamp": "2024-01-07T18:00:00Z"
      }
    ],
    "count": 20
  }
}
```

---

### GET /api/v1/market/:pair
Get specific market data

**Requires authentication**

**Response:**
```json
{
  "success": true,
  "data": {
    "pair": "btcidr",
    "base_currency": "btc",
    "quote_currency": "idr",
    "last_price": 650000000,
    "high_24h": 670000000,
    "low_24h": 640000000,
    "volume_24h": 12.5,
    "volume_idr": 8125000000,
    "best_bid": 649500000,
    "best_ask": 650500000,
    "change_24h": 1.5,
    "pump_score": 45.2,
    "gap_percentage": 0.15,
    "timestamp": "2024-01-07T18:00:00Z"
  }
}
```

---

## 5. Trading

### POST /api/v1/trade/buy
Place automated buy order

**Requires authentication & API key**

**Request:**
```json
{
  "pair": "btcidr",
  "buying_price": 650000000,
  "volume_idr": 1000000,
  "target_profit": 5.0,
  "stop_loss": 3.0
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "pair": "btcidr",
    "side": "buy",
    "order_type": "limit",
    "price": 650000000,
    "amount": 0.00153846,
    "amount_idr": 1000000,
    "status": "open",
    "indodax_order_id": "123456789",
    "target_profit": 5.0,
    "stop_loss": 3.0,
    "created_at": "2024-01-07T18:30:00Z"
  },
  "message": "Buy order placed successfully"
}
```

**Errors:**
- `INSUFFICIENT_BALANCE`: Not enough IDR balance
- `VALIDATION_ERROR`: Invalid input parameters
- `API_KEY_NOT_FOUND`: No API key configured
- `API_KEY_INACTIVE`: API key is disabled
- `INDODAX_API_ERROR`: Error from Indodax

---

### GET /api/v1/trade/orders
Get user's orders

**Requires authentication**

**Query Parameters:**
- `status`: string (filter: "open", "filled", "cancelled", "all") - default: "all"
- `pair`: string (filter by pair)
- `limit`: int (default: 20, max: 100)
- `offset`: int (default: 0)

**Response:**
```json
{
  "success": true,
  "data": {
    "orders": [
      {
        "id": "uuid",
        "pair": "btcidr",
        "side": "buy",
        "order_type": "limit",
        "price": 650000000,
        "amount": 0.00153846,
        "status": "filled",
        "target_profit": 5.0,
        "stop_loss": 3.0,
        "sell_order_id": "uuid-2",
        "created_at": "2024-01-07T18:30:00Z",
        "filled_at": "2024-01-07T18:35:00Z"
      }
    ],
    "pagination": {
      "limit": 20,
      "offset": 0,
      "total": 45
    }
  }
}
```

---

### GET /api/v1/trade/orders/:id
Get specific order details

**Requires authentication**

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "pair": "btcidr",
    "side": "buy",
    "order_type": "limit",
    "price": 650000000,
    "amount": 0.00153846,
    "filled": 0.00153846,
    "status": "filled",
    "target_profit": 5.0,
    "stop_loss": 3.0,
    "sell_order_id": "uuid-2",
    "created_at": "2024-01-07T18:30:00Z",
    "filled_at": "2024-01-07T18:35:00Z",
    "linked_orders": {
      "sell_order": {
        "id": "uuid-2",
        "status": "open",
        "price": 682500000
      }
    }
  }
}
```

---

### DELETE /api/v1/trade/orders/:id
Cancel order

**Requires authentication**

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "status": "cancelled",
    "cancelled_at": "2024-01-07T19:00:00Z"
  },
  "message": "Order cancelled successfully"
}
```

**Errors:**
- `ORDER_NOT_FOUND`: Order does not exist
- `CANNOT_CANCEL`: Order cannot be cancelled (already filled/cancelled)
- `INDODAX_API_ERROR`: Error cancelling order on Indodax

---

### POST /api/v1/trade/orders/:id/sell
Manually sell at market price

**Requires authentication**

**Response:**
```json
{
  "success": true,
  "data": {
    "old_sell_order_id": "uuid-2",
    "old_sell_order_status": "cancelled",
    "new_sell_order": {
      "id": "uuid-3",
      "order_type": "market",
      "amount": 0.00153846,
      "status": "pending",
      "created_at": "2024-01-07T19:05:00Z"
    }
  },
  "message": "Market sell order placed"
}
```

---

### GET /api/v1/trade/balance
Get user's balance

**Requires authentication & API key**

**Response:**
```json
{
  "success": true,
  "data": {
    "balances": [
      {
        "currency": "idr",
        "available": 5000000.0,
        "frozen": 1000000.0,
        "total": 6000000.0
      },
      {
        "currency": "btc",
        "available": 0.00153846,
        "frozen": 0,
        "total": 0.00153846
      }
    ],
    "last_update": "2024-01-07T19:10:00Z"
  }
}
```

---

## 6. System (Admin Only)

### GET /api/v1/admin/stats
Get system statistics

**Requires admin role**

**Response:**
```json
{
  "success": true,
  "data": {
    "total_users": 150,
    "active_users": 120,
    "total_orders": 5420,
    "active_orders": 85,
    "orders_today": 245,
    "volume_today_idr": 45000000000,
    "connected_websockets": 42,
    "system_uptime": 864000,
    "redis_memory_mb": 125.5
  }
}
```

---

### GET /api/v1/admin/logs
Get system logs

**Requires admin role**

**Query Parameters:**
- `level`: string (filter: "info", "warning", "error")
- `limit`: int (default: 100, max: 1000)
- `offset`: int (default: 0)

**Response:**
```json
{
  "success": true,
  "data": {
    "logs": [
      {
        "timestamp": "2024-01-07T19:15:00Z",
        "level": "error",
        "message": "Failed to place order",
        "details": "Insufficient balance",
        "user_id": "uuid"
      }
    ],
    "pagination": {
      "limit": 100,
      "offset": 0,
      "total": 5000
    }
  }
}
```

---

## 7. Health Check

### GET /health
System health check

**Public endpoint**

**Response:**
```json
{
  "status": "healthy",
  "timestamp": "2024-01-07T19:20:00Z",
  "uptime": 864000,
  "version": "1.0.0",
  "services": {
    "redis": "healthy",
    "indodax_ws": "healthy",
    "indodax_api": "healthy"
  }
}
```

---

### GET /api/v1/version
API version info

**Public endpoint**

**Response:**
```json
{
  "version": "1.0.0",
  "api_version": "v1",
  "build_time": "2024-01-07T10:00:00Z",
  "git_commit": "abc123def"
}
```

---

## WebSocket API

### Connection URL
```
wss://api.tuyul.com/ws?token={jwt_access_token}
```

### Authentication
Token passed as query parameter.

### Message Format

#### Subscribe to Market Updates
```json
{
  "action": "subscribe",
  "channel": "market_summary",
  "pairs": ["btcidr", "ethidr"]  // empty = all pairs
}
```

#### Unsubscribe
```json
{
  "action": "unsubscribe",
  "channel": "market_summary",
  "pairs": ["btcidr"]
}
```

#### Market Update (Server → Client)
```json
{
  "type": "market_update",
  "data": {
    "pair": "btcidr",
    "last_price": 650500000,
    "change_24h": 1.58,
    "volume_idr": 8150000000,
    "timestamp": "2024-01-07T18:00:30Z"
  }
}
```

#### Order Update (Server → Client)
```json
{
  "type": "order_update",
  "data": {
    "order_id": "uuid",
    "status": "filled",
    "filled": 0.00153846,
    "filled_at": "2024-01-07T18:35:00Z"
  }
}
```

#### Stop-Loss Triggered (Server → Client)
```json
{
  "type": "stop_loss_triggered",
  "data": {
    "order_id": "uuid",
    "pair": "btcidr",
    "buy_price": 650000000,
    "current_price": 630500000,
    "stop_loss_percentage": 3.0,
    "action": "Market sell order placed"
  }
}
```

#### Ping/Pong (Heartbeat)
```json
// Client → Server
{ "action": "ping" }

// Server → Client
{ "type": "pong", "timestamp": 1704729600000 }
```

---

## Postman Collection

A Postman collection is available at `/docs/postman/tuyul-api.json` with pre-configured requests for all endpoints.

---

## SDK Examples

### JavaScript/TypeScript
```typescript
import axios from 'axios';

const api = axios.create({
  baseURL: 'https://api.tuyul.com/api/v1',
  headers: {
    'Content-Type': 'application/json'
  }
});

// Login
const { data } = await api.post('/auth/login', {
  username: 'john_doe',
  password: 'password123'
});

const token = data.data.access_token;

// Authenticated request
const orders = await api.get('/trade/orders', {
  headers: {
    'Authorization': `Bearer ${token}`
  }
});
```

### Go
```go
package main

import (
    "bytes"
    "encoding/json"
    "net/http"
)

func main() {
    // Login
    loginData := map[string]string{
        "username": "john_doe",
        "password": "password123",
    }
    
    jsonData, _ := json.Marshal(loginData)
    resp, _ := http.Post(
        "https://api.tuyul.com/api/v1/auth/login",
        "application/json",
        bytes.NewBuffer(jsonData),
    )
    
    var result map[string]interface{}
    json.NewDecoder(resp.Body).Decode(&result)
    token := result["data"].(map[string]interface{})["access_token"].(string)
    
    // Authenticated request
    req, _ := http.NewRequest("GET", "https://api.tuyul.com/api/v1/trade/orders", nil)
    req.Header.Set("Authorization", "Bearer "+token)
    client := &http.Client{}
    resp, _ = client.Do(req)
}
```

---

## Testing

### Running Tests
```bash
# Unit tests
go test ./...

# Integration tests
go test -tags=integration ./...

# API tests with newman (Postman)
newman run docs/postman/tuyul-api.json
```

---

## Changelog

### v1.0.0 (2024-01-07)
- Initial API release
- Authentication endpoints
- User management (admin)
- API key management
- Market analysis endpoints
- Trading automation
- WebSocket support

