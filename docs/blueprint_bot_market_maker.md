# Market Maker Bot

## Overview

The Market Maker Bot implements a **spread-capturing strategy** that profits from the bid-ask spread on Indodax. It continuously places competitive buy and sell orders, adjusting prices based on market conditions to stay at the front of the order book.

**Strategy Type**: Mean reversion / Market making  
**Risk Level**: Low to moderate  
**Suitable Markets**: Low volatility pairs with consistent volume  
**Timeframe**: Seconds to minutes per trade cycle

---

## Key Characteristics

- ✅ **Single pair trading** per bot instance
- ✅ **Inventory-aware** order placement (based on current balance)
- ✅ **Competitive pricing** (tick-size based adjustments)
- ✅ **Automatic rebalancing** (buy → sell → buy cycle)
- ✅ **Gap-based filtering** (only trade when spread is profitable)
- ✅ **Virtual balance tracking** (unified for paper & live trading)
- ✅ **Real-time price monitoring** via WebSocket ticker updates
- ✅ **Order adjustment** on price movement

---

## Bot Configuration

### Core Settings

```json
{
  "name": "BTC/IDR Market Maker",
  "type": "market_maker",
  "pair": "btcidr",
  "is_paper_trading": true,
  "user_id": 1,
  "api_key_id": null
}
```

### Trading Parameters

```json
{
  "initial_balance_idr": 10000000,           // Starting capital (IDR)
  "order_size_idr": 100000,                   // Size per order (IDR)
  "min_gap_percent": 0.5,                     // Minimum spread to trade (%)
  "reposition_threshold_percent": 0.3,        // When to adjust orders (%)
  "max_loss_idr": 500000                      // Circuit breaker (total loss limit)
}
```

**Field Descriptions**:
- `initial_balance_idr`: Total capital allocated to the bot (paper mode initial balance)
- `order_size_idr`: Amount in IDR to use per buy order
- `min_gap_percent`: Minimum bid-ask spread required to place orders (must cover fees + profit)
- `reposition_threshold_percent`: Price movement % that triggers order cancellation and replacement
- `max_loss_idr`: Stop bot if total losses exceed this amount (circuit breaker)

### Balance Structure (JSONB)

```json
{
  "idr": 9500000.50,
  "btc": 0.00153846
}
```

**Dynamic Currency**: The non-IDR currency is determined from `coins.base_currency` (e.g., "btc", "eth", "ozone")

---

## State Machine

```
┌──────────────┐
│   STOPPED    │
└──────┬───────┘
       │ StartBot()
       ↓
┌──────────────┐
│   STARTING   │
└──────┬───────┘
       │ ✓ Load config
       │ ✓ Setup TradeClient (paper/live)
       │ ✓ Setup PrivateWSClient (live)
       │ ✓ Sync balance
       │ ✓ Subscribe to ticker
       │ ✓ Place initial order
       ↓
┌──────────────┐
│   RUNNING    │←──────────────┐
└──────┬───────┘               │
       │                       │
       │ Events:               │
       │ • Ticker update       │
       │ • Order filled        │
       │ • Manual stop         │
       │                       │
       ↓                       │
┌──────────────────┐           │
│ Process Event    │───────────┘
│ • Place order    │
│ • Cancel order   │
│ • Update balance │
└──────────────────┘
       │
       │ StopBot()
       ↓
┌──────────────┐
│   STOPPED    │
└──────────────┘
```

---

## Core Trading Logic

### 1. Competitive Pricing

Market makers must be **more competitive** than the current best bid/ask to get filled:

```go
// Get tick size from coins.price_precision
tickSize := 1.0 / math.Pow(10, float64(coin.PricePrecision))

// BUY: Offer MORE than current best bid
competitiveBuyPrice := currentBid + tickSize

// SELL: Offer LESS than current best ask
competitiveSellPrice := currentAsk - tickSize
```

