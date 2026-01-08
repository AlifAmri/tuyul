package repository

import (
	"context"
	"fmt"

	"tuyul/backend/pkg/redis"
)

type BalanceRepository struct {
	redis *redis.Client
}

func NewBalanceRepository(redisClient *redis.Client) *BalanceRepository {
	return &BalanceRepository{
		redis: redisClient,
	}
}

// GetUserPaperBalances retrieves virtual balances for a user's manual/copilot trades
func (r *BalanceRepository) GetUserPaperBalances(ctx context.Context, userID string) (map[string]float64, error) {
	key := redis.PaperBalanceKey(userID)
	var balances map[string]float64
	err := r.redis.GetJSON(ctx, key, &balances)
	if err != nil {
		// Initialize default balance if not found: 100M IDR
		balances = map[string]float64{
			"idr": 100000000,
		}
		return balances, nil
	}
	return balances, nil
}

// SaveUserPaperBalances saves virtual balances for a user's manual/copilot trades
func (r *BalanceRepository) SaveUserPaperBalances(ctx context.Context, userID string, balances map[string]float64) error {
	key := redis.PaperBalanceKey(userID)
	return r.redis.SetJSON(ctx, key, balances, 0)
}

// GetBotPaperBalances retrieves virtual balances for a specific bot instance
func (r *BalanceRepository) GetBotPaperBalances(ctx context.Context, botID int64) (map[string]float64, error) {
	key := redis.BotPaperBalanceKey(botID)
	var balances map[string]float64
	err := r.redis.GetJSON(ctx, key, &balances)
	if err != nil {
		return nil, fmt.Errorf("bot balances not found: %w", err)
	}
	return balances, nil
}

// SaveBotPaperBalances saves virtual balances for a specific bot instance
func (r *BalanceRepository) SaveBotPaperBalances(ctx context.Context, botID int64, balances map[string]float64) error {
	key := redis.BotPaperBalanceKey(botID)
	return r.redis.SetJSON(ctx, key, balances, 0)
}
