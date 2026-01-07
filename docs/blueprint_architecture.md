# TUYUL - Crypto Trading Bot Architecture

## Project Overview

**TUYUL** is a multi-user crypto trading bot for Indodax exchange with automated trading capabilities, market analysis, and real-time monitoring.

### Core Objectives
- Multi-user support with admin privileges
- Secure API key management with encryption
- Real-time market analysis and scoring
- Automated trading with profit targets and stop-loss
- Real-time order monitoring and management

---

## Tech Stack

### Backend
- **Language**: Go 1.21+
- **Framework**: Custom HTTP/WebSocket server
- **Database**: Redis (primary data store)
- **Authentication**: JWT tokens
- **Encryption**: AES-256-GCM for API keys
- **External APIs**: Indodax Public & Private REST + WebSocket APIs

### Frontend
- **Framework**: React 18+ with TypeScript
- **Build Tool**: Vite
- **Styling**: TailwindCSS
- **UI Components**: Radix UI
- **State Management**: Zustand + React Query
- **WebSocket**: Native WebSocket API with auto-reconnect
- **Routing**: React Router v6

### Infrastructure
- **Cache & Data Store**: Redis 7+
- **Session Store**: Redis with TTL
- **Real-time Pub/Sub**: Redis Pub/Sub
- **Rate Limiting**: Redis with sliding window

---

## High-Level Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                         FRONTEND (React)                     │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │   Dashboard  │  │   Market     │  │    Trading   │      │
│  │              │  │   Analysis   │  │    Orders    │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
│         │                  │                  │              │
│         └──────────────────┴──────────────────┘              │
│                           │                                  │
│                    WebSocket + REST API                      │
└───────────────────────────┬─────────────────────────────────┘
                            │
┌───────────────────────────┴─────────────────────────────────┐
│                    BACKEND (Golang)                          │
│                                                               │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │  HTTP Server │  │  WS Server   │  │   Auth       │      │
│  │  (REST API)  │  │  (Real-time) │  │   Service    │      │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘      │
│         │                  │                  │              │
│  ┌──────┴──────────────────┴──────────────────┴───────┐     │
│  │              Service Layer                          │     │
│  │  • User Service    • Trading Service                │     │
│  │  • API Key Service • Market Analysis Service        │     │
│  │  • Order Service   • WebSocket Manager              │     │
│  └──────┬──────────────────┬──────────────────┬───────┘     │
│         │                  │                  │              │
│  ┌──────┴───────┐   ┌──────┴───────┐   ┌─────┴────────┐    │
│  │   Redis      │   │   Indodax    │   │  Encryption  │    │
│  │   Client     │   │   API Client │   │  Service     │    │
│  └──────────────┘   └──────────────┘   └──────────────┘    │
└───────────────────────────┬─────────────────────────────────┘
                            │
┌───────────────────────────┴─────────────────────────────────┐
│                         REDIS                                │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │    Users     │  │  API Keys    │  │   Sessions   │      │
│  │    Hash      │  │   Hash       │  │    String    │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │   Orders     │  │  Market Data │  │   Pub/Sub    │      │
│  │   Hash       │  │   Sorted Set │  │   Channels   │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
└───────────────────────────────────────────────────────────────┘
                            │
┌───────────────────────────┴─────────────────────────────────┐
│                    INDODAX EXCHANGE                          │
│  • Public REST API    • Private REST API                     │
│  • Public WebSocket   • Private WebSocket                    │
└───────────────────────────────────────────────────────────────┘
```

---

## System Components

### 1. Authentication & Authorization
- JWT-based authentication
- Role-based access control (Admin, User)
- Secure session management
- API key encryption/decryption

### 2. Market Data Engine
- Real-time WebSocket connection to Indodax public WS
- Market summary aggregation
- Pump score calculation
- Bid-ask gap analysis with volume filtering

### 3. Trading Engine
- Automated order placement (buy/sell)
- Order lifecycle management
- Target profit and stop-loss execution
- Private WebSocket for order updates

### 4. Order Management
- Active orders tracking
- Order history
- Manual order cancellation
- Automatic sell order placement on fill

### 5. User Management
- Multi-user support
- Admin dashboard
- User CRUD operations
- API key validation and storage

---

## Data Flow

### Market Analysis Flow
```
Indodax Public WS → Market Data Service → Redis (Sorted Sets) 
                                         ↓
                              WebSocket Broadcast → Frontend
```

### Trading Flow
```
User Action → REST API → Trading Service → Validate Balance
                                         ↓
                              Place Buy Order → Indodax API
                                         ↓
                              Store Order → Redis
                                         ↓
                         Subscribe Private WS → Order Updates
                                         ↓
                         On Fill → Auto Place Sell Order
                                         ↓
                         Broadcast Update → Frontend WS
```

### Authentication Flow
```
Login Request → Auth Service → Validate Credentials (Redis)
                                         ↓
                              Generate JWT → Store Session (Redis)
                                         ↓
                              Return Token → Frontend
