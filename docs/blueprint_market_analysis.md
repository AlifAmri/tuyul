# Market Analysis

## Overview

TUYUL provides real-time market analysis for all Indodax trading pairs using a **unified coin data structure** stored in Redis. The system calculates two key metrics:

1. **Pump Score**: Multi-timeframe momentum indicator (1m, 5m, 15m, 30m)
2. **Gap Analysis**: Bid-ask spread percentage for arbitrage opportunities

---

## Key Features

### 1. Real-time Market Data
- Subscribe to Indodax Public WebSocket (market summary channel)
- Receive updates every 1-2 seconds for all trading pairs
- **Single unified data structure** per coin in Redis
- Automatic timeframe resets (rolling windows)

### 2. Pump Score Calculation
Multi-timeframe momentum indicator based on:
- **Price change %** across 4 timeframes (1m, 5m, 15m, 30m)
- **Transaction count** (volume confirmation)
- **Weighted scoring** (5m timeframe has highest weight at 40%)
- **Real-time updates** every minute when timeframes reset

### 3. Gap Analysis
Identifies arbitrage opportunities based on:
- **Bid-Ask spread** percentage
- **Volume threshold** (filter low-liquidity pairs)
- **Orderbook depth** (bid/ask volumes)

### 4. Sortable Market Table
- Sort by pump score (highest first)
- Sort by gap percentage (widest first)
- Sort by price, volume, change 24h
- Filter by minimum volume
- Search by coin symbol

### 5. Trade Action Integration
- Click on any coin to initiate trade
- Pre-fill trading form with selected pair
- View timeframe data and pump score breakdown

---

## Data Models

### Unified Coin Model
```go
type Coin struct {
    // Basic Info
    PairID          string    `json:"pair_id"`           // e.g., "btcidr"
    BaseCurrency    string    `json:"base_currency"`     // e.g., "btc"
    QuoteCurrency   string    `json:"quote_currency"`    // e.g., "idr"
    
    // Current Market Data
    CurrentPrice    float64   `json:"current_price"`     // Last traded price
    High24h         float64   `json:"high_24h"`
    Low24h          float64   `json:"low_24h"`
    Open24h         float64   `json:"open_24h"`
    Volume24h       float64   `json:"volume_24h"`        // Base currency
    VolumeIDR       float64   `json:"volume_idr"`        // In IDR
    Change24h       float64   `json:"change_24h"`        // Percentage
    
    // Orderbook Data
    BestBid         float64   `json:"best_bid"`
    BestAsk         float64   `json:"best_ask"`
    BidVolume       float64   `json:"bid_volume"`
    AskVolume       float64   `json:"ask_volume"`
    
    // Gap Analysis (calculated)
    GapPercentage   float64   `json:"gap_percentage"`    // ((Ask - Bid) / Bid) * 100
    Spread          float64   `json:"spread"`            // Ask - Bid (absolute)
    
    // Timeframe Data (OHLC + Transaction Count)
    Timeframes      Timeframes `json:"timeframes"`
    LastReset       map[string]time.Time `json:"last_reset"`
    
    // Pump Score (calculated)
    PumpScore       float64   `json:"pump_score"`        // 0 to infinity
    
    // Metadata
    LastUpdate      time.Time `json:"last_update"`
}
```

### Timeframes Structure
```go
type Timeframes struct {
    OneMinute   TimeframeData `json:"1m"`
    FiveMinute  TimeframeData `json:"5m"`
    FifteenMin  TimeframeData `json:"15m"`
    ThirtyMin   TimeframeData `json:"30m"`
}

type TimeframeData struct {
    Open  float64 `json:"open"`   // Opening price when timeframe started
    High  float64 `json:"high"`   // Highest price in timeframe
    Low   float64 `json:"low"`    // Lowest price in timeframe
    Trx   int     `json:"trx"`    // Transaction count in timeframe
}
```

---

## Redis Schema

