package market

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"tuyul/backend/pkg/logger"

	"tuyul/backend/internal/model"
	"tuyul/backend/pkg/indodax"
	"tuyul/backend/pkg/redis"
)

type MarketDataService struct {
	redisClient *redis.Client
	wsClient    *indodax.WSClient
	restClient  *indodax.Client

	// Cache for coins to avoid frequent unmarshal from Redis during updates
	// Key: pairID
	coinCache sync.Map

	// Metadata cache
	pairs      sync.Map // Key: pairID, Value: indodax.Pair
	increments sync.Map // Key: pairID, Value: float64

	updateChan chan *model.Coin

	// Subscriptions
	subscribers []func(coin *model.Coin)
	mu          sync.RWMutex
}

func NewMarketDataService(redisClient *redis.Client, wsClient *indodax.WSClient, restClient *indodax.Client) *MarketDataService {
	return &MarketDataService{
		redisClient: redisClient,
		wsClient:    wsClient,
		restClient:  restClient,
		updateChan:  make(chan *model.Coin, 100), // Buffer updates
	}
}

// Start begins listening to market data
func (s *MarketDataService) Start() {
	// Register WS handlers (add, don't replace existing handlers)
	s.wsClient.AddMessageHandler(s.handleWSMessage)

	// Connect to WS
	if err := s.wsClient.Connect(); err != nil {
		logger.Errorf("Failed to connect to public WS: %v", err)
	}

	// Subscribe to market summaries (all pairs)
	s.wsClient.Subscribe("market:summary-24h")

	// Load existing metadata from Redis
	// If empty, sync from exchange automatically
	if !s.LoadMetadata() {
		logger.Infof("Metadata cache empty, performing initial sync from Indodax...")
		go s.RefreshMetadata()
	}

	// Initial gap update (synchronous - must complete before server starts)
	logger.Infof("Performing initial gap/spread update from REST API...")
	s.updateGapsFromREST()
	logger.Infof("Initial gap/spread update completed")

	// Start periodic REST poller for Best Bid / Best Ask and Gap Analysis
	// (WebSocket market:summary-24h doesn't provide Bid/Ask)
	go s.pollGapData()
}

// RefreshMetadata fetches pairs and price increments from Indodax and saves to Redis
func (s *MarketDataService) RefreshMetadata() error {
	ctx := context.Background()

	// Fetch pairs
	pairs, err := s.restClient.GetPairs(ctx)
	if err == nil {
		for _, p := range pairs {
			s.pairs.Store(p.ID, p)
			// Pre-initialize coin object if not exists
			if _, ok := s.coinCache.Load(p.ID); !ok {
				coin, _ := s.getOrCreateCoin(p.ID)
				go s.saveCoinToRedis(coin)
			}
		}
		// Save to Redis
		s.redisClient.SetJSON(ctx, redis.CachePairsKey(), pairs, 0)
		logger.Infof("Successfully refreshed %d pairs from Indodax and initialized coins", len(pairs))
	} else {
		logger.Errorf("Failed to refresh pairs: %v", err)
		return err
	}

	// Fetch increments
	increments, err := s.restClient.GetPriceIncrements(ctx)
	if err == nil {
		// Normalize keys: remove underscores to match pair IDs (cst_idr -> cstidr)
		normalizedIncrements := make(map[string]string)
		for pair, inc := range increments {
			f, _ := strconv.ParseFloat(inc, 64)
			normalizedPair := strings.ReplaceAll(pair, "_", "")
			s.increments.Store(normalizedPair, f)
			normalizedIncrements[normalizedPair] = inc
		}
		// Save normalized increments to Redis
		s.redisClient.SetJSON(ctx, redis.CachePriceIncrementsKey(), normalizedIncrements, 0)
		logger.Infof("Successfully refreshed %d price increments from Indodax (normalized keys)", len(increments))
	} else {
		logger.Errorf("Failed to refresh increments: %v", err)
		return err
	}

	logger.Infof("Market metadata synchronization complete")
	return nil
}

