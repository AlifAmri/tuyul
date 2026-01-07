# Trading Automation

## Overview

TUYUL's automated trading engine executes buy orders, monitors order fills, and automatically places sell orders with configurable profit targets and stop-loss. The system manages the complete order lifecycle from placement to execution.

---

## Features

### 1. Automated Buy Orders
- User specifies: buying price, volume (IDR), target profit (%), stop-loss (%)
- System validates balance and parameters
- Places limit buy order on Indodax
- Monitors order status via Private WebSocket

### 2. Automatic Sell Orders
- When buy order fills → automatically place sell order
- Sell price = buy price × (1 + target_profit%)
- All available coins are sold (balance check)
- Stop-loss monitoring in real-time

### 3. Order Management
- List active orders with status
- Cancel orders (both buy and sell)
- Manual sell action (cancels old sell order, places market order)
- Order history and tracking

### 4. Stop-Loss Protection
- Continuous price monitoring
- Auto-sell at market price if stop-loss triggered
- Cancel profit-target sell order before stop-loss sell

### 5. Balance Management
- Pre-trade balance validation
- Post-fill balance updates
- Available balance calculation
- Reserved balance tracking

---

## Data Models

### Trade Request Model
```go
type TradeRequest struct {
    Pair           string  `json:"pair"`              // e.g., "btcidr"
    BuyingPrice    float64 `json:"buying_price"`      // Limit price for buy order
    VolumeIDR      float64 `json:"volume_idr"`        // Amount in IDR to spend
    TargetProfit   float64 `json:"target_profit"`     // Percentage (e.g., 5.0 = 5%)
    StopLoss       float64 `json:"stop_loss"`         // Percentage (e.g., 3.0 = 3%)
}
```

### Trade Order Model
```go
type TradeOrder struct {
    ID                string    `json:"id"`                  // Our internal ID (UUID)
    UserID            string    `json:"user_id"`
    Pair              string    `json:"pair"`
    Side              string    `json:"side"`                // "buy" or "sell"
    OrderType         string    `json:"order_type"`          // "limit", "market"
    Price             float64   `json:"price"`               // Order price
    Amount            float64   `json:"amount"`              // Amount of base currency
    AmountIDR         float64   `json:"amount_idr"`          // Amount in IDR (for buy)
    Filled            float64   `json:"filled"`              // Filled amount
    Status            string    `json:"status"`              // "pending", "open", "filled", "cancelled", "failed"
    IndodaxOrderID    string    `json:"indodax_order_id"`    // Order ID from Indodax
    TargetProfit      float64   `json:"target_profit"`       // Percentage
    StopLoss          float64   `json:"stop_loss"`           // Percentage
    BuyOrderID        string    `json:"buy_order_id"`        // For sell orders: reference to buy order
    SellOrderID       string    `json:"sell_order_id"`       // For buy orders: reference to sell order
    CreatedAt         time.Time `json:"created_at"`
    UpdatedAt         time.Time `json:"updated_at"`
    FilledAt          *time.Time `json:"filled_at"`
    CancelledAt       *time.Time `json:"cancelled_at"`
    ErrorMessage      string    `json:"error_message"`
}
```

### Balance Model
```go
type Balance struct {
    Currency  string  `json:"currency"`    // e.g., "btc", "idr"
    Available float64 `json:"available"`   // Available balance
    Frozen    float64 `json:"frozen"`      // In open orders
    Total     float64 `json:"total"`       // Available + Frozen
}
```

---

## Redis Schema

### Active Orders Storage
```
Key Pattern: order:{order_id}
Type: Hash
Fields: (all fields from TradeOrder model)
TTL: None (manually deleted when completed/cancelled)

# User's orders index
Key Pattern: user_orders:{user_id}
Type: Sorted Set
Score: Created timestamp
Member: order_id
```

### Order Status Index
```
Key Pattern: orders_by_status:{status}
Type: Set
Members: order_id
Examples: orders_by_status:open, orders_by_status:pending
```

