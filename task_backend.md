# TUYUL Backend Development Task List

## Project Overview
Backend development for TUYUL crypto trading bot platform using Golang and Redis.

---

## Phase 1: Foundation & Infrastructure
**Estimated Time: 2-3 days**

### 1.1 Project Setup
- [x] Initialize Go project structure with clean architecture
- [x] Setup dependency management (go.mod)
- [x] Configure Redis connection with connection pooling
- [x] Setup environment configuration (.env support)
- [x] Create logging infrastructure (structured logging)
- [x] Setup error handling utilities
- [x] Create base HTTP server with graceful shutdown

**Time: 4-6 hours** ✅ **Completed: ~4 hours**

### 1.2 Database Layer (Redis)
- [x] Implement Redis client wrapper
- [x] Create connection pool manager
- [x] Implement pipeline support for batch operations
- [x] Setup Redis key patterns and constants
- [x] Create helper functions for common operations (HSET, ZADD, etc.)
- [x] Implement Redis health check

**Time: 4-6 hours** ✅ **Completed: ~4 hours**

### 1.3 Core Middleware
- [x] Request logging middleware
- [x] Error recovery middleware
- [x] CORS middleware
- [x] Rate limiting middleware (per IP, per user)
- [x] Request ID middleware for tracing

**Time: 3-4 hours** ✅ **Completed: ~3 hours**

---

## Phase 2: Authentication & User Management
**Estimated Time: 2-3 days**

### 2.1 User Authentication
- [ ] User registration endpoint with validation
- [ ] Password hashing with bcrypt (cost 12)
- [ ] Login endpoint with JWT generation
- [ ] Refresh token endpoint
- [ ] Logout with token blacklisting
- [ ] JWT middleware for protected routes
- [ ] Role-based access control (admin/user)

**Time: 6-8 hours**

### 2.2 User Management
- [ ] Get user profile endpoint
- [ ] Update user profile endpoint
- [ ] Change password endpoint
- [ ] Admin: List all users endpoint
- [ ] Admin: Update user role endpoint
- [ ] Admin: Delete user endpoint
- [ ] User session management in Redis

**Time: 4-6 hours**

### 2.3 Security
- [ ] Input validation for all user endpoints
- [ ] Email format validation
- [ ] Password strength validation
- [ ] Rate limiting for auth endpoints (prevent brute force)
- [ ] Token expiry handling (15min access, 7day refresh)

**Time: 2-3 hours**

---

## Phase 3: API Key Management
**Estimated Time: 2-3 days**

### 3.1 Encryption/Decryption
- [ ] Implement AES-256-GCM encryption
- [ ] Implement AES-256-GCM decryption
- [ ] Generate secure encryption keys
- [ ] Key derivation from master secret
- [ ] Secure key storage in environment

**Time: 4-6 hours**

### 3.2 Indodax API Key Validation
- [ ] Create Indodax client for API validation
- [ ] Implement HMAC-SHA512 signature generation
- [ ] Test API key with getInfo endpoint
- [ ] Parse and validate Indodax response
- [ ] Handle Indodax API errors

**Time: 4-5 hours**

### 3.3 API Key CRUD Operations
- [ ] Create API key endpoint (with validation)
- [ ] Store encrypted API key in Redis
- [ ] Get API key status endpoint (without exposing secret)
- [ ] Update API key endpoint
- [ ] Delete API key endpoint
- [ ] Get decrypted API key (internal use only)

**Time: 4-6 hours**

---

## Phase 4: Indodax Integration
**Estimated Time: 3-4 days**

### 4.1 Public REST API Client
- [ ] Implement rate limiter (180 req/min)
- [ ] GET /api/server_time
- [ ] GET /api/pairs (with caching)
- [ ] GET /api/price_increments (with caching)
- [ ] GET /api/summaries
- [ ] GET /api/ticker/{pair}
- [ ] GET /api/ticker_all
- [ ] GET /api/trades/{pair}
- [ ] GET /api/depth/{pair}
- [ ] Error handling and retry logic

**Time: 6-8 hours**

