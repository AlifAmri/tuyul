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

type PositionRepository struct {
	redis *redis.Client
}

func NewPositionRepository(redisClient *redis.Client) *PositionRepository {
	return &PositionRepository{
		redis: redisClient,
	}
}

// Create creates a new position
func (r *PositionRepository) Create(ctx context.Context, pos *model.Position) error {
	if pos.ID == 0 {
		id, err := r.redis.Incr(ctx, "sequences:position_id")
		if err != nil {
			return err
		}
		pos.ID = id
	}

	pos.CreatedAt = time.Now()
	pos.UpdatedAt = pos.CreatedAt

	posIDStr := strconv.FormatInt(pos.ID, 10)
	botIDStr := strconv.FormatInt(pos.BotConfigID, 10)

	// Store position object
	key := redis.PositionKey(posIDStr)
	err := r.redis.SetJSON(ctx, key, pos, 0)
	if err != nil {
		return err
	}

	// Add to bot's positions set (all positions)
	botPositionsKey := redis.BotPositionsKey(botIDStr)
	err = r.redis.SAdd(ctx, botPositionsKey, posIDStr)
	if err != nil {
		return err
	}

	// Add to active positions if not closed
	if pos.Status != model.PositionStatusClosed {
		activeKey := redis.ActivePositionsKey(botIDStr)
		err = r.redis.SAdd(ctx, activeKey, posIDStr)
		if err != nil {
			return err
		}
	}

	return nil
}

// GetByID retrieves a position by ID
func (r *PositionRepository) GetByID(ctx context.Context, posID int64) (*model.Position, error) {
	key := redis.PositionKey(strconv.FormatInt(posID, 10))
	var pos model.Position
	err := r.redis.GetJSON(ctx, key, &pos)
	if err != nil {
		if err == redislib.Nil {
			return nil, fmt.Errorf("position not found")
		}
		return nil, err
	}
	return &pos, nil
}

// Update updates a position
func (r *PositionRepository) Update(ctx context.Context, pos *model.Position) error {
	pos.UpdatedAt = time.Now()
	posIDStr := strconv.FormatInt(pos.ID, 10)
	botIDStr := strconv.FormatInt(pos.BotConfigID, 10)

	key := redis.PositionKey(posIDStr)
	err := r.redis.SetJSON(ctx, key, pos, 0)
	if err != nil {
		return err
	}

	// Manage active positions set
	activeKey := redis.ActivePositionsKey(botIDStr)
	if pos.Status == model.PositionStatusClosed {
		r.redis.SRem(ctx, activeKey, posIDStr)
	} else {
		r.redis.SAdd(ctx, activeKey, posIDStr)
	}

	return nil
}

// ListByBot retrieves all positions for a bot
func (r *PositionRepository) ListByBot(ctx context.Context, botID int64) ([]*model.Position, error) {
	botPositionsKey := redis.BotPositionsKey(strconv.FormatInt(botID, 10))

	IDs, err := r.redis.SMembers(ctx, botPositionsKey)
	if err != nil {
		return nil, err
	}

	positions := make([]*model.Position, 0, len(IDs))
	for _, idStr := range IDs {
		id, _ := strconv.ParseInt(idStr, 10, 64)
		pos, err := r.GetByID(ctx, id)
		if err == nil {
			positions = append(positions, pos)
		}
	}

	return positions, nil
}

// ListActiveByBot retrieves active positions for a bot
func (r *PositionRepository) ListActiveByBot(ctx context.Context, botID int64) ([]*model.Position, error) {
	activeKey := redis.ActivePositionsKey(strconv.FormatInt(botID, 10))

	IDs, err := r.redis.SMembers(ctx, activeKey)
	if err != nil {
		return nil, err
	}

	positions := make([]*model.Position, 0, len(IDs))
	for _, idStr := range IDs {
		id, _ := strconv.ParseInt(idStr, 10, 64)
		pos, err := r.GetByID(ctx, id)
		if err == nil {
			positions = append(positions, pos)
		}
	}

	return positions, nil
}

// UpdateStatus updates position status
func (r *PositionRepository) UpdateStatus(ctx context.Context, posID int64, status string) error {
	pos, err := r.GetByID(ctx, posID)
	if err != nil {
		return err
	}

	pos.Status = status
	return r.Update(ctx, pos)
}

// UpdatePriceTracking updates highest and lowest price seen
func (r *PositionRepository) UpdatePriceTracking(ctx context.Context, posID int64, highest, lowest float64) error {
	pos, err := r.GetByID(ctx, posID)
	if err != nil {
		return err
	}

	pos.HighestPrice = highest
	pos.LowestPrice = lowest
	return r.Update(ctx, pos)
}

// Delete deletes a position
func (r *PositionRepository) Delete(ctx context.Context, posID int64) error {
	pos, err := r.GetByID(ctx, posID)
	if err != nil {
		return err
	}

	posIDStr := strconv.FormatInt(posID, 10)
	botIDStr := strconv.FormatInt(pos.BotConfigID, 10)

	// Remove position object
	key := redis.PositionKey(posIDStr)
	err = r.redis.Del(ctx, key)
	if err != nil {
		return err
	}

	// Remove from bot's positions set
	botPositionsKey := redis.BotPositionsKey(botIDStr)
	r.redis.SRem(ctx, botPositionsKey, posIDStr)

	// Remove from active positions set (if it was active)
	activeKey := redis.ActivePositionsKey(botIDStr)
	r.redis.SRem(ctx, activeKey, posIDStr)

	return nil
}
