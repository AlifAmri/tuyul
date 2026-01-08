# Pump Hunter Bot

## Overview

The Pump Hunter Bot is an **event-driven momentum trading strategy** that identifies and trades rapid price increases ("pumps") across all Indodax trading pairs simultaneously. It uses multi-timeframe analysis and pump score calculation to detect buying pressure and automatically opens/closes positions based on profit targets and stop-loss conditions.

**Strategy Type**: Momentum / Trend following
**Risk Level**: High
**Suitable Markets**: Volatile markets with pump activity
**Timeframe**: Minutes to hours per position

---

## Key Characteristics

- ✅ **Scans all trading pairs** simultaneously (market-wide monitoring)
- ✅ **Multi-timeframe analysis** (1m, 5m, 15m, 30m)
- ✅ **Pump score based entry** (from Market Analysis data)
- ✅ **Multiple concurrent positions** with risk limits
- ✅ **Automated exit conditions** (take profit, stop-loss, trailing stop, time-based)
- ✅ **Event-driven architecture** (reacts to timeframe updates from Market Analysis)
- ✅ **Directional trading** (long only - buy then sell)
- ✅ **Risk management** (position sizing, daily loss limits, cooldown periods)

---

## Bot Configuration

### Core Settings

```json
{
  "name": "Pump Hunter Pro",
  "type": "pump_hunter",
  "is_paper_trading": true,
  "user_id": 1,
  "api_key_id": null
}
```

### Entry Rules

```json
{
  "min_pump_score": 50.0,                     // Minimum pump score to enter (required)
  "min_timeframes_positive": 2,               // Min positive timeframes (default: 2)
  "min_24h_volume_idr": 1000000000,           // Min 24h volume (1B IDR, ~$65k)
  "min_price_idr": 100,                       // Min price to avoid dust coins
  "excluded_pairs": ["usdtidr", "usdcidr"],   // Stablecoins to exclude
  "allowed_pairs": []                         // If set, only trade these pairs
}
```

**Entry Logic**:
- `min_pump_score`: From Market Analysis pump score (see `blueprint_market_analysis.md`)
- `min_timeframes_positive`: How many of the 4 timeframes (1m, 5m, 15m, 30m) must be positive
- `min_24h_volume_idr`: Filter low-liquidity pairs
- `min_price_idr`: Avoid extremely cheap coins (high volatility, low volume)

### Exit Rules

```json
{
  "target_profit_percent": 3.0,               // Take profit at +3% (required)
  "stop_loss_percent": 1.5,                   // Stop loss at -1.5% (required)
  "trailing_stop_enabled": true,              // Enable trailing stop
  "trailing_stop_percent": 1.0,               // Trail by 1% from highest
  "max_hold_minutes": 30,                     // Max time to hold position
  "exit_on_pump_score_drop": true,            // Exit if score drops
  "pump_score_drop_threshold": 20.0           // Exit if score < 20
}
```

**Exit Conditions** (first one triggered closes position):
1. **Take Profit**: Current price >= entry price × (1 + target_profit_percent/100)
2. **Stop Loss**: Current price <= entry price × (1 - stop_loss_percent/100)
3. **Trailing Stop**: Price drops from highest by trailing_stop_percent
4. **Max Hold Time**: Position held longer than max_hold_minutes
5. **Pump Score Drop**: Pump score falls below pump_score_drop_threshold

### Risk Management

```json
{
  "max_position_idr": 500000,                 // Max IDR per position (required)
  "max_concurrent_positions": 3,              // Max open positions at once (required)
  "daily_loss_limit_idr": 1000000,            // Stop after losing this much today
  "cooldown_after_loss_minutes": 10,          // Wait after a losing trade
  "min_balance_idr": 100000                   // Minimum balance to maintain
}
```

### Paper Trading Balance

