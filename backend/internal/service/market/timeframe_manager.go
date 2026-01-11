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
	activePairs, err := tm.redisClient.SMembers(ctx, redis.ActivePairsKey())
	if err != nil {
		logger.Errorf("Failed to get active pairs: %v", err)
		return
	}

	// For each pair, check if timeframes need reset
	for _, pairID := range activePairs {
		tm.marketService.PerformTimeframeReset(pairID, now)
	}
}