// LoadMetadata loads pairs and price increments from Redis
// Returns true if metadata was successfully loaded
func (s *MarketDataService) LoadMetadata() bool {
	ctx := context.Background()
	loaded := false

	// Load pairs
	var pairs []indodax.Pair
	if err := s.redisClient.GetJSON(ctx, redis.CachePairsKey(), &pairs); err == nil && len(pairs) > 0 {
		for _, p := range pairs {
			s.pairs.Store(p.ID, p)
			// Pre-initialize coin object if not exists
			if _, ok := s.coinCache.Load(p.ID); !ok {
				coin, _ := s.getOrCreateCoin(p.ID)
				go s.saveCoinToRedis(coin)
			}
		}
		logger.Infof("Loaded %d pairs from Redis and initialized coins", len(pairs))
		loaded = true
	}

	// Load increments
	var increments map[string]string
	if err := s.redisClient.GetJSON(ctx, redis.CachePriceIncrementsKey(), &increments); err == nil {
		for pair, inc := range increments {
			f, _ := strconv.ParseFloat(inc, 64)
			// Keys should already be normalized in Redis, but ensure consistency
			normalizedPair := strings.ReplaceAll(pair, "_", "")
			s.increments.Store(normalizedPair, f)
		}
		logger.Infof("Loaded %d price increments from Redis", len(increments))
	}

	return loaded
}

// GetPairInfo returns metadata for a pair
func (s *MarketDataService) GetPairInfo(pairID string) (indodax.Pair, bool) {
	val, ok := s.pairs.Load(pairID)
	if !ok {
		return indodax.Pair{}, false
	}
	return val.(indodax.Pair), true
}

// GetPriceIncrement returns the price increment for a pair
func (s *MarketDataService) GetPriceIncrement(pairID string) (float64, bool) {
	val, ok := s.increments.Load(pairID)
	if !ok {
		return 0, false
	}
	return val.(float64), true
}

// OnUpdate registers a handler for coin updates
func (s *MarketDataService) OnUpdate(handler func(coin *model.Coin)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subscribers = append(s.subscribers, handler)
}

func (s *MarketDataService) handleWSMessage(channel string, data []byte) {
	if channel == "market:summary-24h" {
		s.processSummaryUpdate(data)
	}
}

func (s *MarketDataService) processSummaryUpdate(data []byte) {
	// Silent processing - no logging for market summary updates
	var summaryData struct {
		Data [][]interface{} `json:"data"`
	}

	if err := json.Unmarshal(data, &summaryData); err != nil {
		logger.Errorf("Failed to parse summary data: %v", err)
		return
	}

	for _, item := range summaryData.Data {
		if len(item) < 8 {
			continue
		}

		pairID, ok := item[0].(string)
		if !ok {
			continue
		}

		lastPrice := parseFloat(item[2])
		high24h := parseFloat(item[4])
		low24h := parseFloat(item[3])
		open24h := parseFloat(item[5])
		volIDR := parseStringFloat(item[6])
		volBase := parseStringFloat(item[7])

		// Update Coin
		s.updateCoin(pairID, lastPrice, high24h, low24h, open24h, volBase, volIDR)
	}
}

func (s *MarketDataService) updateCoin(pairID string, price, high, low, open, volBase, volIDR float64) {
	// Get from cache or create
	coin, _ := s.getOrCreateCoin(pairID)

	// Update 24h Data
	coin.CurrentPrice = price
	coin.High24h = high
	coin.Low24h = low
	coin.Open24h = open
	coin.Volume24h = volBase
	coin.VolumeIDR = volIDR

	// Calculate Change 24h
	if open > 0 {
		coin.Change24h = ((price - open) / open) * 100
	}

	coin.LastUpdate = time.Now()

	// Update Timeframes (OHLC + Trx)
	s.updateTimeframes(coin, price)

	// Calculate Pump Score
	coin.PumpScore = CalculatePumpScore(coin)

	// Calculate Volatility
	coin.Volatility1m = CalculateVolatility(coin)

	// Calculate Gap (If we had Bid/Ask. Summary WS lacks it, but we'll call anyway)
	CalculateGap(coin)

	// Save to Cache & Redis
	s.coinCache.Store(pairID, coin)
	go s.saveCoinToRedis(coin)

	// Sample log every 100 updates per pair to see it's working
	if coin.Timeframes.OneMinute.Trx%100 == 0 {
		logger.Infof("Updated coin %s: Price=%.2f, PumpScore=%.2f", pairID, price, coin.PumpScore)
	}

	// Notify subscribers
	s.mu.RLock()
	handlers := s.subscribers
	s.mu.RUnlock()
	for _, h := range handlers {
		h(coin)
	}
}

