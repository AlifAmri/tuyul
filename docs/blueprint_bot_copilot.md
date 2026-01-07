# Copilot Bot (Automated Trading Assistant)

## Overview

The Copilot Bot is an **automated trading assistant** that executes individual trades with pre-configured profit targets and stop-loss protection. Unlike the Market Maker and Pump Hunter bots that run continuously, Copilot executes **one-time trades** with automatic sell order placement when the buy order fills.

**Strategy Type**: Profit-target trading / Semi-automated trading  
**Risk Level**: Moderate (configurable via stop-loss)  
**Suitable For**: Manual traders who want automation for sell orders  
**Timeframe**: Minutes to hours (depends on market conditions)

---

## Key Characteristics

- ✅ **One-time trade execution** (not continuous like other bots)
- ✅ **User-specified parameters** (buying price, volume, profit target, stop-loss)
- ✅ **Automatic sell order placement** when buy fills
- ✅ **Stop-loss monitoring** for risk protection
- ✅ **Manual sell override** (cancel auto-sell, place market order)
- ✅ **Order lifecycle tracking** (pending → open → filled)
- ✅ **Real-time status updates** via WebSocket

---

## Trade Configuration

### Trade Request

```json
{
  "pair": "btcidr",
  "buying_price": 650000000,
  "volume_idr": 1000000,
  "target_profit": 5.0,
  "stop_loss": 3.0
}
```

**Field Descriptions**:
- `pair`: Trading pair (e.g., "btcidr", "ethidr")
- `buying_price`: Limit price for buy order (IDR)
- `volume_idr`: Amount in IDR to spend on buy order
- `target_profit`: Percentage profit target (%)
- `stop_loss`: Percentage stop-loss (%)

**Validation Rules**:
- `buying_price` > 0, must follow Indodax price increments
- `volume_idr` >= 10,000 IDR (minimum)
- `target_profit` >= 0.1%, <= 1000%
- `stop_loss` > 0%, <= 100%
- `stop_loss` < `target_profit`
- User must have sufficient IDR balance

---

## State Machine

```
┌──────────────┐
│   PENDING    │ (Buy order placed, waiting for fill)
└──────┬───────┘
       │
       │ Buy order filled
       ↓
┌──────────────┐
│   FILLED     │ (Buy filled, auto-sell order placed)
└──────┬───────┘
       │
       │ Sell order filled
       │ OR Stop-loss triggered
       │ OR Manual sell
       ↓
┌──────────────┐
│  COMPLETED   │ (Trade closed, profit/loss calculated)
└──────────────┘

       │
       │ User cancels buy order
       ↓
┌──────────────┐
│  CANCELLED   │
└──────────────┘
```

---

## Trading Flow

### 1. Place Buy Order

**User submits trade request** → **System processes**:

```go
func (s *CopilotService) PlaceBuyOrder(userID int64, req *TradeRequest) (*Trade, error) {
    // 1. Validate input
    if err := s.validateTradeRequest(req); err != nil {
        return nil, err
    }
    
    // 2. Get user's API credentials
    apiKey, err := s.getAPICredentials(userID)
    if err != nil {
        return nil, err
    }
    
    // 3. Check balance
    balance, err := s.indodaxClient.GetBalance(apiKey)
    if err != nil {
        return nil, err
    }
    
    if balance["idr"] < req.VolumeIDR {
        return nil, errors.New("Insufficient IDR balance")
    }
    
    // 4. Calculate amount
    amount := req.VolumeIDR / req.BuyingPrice
    
    // 5. Round to volume precision
    coin := s.getCoin(req.Pair)
    amount = roundToDecimal(amount, coin.VolumePrecision)
    
    // 6. Validate minimum volume
    if amount < coin.MinVolume {
        return nil, fmt.Errorf("Amount %.8f below minimum %.8f", amount, coin.MinVolume)
    }
    
    // 7. Place buy order on Indodax
    result, err := s.indodaxClient.PlaceOrder("buy", req.Pair, req.BuyingPrice, amount)
    if err != nil {
        return nil, err
    }
    
    // 8. Create trade record
    trade := &models.Trade{
        UserID:        userID,
        Pair:          req.Pair,
        BuyOrderID:    result.OrderID,
        BuyPrice:      req.BuyingPrice,
        BuyAmount:     amount,
        BuyAmountIDR:  req.VolumeIDR,
        TargetProfit:  req.TargetProfit,
        StopLoss:      req.StopLoss,
        Status:        "pending",
        CreatedAt:     time.Now(),
    }
    
    s.tradeRepo.Create(trade)
    
    // 9. Subscribe to order updates (Private WebSocket)
    s.subscribeOrderUpdate(userID, result.OrderID, trade.ID)
    
    // 10. Broadcast to user
    s.broadcastTradeUpdate(userID, "trade_created", trade)
    
    return trade, nil
}
```