### Unified Coin Data
```
Key Pattern: coin:{pair_id}
Type: Hash
Fields:
  pair_id          → string (e.g., "btcidr")
  base_currency    → string (e.g., "btc")
  quote_currency   → string (e.g., "idr")
  current_price    → float
  high_24h         → float
  low_24h          → float
  open_24h         → float
  volume_24h       → float
  volume_idr       → float
  change_24h       → float
  best_bid         → float
  best_ask         → float
  bid_volume       → float
  ask_volume       → float
  gap_percentage   → float (calculated)
  spread           → float (calculated)
  timeframes       → JSON string (OHLC + Trx for 1m, 5m, 15m, 30m)
  last_reset       → JSON string (reset timestamps)
  pump_score       → float (calculated)
  last_update      → timestamp

TTL: 30 seconds (stale if WebSocket disconnects)

Example:
HSET coin:btcidr 
  pair_id "btcidr"
  current_price "650000000"
  volume_idr "8125000000"
  pump_score "282.4"
  gap_percentage "0.15"
  timeframes '{"1m":{"open":500000000,"high":510000000,"low":499000000,"trx":15},...}'
  last_reset '{"1m":"2025-01-07T10:05:00Z","5m":"2025-01-07T10:05:00Z",...}'
  last_update "1704643200000"
```

### Sorted Sets for Efficient Querying

#### Pump Score Index
```
Key Pattern: market:sorted:pump_score
Type: Sorted Set
Score: Pump score (can be 0 to infinity)
Member: pair_id (e.g., "btcidr")

Purpose: Fast retrieval of top pumping coins
Query: ZREVRANGE market:sorted:pump_score 0 19 WITHSCORES  # Top 20

Example:
ZADD market:sorted:pump_score 282.4 "btcidr"
ZADD market:sorted:pump_score 156.8 "ethidr"
ZADD market:sorted:pump_score -45.2 "dogeidr"  # Negative = dump
```

#### Gap Percentage Index
```
Key Pattern: market:sorted:gap_percentage
Type: Sorted Set
Score: Gap percentage
Member: pair_id

Purpose: Fast retrieval of widest spreads
Query: ZREVRANGE market:sorted:gap_percentage 0 19 WITHSCORES  # Top 20

Example:
ZADD market:sorted:gap_percentage 0.85 "ethidr"
ZADD market:sorted:gap_percentage 0.72 "bnbidr"
```

#### Volume Index
```
Key Pattern: market:sorted:volume_idr
Type: Sorted Set
Score: Volume in IDR (24h)
Member: pair_id

Purpose: Filter by minimum volume, sort by volume
Query: ZREVRANGE market:sorted:volume_idr 0 19 WITHSCORES  # Top 20 by volume

Example:
ZADD market:sorted:volume_idr 8125000000 "btcidr"
ZADD market:sorted:volume_idr 12257000000 "ethidr"
```

#### Price Change Index
```
Key Pattern: market:sorted:change_24h
Type: Sorted Set
Score: 24h change percentage
Member: pair_id

Purpose: Top gainers/losers
Query: 
  ZREVRANGE market:sorted:change_24h 0 19 WITHSCORES  # Top gainers
  ZRANGE market:sorted:change_24h 0 19 WITHSCORES     # Top losers

Example:
ZADD market:sorted:change_24h 15.5 "shibidr"
ZADD market:sorted:change_24h -8.3 "lunaidr"
```

### Active Pairs Set
```
Key Pattern: market:active_pairs
Type: Set
Members: pair_id

Purpose: Track which pairs are currently active
Query: SMEMBERS market:active_pairs

Example:
SADD market:active_pairs "btcidr" "ethidr" "bnbidr"
```

---

## Pump Score Calculation

### Formula

Based on the `pumpscore.md` specification:

```
Pump Score = Σ (Timeframe Score × Weight)

where:
Timeframe Score = Price Change % × Transaction Count × Weight

Detailed:
Pump Score = (1m_pct × 1m_trx × 0.20) +
             (5m_pct × 5m_trx × 0.40) +
             (15m_pct × 15m_trx × 0.30) +
             (30m_pct × 30m_trx × 0.10)
```

### Timeframe Weights