### Pair Orders Index
```
Key Pattern: pair_orders:{pair}
Type: Set
Members: order_id
Purpose: Quick lookup of all orders for a pair (for stop-loss monitoring)
```

### Buy-Sell Order Mapping
```
Key Pattern: buy_sell_map:{buy_order_id}
Type: String
Value: sell_order_id
Purpose: Link buy order to its auto-generated sell order
```

### Balance Cache
```
Key Pattern: balance:{user_id}:{currency}
Type: String (JSON)
Value: {
  "available": 1000000.0,
  "frozen": 50000.0,
  "total": 1050000.0
}
TTL: 5 seconds (refreshed on each balance query)
```

---

## Trading Flow

### 1. Buy Order Placement Flow
```
User submits trade request
        ↓
Validate input (price, volume, profit%, stop-loss%)
        ↓
Check user has API key configured
        ↓
Get decrypted API credentials
        ↓
Fetch current balance (IDR) from Indodax
        ↓
Validate sufficient balance
        ↓
Calculate amount of base currency to buy
  amount = volume_idr / buying_price
        ↓
Place limit buy order on Indodax
        ↓
Store order in Redis with status "open"
        ↓
Subscribe to order updates (Private WebSocket)
        ↓
Return order details to user
```

### 2. Buy Order Fill Detection Flow
```
Private WebSocket receives order update
        ↓
Check if order is fully filled
        ↓
Update order status to "filled"
        ↓
Set filled_at timestamp
        ↓
Fetch current balance of base currency
        ↓
Calculate sell price
  sell_price = buy_price × (1 + target_profit/100)
        ↓
Place limit sell order on Indodax
  amount = all available coins from buy order
        ↓
Store sell order in Redis
        ↓
Link buy order to sell order (buy_sell_map)
        ↓
Start monitoring for stop-loss condition
        ↓
Broadcast update to user via WebSocket
```

### 3. Stop-Loss Monitoring Flow
```
Market price update received (Public WebSocket)
        ↓
For each pair, check active buy orders with filled status
        ↓
Calculate stop-loss price
  stop_loss_price = buy_price × (1 - stop_loss/100)
        ↓
If current_price <= stop_loss_price
        ↓
Cancel existing limit sell order (if any)
        ↓
Place market sell order (sell all available coins)
        ↓
Update order status to "stopped"
        ↓
Broadcast alert to user
```

### 4. Manual Sell Action Flow
```
User clicks "Sell" on active order
        ↓
Validate order exists and is filled
        ↓
Get linked sell order ID (if any)
        ↓
Cancel existing limit sell order on Indodax
        ↓
Fetch current market price
        ↓
Place market sell order
  amount = all available coins
        ↓
Update order status
        ↓
Return success to user
```

### 5. Order Cancellation Flow
```
User clicks "Cancel" on order
        ↓
Validate order can be cancelled (status = "open")
        ↓
Call Indodax cancelOrder API
        ↓
Update order status to "cancelled"
        ↓
Set cancelled_at timestamp
        ↓
Remove from active orders indices
        ↓
Broadcast update to user
```

---

## Validation Rules

