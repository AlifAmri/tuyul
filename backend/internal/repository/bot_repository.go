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

type BotRepository struct {
	redis *redis.Client
}

func NewBotRepository(redisClient *redis.Client) *BotRepository {
	return &BotRepository{
		redis: redisClient,
	}
}

// Create creates a new bot configuration
func (r *BotRepository) Create(ctx context.Context, bot *model.BotConfig) error {
	if bot.ID == 0 {
		id, err := r.redis.Incr(ctx, "sequences:bot_id")
		if err != nil {
			return err
		}
		bot.ID = id
	}

	bot.CreatedAt = time.Now()
	bot.UpdatedAt = bot.CreatedAt
	bot.Status = model.BotStatusStopped

	// Initialize balances if nil
	if bot.Balances == nil {
		bot.Balances = map[string]float64{
			"idr": bot.InitialBalanceIDR,
		}
	}

	botIDStr := strconv.FormatInt(bot.ID, 10)
	userIDStr := bot.UserID

	// Store bot object (excluding balances from the main hash for standardization if desired,
	// but we'll keep it as part of the model and just ensure it's saved to the dedicated key too)
	botKey := redis.BotKey(botIDStr)
	err := r.redis.SetJSON(ctx, botKey, bot, 0)
	if err != nil {
		return err
	}

	// Store balances in dedicated paper balance key
	err = r.redis.SetJSON(ctx, redis.BotPaperBalanceKey(bot.ID), bot.Balances, 0)
	if err != nil {
		return err
	}

	// Add to user's bots set
	userBotsKey := redis.UserBotsKey(userIDStr)
	err = r.redis.SAdd(ctx, userBotsKey, botIDStr)
	if err != nil {
		return err
	}

	// Add to type index
	typeKey := redis.BotsByTypeKey(bot.Type)
	err = r.redis.SAdd(ctx, typeKey, botIDStr)
	if err != nil {
		return err
	}

	// Add to status index
	statusKey := redis.BotsByStatusKey(bot.Status)
	err = r.redis.SAdd(ctx, statusKey, botIDStr)
	if err != nil {
		return err
	}

	return nil
}

// GetByID retrieves a bot by ID
func (r *BotRepository) GetByID(ctx context.Context, botID int64) (*model.BotConfig, error) {
	key := redis.BotKey(strconv.FormatInt(botID, 10))
	var bot model.BotConfig
	err := r.redis.GetJSON(ctx, key, &bot)
	if err != nil {
		if err == redislib.Nil {
			return nil, fmt.Errorf("bot not found")
		}
		return nil, err
	}

	// Load balances from dedicated paper balance key
	var balances map[string]float64
	err = r.redis.GetJSON(ctx, redis.BotPaperBalanceKey(botID), &balances)
	if err == nil {
		bot.Balances = balances
	}

	return &bot, nil
}

// Update updates a bot configuration
func (r *BotRepository) Update(ctx context.Context, bot *model.BotConfig, oldStatus string) error {
	bot.UpdatedAt = time.Now()
	botIDStr := strconv.FormatInt(bot.ID, 10)

	key := redis.BotKey(botIDStr)
	err := r.redis.SetJSON(ctx, key, bot, 0)
	if err != nil {
		return err
	}

	// Update balances in dedicated paper balance key
	if bot.Balances != nil {
		err = r.redis.SetJSON(ctx, redis.BotPaperBalanceKey(bot.ID), bot.Balances, 0)
		if err != nil {
			return err
		}
	}

	// Update status index if changed
	if oldStatus != "" && oldStatus != bot.Status {
		oldStatusKey := redis.BotsByStatusKey(oldStatus)
		newStatusKey := redis.BotsByStatusKey(bot.Status)

		r.redis.SRem(ctx, oldStatusKey, botIDStr)
		r.redis.SAdd(ctx, newStatusKey, botIDStr)
	}

	return nil
}

// Delete deletes a bot configuration
func (r *BotRepository) Delete(ctx context.Context, botID int64) error {
	bot, err := r.GetByID(ctx, botID)
	if err != nil {
		return err
	}

	botIDStr := strconv.FormatInt(botID, 10)
	userIDStr := bot.UserID

	// Remove from Redis
	botKey := redis.BotKey(botIDStr)
	err = r.redis.Del(ctx, botKey)
	if err != nil {
		return err
	}

	// Remove balances
	r.redis.Del(ctx, redis.BotPaperBalanceKey(botID))

	// Remove from user's bots
	userBotsKey := redis.UserBotsKey(userIDStr)
	r.redis.SRem(ctx, userBotsKey, botIDStr)

	// Remove from type index
	typeKey := redis.BotsByTypeKey(bot.Type)
	r.redis.SRem(ctx, typeKey, botIDStr)

	// Remove from status index
	statusKey := redis.BotsByStatusKey(bot.Status)
	r.redis.SRem(ctx, statusKey, botIDStr)

	return nil
}

// ListByUser retrieves all bots for a user
func (r *BotRepository) ListByUser(ctx context.Context, userID string) ([]*model.BotConfig, error) {
	userBotsKey := redis.UserBotsKey(userID)

	botIDs, err := r.redis.SMembers(ctx, userBotsKey)
	if err != nil {
		return nil, err
	}

	bots := make([]*model.BotConfig, 0, len(botIDs))
	for _, idStr := range botIDs {
		id, _ := strconv.ParseInt(idStr, 10, 64)
		bot, err := r.GetByID(ctx, id)
		if err == nil {
			bots = append(bots, bot)
		}
	}

	return bots, nil
}

// ListByStatus retrieves all bots with a specific status
func (r *BotRepository) ListByStatus(ctx context.Context, status string) ([]*model.BotConfig, error) {
	statusKey := redis.BotsByStatusKey(status)

	botIDs, err := r.redis.SMembers(ctx, statusKey)
	if err != nil {
		return nil, err
	}

	bots := make([]*model.BotConfig, 0, len(botIDs))
	for _, idStr := range botIDs {
		id, _ := strconv.ParseInt(idStr, 10, 64)
		bot, err := r.GetByID(ctx, id)
		if err == nil {
			bots = append(bots, bot)
		}
	}

	return bots, nil
}

// UpdateBalance updates the bot's balance
func (r *BotRepository) UpdateBalance(ctx context.Context, botID int64, balances map[string]float64) error {
	bot, err := r.GetByID(ctx, botID)
	if err != nil {
		return err
	}

	bot.Balances = balances
	bot.UpdatedAt = time.Now()

	return r.Update(ctx, bot, "")
}

// UpdateStats updates bot statistics
func (r *BotRepository) UpdateStats(ctx context.Context, botID int64, totalTrades, winningTrades int, totalProfit float64) error {
	bot, err := r.GetByID(ctx, botID)
	if err != nil {
		return err
	}

	bot.TotalTrades = totalTrades
	bot.WinningTrades = winningTrades
	bot.TotalProfitIDR = totalProfit
	bot.UpdatedAt = time.Now()

	return r.Update(ctx, bot, "")
}

// UpdateStatus updates bot status
func (r *BotRepository) UpdateStatus(ctx context.Context, botID int64, status string, errorMsg *string) error {
	bot, err := r.GetByID(ctx, botID)
	if err != nil {
		return err
	}

	oldStatus := bot.Status
	bot.Status = status
	bot.ErrorMessage = errorMsg
	bot.UpdatedAt = time.Now()

	return r.Update(ctx, bot, oldStatus)
}