| Timeframe | Weight | Reasoning |
|-----------|--------|-----------|
| **1m** | 20% | Short-term momentum (high noise) |
| **5m** | **40%** | **Highest** - Best signal-to-noise ratio |
| **15m** | 30% | Medium-term trend confirmation |
| **30m** | 10% | Long-term trend (slower to react) |

**Why 5m has highest weight?**
- 1m is too noisy (false signals from market microstructure)
- 30m is too slow (pump may have already peaked)
- 5m is the sweet spot (captures real momentum without excessive noise)

### Price Change Calculation

```go
func calculatePriceChange(currentPrice, openPrice float64) float64 {
    return ((currentPrice - openPrice) / openPrice) * 100
}
```

### Implementation

```go
func CalculatePumpScore(coin *Coin) float64 {
    tf := coin.Timeframes
    currentPrice := coin.CurrentPrice
    
    // Calculate price change % for each timeframe
    change1m := ((currentPrice - tf.OneMinute.Open) / tf.OneMinute.Open) * 100
    change5m := ((currentPrice - tf.FiveMinute.Open) / tf.FiveMinute.Open) * 100
    change15m := ((currentPrice - tf.FifteenMin.Open) / tf.FifteenMin.Open) * 100
    change30m := ((currentPrice - tf.ThirtyMin.Open) / tf.ThirtyMin.Open) * 100
    
    // Calculate weighted scores
    score1m := change1m * float64(tf.OneMinute.Trx) * 0.20
    score5m := change5m * float64(tf.FiveMinute.Trx) * 0.40
    score15m := change15m * float64(tf.FifteenMin.Trx) * 0.30
    score30m := change30m * float64(tf.ThirtyMin.Trx) * 0.10
    
    // Total pump score
    return score1m + score5m + score15m + score30m
}
```

### Example Calculation

**Strong Pump Signal - BTC/IDR:**

| Timeframe | Open | Current | Change | Trx | Weight | Calculation | Score |
|-----------|------|---------|--------|-----|--------|-------------|-------|
| 1m | 500,000,000 | 510,000,000 | +2.0% | 15 | 0.20 | 2.0 × 15 × 0.20 | **6.0** |
| 5m | 500,000,000 | 515,000,000 | +3.0% | 45 | 0.40 | 3.0 × 45 × 0.40 | **54.0** |
| 15m | 498,000,000 | 515,000,000 | +3.4% | 120 | 0.30 | 3.4 × 120 × 0.30 | **122.4** |
| 30m | 495,000,000 | 515,000,000 | +4.0% | 250 | 0.10 | 4.0 × 250 × 0.10 | **100.0** |

**Total Pump Score = 282.4** ✅ **VERY STRONG PUMP**

---

## Gap Analysis Calculation

### Formula

```
Gap Percentage = ((Best Ask - Best Bid) / Best Bid) × 100
Spread = Best Ask - Best Bid (absolute value)
```

### Implementation

```go
func CalculateGap(coin *Coin) {
    coin.GapPercentage = ((coin.BestAsk - coin.BestBid) / coin.BestBid) * 100
    coin.Spread = coin.BestAsk - coin.BestBid
}
```

### Volume Filtering

```go
const MIN_VOLUME_IDR = 10_000_000 // 10M IDR (~$650 USD)

func ShouldIncludeInGapAnalysis(coin *Coin) bool {
    return coin.VolumeIDR >= MIN_VOLUME_IDR
}
```

### Gap Interpretation

| Gap % | Interpretation |
|-------|----------------|
| **> 5%** | Very wide spread (potential arbitrage) |
| **2-5%** | Wide spread (careful trading) |
| **1-2%** | Normal spread |
| **< 1%** | Tight spread (high liquidity) |

---

## Score Interpretation

### Pump Score Ranges

| Pump Score Range | Signal Strength | Interpretation | Action |
|-----------------|----------------|----------------|--------|
| **< 0** | Dump | Negative momentum, downtrend | ❌ **Avoid** |
| **0 - 5** | Flat/Neutral | No significant movement | ⏸️ **Wait** |
| **5 - 20** | Weak Pump | Minor upward movement | ⚠️ **Watch** |
| **20 - 50** | Moderate Pump | Decent momentum building | 🟡 **Consider Entry** |
| **50 - 100** | Strong Pump | Clear buying pressure | 🟢 **Entry Signal** |
| **100+** | Very Strong Pump | Exceptional momentum | 🚀 **High Conviction** |