### 2. Buy Order Fill Detection

**Private WebSocket receives order.update** → **Auto-place sell order**:

```go
func (s *CopilotService) handleBuyOrderFilled(trade *models.Trade, filledAmount float64) {
    // 1. Update trade status
    trade.Status = "filled"
    trade.BuyFilledAmount = filledAmount
    trade.BuyFilledAt = time.Now()
    s.tradeRepo.Update(trade)
    
    // 2. Fetch current balance
    balance, err := s.indodaxClient.GetBalance(trade.UserID)
    if err != nil {
        s.logger.Error.Printf("Failed to fetch balance: %v", err)
        return
    }
    
    // 3. Calculate sell price (buy price + profit target)
    sellPrice := trade.BuyPrice * (1 + trade.TargetProfit/100)
    
    // 4. Round to price precision
    coin := s.getCoin(trade.Pair)
    sellPrice = roundToDecimal(sellPrice, coin.PricePrecision)
    
    // 5. Determine sell amount (all available coins)
    baseCurrency := strings.TrimSuffix(trade.Pair, "idr")
    sellAmount := balance[baseCurrency]
    
    // 6. Place sell order on Indodax
    result, err := s.indodaxClient.PlaceOrder("sell", trade.Pair, sellPrice, sellAmount)
    if err != nil {
        s.logger.Error.Printf("Failed to place sell order: %v", err)
        trade.Status = "error"
        trade.ErrorMessage = err.Error()
        s.tradeRepo.Update(trade)
        return
    }
    
    // 7. Update trade with sell order info
    trade.SellOrderID = result.OrderID
    trade.SellPrice = sellPrice
    trade.SellAmount = sellAmount
    s.tradeRepo.Update(trade)
    
    // 8. Subscribe to sell order updates
    s.subscribeOrderUpdate(trade.UserID, result.OrderID, trade.ID)
    
    // 9. Start stop-loss monitoring
    s.startStopLossMonitor(trade)
    
    // 10. Broadcast update
    s.broadcastTradeUpdate(trade.UserID, "sell_order_placed", trade)
    
    s.logger.Info.Printf("Auto-placed sell order: %s @ %.2f", trade.Pair, sellPrice)
}
```

### 3. Stop-Loss Monitoring

**Background process checks prices every second**:

```go
func (s *CopilotService) startStopLossMonitor(trade *models.Trade) {
    go func() {
        ticker := time.NewTicker(1 * time.Second)
        defer ticker.Stop()
        
        for {
            select {
            case <-ticker.C:
                // Check if trade still active
                currentTrade, _ := s.tradeRepo.GetByID(trade.ID)
                if currentTrade.Status != "filled" {
                    return // Trade no longer active
                }
                
                // Get current market price
                currentPrice := s.getCurrentPrice(trade.Pair)
                
                // Calculate stop-loss price
                stopLossPrice := trade.BuyPrice * (1 - trade.StopLoss/100)
                
                // Check if stop-loss triggered
                if currentPrice <= stopLossPrice {
                    s.triggerStopLoss(trade, currentPrice)
                    return
                }
            }
        }
    }()
}

func (s *CopilotService) triggerStopLoss(trade *models.Trade, currentPrice float64) {
    s.logger.Warn.Printf("Stop-loss triggered for trade %d: %.2f <= %.2f",
        trade.ID, currentPrice, trade.BuyPrice*(1-trade.StopLoss/100))
    
    // 1. Cancel existing sell order
    if trade.SellOrderID != "" {
        err := s.indodaxClient.CancelOrder(trade.Pair, trade.SellOrderID)
        if err != nil {
            s.logger.Error.Printf("Failed to cancel sell order: %v", err)
        }
    }
    
    // 2. Place market sell order
    result, err := s.indodaxClient.PlaceOrder("sell", trade.Pair, currentPrice, trade.SellAmount, "market")
    if err != nil {
        s.logger.Error.Printf("Failed to place stop-loss sell: %v", err)
        return
    }
    
    // 3. Update trade
    trade.Status = "stopped"
    trade.SellOrderID = result.OrderID
    trade.SellPrice = currentPrice
    trade.StopLossTriggered = true
    s.tradeRepo.Update(trade)
    
    // 4. Broadcast alert
    s.broadcastTradeUpdate(trade.UserID, "stop_loss_triggered", trade)
}
```