```

---

## Project Structure

```
tuyul/
├── backend/
│   ├── cmd/
│   │   └── server/
│   │       └── main.go                 # Application entry point
│   ├── internal/
│   │   ├── config/                     # Configuration management
│   │   ├── domain/                     # Domain models & interfaces
│   │   │   ├── user.go
│   │   │   ├── apikey.go
│   │   │   ├── order.go
│   │   │   └── market.go
│   │   ├── service/                    # Business logic
│   │   │   ├── auth/
│   │   │   ├── user/
│   │   │   ├── apikey/
│   │   │   ├── market/
│   │   │   ├── trading/
│   │   │   └── websocket/
│   │   ├── repository/                 # Data access layer
│   │   │   └── redis/
│   │   ├── handler/                    # HTTP handlers
│   │   │   ├── http/
│   │   │   └── websocket/
│   │   ├── middleware/                 # HTTP middlewares
│   │   ├── client/                     # External API clients
│   │   │   └── indodax/
│   │   └── crypto/                     # Encryption utilities
│   ├── pkg/                            # Shared utilities
│   │   ├── logger/
│   │   ├── validator/
│   │   └── response/
│   ├── go.mod
│   └── go.sum
├── frontend/
│   ├── src/
│   │   ├── components/                 # Reusable UI components
│   │   │   ├── ui/                     # Radix UI wrappers
│   │   │   ├── layout/
│   │   │   └── common/
│   │   ├── features/                   # Feature-specific components
│   │   │   ├── auth/
│   │   │   ├── dashboard/
│   │   │   ├── market/
│   │   │   ├── trading/
│   │   │   └── admin/
│   │   ├── hooks/                      # Custom React hooks
│   │   ├── services/                   # API clients
│   │   │   ├── api.ts
│   │   │   └── websocket.ts
│   │   ├── stores/                     # Zustand stores
│   │   ├── types/                      # TypeScript types
│   │   ├── utils/                      # Utilities
│   │   ├── App.tsx
│   │   └── main.tsx
│   ├── package.json
│   ├── vite.config.ts
│   └── tailwind.config.js
├── docs/
│   ├── ARCHITECTURE.md                 # This file
│   ├── AUTH_USER_MANAGEMENT.md
│   ├── API_KEY_MANAGEMENT.md
│   ├── MARKET_ANALYSIS.md
│   ├── TRADING_AUTOMATION.md
│   ├── DATABASE_SCHEMA.md
│   ├── API_ENDPOINTS.md
│   └── FRONTEND_ARCHITECTURE.md
└── README.md
```

---

## Security Considerations

### API Key Protection
- **Encryption**: AES-256-GCM with unique nonce per key
- **Key Derivation**: PBKDF2 for master encryption key
- **Storage**: Encrypted keys stored in Redis
- **Decryption**: Only in-memory during API calls
- **Never logged**: Sensitive data never appears in logs

### Authentication
- **JWT Tokens**: Short-lived access tokens (15 min)
- **Refresh Tokens**: Longer-lived (7 days), stored in Redis
- **HTTPS Only**: All production traffic over TLS
- **Rate Limiting**: Per-user and per-endpoint limits

### Authorization
- **Role-based**: Admin vs User permissions
- **Resource ownership**: Users can only access their own data
- **API key isolation**: Each user's API keys are isolated

---

## Performance Considerations

### Caching Strategy
- Market data: TTL 1-5 seconds
- User sessions: TTL 15 minutes
- API key cache: TTL 1 hour (encrypted)
- Order cache: Real-time updates via pub/sub

### WebSocket Optimization
- Connection pooling for Indodax WS
- Selective subscription (only active pairs)
- Message batching for frontend broadcasts
- Auto-reconnect with exponential backoff

### Rate Limiting
- Respect Indodax rate limits:
  - Public API: 180 req/min
  - Trade API: 20 req/sec
  - Cancel API: 30 req/sec
- Implement circuit breaker pattern
- Queue system for order management

---

## Scalability

### Horizontal Scaling
- Stateless backend services
- Redis as shared state store
- WebSocket connection manager with Redis pub/sub
- Load balancer for multiple backend instances

### Data Partitioning
- User data: Hash by user ID
- Market data: Hash by pair symbol
- Orders: Hash by user ID + order ID

---

## Monitoring & Observability

### Metrics
- Order placement success/failure rate
- API response times
- WebSocket connection count
- Redis operation latency
- Indodax API call rates

### Logging
- Structured JSON logging
- Request/response correlation IDs
- Error tracking with stack traces
- Audit logs for admin actions

---

## Deployment

### Environment Variables
```env
# Server
PORT=8080
WS_PORT=8081
ENV=production

# Redis
REDIS_URL=redis://localhost:6379
REDIS_PASSWORD=
REDIS_DB=0

# JWT
JWT_SECRET=
JWT_EXPIRY=15m
REFRESH_TOKEN_EXPIRY=168h

# Encryption
MASTER_ENCRYPTION_KEY=

# Indodax
INDODAX_PUBLIC_WS=wss://ws3.indodax.com/ws/
INDODAX_REST_BASE=https://indodax.com

# Admin
ADMIN_USERNAME=admin
ADMIN_PASSWORD=
```

### Deployment Strategy

#### Backend (Docker)
- Backend container (Go binary)
- Redis container
- Docker Compose for local development
- Deploy to VPS/Cloud (DigitalOcean, AWS, GCP)

#### Frontend (Vercel)
- Deploy to Vercel (recommended)
- Automatic deployments from Git
- Preview deployments for PRs
- Edge network for global CDN
- Zero-config deployment

---

## Development Workflow

1. **Setup**: Clone repo, install dependencies
2. **Backend**: Run Go server with air (hot reload)
3. **Frontend**: Run Vite dev server
4. **Redis**: Run Redis locally or via Docker
5. **Testing**: Unit tests + integration tests
6. **Deployment**: 
   - Frontend: Push to GitHub → Auto-deploy to Vercel
   - Backend: Build Docker image → Deploy to server

---

## Future Enhancements (Phase 2)

- [ ] Advanced charting with TradingView
- [ ] Backtesting engine
- [ ] Custom trading strategies
- [ ] Multi-exchange support
- [ ] Telegram notifications
- [ ] Portfolio analytics
- [ ] Risk management tools
- [ ] Paper trading mode

---

## References

- Indodax API Documentation (see `/docs` folder)
- Redis Documentation: https://redis.io/docs/
- Go Best Practices: https://go.dev/doc/effective_go
- React Best Practices: https://react.dev/