---

## Timeframe Management

### Timeframe Reset Logic

Each timeframe has an independent reset timer on a **rolling window** basis:

```go
type TimeframeManager struct {
    updateChan chan *Coin
}

func (tm *TimeframeManager) Start() {
    ticker := time.NewTicker(1 * time.Minute)
    defer ticker.Stop()
    
    for range ticker.C {
        tm.checkAndResetTimeframes()
    }
}

func (tm *TimeframeManager) checkAndResetTimeframes() {
    now := time.Now()
    
    // Get all active coins
    pairs, _ := redis.Client.SMembers(ctx, "market:active_pairs").Result()
    
    for _, pairID := range pairs {
        coin := tm.getCoin(pairID)
        
        // Check each timeframe
        if tm.shouldReset("1m", coin.LastReset["1m"], now) {
            tm.resetTimeframe(coin, "1m", now)
        }
        if tm.shouldReset("5m", coin.LastReset["5m"], now) {
            tm.resetTimeframe(coin, "5m", now)
        }
        if tm.shouldReset("15m", coin.LastReset["15m"], now) {
            tm.resetTimeframe(coin, "15m", now)
        }
        if tm.shouldReset("30m", coin.LastReset["30m"], now) {
            tm.resetTimeframe(coin, "30m", now)
        }
        
        // Recalculate pump score after resets
        coin.PumpScore = CalculatePumpScore(coin)
        
        // Update in Redis
        tm.saveCoin(coin)
        
        // Broadcast to WebSocket clients
        tm.broadcastUpdate(coin)
    }
}

func (tm *TimeframeManager) shouldReset(timeframe string, lastReset time.Time, now time.Time) bool {
    switch timeframe {
    case "1m":
        return now.Sub(lastReset) >= 1*time.Minute
    case "5m":
        return now.Sub(lastReset) >= 5*time.Minute
    case "15m":
        return now.Sub(lastReset) >= 15*time.Minute
    case "30m":
        return now.Sub(lastReset) >= 30*time.Minute
    }
    return false
}

func (tm *TimeframeManager) resetTimeframe(coin *Coin, timeframe string, now time.Time) {
    switch timeframe {
    case "1m":
        coin.Timeframes.OneMinute = TimeframeData{
            Open: coin.CurrentPrice,
            High: coin.CurrentPrice,
            Low:  coin.CurrentPrice,
            Trx:  0,
        }
        coin.LastReset["1m"] = now
    case "5m":
        coin.Timeframes.FiveMinute = TimeframeData{
            Open: coin.CurrentPrice,
            High: coin.CurrentPrice,
            Low:  coin.CurrentPrice,
            Trx:  0,
        }
        coin.LastReset["5m"] = now
    case "15m":
        coin.Timeframes.FifteenMin = TimeframeData{
            Open: coin.CurrentPrice,
            High: coin.CurrentPrice,
            Low:  coin.CurrentPrice,
            Trx:  0,
        }
        coin.LastReset["15m"] = now
    case "30m":
        coin.Timeframes.ThirtyMin = TimeframeData{
            Open: coin.CurrentPrice,
            High: coin.CurrentPrice,
            Low:  coin.CurrentPrice,
            Trx:  0,
        }
        coin.LastReset["30m"] = now
    }
}
```

### Reset Schedule

| Timeframe | Reset Interval | Example |
|-----------|----------------|---------|
| 1m | Every 1 minute | 10:00:00 → 10:01:00 → 10:02:00 |
| 5m | Every 5 minutes | 10:00:00 → 10:05:00 → 10:10:00 |
| 15m | Every 15 minutes | 10:00:00 → 10:15:00 → 10:30:00 |
| 30m | Every 30 minutes | 10:00:00 → 10:30:00 → 11:00:00 |

**Important:** Reset times are independent for each timeframe!

---

## WebSocket Integration