func (s *MarketDataService) getOrCreateCoin(pairID string) (*model.Coin, bool) {
	if v, ok := s.coinCache.Load(pairID); ok {
		return v.(*model.Coin), true
	}

	now := time.Now()
	coin := &model.Coin{
		PairID:        pairID,
		BaseCurrency:  strings.TrimSuffix(pairID, "idr"),
		QuoteCurrency: "idr",
		LastReset: map[string]time.Time{
			"1m":  now,
			"5m":  now,
			"15m": now,
			"30m": now,
		},
	}
	// Initial Timeframes
	coin.Timeframes.OneMinute.Open = coin.CurrentPrice
	coin.Timeframes.FiveMinute.Open = coin.CurrentPrice
	coin.Timeframes.FifteenMin.Open = coin.CurrentPrice
	coin.Timeframes.ThirtyMin.Open = coin.CurrentPrice

	s.coinCache.Store(pairID, coin)
	return coin, false
}

func (s *MarketDataService) updateTimeframes(coin *model.Coin, price float64) {
	// Update 1m
	if coin.Timeframes.OneMinute.Open == 0 {
		coin.Timeframes.OneMinute.Open = price
	}
	coin.Timeframes.OneMinute.High = max(coin.Timeframes.OneMinute.High, price)
	if coin.Timeframes.OneMinute.Low == 0 {
		coin.Timeframes.OneMinute.Low = price
	} else {
		coin.Timeframes.OneMinute.Low = min(coin.Timeframes.OneMinute.Low, price)
	}
	coin.Timeframes.OneMinute.Trx++

	// Update 5m
	if coin.Timeframes.FiveMinute.Open == 0 {
		coin.Timeframes.FiveMinute.Open = price
	}
	coin.Timeframes.FiveMinute.High = max(coin.Timeframes.FiveMinute.High, price)
	if coin.Timeframes.FiveMinute.Low == 0 {
		coin.Timeframes.FiveMinute.Low = price
	} else {
		coin.Timeframes.FiveMinute.Low = min(coin.Timeframes.FiveMinute.Low, price)
	}
	coin.Timeframes.FiveMinute.Trx++

	// Update 15m
	if coin.Timeframes.FifteenMin.Open == 0 {
		coin.Timeframes.FifteenMin.Open = price
	}
	coin.Timeframes.FifteenMin.High = max(coin.Timeframes.FifteenMin.High, price)
	if coin.Timeframes.FifteenMin.Low == 0 {
		coin.Timeframes.FifteenMin.Low = price
	} else {
		coin.Timeframes.FifteenMin.Low = min(coin.Timeframes.FifteenMin.Low, price)
	}
	coin.Timeframes.FifteenMin.Trx++

	// Update 30m
	if coin.Timeframes.ThirtyMin.Open == 0 {
		coin.Timeframes.ThirtyMin.Open = price
	}
	coin.Timeframes.ThirtyMin.High = max(coin.Timeframes.ThirtyMin.High, price)
	if coin.Timeframes.ThirtyMin.Low == 0 {
		coin.Timeframes.ThirtyMin.Low = price
	} else {
		coin.Timeframes.ThirtyMin.Low = min(coin.Timeframes.ThirtyMin.Low, price)
	}
	coin.Timeframes.ThirtyMin.Trx++
}