### Trade Request Validation
```go
func validateTradeRequest(req *TradeRequest, balance float64) error {
    // 1. Pair validation
    if !isValidPair(req.Pair) {
        return errors.New("Invalid trading pair")
    }
    
    // 2. Price validation
    if req.BuyingPrice <= 0 {
        return errors.New("Buying price must be positive")
    }
    
    // 3. Volume validation
    if req.VolumeIDR <= 0 {
        return errors.New("Volume must be positive")
    }
    if req.VolumeIDR < 10000 {
        return errors.New("Minimum volume is IDR 10,000")
    }
    
    // 4. Balance validation
    if req.VolumeIDR > balance {
        return errors.New("Insufficient balance")
    }
    
    // 5. Target profit validation
    if req.TargetProfit <= 0 {
        return errors.New("Target profit must be positive")
    }
    if req.TargetProfit < 0.1 {
        return errors.New("Minimum target profit is 0.1%")
    }
    if req.TargetProfit > 1000 {
        return errors.New("Maximum target profit is 1000%")
    }
    
    // 6. Stop-loss validation
    if req.StopLoss <= 0 {
        return errors.New("Stop-loss must be positive")
    }
    if req.StopLoss > 100 {
        return errors.New("Stop-loss cannot exceed 100%")
    }
    if req.StopLoss >= req.TargetProfit {
        return errors.New("Stop-loss must be less than target profit")
    }
    
    // 7. Price increment validation (from Indodax rules)
    if !isValidPriceIncrement(req.Pair, req.BuyingPrice) {
        return errors.New("Invalid price increment for this pair")
    }
    
    return nil
}
```

### Indodax API Validation
```go
func isValidPriceIncrement(pair string, price float64) bool {
    // Get price increment rules from Indodax
    // e.g., BTC/IDR: increment = 1000, ETH/IDR: increment = 500
    increment := getPairPriceIncrement(pair)
    return math.Mod(price, increment) == 0
}
```

---

## API Endpoints

### POST /api/v1/trade/buy
Place automated buy order

**Headers:**
```
Authorization: Bearer {access_token}
```

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

**Validation:**
- Pair exists and is active
- Price > 0 and follows increment rules
- Volume >= 10,000 IDR
- User has sufficient IDR balance
- Target profit: 0.1% - 1000%
- Stop-loss: 0.1% - 100% (must be < target profit)

**Response (Success):**
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

**Response (Insufficient Balance):**
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