### Indodax Public WebSocket Connection

```go
type MarketDataService struct {
    wsConn      *websocket.Conn
    redis       *redis.Client
    updateChan  chan *Coin
}

func (s *MarketDataService) Start() error {
    // 1. Connect to Indodax Public WS
    conn, err := websocket.Dial("wss://ws3.indodax.com/ws/")
    if err != nil {
        return err
    }
    s.wsConn = conn
    
    // 2. Authenticate (static token from docs)
    authMsg := map[string]interface{}{
        "params": map[string]string{
            "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
        },
        "id": 1,
    }
    s.wsConn.WriteJSON(authMsg)
    
    // 3. Subscribe to market summaries (all pairs)
    subscribeMsg := map[string]interface{}{
        "method": "subscribeSummaries",
        "params": map[string]interface{}{},
        "id": 2,
    }
    s.wsConn.WriteJSON(subscribeMsg)
    
    // 4. Start listening for messages
    go s.listen()
    
    return nil
}

func (s *MarketDataService) listen() {
    for {
        var msg IndodaxWSMessage
        err := s.wsConn.ReadJSON(&msg)
        if err != nil {
            log.Error("WebSocket read error:", err)
            s.reconnect()
            continue
        }
        
        // Process market summary update
        if msg.Method == "summary.update" {
            s.processMarketUpdate(msg.Params)
        }
    }
}
```

### Message Processing

```go
func (s *MarketDataService) processMarketUpdate(data interface{}) {
    // 1. Parse market data from Indodax
    marketData := parseMarketData(data)
    
    // 2. Get or create coin
    coin := s.getOrCreateCoin(marketData.PairID)
    
    // 3. Update current price and 24h stats
    coin.CurrentPrice = marketData.LastPrice
    coin.High24h = marketData.High24h
    coin.Low24h = marketData.Low24h
    coin.Open24h = marketData.Open24h
    coin.Volume24h = marketData.Volume24h
    coin.VolumeIDR = marketData.VolumeIDR
    coin.Change24h = marketData.Change24h
    coin.BestBid = marketData.BestBid
    coin.BestAsk = marketData.BestAsk
    coin.BidVolume = marketData.BidVolume
    coin.AskVolume = marketData.AskVolume
    coin.LastUpdate = time.Now()
    
    // 4. Update timeframe OHLC and increment transaction count
    s.updateTimeframes(coin)
    
    // 5. Calculate gap
    coin.GapPercentage = ((coin.BestAsk - coin.BestBid) / coin.BestBid) * 100
    coin.Spread = coin.BestAsk - coin.BestBid
    
    // 6. Calculate pump score
    coin.PumpScore = CalculatePumpScore(coin)
    
    // 7. Save to Redis
    s.saveCoin(coin)
    
    // 8. Update sorted sets
    s.updateSortedSets(coin)
    
    // 9. Broadcast to WebSocket clients
    s.broadcastUpdate(coin)
}

func (s *MarketDataService) updateTimeframes(coin *Coin) {
    price := coin.CurrentPrice
    
    // Update 1m
    if price > coin.Timeframes.OneMinute.High {
        coin.Timeframes.OneMinute.High = price
    }
    if price < coin.Timeframes.OneMinute.Low {
        coin.Timeframes.OneMinute.Low = price
    }
    coin.Timeframes.OneMinute.Trx++
    
    // Similar for 5m, 15m, 30m
    // ... (same logic for each timeframe)
}
```

### Save to Redis