### 4.2 Private REST API Client
- [ ] Implement rate limiter (20/sec trade, 30/sec cancel)
- [ ] POST /tapi - getInfo (balance)
- [ ] POST /tapi - trade (buy/sell)
- [ ] POST /tapi - openOrders
- [ ] POST /tapi - orderHistory
- [ ] POST /tapi - getOrder
- [ ] POST /tapi - cancelOrder
- [ ] HMAC-SHA512 signature implementation
- [ ] Nonce management

**Time: 8-10 hours**

### 4.3 Public WebSocket Client
- [ ] WebSocket connection manager
- [ ] Auto-reconnect logic with exponential backoff
- [ ] Subscribe to market summary channel
- [ ] Subscribe to ticker channel per pair
- [ ] Parse and normalize WebSocket messages
- [ ] Heartbeat/ping-pong handling
- [ ] Connection pool for multiple pairs

**Time: 8-10 hours**

### 4.4 Private WebSocket Client
- [ ] Generate private WebSocket token (POST /api/private_ws/v1/generate_token)
- [ ] WebSocket authentication with token
- [ ] Subscribe to order update channel
- [ ] Parse order update messages
- [ ] Auto-reconnect with token refresh
- [ ] Connection state management per user

**Time: 6-8 hours**

---

## Phase 5: Market Analysis
**Estimated Time: 3-4 days**

### 5.1 Data Ingestion
- [ ] Subscribe to market summary WebSocket (all pairs)
- [ ] Real-time price update handler
- [ ] Real-time volume update handler
- [ ] Transaction count tracking per timeframe
- [ ] Store raw market data in Redis (unified coin hash)

**Time: 6-8 hours**

### 5.2 Timeframe Manager
- [ ] Implement 4 timeframe tracking (1m, 5m, 15m, 30m)
- [ ] Track OHLC per timeframe
- [ ] Track transaction count per timeframe
- [ ] Rolling window reset logic (independent per timeframe)
- [ ] Timeframe data update on each price tick

**Time: 8-10 hours**

### 5.3 Pump Score Calculation
- [ ] Calculate price change % per timeframe
- [ ] Apply transaction count weighting
- [ ] Apply timeframe weights (1m=1, 5m=2, 15m=3, 30m=4)
- [ ] Compute weighted pump score
- [ ] Update pump score in real-time
- [ ] Store in Redis sorted set for ranking

**Time: 6-8 hours**

### 5.4 Gap Analysis
- [ ] Calculate bid-ask gap percentage
- [ ] Volume filtering (minimum volume threshold)
- [ ] Store gap data in Redis sorted set
- [ ] Real-time gap updates

**Time: 4-5 hours**

### 5.5 Market Analysis API
- [ ] GET /api/v1/market/summary (all pairs with pump scores)
- [ ] GET /api/v1/market/top-pumps (sorted by pump score)
- [ ] GET /api/v1/market/top-gaps (sorted by gap percentage)
- [ ] GET /api/v1/market/pair/{pair} (detailed pair info)
- [ ] Pagination support
- [ ] Filtering and sorting options

**Time: 4-6 hours**

---

## Phase 6: Trading Automation (Copilot Bot)
**Estimated Time: 4-5 days**

### 6.1 Trade Request Handling
- [ ] POST /api/v1/trade/buy endpoint
- [ ] Input validation (price, volume, profit%, stop-loss%)
- [ ] Balance validation with Indodax
- [ ] Price increment validation
- [ ] Calculate buy amount from IDR volume
- [ ] Store trade request in Redis

**Time: 6-8 hours**

### 6.2 Order Placement
- [ ] Place limit buy order on Indodax
- [ ] Store order in Redis with status tracking
- [ ] Subscribe to order updates (Private WebSocket)
- [ ] Handle order rejection errors
- [ ] Update order status on fill

**Time: 6-8 hours**

### 6.3 Auto-Sell Logic
- [ ] Detect buy order fill from WebSocket
- [ ] Fetch filled order balance
- [ ] Calculate sell price (buy price + target profit%)
- [ ] Place limit sell order automatically
- [ ] Link buy and sell orders (buy_sell_map)
- [ ] Handle auto-sell errors

**Time: 6-8 hours**