### 4. Sell Order Fill Detection

**Private WebSocket receives order.update** → **Trade completed**:

```go
func (s *CopilotService) handleSellOrderFilled(trade *models.Trade, filledAmount float64, avgPrice float64) {
    // 1. Calculate profit
    sellRevenue := filledAmount * avgPrice
    buySpent := trade.BuyFilledAmount * trade.BuyPrice
    profitIDR := sellRevenue - buySpent
    profitPercent := (profitIDR / buySpent) * 100
    
    // 2. Update trade
    trade.Status = "completed"
    trade.SellFilledAmount = filledAmount
    trade.SellFilledAt = time.Now()
    trade.ProfitIDR = profitIDR
    trade.ProfitPercent = profitPercent
    s.tradeRepo.Update(trade)
    
    // 3. Broadcast update
    s.broadcastTradeUpdate(trade.UserID, "trade_completed", trade)
    
    s.logger.Info.Printf("Trade %d completed: profit %.2f IDR (%.2f%%)",
        trade.ID, profitIDR, profitPercent)
}
```

### 5. Manual Sell Action

**User clicks "Sell Now"** → **Cancel auto-sell, place market order**:

```go
func (s *CopilotService) ManualSell(userID, tradeID int64) error {
    // 1. Get trade
    trade, err := s.tradeRepo.GetByID(tradeID)
    if err != nil || trade.UserID != userID {
        return errors.New("Trade not found")
    }
    
    // 2. Validate status
    if trade.Status != "filled" {
        return errors.New("Trade not in filled status")
    }
    
    // 3. Cancel existing sell order
    if trade.SellOrderID != "" {
        err := s.indodaxClient.CancelOrder(trade.Pair, trade.SellOrderID)
        if err != nil {
            s.logger.Warn.Printf("Failed to cancel sell order: %v", err)
        }
    }
    
    // 4. Get current market price
    currentPrice := s.getCurrentPrice(trade.Pair)
    
    // 5. Place market sell order
    result, err := s.indodaxClient.PlaceOrder("sell", trade.Pair, currentPrice, trade.SellAmount, "market")
    if err != nil {
        return err
    }
    
    // 6. Update trade
    trade.SellOrderID = result.OrderID
    trade.SellPrice = currentPrice
    trade.ManualSell = true
    s.tradeRepo.Update(trade)
    
    // 7. Subscribe to order updates
    s.subscribeOrderUpdate(userID, result.OrderID, trade.ID)
    
    // 8. Broadcast update
    s.broadcastTradeUpdate(userID, "manual_sell_placed", trade)
    
    return nil
}
```

### 6. Cancel Buy Order

**User cancels before buy fills**:

```go
func (s *CopilotService) CancelBuyOrder(userID, tradeID int64) error {
    // 1. Get trade
    trade, err := s.tradeRepo.GetByID(tradeID)
    if err != nil || trade.UserID != userID {
        return errors.New("Trade not found")
    }
    
    // 2. Validate status
    if trade.Status != "pending" {
        return errors.New("Can only cancel pending orders")
    }
    
    // 3. Cancel on Indodax
    err = s.indodaxClient.CancelOrder(trade.Pair, trade.BuyOrderID)
    if err != nil {
        return err
    }
    
    // 4. Update trade
    trade.Status = "cancelled"
    trade.CancelledAt = time.Now()
    s.tradeRepo.Update(trade)
    
    // 5. Broadcast update
    s.broadcastTradeUpdate(userID, "trade_cancelled", trade)
    
    return nil
}
```