**Example (BTC/IDR, price_precision = 2)**:
```
Current orderbook:
  Best Bid: 650,000,000 IDR
  Best Ask: 650,500,000 IDR

Our competitive prices:
  Buy Price:  650,010,000 IDR (better than 650,000,000)
  Sell Price: 650,490,000 IDR (better than 650,500,000)
```

### 2. Minimum Gap Check

Only trade when the spread is profitable enough to cover fees:

```go
func checkMinimumGap(bestBid, bestAsk, minGapPercent float64) bool {
    spreadPercent := (bestAsk - bestBid) / bestBid * 100
    return spreadPercent >= minGapPercent
}
```

**Why?**
- Indodax fees: ~0.1% maker + 0.1% taker = 0.2% total
- `min_gap_percent` should be > 0.2% to ensure profit after fees

**Example**:
```
min_gap_percent = 0.5%

Scenario 1: Tight spread
  Bid: 650,000,000, Ask: 650,100,000
  Spread: (650,100,000 - 650,000,000) / 650,000,000 × 100 = 0.015%
  0.015% < 0.5% → SKIP (not profitable)

Scenario 2: Good spread
  Bid: 650,000,000, Ask: 653,250,000
  Spread: (653,250,000 - 650,000,000) / 650,000,000 × 100 = 0.5%
  0.5% >= 0.5% → PLACE ORDER ✓
```

### 3. Inventory-Based Order Placement

Decision tree for initial order:

```
No Active Orders
       │
       ↓
Check Current Inventory
       │
┌──────┴──────┬──────────┐
│             │          │
Have Coins    Have IDR   Have Both
(sell first)  (buy first) (prefer sell)
│             │          │
↓             ↓          ↓
Place SELL    Place BUY  Place SELL
ALL coins     orderSize  ALL coins
```

**Key Rule**: SELL orders always sell **ALL available coins**, not just `order_size_idr` worth.

**Example**:
```
order_size_idr = 100,000 IDR
Available balance:
  IDR: 9,500,000
  BTC: 0.00153846

Decision: Have coins → Place SELL for ALL 0.00153846 BTC
  (not just 100,000 IDR worth)
```

### 4. Order Adjustment Logic

When active order exists, check if price has moved beyond threshold:

```go
func shouldRepositionOrder(orderPrice, currentMarketPrice, thresholdPercent float64) bool {
    diff := math.Abs(orderPrice - currentMarketPrice) / orderPrice * 100
    return diff > thresholdPercent
}
```

**Flow**:
```
Active BUY @ 650,000,000
       │
       ↓
Ticker: currentBid = 650,500,000
       │
       ↓
Check: |650,000,000 - 650,500,000| / 650,000,000 = 0.077%
       │
       ↓
0.077% < 0.3% (threshold) → DO NOTHING
       │
       ↓
Ticker: currentBid = 652,000,000
       │
       ↓
Check: |650,000,000 - 652,000,000| / 650,000,000 = 0.308%
       │
       ↓
0.308% > 0.3% → CANCEL order, wait for next ticker
       │
       ↓
Next ticker will see no active order → place new BUY @ 652,010,000
```

---

## Main Event Loop

### Event-Driven Architecture

```go
func (s *MarketMakingService) runBot(inst *BotInstance) {
    ticker := time.NewTicker(500 * time.Millisecond) // Rate limiter
    defer ticker.Stop()
    
    for {
        select {
        case <-inst.StopChan:
            return
            
        case tickerUpdate := <-inst.TickerChan:
            // Rate limit: only process if enough time has passed
            if !s.rateLimiter.CanPlaceOrder() {
                continue
            }
            s.handleTicker(inst, tickerUpdate)
            
        case orderFilled := <-inst.OrderFilledChan:
            s.handleFilled(inst, orderFilled)
        }
    }
}
```

### handleTicker() - ALL Order Logic Here