```go
func (s *MarketDataService) saveCoin(coin *Coin) error {
    ctx := context.Background()
    key := fmt.Sprintf("coin:%s", coin.PairID)
    
    // Serialize timeframes and last_reset to JSON
    timeframesJSON, _ := json.Marshal(coin.Timeframes)
    lastResetJSON, _ := json.Marshal(coin.LastReset)
    
    // Save to Redis hash
    pipe := s.redis.Pipeline()
    pipe.HSet(ctx, key, map[string]interface{}{
        "pair_id":          coin.PairID,
        "base_currency":    coin.BaseCurrency,
        "quote_currency":   coin.QuoteCurrency,
        "current_price":    coin.CurrentPrice,
        "high_24h":         coin.High24h,
        "low_24h":          coin.Low24h,
        "open_24h":         coin.Open24h,
        "volume_24h":       coin.Volume24h,
        "volume_idr":       coin.VolumeIDR,
        "change_24h":       coin.Change24h,
        "best_bid":         coin.BestBid,
        "best_ask":         coin.BestAsk,
        "bid_volume":       coin.BidVolume,
        "ask_volume":       coin.AskVolume,
        "gap_percentage":   coin.GapPercentage,
        "spread":           coin.Spread,
        "timeframes":       string(timeframesJSON),
        "last_reset":       string(lastResetJSON),
        "pump_score":       coin.PumpScore,
        "last_update":      coin.LastUpdate.UnixMilli(),
    })
    pipe.Expire(ctx, key, 30*time.Second) // TTL 30 seconds
    
    _, err := pipe.Exec(ctx)
    return err
}

func (s *MarketDataService) updateSortedSets(coin *Coin) error {
    ctx := context.Background()
    
    pipe := s.redis.Pipeline()
    
    // Update sorted sets for efficient querying
    pipe.ZAdd(ctx, "market:sorted:pump_score", &redis.Z{
        Score:  coin.PumpScore,
        Member: coin.PairID,
    })
    
    pipe.ZAdd(ctx, "market:sorted:gap_percentage", &redis.Z{
        Score:  coin.GapPercentage,
        Member: coin.PairID,
    })
    
    pipe.ZAdd(ctx, "market:sorted:volume_idr", &redis.Z{
        Score:  coin.VolumeIDR,
        Member: coin.PairID,
        })
    
    pipe.ZAdd(ctx, "market:sorted:change_24h", &redis.Z{
        Score:  coin.Change24h,
        Member: coin.PairID,
    })
    
    // Add to active pairs set
    pipe.SAdd(ctx, "market:active_pairs", coin.PairID)
    
    _, err := pipe.Exec(ctx)
    return err
}
```

---

## API Endpoints

### GET /api/v1/market/summary
Get all market summaries

**Query Parameters:**
- `limit`: int (default: 50, max: 200)
- `min_volume`: float (minimum volume in IDR, default: 0)
- `sort_by`: string (pump_score, gap_percentage, volume_idr, change_24h) - default: pump_score

**Response:**
```json
{
  "success": true,
  "data": {
    "coins": [
      {
        "pair_id": "btcidr",
        "base_currency": "btc",
        "quote_currency": "idr",
        "current_price": 650000000,
        "high_24h": 670000000,
        "low_24h": 640000000,
        "volume_24h": 12.5,
        "volume_idr": 8125000000,
        "change_24h": 1.5,
        "best_bid": 649500000,
        "best_ask": 650500000,
        "gap_percentage": 0.15,
        "spread": 1000000,
        "pump_score": 282.4,
        "timeframes": {
          "1m": {"open": 500000000, "high": 510000000, "low": 499000000, "trx": 15},
          "5m": {"open": 500000000, "high": 515000000, "low": 497000000, "trx": 45},
          "15m": {"open": 498000000, "high": 520000000, "low": 495000000, "trx": 120},
          "30m": {"open": 495000000, "high": 525000000, "low": 493000000, "trx": 250}
        },
        "last_update": "2024-01-07T18:00:00Z"
      }
    ],
    "count": 50,
    "last_update": "2024-01-07T18:00:00Z"
  }
}
```

**Implementation:**
```go
func (h *MarketHandler) GetSummary(c *gin.Context) {
    // Parse query params
    limit := c.DefaultQuery("limit", "50")
    minVolume := c.DefaultQuery("min_volume", "0")
    sortBy := c.DefaultQuery("sort_by", "pump_score")
    
    // Query sorted set based on sort_by
    sortKey := fmt.Sprintf("market:sorted:%s", sortBy)
    pairIDs, _ := h.redis.ZRevRange(ctx, sortKey, 0, limit-1).Result()
    
    // Get coin data for each pair
    coins := []Coin{}
    for _, pairID := range pairIDs {
        coin := h.getCoin(pairID)
        if coin.VolumeIDR >= minVolume {
            coins = append(coins, coin)
        }
    }
    
    c.JSON(200, gin.H{
        "success": true,
        "data": gin.H{
            "coins":       coins,
            "count":       len(coins),
            "last_update": time.Now(),
        },
    })
}
```