### 6.4 Stop-Loss Monitor
- [ ] Background goroutine for stop-loss checking (1sec interval)
- [ ] Monitor all filled buy orders
- [ ] Calculate stop-loss price per order
- [ ] Compare with current market price
- [ ] Trigger market sell when stop-loss hit
- [ ] Cancel existing limit sell order before stop-loss sell
- [ ] Send WebSocket alert to user

**Time: 8-10 hours**

### 6.5 Order Management API
- [ ] GET /api/v1/trade/orders (list with filters)
- [ ] GET /api/v1/trade/orders/:id (order details)
- [ ] DELETE /api/v1/trade/orders/:id (cancel order)
- [ ] POST /api/v1/trade/orders/:id/sell (manual market sell)
- [ ] GET /api/v1/trade/balance (user balance)
- [ ] Pagination for order list

**Time: 6-8 hours**

---

## Phase 7: Market Maker Bot
**Estimated Time: 4-5 days**

### 7.1 Bot Configuration
- [ ] Bot configuration data model
- [ ] POST /api/v1/bots/market-maker (create bot)
- [ ] Store bot config in Redis
- [ ] Validate bot parameters (pair, spread, limits)
- [ ] Paper trading vs live trading mode selection

**Time: 4-5 hours**

### 7.2 Orderbook Monitoring
- [ ] Subscribe to orderbook WebSocket for bot's pair
- [ ] Track best bid/ask in real-time
- [ ] Calculate current spread
- [ ] Detect orderbook changes
- [ ] Shared WebSocket connection (reference counting)

**Time: 6-8 hours**

### 7.3 Order Placement Logic
- [ ] Calculate competitive bid (bestBid + tick)
- [ ] Calculate competitive ask (bestAsk - tick)
- [ ] Check minimum spread threshold
- [ ] Place buy and sell orders simultaneously
- [ ] Handle order placement errors
- [ ] Rate limiting and debouncing (500ms)

**Time: 8-10 hours**

### 7.4 Inventory Management
- [ ] Track virtual balance (IDR + coins)
- [ ] Calculate available vs reserved balance
- [ ] Check inventory limits before orders
- [ ] Adaptive order sizing based on inventory
- [ ] Prevent overexposure (max long/short limits)

**Time: 6-8 hours**

### 7.5 Dynamic Reposition
- [ ] Detect when orders are no longer competitive
- [ ] Cancel old orders
- [ ] Recalculate optimal prices
- [ ] Place new orders at new prices
- [ ] Reposition threshold logic (e.g., 0.1% price move)

**Time: 6-8 hours**

### 7.6 Risk Management
- [ ] Track cumulative P&L per bot
- [ ] Circuit breaker (max loss limit)
- [ ] Stop bot when circuit breaker triggered
- [ ] Price volatility detection
- [ ] Pause trading during high volatility

**Time: 4-6 hours**

### 7.7 Paper Trading Simulation
- [ ] Simulate order fills (5sec delay)
- [ ] Virtual balance updates
- [ ] Track simulated P&L
- [ ] No real API calls to Indodax

**Time: 4-5 hours**