```json
{
  "paper_balance": {
    "idr": 5000000
  }
}
```

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
       │ ✓ Setup TradeClient
       │ ✓ Setup PrivateWSClient (live)
       │ ✓ Load open positions from DB
       │ ✓ Restore order tracking (live)
       │ ✓ Subscribe to coin updates
       │ ✓ Start runBot goroutine
       ↓
┌──────────────┐
│   RUNNING    │←──────────────┐
└──────┬───────┘               │
       │                       │
       │ Events:               │
       │ • Coin update         │
       │ • Entry signal        │
       │ • Exit signal         │
       │ • Order filled        │
       │                       │
       ↓                       │
┌──────────────────┐           │
│ Process Event    │───────────┘
│ • Open position  │
│ • Close position │
│ • Monitor exits  │
└──────────────────┘
       │
       │ StopBot()
       ↓
┌──────────────┐
│   STOPPED    │
└──────────────┘
```

---

## Integration with Market Analysis

The Pump Hunter Bot depends on the **Market Analysis** system (see `blueprint_market_analysis.md`) for:

1. **Coin Updates**: Real-time coin data with pump scores
2. **Timeframe Data**: OHLC data for 1m, 5m, 15m, 30m
3. **Pump Score Calculation**: Multi-timeframe momentum indicator

**Data Flow**:
```
Indodax WebSocket
       ↓
MarketDataService (processes market_summary)
       ↓
TimeframeManager (updates timeframes every minute)
       ↓
CalculatePumpScore() (computes score)
       ↓
Broadcast coin update to PumpBotService
       ↓
PumpBotService distributes to all running pump bot instances
       ↓
Each bot instance checks entry/exit conditions
```

**Coin Update Structure**:
```go
type Coin struct {
    PairID          string
    CurrentPrice    float64
    Volume24h       float64
    VolumeIDR       float64
    Change24h       float64
    Timeframes      Timeframes  // 1m, 5m, 15m, 30m OHLC + Trx
    PumpScore       float64     // Calculated score
    LastUpdate      time.Time
}
```

### Signal Buffering and Prioritization

**Problem**: When multiple coins pump simultaneously, the bot needs to decide which signals to act on first.

**Solution**: Signal buffering with priority-based processing:

```go
type PumpSignal struct {
    Coin      *model.Coin
    Score     float64
    Timestamp time.Time
}

type PumpHunterInstance struct {
    SignalBuffer  map[string]*PumpSignal  // pair -> signal
    signalMu      sync.Mutex
    // ... other fields
}
```

**How It Works**:

1. **Buffer Signals**: When a coin update arrives with a pump score above threshold, it's added to the signal buffer (not immediately executed)
2. **Process by Priority**: A background goroutine processes buffered signals every 1 second, sorted by pump score (highest first)
3. **Respect Limits**: Only opens positions up to `max_concurrent_positions` limit
4. **Overwrite Lower Scores**: If a pair already has a buffered signal, only keep the one with the higher score

**Benefits**:
- ✅ Prioritizes strongest signals when multiple pumps occur
- ✅ Prevents opening too many positions at once
- ✅ Reduces noise from rapid score fluctuations
- ✅ Ensures best use of available capital

**Example Scenario**:
```
Time 10:00:00 - BTC pumps (score: 75) → buffered
Time 10:00:01 - ETH pumps (score: 85) → buffered
Time 10:00:02 - DOGE pumps (score: 60) → buffered
Time 10:00:03 - Process signals:
  1. ETH (85) → Open position ✓
  2. BTC (75) → Open position ✓
  3. DOGE (60) → Skip (max positions reached)
