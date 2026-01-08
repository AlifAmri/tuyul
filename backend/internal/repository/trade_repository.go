// Package repository provides data access for the application and interacts with Redis.
package repository

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"tuyul/backend/internal/model"
	"tuyul/backend/pkg/redis"

	redislib "github.com/redis/go-redis/v9"
)

type TradeRepository struct {
	redis *redis.Client
}

func NewTradeRepository(redisClient *redis.Client) *TradeRepository {
	return &TradeRepository{
		redis: redisClient,
	}
}

// Create stores a new trade in Redis and updates indexes
func (r *TradeRepository) Create(ctx context.Context, trade *model.Trade) error {
	if trade.ID == 0 {
		// Simple ID generation for MVP if not set
		// In a real system, we'd use a UUID or a sequence from Redis
		id, err := r.redis.Incr(ctx, "sequences:trade_id")
		if err != nil {
			return err
		}
		trade.ID = id
	}

	trade.CreatedAt = time.Now()
	trade.UpdatedAt = trade.CreatedAt

	tradeIDStr := strconv.FormatInt(trade.ID, 10)
	userIDStr := trade.UserID

	// 1. Store trade object
	tradeKey := redis.TradeKey(tradeIDStr)
	err := r.redis.SetJSON(ctx, tradeKey, trade, 0)
	if err != nil {
		return err
	}

	// 2. Add to User's trades sorted set
	userTradesKey := redis.UserTradesKey(userIDStr)
	err = r.redis.ZAdd(ctx, userTradesKey, redis.Z{
		Score:  float64(trade.CreatedAt.UnixMilli()),
		Member: tradeIDStr,
	})
	if err != nil {
		return err
	}

	// 3. Add to status index
	statusKey := redis.TradesByStatusKey(trade.Status)
	err = r.redis.SAdd(ctx, statusKey, tradeIDStr)
	if err != nil {
		return err
	}

	return nil
}

// GetByID retrieves a trade from Redis
func (r *TradeRepository) GetByID(ctx context.Context, tradeID int64) (*model.Trade, error) {
	key := redis.TradeKey(strconv.FormatInt(tradeID, 10))
	var trade model.Trade
	err := r.redis.GetJSON(ctx, key, &trade)
	if err != nil {
		if err == redislib.Nil {
			return nil, fmt.Errorf("trade not found")
		}
		return nil, err
	}
	return &trade, nil
}

// Update updates an existing trade and refreshes status index if changed
func (r *TradeRepository) Update(ctx context.Context, trade *model.Trade, oldStatus string) error {
	trade.UpdatedAt = time.Now()
	tradeIDStr := strconv.FormatInt(trade.ID, 10)

	key := redis.TradeKey(tradeIDStr)
	err := r.redis.SetJSON(ctx, key, trade, 0)
	if err != nil {
		return err
	}

	if oldStatus != "" && oldStatus != trade.Status {
		// Update status indexes
		oldStatusKey := redis.TradesByStatusKey(oldStatus)
		newStatusKey := redis.TradesByStatusKey(trade.Status)

		r.redis.SRem(ctx, oldStatusKey, tradeIDStr)
		r.redis.SAdd(ctx, newStatusKey, tradeIDStr)
	}

	return nil
}

// ListByUser retrieves trades for a specific user with pagination
func (r *TradeRepository) ListByUser(ctx context.Context, userID string, offset, limit int) ([]*model.Trade, int64, error) {
	userTradesKey := redis.UserTradesKey(userID)

	total, err := r.redis.ZCard(ctx, userTradesKey)
	if err != nil {
		return nil, 0, err
	}

	// ZREVRANGE to get newest first
	memberIDs, err := r.redis.ZRevRange(ctx, userTradesKey, int64(offset), int64(offset+limit-1))
	if err != nil {
		return nil, 0, err
	}

	trades := make([]*model.Trade, 0, len(memberIDs))
	for _, idStr := range memberIDs {
		id, _ := strconv.ParseInt(idStr, 10, 64)
		trade, err := r.GetByID(ctx, id)
		if err == nil {
			trades = append(trades, trade)
		}
	}

	return trades, total, nil
}

// SetBuySellMap links a buy order ID to a sell order ID (both Indodax IDs or ours?)
// Blueprints say Indodax Order ID.
func (r *TradeRepository) SetBuySellMap(ctx context.Context, buyOrderID, sellOrderID string) error {
	key := redis.BuySellMapKey(buyOrderID)
	return r.redis.Set(ctx, key, sellOrderID, 0)
}

func (r *TradeRepository) GetSellOrderID(ctx context.Context, buyOrderID string) (string, error) {
	key := redis.BuySellMapKey(buyOrderID)
	return r.redis.Get(ctx, key)
}