### 7.8 Bot Management API
- [ ] GET /api/v1/bots/market-maker (list user's bots)
- [ ] GET /api/v1/bots/market-maker/:id (bot details)
- [ ] PUT /api/v1/bots/market-maker/:id (update config)
- [ ] POST /api/v1/bots/market-maker/:id/start
- [ ] POST /api/v1/bots/market-maker/:id/stop
- [ ] DELETE /api/v1/bots/market-maker/:id
- [ ] GET /api/v1/bots/market-maker/:id/summary (P&L, stats)

**Time: 6-8 hours**

---

## Phase 8: Pump Hunter Bot
**Estimated Time: 4-5 days**

### 8.1 Bot Configuration
- [ ] Bot configuration data model
- [ ] POST /api/v1/bots/pump-hunter (create bot)
- [ ] Store bot config in Redis
- [ ] Validate bot parameters (thresholds, limits)
- [ ] Paper trading vs live trading mode selection

**Time: 4-5 hours**

### 8.2 Market-Wide Monitoring
- [ ] Subscribe to pump score updates (all pairs)
- [ ] Event-driven architecture (React to coin updates)
- [ ] Real-time pump score threshold checking
- [ ] Multi-condition entry validation
- [ ] Volume surge detection

**Time: 6-8 hours**

### 8.3 Entry Logic
- [ ] Check pump score threshold
- [ ] Validate positive timeframes (3 out of 4)
- [ ] Check volume surge vs average
- [ ] Validate price momentum (rising)
- [ ] Check minimum liquidity
- [ ] Pre-entry risk checks (balance, limits, cooldown)
- [ ] Calculate position size
- [ ] Place market buy order

**Time: 8-10 hours**

### 8.4 Position Tracking
- [ ] Store position data in Redis
- [ ] Track entry price and amount
- [ ] Calculate and store exit targets (TP, SL, trailing)
- [ ] Update position status lifecycle
- [ ] Link to Indodax orders

**Time: 6-8 hours**

### 8.5 Exit Monitoring
- [ ] Monitor take-profit condition
- [ ] Monitor stop-loss condition
- [ ] Monitor trailing stop condition
- [ ] Monitor pump score drop condition
- [ ] Monitor max hold time (timer-based)
- [ ] Monitor volume dry-up condition
- [ ] Place market sell when any condition triggers

**Time: 10-12 hours**

### 8.6 Multi-Position Management
- [ ] Track multiple positions simultaneously
- [ ] Independent exit monitoring per position
- [ ] Max concurrent position limit (e.g., 5)
- [ ] Balance allocation across positions
- [ ] Position priority logic

**Time: 6-8 hours**

### 8.7 Risk Management
- [ ] Daily P&L tracking
- [ ] Daily loss limit (circuit breaker)
- [ ] Cooldown after losses (e.g., 5 min)
- [ ] Pair blacklist (temporary, after repeated losses)
- [ ] Stop all trading when daily limit hit

**Time: 6-8 hours**

### 8.8 Bot Management API
- [ ] GET /api/v1/bots/pump-hunter (list user's bots)
- [ ] GET /api/v1/bots/pump-hunter/:id (bot details)
- [ ] PUT /api/v1/bots/pump-hunter/:id (update config)
- [ ] POST /api/v1/bots/pump-hunter/:id/start
- [ ] POST /api/v1/bots/pump-hunter/:id/stop
- [ ] DELETE /api/v1/bots/pump-hunter/:id
- [ ] GET /api/v1/bots/pump-hunter/:id/positions (active positions)
- [ ] GET /api/v1/bots/pump-hunter/:id/summary (P&L, stats)

**Time: 8-10 hours**

---

## Phase 9: WebSocket Server (Real-time Updates)
**Estimated Time: 2-3 days**

### 9.1 WebSocket Infrastructure
- [ ] WebSocket server setup
- [ ] Client connection manager
- [ ] User authentication for WebSocket (JWT)
- [ ] Connection storage (user_id → conn mapping in Redis)
- [ ] Heartbeat/ping-pong for connection health

**Time: 6-8 hours**

### 9.2 Broadcasting System
- [ ] Redis Pub/Sub integration
- [ ] Subscribe to internal channels (orders, market, bots)
- [ ] Broadcast to specific user (user-specific updates)
- [ ] Broadcast to all users (market-wide updates)
- [ ] Message serialization (JSON)

**Time: 6-8 hours**

### 9.3 Real-time Channels
- [ ] Market data updates channel
- [ ] Order status updates channel
- [ ] Bot status updates channel
- [ ] Stop-loss triggered alerts channel
- [ ] Balance updates channel
- [ ] Pump signals channel

**Time: 4-6 hours**

---

## Phase 10: Testing & Quality Assurance
**Estimated Time: 3-4 days**

### 10.1 Unit Tests
- [ ] Test validation functions (auth, trade requests)
- [ ] Test encryption/decryption
- [ ] Test pump score calculation
- [ ] Test profit/loss calculations
- [ ] Test order state transitions
- [ ] Test inventory management logic

**Time: 8-10 hours**

### 10.2 Integration Tests
- [ ] Test full auth flow (register → login → refresh)
- [ ] Test API key validation with Indodax
- [ ] Test buy → fill → auto-sell flow
- [ ] Test stop-loss trigger flow
- [ ] Test Market Maker full cycle
- [ ] Test Pump Hunter entry → exit flow

**Time: 10-12 hours**

### 10.3 Load Tests
- [ ] Test concurrent user authentication
- [ ] Test multiple concurrent orders
- [ ] Test high-frequency WebSocket messages
- [ ] Test multiple bots running simultaneously
- [ ] Test Redis performance under load

**Time: 6-8 hours**

---

## Phase 11: Deployment & DevOps
**Estimated Time: 1-2 days**

### 11.1 Dockerization
- [ ] Create Dockerfile for Go backend
- [ ] Create docker-compose.yml (backend + Redis)
- [ ] Multi-stage build for smaller image
- [ ] Environment variable configuration
- [ ] Health check endpoints

**Time: 4-6 hours**

### 11.2 Monitoring & Logging
- [ ] Structured logging (JSON format)
- [ ] Log levels configuration
- [ ] Request/response logging
- [ ] Error tracking and alerting
- [ ] Performance metrics collection

**Time: 4-5 hours**

### 11.3 Documentation
- [ ] API documentation (Swagger/OpenAPI)
- [ ] Deployment guide
- [ ] Environment variables documentation
- [ ] Troubleshooting guide

**Time: 3-4 hours**

---

## Summary

### Total Estimated Time: **35-45 days** (solo developer, full-time)

### Phase Breakdown:
1. **Foundation & Infrastructure**: 2-3 days
2. **Authentication & User Management**: 2-3 days
3. **API Key Management**: 2-3 days
4. **Indodax Integration**: 3-4 days
5. **Market Analysis**: 3-4 days
6. **Trading Automation (Copilot)**: 4-5 days
7. **Market Maker Bot**: 4-5 days
8. **Pump Hunter Bot**: 4-5 days
9. **WebSocket Server**: 2-3 days
10. **Testing & QA**: 3-4 days
11. **Deployment & DevOps**: 1-2 days

### Critical Path:
1. Foundation → Auth → API Keys → Indodax Integration → Market Analysis → Bots
2. Market Analysis must be completed before Pump Hunter Bot
3. Private WebSocket integration needed for all trading features

### Parallelization Opportunities:
- Market Maker and Pump Hunter can be developed in parallel (after Phase 5)
- Testing can be done incrementally alongside development
- Documentation can be written alongside feature development

### Risk Factors:
- **Indodax API changes**: May require rework (2-3 days buffer)
- **WebSocket stability**: May need extra debugging (1-2 days buffer)
- **Complex bot logic**: Edge cases may extend testing (2-3 days buffer)
- **Redis optimization**: May need performance tuning (1-2 days buffer)

### Recommended Approach:
1. **MVP First** (Phases 1-6): ~18-22 days
   - Core functionality with Copilot bot
   - Get working system deployed
   - Gather user feedback
   
2. **Advanced Bots** (Phases 7-8): ~8-10 days
   - Market Maker and Pump Hunter
   - More complex strategies
   
3. **Polish & Scale** (Phases 9-11): ~6-9 days
   - Real-time updates
   - Testing and optimization
   - Production-ready deployment

---

## Priority Levels

### 🔴 Critical (Must Have for MVP)
- Authentication & User Management
- API Key Management with Indodax validation
- Market Analysis (pump score & gap)
- Trading Automation (Copilot bot)
- Basic WebSocket for order updates

### 🟡 Important (High Value)
- Pump Hunter Bot (main selling point)
- Market Maker Bot (diversification)
- Full WebSocket broadcasting system
- Comprehensive testing

### 🟢 Nice to Have (Future Enhancements)
- Advanced analytics dashboard
- Multiple bot instances per user
- Bot performance comparison
- Historical backtesting

---

**Note**: Time estimates assume:
- Experienced Go developer
- Familiarity with Redis
- Some crypto trading knowledge
- Working independently
- No major blockers or external dependencies

**Adjustment factors**:
- Junior developer: Add 50-100% more time
- Team of 2-3: Reduce total time by 40-60% (parallelization)
- Part-time: Multiply by schedule factor