```

---

## Core Trading Logic

### 1. Entry Signal Detection

**When coin update received**, check all conditions:

```go
func (s *PumpBotService) checkEntryConditions(inst *BotInstance, coin *models.Coin) bool {
    config := inst.Config

    // 1. Check pump score
    if coin.PumpScore < config.EntryRules.MinPumpScore {
        return false
    }

    // 2. Check positive timeframes
    positiveCount := 0
    tf := coin.Timeframes

    if coin.CurrentPrice > tf.OneMinute.Open {
        positiveCount++
    }
    if coin.CurrentPrice > tf.FiveMinute.Open {
        positiveCount++
    }
    if coin.CurrentPrice > tf.FifteenMin.Open {
        positiveCount++
    }
    if coin.CurrentPrice > tf.ThirtyMin.Open {
        positiveCount++
    }

    if positiveCount < config.EntryRules.MinTimeframesPositive {
        return false
    }

    // 3. Check 24h volume
    if coin.VolumeIDR < config.EntryRules.Min24hVolumeIDR {
        return false
    }

    // 4. Check minimum price
    if coin.CurrentPrice < config.EntryRules.MinPriceIDR {
        return false
    }

    // 5. Check if pair is excluded
    for _, excluded := range config.EntryRules.ExcludedPairs {
        if excluded == coin.PairID {
            return false
        }
    }

    // 6. Check if already have position on this pair
    for _, pos := range inst.OpenPositions {
        if pos.Pair == coin.PairID {
            return false
        }
    }

    return true
}
```

**Example Entry Signal**:
```
Coin: SHIB/IDR
Current Price: 0.00015 IDR
Pump Score: 87.5
Timeframes:
  1m:  +2.0% (15 trx)
  5m:  +3.0% (45 trx)
  15m: +3.4% (120 trx)
  30m: +4.0% (250 trx)
  All 4 positive ✓

Volume 24h: 4.5B IDR ✓
Min Score: 50 ✓ (87.5 > 50)
Min Positive TFs: 2 ✓ (4 >= 2)