```go
func (s *MarketMakingService) handleTicker(inst *BotInstance, ticker *TickerData) {
    // 1. Update current prices
    inst.CurrentBid = ticker.Buy
    inst.CurrentAsk = ticker.Sell
    
    // 2. Check minimum gap
    if !checkMinimumGap(ticker.Buy, ticker.Sell, inst.Config.MinGapPercent) {
        return // Spread too tight
    }
    
    // 3. Check for active orders
    if inst.ActiveOrder == nil {
        // No active order → place new based on inventory
        s.placeNewOrder(inst)
    } else {
        // Active order → check if needs repositioning
        s.checkReposition(inst)
    }
}
```

### placeNewOrder() - Inventory-Based

```go
func (s *MarketMakingService) placeNewOrder(inst *BotInstance) {
    balance := inst.VirtualBalance
    coin := inst.Coin
    
    // Determine order side based on inventory
    idrBalance := balance["idr"]
    coinBalance := balance[coin.BaseCurrency]
    
    var side string
    var price, amount float64
    
    if coinBalance >= coin.MinVolume {
        // Have coins → SELL ALL
        side = "sell"
        price = inst.CurrentAsk - getTickSize(coin.PricePrecision)
        amount = coinBalance
    } else if idrBalance >= inst.Config.OrderSizeIDR {
        // Have IDR → BUY
        side = "buy"
        price = inst.CurrentBid + getTickSize(coin.PricePrecision)
        amount = inst.Config.OrderSizeIDR / price
    } else {
        // Insufficient balance
        s.logger.Warn.Printf("Insufficient balance: IDR=%.2f, %s=%.8f",
            idrBalance, coin.BaseCurrency, coinBalance)
        return
    }
    
    // Round amount to volume_precision
    amount = roundToDecimal(amount, coin.VolumePrecision)
    
    // Validate minimum volume and amount
    if amount < coin.MinVolume {
        return
    }
    if amount * price < coin.MinAmount {
        return
    }
    
    // Place order
    result, err := inst.TradeClient.Trade(side, inst.Config.Pair, price, amount, "limit")
    if err != nil {
        s.logger.Error.Printf("Failed to place %s order: %v", side, err)
        return
    }
    
    // Save order to DB and track
    order := &models.Order{
        BotConfigID:  inst.BotID,
        UserID:       inst.Config.UserID,
        OrderID:      result.OrderID,
        Pair:         inst.Config.Pair,
        Side:         side,
        Status:       "open",
        Price:        price,
        Amount:       amount,
        IsPaperTrade: inst.Config.IsPaperTrading,
    }
    s.orderRepo.Create(order)
    
    inst.ActiveOrder = order
    
    s.logger.Info.Printf("Placed %s order: %.8f @ %.2f", side, amount, price)
}
```

### checkReposition() - Price Movement Check

```go
func (s *MarketMakingService) checkReposition(inst *BotInstance) {
    order := inst.ActiveOrder
    
    var currentMarketPrice float64
    if order.Side == "buy" {
        currentMarketPrice = inst.CurrentBid
    } else {
        currentMarketPrice = inst.CurrentAsk
    }
    
    // Check if price moved beyond threshold
    if shouldRepositionOrder(order.Price, currentMarketPrice, inst.Config.RepositionThresholdPercent) {
        s.logger.Info.Printf("Price moved, cancelling %s order @ %.2f", order.Side, order.Price)
        
        // Cancel order (paper or live)
        if inst.Config.IsPaperTrading {
            order.Status = "cancelled"
            s.orderRepo.UpdateStatus(order.ID, "cancelled")
        } else {
            err := inst.TradeClient.CancelOrder(inst.Config.Pair, order.OrderID)
            if err != nil {
                s.logger.Error.Printf("Failed to cancel order: %v", err)
                return
            }
            s.orderRepo.UpdateStatus(order.ID, "cancelled")
        }
        
        inst.ActiveOrder = nil
        
        // Next ticker will place new order at current price
    }
}
```