### GET /api/v1/market/pump-scores
Get markets sorted by pump score

**Query Parameters:**
- `limit`: int (default: 20, max: 100)
- `min_score`: float (minimum pump score, default: 0)
- `min_volume`: float (minimum volume in IDR, default: 1000000)

**Response:**
```json
{
  "success": true,
  "data": {
    "coins": [
      {
        "pair_id": "shibidr",
        "base_currency": "shib",
        "current_price": 0.00085,
        "change_24h": 25.3,
        "volume_idr": 42500000,
        "pump_score": 87.5,
        "timeframes": {...},
        "last_update": "2024-01-07T18:00:00Z"
      }
    ],
    "count": 20
  }
}
```

### GET /api/v1/market/gaps
Get markets sorted by bid-ask gap

**Query Parameters:**
- `limit`: int (default: 20, max: 100)
- `min_gap`: float (minimum gap percentage, default: 0)
- `min_volume`: float (minimum volume in IDR, default: 10000000)

**Response:**
```json
{
  "success": true,
  "data": {
    "coins": [
      {
        "pair_id": "ethidr",
        "base_currency": "eth",
        "current_price": 35000000,
        "gap_percentage": 0.85,
        "best_bid": 35000000,
        "best_ask": 35300000,
        "spread": 300000,
        "volume_idr": 12257000000,
        "last_update": "2024-01-07T18:00:00Z"
      }
    ],
    "count": 20
  }
}
```

### GET /api/v1/market/:pair
Get specific market data

**Response:**
```json
{
  "success": true,
  "data": {
    "pair_id": "btcidr",
    "base_currency": "btc",
    "quote_currency": "idr",
    "current_price": 650000000,
    "high_24h": 670000000,
    "low_24h": 640000000,
    "volume_24h": 12.5,
    "volume_idr": 8125000000,
    "change_24h": 1.5,
    "best_bid": 649500000,
    "best_ask": 650500000,
    "gap_percentage": 0.15,
    "spread": 1000000,
    "pump_score": 282.4,
    "timeframes": {
      "1m": {"open": 500000000, "high": 510000000, "low": 499000000, "trx": 15},
      "5m": {"open": 500000000, "high": 515000000, "low": 497000000, "trx": 45},
      "15m": {"open": 498000000, "high": 520000000, "low": 495000000, "trx": 120},
      "30m": {"open": 495000000, "high": 525000000, "low": 493000000, "trx": 250}
    },
    "last_reset": {
      "1m": "2024-01-07T18:00:00Z",
      "5m": "2024-01-07T17:55:00Z",
      "15m": "2024-01-07T17:45:00Z",
      "30m": "2024-01-07T17:30:00Z"
    },
    "last_update": "2024-01-07T18:00:30Z"
  }
}
```

---

## WebSocket Real-time Updates

### Client Connection
```
wss://your-backend.com/ws/market?token={jwt_access_token}
```

### Subscribe to Market Updates
```json
{
  "action": "subscribe",
  "channel": "market",
  "pairs": ["btcidr", "ethidr"]  // Empty array = all pairs
}
```

### Market Update Message (Server → Client)
```json
{
  "type": "market_update",
  "data": {
    "pair_id": "btcidr",
    "current_price": 650500000,
    "change_24h": 1.58,
    "volume_idr": 8150000000,
    "pump_score": 285.2,
    "gap_percentage": 0.15,
    "timestamp": "2024-01-07T18:00:30Z"
  }
}
```