### GET /api/v1/trade/orders
Get user's orders

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
        "amount_idr": 1000000,
        "filled": 0.00153846,
        "status": "filled",
        "indodax_order_id": "123456789",
        "target_profit": 5.0,
        "stop_loss": 3.0,
        "sell_order_id": "uuid-2",
        "created_at": "2024-01-07T18:30:00Z",
        "filled_at": "2024-01-07T18:35:00Z"
      },
      {
        "id": "uuid-2",
        "pair": "btcidr",
        "side": "sell",
        "order_type": "limit",
        "price": 682500000,
        "amount": 0.00153846,
        "filled": 0,
        "status": "open",
        "indodax_order_id": "123456790",
        "buy_order_id": "uuid",
        "created_at": "2024-01-07T18:35:05Z"
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

### GET /api/v1/trade/orders/:id
Get specific order details

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
    "filled": 0.00153846,
    "status": "filled",
    "indodax_order_id": "123456789",
    "target_profit": 5.0,
    "stop_loss": 3.0,
    "sell_order_id": "uuid-2",
    "created_at": "2024-01-07T18:30:00Z",
    "updated_at": "2024-01-07T18:35:00Z",
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

### DELETE /api/v1/trade/orders/:id
Cancel order

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

### POST /api/v1/trade/orders/:id/sell
Manually sell (market price)

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

### GET /api/v1/trade/balance
Get user's balance

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

## WebSocket Real-time Updates

### Order Status Updates
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

### Auto-sell Notification
```json
{
  "type": "auto_sell_placed",
  "data": {
    "buy_order_id": "uuid",
    "sell_order_id": "uuid-2",
    "sell_price": 682500000,
    "amount": 0.00153846,
    "target_profit": 5.0
  }
}
```

### Stop-loss Triggered
```json
{
  "type": "stop_loss_triggered",
  "data": {
    "order_id": "uuid",
    "pair": "btcidr",
    "buy_price": 650000000,
    "current_price": 630500000,
    "stop_loss_percentage": 3.0,
    "action": "Market sell order placed",
    "timestamp": "2024-01-07T19:15:00Z"
  }
}
```

### Balance Update
```json
{
  "type": "balance_update",
  "data": {
    "currency": "idr",
    "available": 5500000.0,
    "frozen": 500000.0,
    "total": 6000000.0
  }
}
```

---

## Indodax Private API Integration

### Place Buy Order
```go
func (c *IndodaxClient) PlaceBuyOrder(pair string, price, amount float64) (*OrderResponse, error) {
    endpoint := "https://indodax.com/tapi"
    method := "trade"
    nonce := time.Now().UnixMilli()
    
    payload := fmt.Sprintf(
        "method=%s&pair=%s&type=buy&price=%d&idr=%d&nonce=%d",
        method, pair, int64(price), int64(price*amount), nonce,
    )
    
    signature := createHMAC512(payload, c.secret)
    
    req := http.NewRequest("POST", endpoint, strings.NewReader(payload))
    req.Header.Set("Key", c.key)
    req.Header.Set("Sign", signature)
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
    
    resp, err := c.httpClient.Do(req)
    // ... handle response
}
```

### Cancel Order
```go
func (c *IndodaxClient) CancelOrder(pair, orderID string) error {
    endpoint := "https://indodax.com/tapi"
    method := "cancelOrder"
    nonce := time.Now().UnixMilli()
    
    payload := fmt.Sprintf(
        "method=%s&pair=%s&order_id=%s&type=buy&nonce=%d",
        method, pair, orderID, nonce,
    )
    
    signature := createHMAC512(payload, c.secret)
    
    req := http.NewRequest("POST", endpoint, strings.NewReader(payload))
    req.Header.Set("Key", c.key)
    req.Header.Set("Sign", signature)
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
    
    resp, err := c.httpClient.Do(req)
    // ... handle response
}
```

### Get Balance
```go
func (c *IndodaxClient) GetBalance() (map[string]*Balance, error) {
    endpoint := "https://indodax.com/tapi"
    method := "getInfo"
    nonce := time.Now().UnixMilli()
    
    payload := fmt.Sprintf("method=%s&nonce=%d", method, nonce)
    signature := createHMAC512(payload, c.secret)
    
    req := http.NewRequest("POST", endpoint, strings.NewReader(payload))
    req.Header.Set("Key", c.key)
    req.Header.Set("Sign", signature)
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
    
    resp, err := c.httpClient.Do(req)
    // ... parse balance from response
}
```

---

## Private WebSocket Integration

### Connect and Subscribe
```go
func (s *TradingService) subscribePrivateWS(userID string) error {
    // 1. Generate token for private WebSocket
    token, err := s.generatePrivateWSToken(userID)
    if err != nil {
        return err
    }
    
    // 2. Connect to private WebSocket
    wsURL := "wss://pws.indodax.com/ws/?cf_ws_frame_ping_pong=true"
    conn, err := websocket.Dial(wsURL)
    if err != nil {
        return err
    }
    
    // 3. Authenticate
    authMsg := map[string]interface{}{
        "params": map[string]string{
            "token": token,
        },
        "id": 1,
    }
    conn.WriteJSON(authMsg)
    
    // 4. Subscribe to order updates
    subscribeMsg := map[string]interface{}{
        "method": "subscribe",
        "params": map[string]interface{}{
            "channel": "order",
        },
        "id": 2,
    }
    conn.WriteJSON(subscribeMsg)
    
    // 5. Start listening
    go s.listenPrivateWS(userID, conn)
    
    return nil
}
```

### Handle Order Updates
```go
func (s *TradingService) listenPrivateWS(userID string, conn *websocket.Conn) {
    for {
        var msg IndodaxPrivateWSMessage
        err := conn.ReadJSON(&msg)
        if err != nil {
            log.Error("Private WS error:", err)
            s.reconnectPrivateWS(userID)
            return
        }
        
        if msg.Method == "order.update" {
            s.handleOrderUpdate(userID, msg.Params)
        }
    }
}

func (s *TradingService) handleOrderUpdate(userID string, data interface{}) {
    // Parse order update
    update := parseOrderUpdate(data)
    
    // Update order in Redis
    order := s.getOrder(update.OrderID)
    order.Status = update.Status
    order.Filled = update.Filled
    
    if update.Status == "filled" {
        order.FilledAt = time.Now()
        
        // Trigger auto-sell
        go s.autoSell(order)
    }
    
    s.saveOrder(order)
    
    // Broadcast to user
    s.broadcastOrderUpdate(userID, order)
}
```

---

## Stop-Loss Monitor

### Background Process
```go
func (s *TradingService) startStopLossMonitor() {
    ticker := time.NewTicker(1 * time.Second)
    defer ticker.Stop()
    
    for range ticker.C {
        // Get all filled buy orders with active sell orders
        orders := s.getActiveOrders()
        
        for _, order := range orders {
            if order.Side == "buy" && order.Status == "filled" {
                s.checkStopLoss(order)
            }
        }
    }
}

func (s *TradingService) checkStopLoss(order *TradeOrder) {
    // Get current market price
    currentPrice := s.getCurrentPrice(order.Pair)
    
    // Calculate stop-loss price
    stopLossPrice := order.Price * (1 - order.StopLoss/100)
    
    // Check if stop-loss triggered
    if currentPrice <= stopLossPrice {
        log.Warn("Stop-loss triggered for order", order.ID)
        
        // Cancel existing sell order
        if order.SellOrderID != "" {
            s.cancelSellOrder(order.SellOrderID)
        }
        
        // Place market sell order
        sellOrder := s.placeMarketSellOrder(order)
        
        // Update order status
        order.Status = "stopped"
        s.saveOrder(order)
        
        // Notify user
        s.notifyStopLoss(order, currentPrice, stopLossPrice)
    }
}
```

---

## Error Handling

### Indodax API Errors
```json
{
  "success": false,
  "error": {
    "code": "INDODAX_API_ERROR",
    "message": "Error from Indodax",
    "details": "Insufficient balance"
  }
}
```

### Order Not Found
```json
{
  "success": false,
  "error": {
    "code": "ORDER_NOT_FOUND",
    "message": "Order not found"
  }
}
```

### Cannot Cancel Order
```json
{
  "success": false,
  "error": {
    "code": "CANNOT_CANCEL",
    "message": "Order cannot be cancelled",
    "details": "Order is already filled"
  }
}
```

---

## Rate Limiting

### Indodax Rate Limits
- **Trade API**: 20 requests/second
- **Cancel API**: 30 requests/second
- **getInfo API**: 180 requests/minute

### Implementation
```go
type RateLimiter struct {
    tradeLimit  *rate.Limiter  // 20/sec
    cancelLimit *rate.Limiter  // 30/sec
    infoLimit   *rate.Limiter  // 3/sec (180/min)
}

func (rl *RateLimiter) WaitForTrade(ctx context.Context) error {
    return rl.tradeLimit.Wait(ctx)
}
```

---

## Security Considerations

- **API keys**: Always decrypted in-memory only
- **Balance checks**: Before every trade
- **Order validation**: Strict input validation
- **Rate limiting**: Respect Indodax limits
- **Error handling**: Never expose API keys in logs
- **User isolation**: Users can only access their own orders

---

## Testing Strategy

### Unit Tests
- Validation logic
- Profit/stop-loss calculations
- Order state transitions

### Integration Tests
- Full buy → fill → auto-sell flow
- Stop-loss trigger
- Manual sell action
- Order cancellation

### Load Tests
- Multiple concurrent orders
- High-frequency order updates
- WebSocket connection stability

---

## Future Enhancements

- [ ] Trailing stop-loss
- [ ] Dollar-cost averaging (DCA)
- [ ] Grid trading strategy
- [ ] OCO orders (One-Cancels-Other)
- [ ] Take-profit ladders
- [ ] Position sizing calculator
- [ ] Risk management presets
- [ ] Trade analytics dashboard