→ ENTER POSITION
```

### 2. Risk Management Checks

Before opening position:

```go
func (s *PumpBotService) canOpenNewPosition(inst *BotInstance) bool {
    config := inst.Config

    // 1. Max concurrent positions
    if len(inst.OpenPositions) >= config.RiskManagement.MaxConcurrentPositions {
        return false
    }

    // 2. Cooldown after loss
    if config.RiskManagement.CooldownAfterLossMinutes > 0 {
        cooldown := time.Duration(config.RiskManagement.CooldownAfterLossMinutes) * time.Minute
        if time.Since(inst.LastLossTime) < cooldown {
            return false
        }
    }

    // 3. Daily loss limit
    if config.RiskManagement.DailyLossLimitIDR > 0 {
        if inst.DailyLoss >= config.RiskManagement.DailyLossLimitIDR {
            s.logger.Warn.Printf("Daily loss limit reached: %.2f", inst.DailyLoss)
            s.pauseBot(inst.BotID, "Daily loss limit reached")
            return false
        }
    }

    return true
}
```

### 3. Position Sizing

```go
func (s *PumpBotService) calculatePositionSize(inst *BotInstance) float64 {
    config := inst.Config

    // Get available balance
    var balance float64
    if config.IsPaperTrading {
        balance = config.PaperBalance["idr"]
    } else {
        info, _ := inst.TradeClient.GetInfo()
        balance = info.Balance["idr"]
    }

    // Respect minimum balance
    if config.RiskManagement.MinBalanceIDR > 0 {
        if balance <= config.RiskManagement.MinBalanceIDR {
            return 0
        }
        balance -= config.RiskManagement.MinBalanceIDR
    }

    // Use MaxPositionIDR or available balance, whichever is smaller
    positionSize := config.RiskManagement.MaxPositionIDR
    if balance < positionSize {
        positionSize = balance
    }

    // Minimum 10k IDR per position
    if positionSize < 10000 {
        return 0
    }

    return positionSize
}
```

### 4. Open Position

```go
func (s *PumpBotService) openPosition(inst *BotInstance, coin *models.Coin, positionSizeIDR float64) {
    config := inst.Config
    pair := coin.PairID

    // Calculate entry price (use market buy)
    entryPrice := coin.CurrentPrice

    // Calculate quantity
    quantity := positionSizeIDR / entryPrice

    // Place buy order (market)
    result, err := inst.TradeClient.Trade("buy", pair, entryPrice, quantity, "market")
    if err != nil {
        s.logger.Error.Printf("Failed to place buy order: %v", err)
        return
    }

    // Create position record
    position := &models.Position{
        BotConfigID:    inst.BotID,
        UserID:         config.UserID,
        CoinID:         coin.ID,
        Pair:           pair,
        Status:         "buying",
        EntryPrice:     entryPrice,
        EntryQuantity:  quantity,
        EntryAmountIDR: positionSizeIDR,
        EntryOrderID:   &result.OrderID,
        EntryPumpScore: &coin.PumpScore,
        EntryAt:        time.Now(),
        HighestPrice:   entryPrice,
        LowestPrice:    entryPrice,
        IsPaperTrade:   config.IsPaperTrading,
    }

    s.positionRepo.Create(position)

    // Save buy order
    order := &models.Order{
        BotConfigID:  inst.BotID,
        UserID:       config.UserID,
        PositionID:   &position.ID,
        OrderID:      result.OrderID,
        Pair:         pair,
        Side:         "buy",
        Status:       "new",
        Price:        entryPrice,
        Amount:       quantity,
        IsPaperTrade: config.IsPaperTrading,
    }
    s.orderRepo.Create(order)

    // Track order fulfillment
    if !config.IsPaperTrading {
        inst.OrderTracker.TrackOrder(result.OrderID, func(filledOrder *Order) {
            s.handleBuyFilled(inst.BotID, position.ID, filledOrder)
        })
    } else {
        // Paper trading: mark as filled immediately
        position.Status = "open"
        s.positionRepo.UpdateStatus(position.ID, "open")

        // Update balance
        config.PaperBalance["idr"] -= positionSizeIDR
        baseCurrency := strings.TrimSuffix(pair, "idr")
        config.PaperBalance[baseCurrency] += quantity
        s.botRepo.UpdatePaperBalance(inst.BotID, config.PaperBalance)
    }

    // Add to open positions
    inst.OpenPositions[position.ID] = position

    s.logger.Info.Printf("Opened position %d: %s @ %.8f, amount: %.2f IDR, score: %.2f",
        position.ID, pair, entryPrice, positionSizeIDR, coin.PumpScore)
}
```

### 5. Exit Signal Detection

**Periodic check (every 10 seconds)**:

```go
func (s *PumpBotService) processOpenPositions(inst *BotInstance) {
    for _, position := range inst.OpenPositions {
        if position.Status != "open" {
            continue
        }

        // Get current price
        coin, _ := s.coinRepo.GetByPairID(position.Pair)
        currentPrice := coin.CurrentPrice

        // Update highest/lowest
        if currentPrice > position.HighestPrice {
            position.HighestPrice = currentPrice
            s.positionRepo.UpdateHighestPrice(position.ID, currentPrice)
        }
        if currentPrice < position.LowestPrice {
            position.LowestPrice = currentPrice
            s.positionRepo.UpdateLowestPrice(position.ID, currentPrice)
        }

        // Check exit conditions
        exitReason := s.checkExitConditions(inst, position, coin)

        if exitReason != "" {
            s.closePosition(inst, position, currentPrice, exitReason)
        }
    }
}
```

### 6. Exit Conditions Check

```go
func (s *PumpBotService) checkExitConditions(inst *BotInstance, position *models.Position, coin *models.Coin) string {
    config := inst.Config
    currentPrice := coin.CurrentPrice

    // Calculate profit %
    profitPercent := (currentPrice - position.EntryPrice) / position.EntryPrice * 100

    // 1. Take profit
    if profitPercent >= config.ExitRules.TargetProfitPercent {
        return "take_profit"
    }

    // 2. Stop loss
    if profitPercent <= -config.ExitRules.StopLossPercent {
        return "stop_loss"
    }

    // 3. Trailing stop
    if config.ExitRules.TrailingStopEnabled {
        dropFromHighest := (position.HighestPrice - currentPrice) / position.HighestPrice * 100
        if dropFromHighest >= config.ExitRules.TrailingStopPercent {
            return "trailing_stop"
        }
    }

    // 4. Max hold time
    if config.ExitRules.MaxHoldMinutes > 0 {
        holdDuration := time.Since(position.EntryAt)
        maxDuration := time.Duration(config.ExitRules.MaxHoldMinutes) * time.Minute
        if holdDuration >= maxDuration {
            return "max_hold_time"
        }
    }

    // 5. Pump score drop
    if config.ExitRules.ExitOnPumpScoreDrop {
        if coin.PumpScore < config.ExitRules.PumpScoreDropThreshold {
            return "pump_score_drop"
        }
    }

    return ""
}
```

### 7. Close Position

```go
func (s *PumpBotService) closePosition(inst *BotInstance, position *models.Position, exitPrice float64, reason string) {
    config := inst.Config
    pair := position.Pair

    // Place sell order (market)
    result, err := inst.TradeClient.Trade("sell", pair, exitPrice, position.EntryQuantity, "market")
    if err != nil {
        s.logger.Error.Printf("Failed to place sell order: %v", err)
        return
    }

    // Update position
    position.Status = "selling"
    position.ExitOrderID = &result.OrderID
    position.CloseReason = &reason
    s.positionRepo.UpdateStatus(position.ID, "selling")

    // Save sell order
    order := &models.Order{
        BotConfigID:  inst.BotID,
        UserID:       config.UserID,
        PositionID:   &position.ID,
        OrderID:      result.OrderID,
        Pair:         pair,
        Side:         "sell",
        Status:       "new",
        Price:        exitPrice,
        Amount:       position.EntryQuantity,
        IsPaperTrade: config.IsPaperTrading,
    }
    s.orderRepo.Create(order)

    // Track order fulfillment
    if !config.IsPaperTrading {
        inst.OrderTracker.TrackOrder(result.OrderID, func(filledOrder *Order) {
            s.handleSellFilled(inst.BotID, position.ID, filledOrder)
        })
    } else {
        // Paper trading: finalize immediately
        s.finalizePositionClose(inst, position, exitPrice)
    }

    s.logger.Info.Printf("Closing position %d: %s @ %.8f, reason: %s",
        position.ID, pair, exitPrice, reason)
}
```

### 8. Finalize Position Close

```go
func (s *PumpBotService) finalizePositionClose(inst *BotInstance, position *models.Position, exitPrice float64) {
    config := inst.Config

    // Calculate profit
    exitAmountIDR := exitPrice * position.EntryQuantity
    profitIDR := exitAmountIDR - position.EntryAmountIDR
    profitPercent := profitIDR / position.EntryAmountIDR * 100

    // Update position
    now := time.Now()
    position.Status = "closed"
    position.ExitPrice = &exitPrice
    position.ExitQuantity = &position.EntryQuantity
    position.ExitAmountIDR = &exitAmountIDR
    position.ExitAt = &now
    position.ProfitIDR = &profitIDR
    position.ProfitPercent = &profitPercent

    s.positionRepo.UpdateStatus(position.ID, "closed")
    s.positionRepo.UpdateProfit(position.ID, profitIDR, profitPercent)

    // Update bot statistics
    config.TotalTrades++
    if profitIDR > 0 {
        config.WinningTrades++
    }
    config.TotalProfitIDR += profitIDR

    s.botRepo.UpdateStats(config.ID, config.TotalTrades, config.WinningTrades, config.TotalProfitIDR)

    // Update daily loss tracking
    if profitIDR < 0 {
        inst.DailyLoss += math.Abs(profitIDR)
        inst.LastLossTime = time.Now()
    }

    // Update balance
    if config.IsPaperTrading {
        config.PaperBalance["idr"] += exitAmountIDR
        baseCurrency := strings.TrimSuffix(position.Pair, "idr")
        config.PaperBalance[baseCurrency] -= position.EntryQuantity
        s.botRepo.UpdatePaperBalance(inst.BotID, config.PaperBalance)
    } else {
        // Live: sync from exchange
        info, _ := inst.TradeClient.GetInfo()
        s.syncLiveBalance(inst.BotID, info.Balance)
    }

    // Remove from open positions
    delete(inst.OpenPositions, position.ID)

    // Broadcast update
    s.broadcastPositionClose(position)

    s.logger.Info.Printf("Position %d closed: profit %.2f IDR (%.2f%%), reason: %s",
        position.ID, profitIDR, profitPercent, *position.CloseReason)
}
```

---

## Redis Schema

### Bot Configuration

```
Key: bot:pump:{bot_id}
Type: Hash
Fields:
  user_id
  name
  is_paper_trading
  api_key_id
  entry_rules (JSON)
  exit_rules (JSON)
  risk_management (JSON)
  paper_balance (JSON)
  total_trades
  winning_trades
  total_profit_idr
  status
  created_at
  updated_at