### handleFilled() - ONLY Update Balance

```go
func (s *MarketMakingService) handleFilled(inst *BotInstance, order *models.Order) {
    coin := inst.Coin
    balance := inst.VirtualBalance
    
    // Update balance based on side
    if order.Side == "buy" {
        // BUY filled: IDR decreased, coins increased
        balance["idr"] -= order.Amount * order.Price
        balance[coin.BaseCurrency] += order.Amount
    } else {
        // SELL filled: coins decreased, IDR increased
        balance[coin.BaseCurrency] -= order.Amount
        balance["idr"] += order.Amount * order.Price
        
        // Calculate and update profit
        profit := s.calculateProfit(order)
        inst.Config.TotalProfitIDR += profit
        inst.Config.TotalTrades++
        if profit > 0 {
            inst.Config.WinningTrades++
        }
        
        s.botRepo.UpdateStats(inst.BotID, inst.Config.TotalTrades, 
            inst.Config.WinningTrades, inst.Config.TotalProfitIDR)
        
        // Check circuit breaker
        if inst.Config.TotalProfitIDR < -inst.Config.MaxLossIDR {
            s.logger.Warn.Printf("Max loss reached: %.2f", inst.Config.TotalProfitIDR)
            s.StopBot(inst.BotID, "Max loss limit reached")
            return
        }
    }
    
    // Save balance to DB
    s.botRepo.UpdateBalance(inst.BotID, balance)
    inst.VirtualBalance = balance
    
    // Remove from active orders
    inst.ActiveOrder = nil
    
    // Update order status
    order.Status = "filled"
    s.orderRepo.UpdateStatus(order.ID, "filled")
    
    // Broadcast update to frontend
    s.broadcastBotUpdate("balance_updated", inst)
    
    // DONE - Next ticker will place new order based on current inventory
}
```

---

## Paper Trading vs Live Trading

### Paper Trading

**Order Execution**:
```go
func (c *PaperTradeClient) Trade(side, pair string, price, amount float64, orderType string) (*TradeResult, error) {
    // Generate paper order ID
    orderID := generatePaperOrderID()
    
    // Simulate 5-second fill delay
    go func() {
        time.Sleep(5 * time.Second)
        
        // Mark as filled
        c.onFilled(&Order{
            OrderID: orderID,
            Side:    side,
            Price:   price,
            Amount:  amount,
        })
    }()
    
    return &TradeResult{OrderID: orderID}, nil
}
```

**Balance**: Stored in `bot_configs.balances` (JSONB)

### Live Trading

**Order Execution**:
```go
func (c *LiveTradeClient) Trade(side, pair string, price, amount float64, orderType string) (*TradeResult, error) {
    // Place order on Indodax
    result, err := c.indodaxClient.PlaceOrder(side, pair, price, amount)
    if err != nil {
        return nil, err
    }
    
    // Track order via Private WebSocket
    c.orderTracker.Track(result.OrderID, func(order *Order) {
        c.onFilled(order)
    })
    
    return result, nil
}
```