func (s *MarketDataService) saveCoinToRedis(coin *model.Coin) {
	ctx := context.Background()
	key := redis.CoinKey(coin.PairID)

	data, err := coin.ToMap()
	if err != nil {
		logger.Errorf("Failed to map coin: %v", err)
		return
	}

	err = s.redisClient.HSet(ctx, key, data)
	if err != nil {
		logger.Errorf("Redis error: %v", err)
	}

	// Update Sorted Sets using exposed types
	s.redisClient.ZAdd(ctx, redis.PumpScoreRankKey(), redis.Z{Score: coin.PumpScore, Member: coin.PairID})
	s.redisClient.ZAdd(ctx, redis.GapRankKey(), redis.Z{Score: coin.GapPercentage, Member: coin.PairID})
	s.redisClient.ZAdd(ctx, redis.ChangeRankKey(), redis.Z{Score: coin.Change24h, Member: coin.PairID})
	s.redisClient.ZAdd(ctx, redis.VolumeRankKey(), redis.Z{Score: coin.VolumeIDR, Member: coin.PairID})

	// Add to active pairs set
	s.redisClient.SAdd(ctx, redis.ActivePairsKey(), coin.PairID)
}

func (s *MarketDataService) PerformTimeframeReset(pairID string, now time.Time) {
	val, ok := s.coinCache.Load(pairID)
	if !ok {
		return
	}
	coin := val.(*model.Coin)

	if coin.LastReset == nil {
		logger.Warnf("Pair %s: LastReset map is nil", pairID)
		return
	}

	updated := false
	if s.shouldReset("1m", coin.LastReset["1m"], now) {
		s.resetTimeframe(coin, "1m", now)
		updated = true
	}
	if s.shouldReset("5m", coin.LastReset["5m"], now) {
		s.resetTimeframe(coin, "5m", now)
		updated = true
	}
	if s.shouldReset("15m", coin.LastReset["15m"], now) {
		s.resetTimeframe(coin, "15m", now)
		updated = true
	}
	if s.shouldReset("30m", coin.LastReset["30m"], now) {
		s.resetTimeframe(coin, "30m", now)
		updated = true
	}

	if updated {
		coin.PumpScore = CalculatePumpScore(coin)
		s.saveCoinToRedis(coin)

		// Notify subscribers about timeframe reset
		s.mu.RLock()
		handlers := s.subscribers
		s.mu.RUnlock()
		for _, h := range handlers {
			h(coin)
		}
	}
}

func (s *MarketDataService) shouldReset(tf string, lastReset, now time.Time) bool {
	switch tf {
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

func (s *MarketDataService) resetTimeframe(coin *model.Coin, tf string, now time.Time) {
	price := coin.CurrentPrice
	zeroTF := model.TimeframeData{Open: price, High: price, Low: price, Trx: 0}
	switch tf {
	case "1m":
		coin.Timeframes.OneMinute = zeroTF
		coin.LastReset["1m"] = now
	case "5m":
		coin.Timeframes.FiveMinute = zeroTF
		coin.LastReset["5m"] = now
	case "15m":
		coin.Timeframes.FifteenMin = zeroTF
		coin.LastReset["15m"] = now
	case "30m":
		coin.Timeframes.ThirtyMin = zeroTF
		coin.LastReset["30m"] = now
	}
}

func parseFloat(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case string:
		f, _ := strconv.ParseFloat(val, 64)
		return f
	default:
		return 0
	}
}

func parseStringFloat(v interface{}) float64 {
	if s, ok := v.(string); ok {
		f, _ := strconv.ParseFloat(s, 64)
		return f
	}
	return parseFloat(v)
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// GetCoin retrieves coin data from Redis or Cache
func (s *MarketDataService) GetCoin(ctx context.Context, pairID string) (*model.Coin, error) {
	// Try Cache first
	if val, ok := s.coinCache.Load(pairID); ok {
		return val.(*model.Coin), nil
	}

	// Fetch from Redis
	key := redis.CoinKey(pairID)
	data, err := s.redisClient.HGetAll(ctx, key)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("coin not found")
	}

	coin, err := model.CoinFromMap(data)
	if err != nil {
		return nil, err
	}

	// Store in cache
	s.coinCache.Store(pairID, coin)
	return coin, nil
}

// GetSortedCoins retrieves a list of coins from a sorted set
func (s *MarketDataService) GetSortedCoins(ctx context.Context, sortKey string, limit int, minVolume float64, minPumpScore float64) ([]*model.Coin, error) {
	// Get pair IDs from sorted set
	stop := int64(limit - 1)
	if limit <= 0 {
		stop = -1 // Get all
	}
	pairIDs, err := s.redisClient.ZRevRange(ctx, sortKey, 0, stop)
	if err != nil {
		return nil, err
	}

	coins := make([]*model.Coin, 0, len(pairIDs))
	for _, pairID := range pairIDs {
		coin, err := s.GetCoin(ctx, pairID)
		if err != nil {
			continue
		}

		// Filter by volume: Only exclude if NO volume (VolumeIDR <= 0)
		// We trust the minVolume param from handler if provided, otherwise standard is > 0
		if coin.VolumeIDR > 0 && coin.VolumeIDR >= minVolume {
			// Filter by pump score if specified
			if minPumpScore <= 0 || coin.PumpScore >= minPumpScore {
				coins = append(coins, coin)
			}
		}
	}

	return coins, nil
}

func (s *MarketDataService) pollGapData() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.updateGapsFromREST()
	}
}

