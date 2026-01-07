# TUYUL - Crypto Trading Bot for Indodax

![Version](https://img.shields.io/badge/version-1.0.0-blue.svg)
![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go)
![React](https://img.shields.io/badge/React-18+-61DAFB?logo=react)
![Redis](https://img.shields.io/badge/Redis-7+-DC382D?logo=redis)
![License](https://img.shields.io/badge/license-MIT-green.svg)

**TUYUL** is a professional-grade, multi-user cryptocurrency trading bot for Indodax exchange with automated trading capabilities, real-time market analysis, and comprehensive order management.

---

## 🚀 Features

### Core Features
- ✅ **Multi-User Support** - Admin and user roles with secure authentication
- ✅ **Secure API Key Management** - AES-256-GCM encryption for Indodax API keys
- ✅ **Real-Time Market Analysis** - Pump score and bid-ask gap analysis with multi-timeframe data
- ✅ **Three Bot Types** - Market Maker, Pump Hunter, and Copilot trading strategies
- ✅ **Paper & Live Trading** - Test strategies risk-free before going live
- ✅ **Stop-Loss Protection** - Automatic stop-loss monitoring and execution
- ✅ **Order Management** - Complete order lifecycle tracking
- ✅ **WebSocket Support** - Real-time updates for market data and orders
- ✅ **Admin Dashboard** - User management and system monitoring

### Market Analysis
- **Pump Score**: Multi-timeframe momentum indicator (1m, 5m, 15m, 30m) with transaction count weighting
- **Gap Analysis**: Bid-ask spread opportunities with volume filtering
- **Real-Time Updates**: WebSocket connection for live market data (every 1-2 seconds)
- **Unified Data Structure**: All market data stored in Redis for sub-millisecond queries
- **Sortable Tables**: Sort by pump score, gap percentage, volume, price change

### Trading Bots

#### 1. Market Maker Bot
**Strategy**: Profit from bid-ask spread  
**Risk**: Low to moderate  
**Best For**: Low volatility, high liquidity pairs

- Continuously places competitive buy/sell orders
- Inventory-aware order placement
- Automatic order adjustment on price movement
- Gap-based filtering (only trade when profitable)
- Virtual balance tracking for paper & live trading

#### 2. Pump Hunter Bot
**Strategy**: Momentum trading (catch pumps)  
**Risk**: High  
**Best For**: Volatile markets with pump activity

- Scans all trading pairs simultaneously
- Opens positions based on pump score signals
- Multiple concurrent positions with risk limits
- Automated exits (take profit, stop-loss, trailing stop, time-based)
- Daily loss limits and cooldown periods

#### 3. Copilot Bot
**Strategy**: Semi-automated trading assistant  
**Risk**: Moderate (configurable)  
**Best For**: Manual traders who want automation

- User-specified buy orders with target profit and stop-loss
- Automatic sell order placement when buy fills
- Real-time stop-loss monitoring
- Manual sell override available
- One-time trade execution (not continuous)

### Bot Comparison

| Feature | Market Maker | Pump Hunter | Copilot |
|---------|--------------|-------------|---------|
| **Strategy** | Mean reversion | Momentum/Trend | Profit-target |
| **Pairs** | Single pair | All pairs (scan) | User choice |
| **Execution** | Continuous | Event-driven | One-time |
| **Entry** | Inventory-based | Pump score signals | User-specified |
| **Exit** | Continuous cycle | TP/SL/Trailing/Time | Target profit/SL |
| **Risk Level** | Low-Moderate | High | Moderate |
| **Hold Time** | Seconds-Minutes | Minutes-Hours | User-dependent |
| **Max Positions** | 1 active order | 1-10 positions | Unlimited trades |
| **Best Markets** | Low volatility | Volatile/Pumping | Any |
| **Profit Source** | Bid-ask spread | Price momentum | Price appreciation |
| **Suitable For** | Passive income | Active traders | Semi-automated |

---

## 📋 Documentation

Comprehensive documentation is available in the `/docs` folder:

### Core Blueprints
- **[blueprint_architecture.md](docs/blueprint_architecture.md)** - System architecture and tech stack overview
- **[blueprint_auth_user_management.md](docs/blueprint_auth_user_management.md)** - Authentication and user management
- **[blueprint_api_key_management.md](docs/blueprint_api_key_management.md)** - Secure API key handling
- **[blueprint_market_analysis.md](docs/blueprint_market_analysis.md)** - Multi-timeframe pump score & gap analysis
- **[blueprint_database_schema.md](docs/blueprint_database_schema.md)** - Redis database schema
- **[blueprint_api_endpoints.md](docs/blueprint_api_endpoints.md)** - Complete API reference
- **[blueprint_frontend_architecture.md](docs/blueprint_frontend_architecture.md)** - Frontend architecture and components

### Bot Blueprints
- **[blueprint_bot_market_maker.md](docs/blueprint_bot_market_maker.md)** - Market maker bot (spread-capturing strategy)
- **[blueprint_bot_pump_hunter.md](docs/blueprint_bot_pump_hunter.md)** - Pump hunter bot (momentum trading)
- **[blueprint_bot_copilot.md](docs/blueprint_bot_copilot.md)** - Copilot bot (automated trading assistant)

### Indodax API Documentation
- **[indodax_public_rest.md](docs/indodax_public_rest.md)** - Public REST API
- **[indodax_public_ws.md](docs/indodax_public_ws.md)** - Public WebSocket API
- **[indodax_private_rest.md](docs/indodax_private_rest.md)** - Private REST API
- **[indodax_private_ws.md](docs/indodax_private_ws.md)** - Private WebSocket API
- **[indodax_private_deadman.md](docs/indodax_private_deadman.md)** - Deadman Switch API

---

## 🛠️ Tech Stack

### Backend
- **Go 1.21+** - High-performance backend services
- **Redis 7+** - Primary database and cache
- **WebSocket** - Real-time bidirectional communication
- **JWT** - Secure authentication tokens
- **AES-256-GCM** - API key encryption

### Frontend
- **React 18+** - Modern UI framework
- **TypeScript** - Type-safe JavaScript
- **Vite** - Fast build tool and dev server
- **TailwindCSS** - Utility-first CSS framework
- **Radix UI** - Accessible component primitives
- **Zustand** - Lightweight state management
- **React Query** - Server state management

### Infrastructure
- **Vercel** - Frontend deployment and hosting
- **Docker** - Backend containerization
- **Redis** - Data store, cache, and pub/sub

---

## 📦 Installation

### Prerequisites
- **Go 1.21+** ([Download](https://go.dev/dl/))
- **Node.js 20+** ([Download](https://nodejs.org/))
- **Redis 7+** ([Download](https://redis.io/download))
- **Indodax API Key** (Get from [Indodax](https://indodax.com/))

### Quick Start

#### 1. Clone Repository
```bash
git clone https://github.com/yourusername/tuyul.git
cd tuyul
```

#### 2. Backend Setup
```bash
cd backend

# Install dependencies
go mod download

# Copy environment file
cp .env.example .env

# Edit .env with your configuration
nano .env

# Run Redis (if not already running)
redis-server

# Run backend
go run cmd/server/main.go
```

#### 3. Frontend Setup
```bash
cd frontend

# Install dependencies
npm install

# Copy environment file
cp .env.example .env

# Edit .env with API URL
nano .env

# Run frontend
npm run dev
```

#### 4. Access Application
- **Frontend**: http://localhost:5173
- **Backend API**: http://localhost:8080
- **WebSocket**: ws://localhost:8081

#### 5. Default Admin Login
```
Username: admin
Password: (from .env ADMIN_PASSWORD)
```

---

## 🚀 Deployment

### Frontend Deployment (Vercel)

#### Option 1: Deploy via GitHub (Recommended)
```bash
# 1. Push your code to GitHub
git add .
git commit -m "Initial commit"
git push origin main

# 2. Import project in Vercel
# - Go to https://vercel.com
# - Click "Add New Project"
# - Import your GitHub repository
# - Configure environment variables (see below)
# - Deploy!
```

#### Option 2: Deploy via Vercel CLI
```bash
# Install Vercel CLI
npm i -g vercel

# Login to Vercel
vercel login

# Deploy (from frontend directory)
cd frontend
vercel --prod
```

#### Frontend Environment Variables (Vercel Dashboard)
```env
VITE_API_URL=https://api.tuyul.com/api/v1
VITE_WS_URL=wss://api.tuyul.com/ws
```

---

### Backend Deployment (Docker)

#### Using Docker Compose
```bash
# Build and run backend + Redis
docker-compose up -d

# View logs
docker-compose logs -f

# Stop services
docker-compose down
```

#### Backend Environment Variables
Create a `.env` file in the backend directory:

```env
# Server
PORT=8080
WS_PORT=8081
ENV=production

# Redis
REDIS_URL=redis://redis:6379
REDIS_PASSWORD=
REDIS_DB=0

# JWT
JWT_SECRET=your-super-secret-jwt-key-change-this
JWT_EXPIRY=15m
REFRESH_TOKEN_EXPIRY=168h

# Encryption
MASTER_ENCRYPTION_KEY=your-32-byte-encryption-key-base64-encoded
ENCRYPTION_SALT=your-16-byte-salt-base64-encoded

# Admin
ADMIN_USERNAME=admin
ADMIN_PASSWORD=change-this-secure-password

# Indodax
INDODAX_PUBLIC_WS=wss://ws3.indodax.com/ws/
INDODAX_REST_BASE=https://indodax.com
```

---

## 🚀 Usage

### 1. Create User Account (Admin)
1. Login as admin
2. Navigate to Admin → Users
3. Click "Create User"
4. Fill in username, email, password, and role
5. Submit

### 2. Add Indodax API Key
1. Login as user
2. Navigate to Settings → API Keys
3. Enter Indodax API key and secret
4. System will validate the key automatically
5. Save

### 3. Market Analysis
1. Navigate to Market Analysis
2. View real-time pump scores (multi-timeframe momentum)
3. View gap analysis (bid-ask spread opportunities)
4. Sort by pump score, gap, volume, or price change
5. Click "Trade" to use Copilot or view pair details

### 4. Create & Start Bots

#### Market Maker Bot
1. Navigate to Bots → Create Bot
2. Select "Market Maker"
3. Choose trading pair (e.g., BTC/IDR)
4. Configure:
   - Initial balance (paper mode) or API key (live mode)
   - Order size per trade
   - Minimum gap percentage (spread requirement)
   - Reposition threshold
   - Max loss limit (circuit breaker)
5. Click "Create & Start"
6. Monitor balance, orders, and profit in real-time

#### Pump Hunter Bot
1. Navigate to Bots → Create Bot
2. Select "Pump Hunter"
3. Configure entry rules:
   - Minimum pump score (50-100 recommended)
   - Minimum positive timeframes (2-3)
   - Minimum 24h volume and price filters
4. Configure exit rules:
   - Target profit % (3-10%)
   - Stop-loss % (1.5-3%)
   - Trailing stop, max hold time
5. Configure risk management:
   - Max position size
   - Max concurrent positions (1-5)
   - Daily loss limit
6. Click "Create & Start"
7. Monitor open positions and performance

#### Copilot Bot (One-Time Trades)
1. Navigate to Copilot → New Trade
2. Select pair (e.g., BTC/IDR)
3. Enter buying price (limit)
4. Enter volume in IDR
5. Set target profit % (e.g., 5%)
6. Set stop-loss % (e.g., 3%)
7. Click "Place Buy Order"
8. System auto-places sell order when buy fills
9. Monitor via real-time WebSocket updates

### 5. Monitor Bots & Trades
1. **Bots Dashboard**: View all running bots, status, profit/loss
2. **Bot Details**: Real-time balance, active orders, statistics
3. **Positions** (Pump Hunter): Open/closed positions, P&L per trade
4. **Orders**: Complete order history with filters
5. **Stop/Pause**: Stop bots anytime, balances are preserved

### 6. Paper Trading
- Test strategies risk-free with virtual balance
- Same functionality as live trading
- Switch to live trading after testing
- No real money at risk

---

## 🧪 Testing

### Backend Tests
```bash
cd backend

# Run unit tests
go test ./...

# Run with coverage
go test -cover ./...

# Run integration tests
go test -tags=integration ./...
```

### Frontend Tests
```bash
cd frontend

# Run unit tests
npm test

# Run with coverage
npm test -- --coverage

# Run E2E tests (if configured)
npm run test:e2e
```

---

## 📊 Project Structure

```
tuyul/
├── backend/                # Go backend
│   ├── cmd/
│   │   └── server/        # Main application
│   ├── internal/          # Private application code
│   │   ├── domain/        # Business entities
│   │   ├── service/       # Business logic
│   │   ├── repository/    # Data access
│   │   ├── handler/       # HTTP handlers
│   │   ├── middleware/    # HTTP middleware
│   │   └── client/        # External API clients
│   └── pkg/               # Public libraries
├── frontend/              # React frontend
│   ├── src/
│   │   ├── components/    # React components
│   │   ├── features/      # Feature modules
│   │   ├── hooks/         # Custom hooks
│   │   ├── services/      # API clients
│   │   ├── stores/        # State management
│   │   └── pages/         # Page components
│   └── public/
├── docs/                  # Documentation
├── docker-compose.yml     # Backend Docker configuration
├── vercel.json           # Frontend Vercel configuration
└── README.md             # This file
```

---

## 🔒 Security

### Best Practices Implemented
- ✅ API keys encrypted with AES-256-GCM
- ✅ JWT-based authentication with refresh tokens
- ✅ Bcrypt password hashing (cost factor 12)
- ✅ Rate limiting on all endpoints
- ✅ Input validation and sanitization
- ✅ CORS protection
- ✅ HTTPS in production
- ✅ Token blacklist on logout
- ✅ Role-based access control

### Important Security Notes
- **Never commit** `.env` files to version control
- **Change default passwords** before production
- **Use strong encryption keys** (32+ bytes)
- **Enable HTTPS** in production
- **Regularly rotate** API keys and secrets
- **Monitor logs** for suspicious activity

---

## 🤝 Contributing

Contributions are welcome! Please follow these steps:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

### Development Guidelines
- Follow Go best practices and conventions
- Write unit tests for new features
- Update documentation as needed
- Use meaningful commit messages
- Ensure all tests pass before submitting

---

## 📝 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

---

## ⚠️ Disclaimer

**IMPORTANT**: This software is provided for educational and informational purposes only. Cryptocurrency trading carries a high level of risk and may not be suitable for all investors.

- **No Financial Advice**: This software does NOT provide financial, investment, or trading advice
- **Use at Your Own Risk**: You are solely responsible for your trading decisions
- **No Guarantees**: Past performance is not indicative of future results
- **Test Thoroughly**: Always test with small amounts first
- **Understand the Risks**: Cryptocurrency markets are highly volatile

The developers and contributors of this software are NOT responsible for any financial losses, damages, or other consequences resulting from the use of this software.

---

## 🙏 Acknowledgments

- [Indodax](https://indodax.com/) - Indonesian cryptocurrency exchange
- [Go](https://go.dev/) - Programming language
- [React](https://react.dev/) - UI framework
- [Redis](https://redis.io/) - In-memory database
- [TailwindCSS](https://tailwindcss.com/) - CSS framework
- [Radix UI](https://www.radix-ui.com/) - Component primitives

---

## 📞 Support

- **Documentation**: See `/docs` folder
- **Issues**: [GitHub Issues](https://github.com/yourusername/tuyul/issues)
- **Discussions**: [GitHub Discussions](https://github.com/yourusername/tuyul/discussions)

---

## 🗺️ Roadmap

### Phase 1 (Current)
- [x] Multi-user authentication & role-based access
- [x] Secure API key management (AES-256-GCM)
- [x] Real-time market analysis (pump score & gap)
- [x] Multi-timeframe data (1m, 5m, 15m, 30m)
- [x] Market Maker Bot (spread-capturing)
- [x] Pump Hunter Bot (momentum trading)
- [x] Copilot Bot (semi-automated trading)
- [x] Paper & live trading modes
- [x] Stop-loss protection
- [x] Order & position management
- [x] Admin dashboard
- [x] WebSocket real-time updates

### Phase 2 (In Progress)
- [ ] Bot performance analytics & reporting
- [ ] Advanced risk management tools
- [ ] Backtesting engine for strategies
- [ ] Custom trading strategies (user-configurable)
- [ ] Portfolio tracker across all bots
- [ ] Telegram/Discord notifications
- [ ] Advanced charting (TradingView integration)

### Phase 3 (Planned)
- [ ] Multi-exchange support (Tokocrypto, Binance ID)
- [ ] Grid trading strategy
- [ ] DCA (Dollar-Cost Averaging) bot
- [ ] Arbitrage bot (cross-exchange)
- [ ] Social trading (copy successful traders)
- [ ] AI-powered signal generation
- [ ] Mobile app (React Native)
- [ ] Webhooks for external integrations
- [ ] API for third-party developers

---

**Built with ❤️ for the crypto community**