### Pump Score Update (every minute when timeframes reset)
```json
{
  "type": "pump_score_update",
  "data": {
    "pair_id": "btcidr",
    "pump_score": 285.2,
    "timeframes": {
      "1m": {"open": 650000000, "high": 651000000, "low": 649000000, "trx": 18},
      "5m": {"open": 500000000, "high": 651000000, "low": 497000000, "trx": 52},
      "15m": {"open": 498000000, "high": 651000000, "low": 495000000, "trx": 135},
      "30m": {"open": 495000000, "high": 651000000, "low": 493000000, "trx": 268}
    },
    "timestamp": "2024-01-07T18:01:00Z"
  }
}
```

---

## Performance & Redis Benefits

### Why Redis is Faster for This Use Case

#### 1. **In-Memory Storage**
- All coin data stored in RAM
- Sub-millisecond read/write operations
- Perfect for real-time market data updates every 1-2 seconds

#### 2. **Atomic Operations**
- HSET updates are atomic (no race conditions)
- ZADD updates sorted sets atomically
- Pipeline operations for batch updates

#### 3. **Sorted Sets for Efficient Querying**
```go
// Get top 20 pumping coins - O(log(N) + M) complexity
ZREVRANGE market:sorted:pump_score 0 19 WITHSCORES

// Get coins with pump score >= 50
ZRANGEBYSCORE market:sorted:pump_score 50 +inf

// Much faster than scanning all coins and sorting in application
```

#### 4. **Native Pub/Sub**
- Real-time updates to WebSocket clients
- No need for additional message queue
- Built-in fan-out pattern

#### 5. **TTL Support**
- Automatic cleanup of stale data (30s TTL)
- No manual garbage collection needed

#### 6. **Single Data Structure**
- No JOINs needed (unlike PostgreSQL)
- One HGETALL to get complete coin data
- Simpler code, fewer queries

### Performance Comparison

| Operation | Redis | PostgreSQL |
|-----------|-------|------------|
| Get single coin | 0.1ms | 5-10ms |
| Get top 20 by pump score | 0.5ms | 20-50ms (with index) |
| Update coin + sorted sets | 1ms | 10-30ms (update + indexes) |
| Real-time updates/sec | 10,000+ | 1,000-3,000 |

### Benchmark Results (Expected)

```
Market data updates:  ~1,000 updates/sec (all pairs)
API queries:          ~10,000 req/sec
WebSocket broadcasts: ~5,000 messages/sec
Memory usage:         ~100-200 MB for 200 pairs
```

---

## Error Handling

### WebSocket Disconnection
```go
func (s *MarketDataService) reconnect() {
    backoff := time.Second
    maxBackoff := time.Minute
    
    for {
        log.Info("Attempting to reconnect to Indodax WebSocket...")
        err := s.Start()
        if err == nil {
            log.Info("Reconnected successfully")
            return
        }
        
        log.Error("Reconnection failed:", err)
        time.Sleep(backoff)
        backoff = time.Duration(math.Min(float64(backoff*2), float64(maxBackoff)))
    }
}
```

### Stale Data Detection
```go
func (s *MarketDataService) monitorStaleData() {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    
    for range ticker.C {
        pairs, _ := s.redis.SMembers(ctx, "market:active_pairs").Result()
        for _, pairID := range pairs {
            coin := s.getCoin(pairID)
            if time.Since(coin.LastUpdate) > 15*time.Second {
                log.Warn("Stale data detected for", pairID)
                // Redis TTL will auto-remove after 30s
            }
        }
    }
}
```

---

## Testing Strategy

### Unit Tests
- Pump score calculation with various inputs
- Gap percentage calculation
- Timeframe reset logic
- Price change calculation

### Integration Tests
- WebSocket connection and reconnection
- Market data parsing
- Redis storage and retrieval
- Sorted set updates
- Client broadcast mechanism

### Load Tests
- Handle 100+ concurrent WebSocket clients
- Process 1000+ market updates per second
- Redis sorted set performance with 200+ pairs
- Memory usage under load

---

## Future Enhancements

- [ ] Historical pump score data (time-series)
- [ ] Custom alerts (price/volume/pump score thresholds)
- [ ] Market heatmap visualization
- [ ] Correlation analysis between coins
- [ ] Volume profile analysis
- [ ] Order book imbalance detection
- [ ] Machine learning price prediction
- [ ] Multi-exchange aggregation