func (s *MarketDataService) updateGapsFromREST() {
	ctx := context.Background()
	summaries, err := s.restClient.GetSummaries(ctx)
	if err != nil {
		logger.Errorf("Failed to poll summaries for gaps: %v", err)
		return
	}

	updatedCount := 0

	for tickerID, detail := range summaries.Tickers {
		// Indodax summaries use btc_idr format, we use btcidr
		pairID := strings.ReplaceAll(tickerID, "_", "")

		// Optimization: only update coins we already know about or initialize if needed
		coin, _ := s.getOrCreateCoin(pairID)

		// Update bid/ask
		bestBid, _ := strconv.ParseFloat(detail.Buy, 64)
		bestAsk, _ := strconv.ParseFloat(detail.Sell, 64)
		lastPrice, _ := strconv.ParseFloat(detail.Last, 64)
		volIDR, _ := strconv.ParseFloat(detail.VolIDR, 64)
		high, _ := strconv.ParseFloat(detail.High, 64)
		low, _ := strconv.ParseFloat(detail.Low, 64)

		changed := false

		// Check Prices
		if coin.CurrentPrice != lastPrice && lastPrice > 0 {
			coin.CurrentPrice = lastPrice
			changed = true
		}
		if coin.VolumeIDR != volIDR && volIDR > 0 {
			coin.VolumeIDR = volIDR
			changed = true
		}
		if coin.High24h != high {
			coin.High24h = high
			changed = true
		}
		if coin.Low24h != low {
			coin.Low24h = low
			changed = true
		}

		// Only save and process if data is fresh or has changed
		if coin.BestBid != bestBid || coin.BestAsk != bestAsk {
			coin.BestBid = bestBid
			coin.BestAsk = bestAsk

			// Recalculate gap
			CalculateGap(coin)
			changed = true
		}

		if changed {
			coin.LastUpdate = time.Now()
			// Update Timeframes slightly (just to ensure price is tracked)
			s.updateTimeframes(coin, coin.CurrentPrice)

			// Recalculate Pump Score if volume/price changed
			coin.PumpScore = CalculatePumpScore(coin)

			// Save to Cache & Redis
			s.coinCache.Store(pairID, coin)
			go s.saveCoinToRedis(coin)

			// Notify subscribers about gap/bid/ask updates
			s.mu.RLock()
			handlers := s.subscribers
			s.mu.RUnlock()
			for _, h := range handlers {
				h(coin)
			}

			updatedCount++
		}
	}

	if updatedCount > 0 {
		logger.Infof("Updated gaps/spreads for %d pairs from REST", updatedCount)
	}
}
