package market

import (
	"context"
	"time"

	"tuyul/backend/pkg/logger"

	"tuyul/backend/pkg/redis"
)

type TimeframeManager struct {
	marketService *MarketDataService
	redisClient   *redis.Client
}

func NewTimeframeManager(marketService *MarketDataService, redisClient *redis.Client) *TimeframeManager {
	return &TimeframeManager{
		marketService: marketService,
		redisClient:   redisClient,
	}
}

func (tm *TimeframeManager) Start() {
	// Check every 10 seconds to align reasonably well with minute boundaries
	ticker := time.NewTicker(10 * time.Second)
	go func() {
		for range ticker.C {
			tm.checkAndResetTimeframes()
		}
	}()
}

func (tm *TimeframeManager) checkAndResetTimeframes() {
	ctx := context.Background()
	now := time.Now()

	// Get active pairs
	// We use the redis client to get active pairs
	activePairs, err := tm.redisClient.SMembers(ctx, "market:active_pairs")
	if err != nil {
		logger.Errorf("Failed to get active pairs: %v", err)
		return
	}

	// For each pair, check if timeframes need reset
	// Note: Fetching and saving every coin every 10s might be heavy if there are many coins.
	// Optimization: Only check coins that are "active" in memory if possible, or batched.
	// For now, assume we can iterate active pairs (usually ~100-200 on Indodax).

	for _, pairID := range activePairs {
		// Get coin from memory cache first to avoid redis hit if possible,
		// but TimeframeManager is separate, it should probably sync.
		// For simplicity, let's assume we rely on the implementation in MarketDataService
		// to hold state or fetch from redis.

		// Accessing private cache? Ideally MarketDataService exposes a method.
		// Let's modify MarketDataService to expose Coin lookup or handle reset itself.
		// But keeping separation of concerns: TimeframeManager manages the schedule.

		// In a real high-perf app, this would be event-driven or sharded.
		// Here, we'll ask MarketDataService to process reset for a pair.
		tm.marketService.PerformTimeframeReset(pairID, now)
	}
}
