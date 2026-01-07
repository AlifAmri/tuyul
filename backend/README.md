# TUYUL Backend

Backend service for TUYUL crypto trading bot platform.

## Tech Stack

- **Language**: Go 1.21+
- **Framework**: Gin (HTTP router)
- **Database**: Redis 7+
- **Logger**: Zerolog
- **Architecture**: Clean Architecture

## Project Structure

```
backend/
├── cmd/
│   └── api/
│       └── main.go           # Application entry point
├── internal/
│   ├── config/               # Configuration management
│   ├── handler/              # HTTP handlers (controllers)
│   ├── middleware/           # HTTP middleware
│   ├── model/                # Data models
│   ├── repository/           # Data access layer
│   ├── service/              # Business logic
│   └── util/                 # Utility functions
├── pkg/
│   ├── crypto/               # Encryption/decryption utilities
│   ├── jwt/                  # JWT token utilities
│   ├── logger/               # Logging wrapper
│   └── redis/                # Redis client wrapper
├── .env.example              # Environment variables template
├── .env                      # Environment variables (gitignored)
├── go.mod                    # Go module definition
└── go.sum                    # Go module checksums
```

## Setup

### Prerequisites

- Go 1.21 or higher
- Redis 7 or higher

### Installation

1. Clone the repository:
```bash
git clone git@github.com:enigma-id/tuyul.git
cd tuyul/backend
```

2. Install dependencies:
```bash
go mod download
```

3. Copy `.env.example` to `.env` and configure:
```bash
cp .env.example .env
```

4. Update `.env` with your configuration (especially JWT_SECRET and ENCRYPTION_KEY)

### Running

#### Development

```bash
go run cmd/api/main.go
```

#### Production

```bash
# Build
go build -o bin/tuyul cmd/api/main.go

# Run
./bin/tuyul
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `SERVER_HOST` | Server host | `0.0.0.0` |
| `SERVER_PORT` | Server port | `8080` |
| `SERVER_ENV` | Environment (development/production) | `development` |
| `REDIS_HOST` | Redis host | `localhost` |
| `REDIS_PORT` | Redis port | `6379` |
| `REDIS_PASSWORD` | Redis password | `` |
| `REDIS_DB` | Redis database number | `0` |
| `JWT_SECRET` | JWT secret key (32+ characters) | **required** |
| `JWT_ACCESS_TOKEN_EXPIRE_MINUTES` | Access token expiry | `15` |
| `JWT_REFRESH_TOKEN_EXPIRE_DAYS` | Refresh token expiry | `7` |
| `ENCRYPTION_KEY` | API key encryption key (exactly 32 bytes) | **required** |
| `INDODAX_API_URL` | Indodax API base URL | `https://indodax.com` |
| `INDODAX_WS_URL` | Indodax public WebSocket URL | `wss://ws3.indodax.com/ws/` |
| `INDODAX_PRIVATE_WS_URL` | Indodax private WebSocket URL | `wss://pws.indodax.com/ws/` |
| `CORS_ALLOWED_ORIGINS` | CORS allowed origins (comma-separated) | `http://localhost:5173` |
| `RATE_LIMIT_REQUESTS_PER_MINUTE` | General rate limit | `60` |
| `RATE_LIMIT_AUTH_REQUESTS_PER_MINUTE` | Auth endpoints rate limit | `5` |
| `LOG_LEVEL` | Log level (debug/info/warn/error) | `info` |
| `LOG_FORMAT` | Log format (json/pretty) | `json` |

## API Endpoints

### Health Check

- **GET** `/health` - Server and Redis health status
- **GET** `/api/v1/ping` - Simple ping-pong endpoint

### Authentication (TODO)

- **POST** `/api/v1/auth/register` - Register new user
- **POST** `/api/v1/auth/login` - User login
- **POST** `/api/v1/auth/refresh` - Refresh access token
- **POST** `/api/v1/auth/logout` - User logout

### Market Analysis (TODO)

- **GET** `/api/v1/market/summary` - Get all market pairs with pump scores
- **GET** `/api/v1/market/top-pumps` - Get top pumping coins
- **GET** `/api/v1/market/top-gaps` - Get coins with best bid-ask gaps

### Trading (TODO)

- **POST** `/api/v1/trade/buy` - Place buy order
- **GET** `/api/v1/trade/orders` - Get user's orders
- **DELETE** `/api/v1/trade/orders/:id` - Cancel order

### Bot Management (TODO)

- **POST** `/api/v1/bots/market-maker` - Create Market Maker bot
- **POST** `/api/v1/bots/pump-hunter` - Create Pump Hunter bot
- **POST** `/api/v1/bots/:id/start` - Start bot
- **POST** `/api/v1/bots/:id/stop` - Stop bot

## Development

### Running Tests

```bash
go test ./...
```

### Running with Hot Reload

Install Air:
```bash
go install github.com/air-verse/air@latest
```

Run with hot reload:
```bash
air
```

### Code Style

Follow standard Go formatting:
```bash
go fmt ./...
```

Run linter:
```bash
golangci-lint run
```

## Docker

### Build Image

```bash
docker build -t tuyul-backend .
```

### Run Container

```bash
docker run -p 8080:8080 --env-file .env tuyul-backend
```

## Status

🚧 **In Development**

### Completed
- ✅ Project structure
- ✅ Configuration management
- ✅ Logging infrastructure
- ✅ Redis client wrapper
- ✅ Basic HTTP server with Gin
- ✅ Health check endpoints

### In Progress
- 🔄 Authentication & User Management
- 🔄 API Key Management
- 🔄 Market Analysis
- 🔄 Trading Automation
- 🔄 Bot Management

## License

MIT