---

## Data Models

### Trade Model

```go
type Trade struct {
    ID              int64     `json:"id"`
    UserID          int64     `json:"user_id"`
    Pair            string    `json:"pair"`
    
    // Buy order
    BuyOrderID      string    `json:"buy_order_id"`
    BuyPrice        float64   `json:"buy_price"`
    BuyAmount       float64   `json:"buy_amount"`
    BuyAmountIDR    float64   `json:"buy_amount_idr"`
    BuyFilledAmount float64   `json:"buy_filled_amount"`
    BuyFilledAt     time.Time `json:"buy_filled_at"`
    
    // Sell order
    SellOrderID      string    `json:"sell_order_id"`
    SellPrice        float64   `json:"sell_price"`
    SellAmount       float64   `json:"sell_amount"`
    SellFilledAmount float64   `json:"sell_filled_amount"`
    SellFilledAt     time.Time `json:"sell_filled_at"`
    
    // Parameters
    TargetProfit    float64   `json:"target_profit"`
    StopLoss        float64   `json:"stop_loss"`
    
    // Profit
    ProfitIDR       float64   `json:"profit_idr"`
    ProfitPercent   float64   `json:"profit_percent"`
    
    // Flags
    StopLossTriggered bool    `json:"stop_loss_triggered"`
    ManualSell        bool    `json:"manual_sell"`
    
    // Status
    Status          string    `json:"status"`  // pending, filled, completed, cancelled, stopped, error
    ErrorMessage    string    `json:"error_message"`
    
    // Timestamps
    CreatedAt       time.Time `json:"created_at"`
    CancelledAt     time.Time `json:"cancelled_at"`
}
```

---

## Redis Schema

### Active Trades

```
Key: trade:{trade_id}
Type: Hash
Fields: (all fields from Trade model)

Key: user_trades:{user_id}
Type: Sorted Set
Score: Created timestamp
Member: trade_id
```

### Trade Status Index

```
Key: trades_by_status:{status}
Type: Set
Members: trade_id
```

### Stop-Loss Monitoring

```
Key: stop_loss_monitor:{pair}
Type: Set
Members: trade_id (only filled trades)
```

---

## Database Schema (PostgreSQL)

### trades Table

```sql
CREATE TABLE trades (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    
    -- Pair
    pair VARCHAR(20) NOT NULL,
    
    -- Buy order
    buy_order_id VARCHAR(255) NOT NULL,
    buy_price NUMERIC(20,8) NOT NULL,
    buy_amount NUMERIC(20,8) NOT NULL,
    buy_amount_idr NUMERIC(20,2) NOT NULL,
    buy_filled_amount NUMERIC(20,8),
    buy_filled_at TIMESTAMP,
    
    -- Sell order
    sell_order_id VARCHAR(255),
    sell_price NUMERIC(20,8),
    sell_amount NUMERIC(20,8),
    sell_filled_amount NUMERIC(20,8),
    sell_filled_at TIMESTAMP,
    
    -- Parameters
    target_profit NUMERIC(10,4) NOT NULL,
    stop_loss NUMERIC(10,4) NOT NULL,
    
    -- Profit
    profit_idr NUMERIC(20,2),
    profit_percent NUMERIC(10,4),
    
    -- Flags
    stop_loss_triggered BOOLEAN DEFAULT FALSE,
    manual_sell BOOLEAN DEFAULT FALSE,
    
    -- Status
    status VARCHAR(50) NOT NULL,
    error_message TEXT,
    
    -- Timestamps
    created_at TIMESTAMP DEFAULT NOW(),
    cancelled_at TIMESTAMP,
    
    -- Indexes
    UNIQUE(buy_order_id)
);

CREATE INDEX idx_trades_user_id ON trades(user_id);
CREATE INDEX idx_trades_status ON trades(status);
CREATE INDEX idx_trades_pair ON trades(pair);
CREATE INDEX idx_trades_created_at ON trades(created_at DESC);
```

---