**Balance**:
- Virtual balance in `bot_configs.balances` (source of truth)
- Synced from exchange on bot start and after each fill
- Cap IDR to `initial_balance_idr` (don't exceed allocated capital)

**Balance Sync**:
```go
func (s *MarketMakingService) syncBalance(inst *BotInstance) error {
    info, err := inst.TradeClient.GetInfo()
    if err != nil {
        return err
    }
    
    // Cap IDR balance
    idrBalance := info.Balance["idr"]
    if idrBalance > inst.Config.InitialBalanceIDR {
        idrBalance = inst.Config.InitialBalanceIDR
    }
    
    inst.VirtualBalance = map[string]float64{
        "idr": idrBalance,
        inst.Coin.BaseCurrency: info.Balance[inst.Coin.BaseCurrency],
    }
    
    s.botRepo.UpdateBalance(inst.BotID, inst.VirtualBalance)
    return nil
}
```

---

## Indodax Integration

### Ticker Subscription (Shared WebSocket)

**Subscription Manager** handles multiple bots subscribing to the same pair:

```go
type SubscriptionManager struct {
    subscriptions map[string][]int64  // pair -> [botIDs]
    mu            sync.RWMutex
    wsClient      *IndodaxWSClient
}

func (sm *SubscriptionManager) Subscribe(pair string, botID int64) error {
    sm.mu.Lock()
    defer sm.mu.Unlock()
    
    // First subscriber? Actually subscribe to WebSocket
    if len(sm.subscriptions[pair]) == 0 {
        if err := sm.wsClient.SubscribeTicker(pair); err != nil {
            return err
        }
    }
    
    sm.subscriptions[pair] = append(sm.subscriptions[pair], botID)
    return nil
}

func (sm *SubscriptionManager) Unsubscribe(pair string, botID int64) {
    sm.mu.Lock()
    defer sm.mu.Unlock()
    
    // Remove bot from list
    bots := sm.subscriptions[pair]
    for i, id := range bots {
        if id == botID {
            sm.subscriptions[pair] = append(bots[:i], bots[i+1:]...)
            break
        }
    }
    
    // Last subscriber? Unsubscribe from WebSocket
    if len(sm.subscriptions[pair]) == 0 {
        sm.wsClient.UnsubscribeTicker(pair)
        delete(sm.subscriptions, pair)
    }
}
```

### Ticker Distribution

```go
func (c *IndodaxWSClient) handleTickerUpdate(data *TickerData) {
    pair := data.Symbol
    
    // Get all bots subscribed to this pair
    botIDs := c.subscriptionManager.GetSubscribers(pair)
    
    // Send to each bot's ticker channel (non-blocking)
    for _, botID := range botIDs {
        instance := c.botManager.GetInstance(botID)
        if instance != nil {
            select {
            case instance.TickerChan <- data:
            default:
                // Channel full, skip
            }
        }
    }
}
```

### Private WebSocket (Live Trading)

**Order Fulfillment Tracking**:
```go
func (c *LiveTradeClient) subscribeOrderUpdates() {
    // Generate token
    token, _ := c.indodaxClient.GeneratePrivateWSToken()
    
    // Connect
    conn, _ := websocket.Dial("wss://pws.indodax.com/ws/")
    
    // Authenticate
    conn.WriteJSON(map[string]interface{}{
        "params": map[string]string{"token": token},
        "id": 1,
    })
    
    // Subscribe to order updates
    conn.WriteJSON(map[string]interface{}{
        "method": "subscribe",
        "params": map[string]interface{}{"channel": "order"},
        "id": 2,
    })
    
    // Listen
    go c.listenOrderUpdates(conn)
}

func (c *LiveTradeClient) listenOrderUpdates(conn *websocket.Conn) {
    for {
        var msg IndodaxOrderUpdate
        if err := conn.ReadJSON(&msg); err != nil {
            c.reconnect()
            return
        }
        
        if msg.Status == "filled" {
            c.handleOrderFilled(msg.OrderID, msg.Amount)
        }
    }
}
```

---

## Rate Limiting & Debouncing

### Indodax Rate Limits

- **Trade API**: 20 req/sec
- **Cancel API**: 30 req/sec
- **getInfo API**: 180 req/minute

### Debounce Strategy

```go
type RateLimiter struct {
    lastOrderTime time.Time
    minInterval   time.Duration  // 500ms
    mu            sync.Mutex
}

func (rl *RateLimiter) CanPlaceOrder() bool {
    rl.mu.Lock()
    defer rl.mu.Unlock()
    
    if time.Since(rl.lastOrderTime) < rl.minInterval {
        return false
    }
    rl.lastOrderTime = time.Now()
    return true
}
```

**Why?**
- Ticker updates arrive every 1-2 seconds
- Debounce prevents hitting rate limits
- Skipped tickers are acceptable (next one will process)

---

## Redis Schema

### Bot Configuration

```
Key: bot:mm:{bot_id}
Type: Hash
Fields:
  user_id
  coin_id
  pair
  initial_balance_idr
  order_size_idr
  min_gap_percent
  reposition_threshold_percent
  max_loss_idr
  balances (JSON)
  total_trades
  winning_trades
  total_profit_idr
  status
  created_at
  updated_at
```

### Active Orders

```
Key: order:{order_id}
Type: Hash
Fields:
  bot_config_id
  user_id
  pair
  side
  price
  amount
  status
  created_at

Key: bot_active_order:{bot_id}
Type: String
Value: order_id
TTL: None
```

### Balance Tracking

```
Key: bot_balance:{bot_id}
Type: Hash
Fields:
  idr
  {base_currency}
TTL: None (persistent, updated on each trade)
```

---

## Database Schema (PostgreSQL)

### bot_configs Table

```sql
CREATE TABLE bot_configs (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    coin_id BIGINT NOT NULL REFERENCES coins(id),
    name VARCHAR(255) NOT NULL,
    type VARCHAR(50) NOT NULL DEFAULT 'market_maker',
    pair VARCHAR(20) NOT NULL,
    is_paper_trading BOOLEAN NOT NULL DEFAULT true,
    api_key_id BIGINT REFERENCES api_credentials(id),
    
    -- Trading parameters
    initial_balance_idr NUMERIC(20,2) NOT NULL,
    order_size_idr NUMERIC(20,2) NOT NULL,
    min_gap_percent NUMERIC(10,4) NOT NULL,
    reposition_threshold_percent NUMERIC(10,4) NOT NULL,
    max_loss_idr NUMERIC(20,2) NOT NULL,
    
    -- Balance (JSONB)
    balances JSONB NOT NULL DEFAULT '{}',
    
    -- Statistics
    total_trades INTEGER NOT NULL DEFAULT 0,
    winning_trades INTEGER NOT NULL DEFAULT 0,
    total_profit_idr NUMERIC(20,2) NOT NULL DEFAULT 0,
    
    -- Status
    status VARCHAR(50) NOT NULL DEFAULT 'stopped',
    error_message TEXT,
    
    -- Timestamps
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    
    -- Constraints
    UNIQUE(user_id, coin_id),
    CHECK (order_size_idr > 0),
    CHECK (min_gap_percent >= 0),
    CHECK (max_loss_idr > 0)
);

CREATE INDEX idx_bot_configs_user_id ON bot_configs(user_id);
CREATE INDEX idx_bot_configs_status ON bot_configs(status);
CREATE INDEX idx_bot_configs_pair ON bot_configs(pair);
```

### orders Table

```sql
CREATE TABLE orders (
    id BIGSERIAL PRIMARY KEY,
    bot_config_id BIGINT NOT NULL REFERENCES bot_configs(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id),
    
    -- Order details
    order_id VARCHAR(255) NOT NULL,
    pair VARCHAR(20) NOT NULL,
    side VARCHAR(10) NOT NULL,
    status VARCHAR(50) NOT NULL,
    price NUMERIC(20,8) NOT NULL,
    amount NUMERIC(20,8) NOT NULL,
    
    -- Paper trading flag
    is_paper_trade BOOLEAN NOT NULL DEFAULT false,
    
    -- Timestamps
    created_at TIMESTAMP DEFAULT NOW(),
    filled_at TIMESTAMP,
    
    -- Indexes
    UNIQUE(order_id, bot_config_id)
);

CREATE INDEX idx_orders_bot_config_id ON orders(bot_config_id);
CREATE INDEX idx_orders_user_id ON orders(user_id);
CREATE INDEX idx_orders_status ON orders(status);
CREATE INDEX idx_orders_pair ON orders(pair);
```

---

## API Endpoints

### POST /api/v1/bots/market-maker
Create market maker bot

**Request**:
```json
{
  "name": "BTC/IDR MM",
  "pair": "btcidr",
  "is_paper_trading": true,
  "api_key_id": null,
  "initial_balance_idr": 10000000,
  "order_size_idr": 100000,
  "min_gap_percent": 0.5,
  "reposition_threshold_percent": 0.3,
  "max_loss_idr": 500000
}
```

**Response**:
```json
{
  "success": true,
  "data": {
    "id": 1,
    "name": "BTC/IDR MM",
    "type": "market_maker",
    "pair": "btcidr",
    "status": "stopped",
    "balances": {
      "idr": 10000000,
      "btc": 0
    },
    "total_trades": 0,
    "total_profit_idr": 0,
    "created_at": "2024-01-07T18:00:00Z"
  }
}
```

### POST /api/v1/bots/:id/start
Start bot

**Response**:
```json
{
  "success": true,
  "data": {
    "id": 1,
    "status": "running",
    "message": "Bot started successfully"
  }
}
```

### POST /api/v1/bots/:id/stop
Stop bot

**Response**:
```json
{
  "success": true,
  "data": {
    "id": 1,
    "status": "stopped",
    "final_balance": {
      "idr": 9985000,
      "btc": 0.00153846
    },
    "total_trades": 25,
    "total_profit_idr": -15000
  }
}
```

### GET /api/v1/bots/:id/summary
Get bot summary

**Response**:
```json
{
  "success": true,
  "data": {
    "id": 1,
    "name": "BTC/IDR MM",
    "pair": "btcidr",
    "status": "running",
    "balances": {
      "idr": 9950000,
      "btc": 0.00076923
    },
    "total_trades": 10,
    "winning_trades": 7,
    "total_profit_idr": -50000,
    "active_order": {
      "id": 123,
      "side": "sell",
      "price": 650500000,
      "amount": 0.00076923,
      "created_at": "2024-01-07T18:30:00Z"
    },
    "last_10_orders": [...]
  }
}
```

---

## WebSocket Real-time Updates

### Bot Status Update

```json
{
  "type": "bot_status",
  "data": {
    "bot_id": 1,
    "status": "running",
    "timestamp": "2024-01-07T18:00:00Z"
  }
}
```

### Balance Update

```json
{
  "type": "balance_update",
  "data": {
    "bot_id": 1,
    "balances": {
      "idr": 9950000,
      "btc": 0.00076923
    },
    "total_profit_idr": -50000
  }
}
```

### Order Placed

```json
{
  "type": "order_placed",
  "data": {
    "bot_id": 1,
    "order_id": 123,
    "side": "buy",
    "price": 650000000,
    "amount": 0.00153846
  }
}
```

### Order Filled

```json
{
  "type": "order_filled",
  "data": {
    "bot_id": 1,
    "order_id": 123,
    "side": "buy",
    "price": 650000000,
    "amount": 0.00153846,
    "new_balance": {
      "idr": 9900000,
      "btc": 0.00153846
    }
  }
}
```

---

## Error Handling

### Insufficient Balance

```json
{
  "success": false,
  "error": {
    "code": "INSUFFICIENT_BALANCE",
    "message": "Insufficient IDR balance to place buy order",
    "details": {
      "required": 100000,
      "available": 50000
    }
  }
}
```

### Order Placement Failed

```json
{
  "success": false,
  "error": {
    "code": "ORDER_FAILED",
    "message": "Failed to place order on exchange",
    "details": "Indodax API error: Invalid price increment"
  }
}
```

### Max Loss Reached

```json
{
  "type": "bot_stopped",
  "data": {
    "bot_id": 1,
    "reason": "max_loss_reached",
    "total_profit_idr": -520000,
    "max_loss_idr": 500000,
    "message": "Bot stopped due to circuit breaker"
  }
}
```

---

## Performance Metrics

### Key Metrics to Track

- **Total Trades**: Number of completed buy-sell cycles
- **Win Rate**: `winning_trades / total_trades * 100`
- **Average Profit per Trade**: `total_profit_idr / total_trades`
- **Uptime**: Time bot has been running
- **Order Fill Rate**: % of orders that get filled
- **Average Spread Captured**: Average profit per cycle

### Example Summary

```
Bot: BTC/IDR Market Maker
Status: Running (3h 25m)
Total Trades: 45
Win Rate: 68.9% (31/45)
Total Profit: +125,000 IDR (+1.25%)
Avg Profit/Trade: 2,778 IDR
Current Balance: 10,125,000 IDR (equiv)
Active Order: SELL 0.00153846 BTC @ 652,000,000
```

---

## Testing Strategy

### Unit Tests
- ✅ Competitive pricing calculation
- ✅ Minimum gap validation
- ✅ Inventory-based order placement logic
- ✅ Reposition threshold checks
- ✅ Profit calculation
- ✅ Circuit breaker logic

### Integration Tests
- ✅ Full trading cycle (buy → fill → sell → fill)
- ✅ Order cancellation and replacement
- ✅ Balance updates
- ✅ WebSocket ticker integration
- ✅ Private WebSocket order tracking (live)

### Paper Trading Testing
1. Start bot with paper balance
2. Verify orders placed at competitive prices
3. Verify automatic fill after 5 seconds
4. Verify balance updates correctly
5. Verify cycle continues (buy → sell → buy)

### Live Trading Testing (Testnet/Small Amounts)
1. Start with minimal balance (100k IDR)
2. Verify real orders placed on exchange
3. Verify order tracking via Private WebSocket
4. Verify balance synced from exchange
5. Monitor for errors/edge cases

---

## Common Issues & Solutions

### Issue: Bot not placing orders

**Causes**:
1. Spread too tight (`min_gap_percent` too high)
2. Insufficient balance
3. Max loss reached

**Solution**: Check logs, adjust `min_gap_percent`, add balance

### Issue: Orders not filling

**Causes**:
1. Prices not competitive enough
2. Low market liquidity
3. Price moved before fill

**Solution**: Orders will auto-adjust on next ticker update

### Issue: Losing money consistently

**Causes**:
1. `min_gap_percent` too low (not covering fees)
2. Market too volatile for strategy
3. Frequent order adjustments (fee accumulation)

**Solution**:
- Increase `min_gap_percent` to 0.5-1.0%
- Increase `reposition_threshold_percent` to reduce adjustments
- Choose less volatile pairs

---

## Best Practices

### Configuration
- ✅ Start with `min_gap_percent` = 0.5% or higher
- ✅ Set `max_loss_idr` to 5-10% of `initial_balance_idr`
- ✅ Use `order_size_idr` = 1-5% of `initial_balance_idr`
- ✅ Test thoroughly in paper mode before live trading

### Pair Selection
- ✅ Choose pairs with consistent volume
- ✅ Avoid highly volatile pairs
- ✅ Check 24h spread patterns before deploying

### Risk Management
- ✅ Monitor bot daily
- ✅ Review profit/loss trends
- ✅ Adjust parameters based on performance
- ✅ Use circuit breaker (`max_loss_idr`)

---

## Conclusion

The Market Maker Bot is a **low-risk, steady-profit strategy** suitable for:
- ✅ Consistent, low-volatility markets
- ✅ Pairs with good liquidity
- ✅ Traders who prefer automated, hands-off trading

**Success Factors**:
- Proper `min_gap_percent` configuration (>0.5%)
- Choosing the right trading pair
- Adequate capital (10M+ IDR recommended)
- Regular monitoring and adjustment