```

### Open Positions

```
Key: position:{position_id}
Type: Hash
Fields:
  bot_config_id
  pair
  status (buying, open, selling, closed)
  entry_price
  entry_quantity
  entry_amount_idr
  entry_pump_score
  highest_price
  lowest_price
  exit_price
  profit_idr
  profit_percent
  close_reason
  entry_at
  exit_at

Key: bot_positions:{bot_id}
Type: Set
Members: position_id (only open/buying/selling positions)
```

### Daily Loss Tracking

```
Key: bot_daily_loss:{bot_id}:{date}
Type: Hash
Fields:
  total_loss
  last_loss_time
TTL: 24 hours
```

---

## Database Schema (PostgreSQL)

### bot_configs Table (Extended)

```sql
CREATE TABLE bot_configs (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    name VARCHAR(255) NOT NULL,
    type VARCHAR(50) NOT NULL, -- 'market_maker', 'pump_hunter', 'copilot'
    is_paper_trading BOOLEAN NOT NULL DEFAULT true,
    api_key_id BIGINT REFERENCES api_credentials(id),

    -- Pump hunter specific
    entry_rules JSONB,
    exit_rules JSONB,
    risk_management JSONB,
    paper_balance JSONB,

    -- Statistics
    total_trades INTEGER NOT NULL DEFAULT 0,
    winning_trades INTEGER NOT NULL DEFAULT 0,
    total_profit_idr NUMERIC(20,2) NOT NULL DEFAULT 0,

    -- Status
    status VARCHAR(50) NOT NULL DEFAULT 'stopped',
    error_message TEXT,

    -- Timestamps
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);
```

### positions Table

```sql
CREATE TABLE positions (
    id BIGSERIAL PRIMARY KEY,
    bot_config_id BIGINT NOT NULL REFERENCES bot_configs(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id),
    coin_id BIGINT NOT NULL REFERENCES coins(id),

    -- Position details
    pair VARCHAR(20) NOT NULL,
    status VARCHAR(50) NOT NULL, -- buying, open, selling, closed

    -- Entry
    entry_price NUMERIC(20,8) NOT NULL,
    entry_quantity NUMERIC(20,8) NOT NULL,
    entry_amount_idr NUMERIC(20,2) NOT NULL,
    entry_order_id VARCHAR(255),
    entry_pump_score NUMERIC(10,2),
    entry_at TIMESTAMP NOT NULL,

    -- Exit
    exit_price NUMERIC(20,8),
    exit_quantity NUMERIC(20,8),
    exit_amount_idr NUMERIC(20,2),
    exit_order_id VARCHAR(255),
    exit_at TIMESTAMP,

    -- Price tracking
    highest_price NUMERIC(20,8),
    lowest_price NUMERIC(20,8),

    -- Profit
    profit_idr NUMERIC(20,2),
    profit_percent NUMERIC(10,4),

    -- Close reason
    close_reason VARCHAR(100),

    -- Paper trade flag
    is_paper_trade BOOLEAN NOT NULL DEFAULT false,

    -- Timestamps
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_positions_bot_config_id ON positions(bot_config_id);
CREATE INDEX idx_positions_user_id ON positions(user_id);
CREATE INDEX idx_positions_status ON positions(status);
CREATE INDEX idx_positions_pair ON positions(pair);
```

---

## API Endpoints

### POST /api/v1/bots/pump-hunter
Create pump hunter bot

**Request**:
```json
{
  "name": "Pump Hunter Pro",
  "is_paper_trading": true,
  "api_key_id": null,
  "entry_rules": {
    "min_pump_score": 50.0,
    "min_timeframes_positive": 2,
    "min_24h_volume_idr": 1000000000,
    "min_price_idr": 100,
    "excluded_pairs": ["usdtidr"],
    "allowed_pairs": []
  },
  "exit_rules": {
    "target_profit_percent": 3.0,
    "stop_loss_percent": 1.5,
    "trailing_stop_enabled": true,
    "trailing_stop_percent": 1.0,
    "max_hold_minutes": 30,
    "exit_on_pump_score_drop": true,
    "pump_score_drop_threshold": 20.0
  },
  "risk_management": {
    "max_position_idr": 500000,
    "max_concurrent_positions": 3,
    "daily_loss_limit_idr": 1000000,
    "cooldown_after_loss_minutes": 10,
    "min_balance_idr": 100000
  },
  "paper_balance": {
    "idr": 5000000
  }
}
```

**Response**:
```json
{
  "success": true,
  "data": {
    "id": 1,
    "name": "Pump Hunter Pro",
    "type": "pump_hunter",
    "status": "stopped",
    "paper_balance": {
      "idr": 5000000
    },
    "total_trades": 0,
    "total_profit_idr": 0,
    "created_at": "2024-01-07T18:00:00Z"
  }
}
```

### GET /api/v1/bots/:id/positions
Get bot positions

**Query Parameters**:
- `status`: "open", "closed", "all" (default: "all")
- `limit`: int (default: 20)
- `offset`: int (default: 0)

**Response**:
```json
{
  "success": true,
  "data": {
    "positions": [
      {
        "id": 123,
        "pair": "shibidr",
        "status": "open",
        "entry_price": 0.00015,
        "entry_quantity": 3333333.33,
        "entry_amount_idr": 500000,
        "entry_pump_score": 87.5,
        "entry_at": "2024-01-07T18:30:00Z",
        "current_price": 0.000155,
        "profit_percent": 3.33,
        "profit_idr": 16650,
        "highest_price": 0.000156,
        "hold_duration_minutes": 5
      }
    ],
    "summary": {
      "open_positions": 1,
      "total_invested": 500000,
      "unrealized_pnl": 16650
    }
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
    "name": "Pump Hunter Pro",
    "type": "pump_hunter",
    "status": "running",
    "paper_balance": {
      "idr": 4850000
    },
    "total_trades": 15,
    "winning_trades": 10,
    "win_rate": 66.67,
    "total_profit_idr": 75000,
    "roi_percent": 1.5,
    "open_positions": 2,
    "daily_loss": 25000,
    "last_trade_at": "2024-01-07T19:15:00Z"
  }
}
```

---

## WebSocket Real-time Updates

### Position Opened

```json
{
  "type": "position_open",
  "data": {
    "bot_id": 1,
    "position_id": 123,
    "pair": "shibidr",
    "entry_price": 0.00015,
    "entry_amount_idr": 500000,
    "entry_pump_score": 87.5,
    "timestamp": "2024-01-07T18:30:00Z"
  }
}
```

### Position Closed

```json
{
  "type": "position_closed",
  "data": {
    "bot_id": 1,
    "position_id": 123,
    "pair": "shibidr",
    "exit_price": 0.000155,
    "profit_idr": 16650,
    "profit_percent": 3.33,
    "close_reason": "take_profit",
    "hold_duration_minutes": 5,
    "timestamp": "2024-01-07T18:35:00Z"
  }
}
```

### Daily Loss Limit Reached

```json
{
  "type": "bot_paused",
  "data": {
    "bot_id": 1,
    "reason": "daily_loss_limit",
    "daily_loss": 1050000,
    "limit": 1000000,
    "message": "Bot paused due to daily loss limit"
  }
}
```

---

## Performance Metrics

### Key Metrics

- **Total Trades**: Number of closed positions
- **Win Rate**: `winning_trades / total_trades × 100`
- **Average Profit**: `total_profit_idr / total_trades`
- **ROI**: `total_profit_idr / initial_balance × 100`
- **Average Hold Time**: Mean duration from entry to exit
- **Exit Reason Distribution**: TP vs SL vs other
- **Daily P&L**: Profit/loss per day

### Example Dashboard

```
Pump Hunter Pro
Status: Running (2h 15m)

Performance:
  Total Trades: 15
  Win Rate: 66.7% (10/15)
  Total Profit: +75,000 IDR (+1.5% ROI)
  Avg Profit/Trade: +5,000 IDR
  Avg Hold Time: 12 minutes

Open Positions: 2
  SHIB/IDR: +3.3% (+16,650 IDR)
  DOGE/IDR: -0.5% (-2,500 IDR)

Risk Management:
  Daily Loss: 25,000 / 1,000,000 IDR
  Open Positions: 2 / 3
  Available Balance: 4,850,000 IDR
```

---

## Testing Strategy

### Unit Tests
- ✅ Entry condition checks
- ✅ Exit condition checks
- ✅ Position sizing calculation
- ✅ Risk management validations
- ✅ Profit calculation

### Integration Tests
- ✅ Position lifecycle (open → close)
- ✅ Order tracking (buy → sell)
- ✅ Balance updates
- ✅ Risk limits enforcement
- ✅ Multiple concurrent positions

### Paper Trading Tests
1. Bot starts and scans all pairs
2. Entry signal triggers position open
3. Take profit exit works
4. Stop loss exit works
5. Trailing stop works
6. Max hold time exit works
7. Daily loss limit stops bot

---

## Common Issues & Solutions

### Issue: No positions opening

**Causes**:
1. `min_pump_score` too high
2. No coins meet criteria
3. Max positions reached
4. Insufficient balance

**Solution**: Lower `min_pump_score` to 30-50, check logs

### Issue: Too many stop losses

**Causes**:
1. `stop_loss_percent` too tight
2. Entering too late (pump already peaked)
3. Volatile market

**Solution**: Increase to 2-3%, enable trailing stop

### Issue: Missing opportunities

**Causes**:
1. `min_timeframes_positive` too strict
2. Volume filter too high

**Solution**: Decrease to 1-2 timeframes, lower volume threshold

---

## Best Practices

### Configuration
- ✅ Start with `min_pump_score` = 50-100
- ✅ Use `stop_loss_percent` = 1.5-3.0%
- ✅ Enable trailing stop for volatile markets
- ✅ Set reasonable `max_hold_minutes` (30-60)

### Risk Management
- ✅ Use `daily_loss_limit_idr` = 10-20% of balance
- ✅ Limit `max_concurrent_positions` to 3-5
- ✅ Enable cooldown after losses
- ✅ Monitor bot performance daily

### Pair Selection
- ✅ Exclude stablecoins
- ✅ Set minimum volume requirement
- ✅ Avoid dust coins (set `min_price_idr`)

---

## Conclusion

The Pump Hunter Bot is a **high-risk, high-reward strategy** for:
- ✅ Volatile, pumping markets
- ✅ Traders who can tolerate losses
- ✅ Markets with good liquidity

**Requires**:
- Market Analysis system running
- Reliable timeframe data
- Fast order execution
- Strict risk management
- Active monitoring