## API Endpoints

### POST /api/v1/copilot/trade
Place new trade

**Request**:
```json
{
  "pair": "btcidr",
  "buying_price": 650000000,
  "volume_idr": 1000000,
  "target_profit": 5.0,
  "stop_loss": 3.0
}
```

**Response**:
```json
{
  "success": true,
  "data": {
    "id": 123,
    "pair": "btcidr",
    "buy_order_id": "123456789",
    "buy_price": 650000000,
    "buy_amount": 0.00153846,
    "buy_amount_idr": 1000000,
    "target_profit": 5.0,
    "stop_loss": 3.0,
    "status": "pending",
    "created_at": "2024-01-07T18:30:00Z"
  },
  "message": "Buy order placed successfully"
}
```

### GET /api/v1/copilot/trades
Get user's trades

**Query Parameters**:
- `status`: "pending", "filled", "completed", "cancelled", "all" (default: "all")
- `pair`: filter by pair
- `limit`: int (default: 20, max: 100)
- `offset`: int (default: 0)

**Response**:
```json
{
  "success": true,
  "data": {
    "trades": [
      {
        "id": 123,
        "pair": "btcidr",
        "buy_price": 650000000,
        "buy_amount": 0.00153846,
        "buy_amount_idr": 1000000,
        "sell_price": 682500000,
        "sell_amount": 0.00153846,
        "target_profit": 5.0,
        "stop_loss": 3.0,
        "profit_idr": 50000,
        "profit_percent": 5.0,
        "status": "completed",
        "created_at": "2024-01-07T18:30:00Z",
        "buy_filled_at": "2024-01-07T18:35:00Z",
        "sell_filled_at": "2024-01-07T19:00:00Z"
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

### GET /api/v1/copilot/trades/:id
Get specific trade

**Response**:
```json
{
  "success": true,
  "data": {
    "id": 123,
    "pair": "btcidr",
    "buy_order_id": "123456789",
    "buy_price": 650000000,
    "buy_amount": 0.00153846,
    "buy_filled_at": "2024-01-07T18:35:00Z",
    "sell_order_id": "123456790",
    "sell_price": 682500000,
    "sell_amount": 0.00153846,
    "target_profit": 5.0,
    "stop_loss": 3.0,
    "status": "filled",
    "created_at": "2024-01-07T18:30:00Z"
  }
}
```

### DELETE /api/v1/copilot/trades/:id
Cancel buy order (before filled)

**Response**:
```json
{
  "success": true,
  "data": {
    "id": 123,
    "status": "cancelled",
    "cancelled_at": "2024-01-07T18:32:00Z"
  },
  "message": "Buy order cancelled successfully"
}
```

### POST /api/v1/copilot/trades/:id/sell
Manual sell (market price)

**Response**:
```json
{
  "success": true,
  "data": {
    "id": 123,
    "old_sell_order_id": "123456790",
    "new_sell_order_id": "123456791",
    "sell_price": 655000000,
    "message": "Market sell order placed"
  }
}
```

---

## WebSocket Real-time Updates

### Trade Created

```json
{
  "type": "trade_created",
  "data": {
    "trade_id": 123,
    "pair": "btcidr",
    "buy_price": 650000000,
    "status": "pending"
  }
}
```

### Buy Order Filled (Auto-Sell Placed)

```json
{
  "type": "buy_filled",
  "data": {
    "trade_id": 123,
    "buy_filled_amount": 0.00153846,
    "buy_filled_at": "2024-01-07T18:35:00Z",
    "sell_order_placed": true,
    "sell_price": 682500000,
    "status": "filled"
  }
}
```

### Stop-Loss Triggered

```json
{
  "type": "stop_loss_triggered",
  "data": {
    "trade_id": 123,
    "pair": "btcidr",
    "buy_price": 650000000,
    "current_price": 630500000,
    "stop_loss_percent": 3.0,
    "action": "Market sell order placed",
    "timestamp": "2024-01-07T19:15:00Z"
  }
}
```

### Trade Completed

```json
{
  "type": "trade_completed",
  "data": {
    "trade_id": 123,
    "pair": "btcidr",
    "profit_idr": 50000,
    "profit_percent": 5.0,
    "sell_filled_at": "2024-01-07T19:00:00Z"
  }
}
```

---

## Indodax Integration

### Private WebSocket

**Token Generation**:
```go
func (c *IndodaxClient) GeneratePrivateWSToken() (string, error) {
    endpoint := "https://indodax.com/api/private_ws/v1/generate_token"
    nonce := time.Now().UnixMilli()
    
    payload := fmt.Sprintf("nonce=%d", nonce)
    signature := createHMAC512(payload, c.secret)
    
    req := http.NewRequest("POST", endpoint, strings.NewReader(payload))
    req.Header.Set("Key", c.key)
    req.Header.Set("Sign", signature)
    
    resp, err := c.httpClient.Do(req)
    // ... parse token from response
}
```

**Order Update Subscription**:
```go
func (c *IndodaxClient) SubscribeOrderUpdates(token string) {
    conn, _ := websocket.Dial("wss://pws.indodax.com/ws/")
    
    // Authenticate
    conn.WriteJSON(map[string]interface{}{
        "params": map[string]string{"token": token},
        "id": 1,
    })
    
    // Subscribe to order channel
    conn.WriteJSON(map[string]interface{}{
        "method": "subscribe",
        "params": map[string]interface{}{"channel": "order"},
        "id": 2,
    })
    
    // Listen for updates
    go c.listenOrderUpdates(conn)
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
    "message": "Insufficient IDR balance",
    "details": {
      "required": 1000000,
      "available": 500000
    }
  }
}
```

### Invalid Parameters

```json
{
  "success": false,
  "error": {
    "code": "INVALID_PARAMETERS",
    "message": "Stop-loss must be less than target profit",
    "details": "stop_loss: 5%, target_profit: 3%"
  }
}
```

### Order Placement Failed

```json
{
  "success": false,
  "error": {
    "code": "ORDER_FAILED",
    "message": "Failed to place order on Indodax",
    "details": "Invalid price increment"
  }
}
```

---

## Performance Metrics

### User Statistics

```json
{
  "total_trades": 50,
  "completed_trades": 45,
  "active_trades": 2,
  "cancelled_trades": 3,
  "win_rate": 73.3,
  "total_profit_idr": 2500000,
  "avg_profit_per_trade": 55555,
  "avg_hold_time_minutes": 45
}
```

---

## Best Practices

### Configuration
- ✅ Set realistic profit targets (3-10%)
- ✅ Use appropriate stop-loss (2-5%)
- ✅ Ensure stop-loss < target profit
- ✅ Start with small volume for testing

### Risk Management
- ✅ Don't invest more than you can afford to lose
- ✅ Monitor active trades regularly
- ✅ Use stop-loss on all trades
- ✅ Avoid overleveraging (too many concurrent trades)

### Market Selection
- ✅ Choose liquid pairs (high volume)
- ✅ Check recent price action
- ✅ Avoid highly volatile pairs for tight stop-loss

---

## Testing Strategy

### Unit Tests
- ✅ Input validation
- ✅ Profit/loss calculations
- ✅ Price rounding (precision)
- ✅ Stop-loss trigger logic

### Integration Tests
- ✅ Full trade flow (buy → fill → auto-sell → fill)
- ✅ Stop-loss trigger
- ✅ Manual sell action
- ✅ Order cancellation
- ✅ WebSocket order tracking

---

## Common Issues & Solutions

### Issue: Buy order not filling

**Cause**: Price too far from market price

**Solution**: Adjust buying_price closer to current market price

### Issue: Sell order not filling

**Cause**: Target profit too high, price hasn't reached

**Solution**: Lower target profit or use manual sell

### Issue: Stop-loss triggered unexpectedly

**Cause**: Stop-loss too tight for volatile market

**Solution**: Increase stop-loss to 3-5% for volatile pairs

---

## Conclusion

The Copilot Bot is perfect for:
- ✅ Manual traders who want automation
- ✅ One-time trades with profit targets
- ✅ Risk management via stop-loss
- ✅ Users who want control but not constant monitoring

**Key Benefits**:
- Automatic sell order placement
- Stop-loss protection
- Manual override available
- Real-time status updates

