// Package repository provides data access for the application and interacts with Redis.
package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"

	"tuyul/backend/internal/model"
	"tuyul/backend/pkg/redis"

	redislib "github.com/redis/go-redis/v9"
)

type OrderRepository struct {
	redis *redis.Client
}

func NewOrderRepository(redisClient *redis.Client) *OrderRepository {
	return &OrderRepository{
		redis: redisClient,
	}
}

// GetRedisClient returns the underlying Redis client (for pipeline operations)
func (r *OrderRepository) GetRedisClient() *redis.Client {
	return r.redis
}

// Create creates a new order
func (r *OrderRepository) Create(ctx context.Context, order *model.Order) error {
	if order.ID == 0 {
		id, err := r.redis.Incr(ctx, "sequences:order_id")
		if err != nil {
			return err
		}
		order.ID = id
	}

	order.CreatedAt = time.Now()
	order.UpdatedAt = order.CreatedAt

	orderIDStr := strconv.FormatInt(order.ID, 10)
	userIDStr := order.UserID

	// Store order object
	orderKey := redis.OrderKey(orderIDStr)
	err := r.redis.SetJSON(ctx, orderKey, order, 0)
	if err != nil {
		return err
	}

	// Add to user's orders set
	userOrdersKey := redis.UserOrdersKey(userIDStr)
	err = r.redis.SAdd(ctx, userOrdersKey, orderIDStr)
	if err != nil {
		return err
	}

	// Add to status index
	statusKey := redis.OrdersByStatusKey(order.Status)
	err = r.redis.SAdd(ctx, statusKey, orderIDStr)
	if err != nil {
		return err
	}

	// Add to pair index
	pairKey := redis.PairOrdersKey(order.Pair)
	err = r.redis.SAdd(ctx, pairKey, orderIDStr)
	if err != nil {
		return err
	}

	// Add to Indodax Order ID map index
	if order.OrderID != "" {
		mapKey := redis.OrderIDMapKey(order.OrderID)
		err = r.redis.Set(ctx, mapKey, orderIDStr, 0)
		if err != nil {
			return err
		}
	}

	// Add to parent index (sorted set by CreatedAt timestamp for efficient querying)
	createdAtScore := float64(order.CreatedAt.UnixMilli())
	if order.ParentType == "bot" {
		botOrdersKey := redis.BotOrdersKey(order.ParentID)
		err := r.redis.ZAdd(ctx, botOrdersKey, redis.Z{
			Score:  createdAtScore,
			Member: orderIDStr,
		})
		if err != nil {
			return err
		}
	} else if order.ParentType == "position" {
		posOrdersKey := redis.PositionOrdersKey(order.ParentID)
		err := r.redis.ZAdd(ctx, posOrdersKey, redis.Z{
			Score:  createdAtScore,
			Member: orderIDStr,
		})
		if err != nil {
			return err
		}
	}

	return nil
}

// GetByID retrieves an order by ID
func (r *OrderRepository) GetByID(ctx context.Context, orderID int64) (*model.Order, error) {
	key := redis.OrderKey(strconv.FormatInt(orderID, 10))
	var order model.Order
	err := r.redis.GetJSON(ctx, key, &order)
	if err != nil {
		if err == redislib.Nil {
			return nil, fmt.Errorf("order not found")
		}
		return nil, err
	}
	return &order, nil
}

// GetByOrderID retrieves an order by Indodax order ID
func (r *OrderRepository) GetByOrderID(ctx context.Context, indodaxOrderID string) (*model.Order, error) {
	mapKey := redis.OrderIDMapKey(indodaxOrderID)
	internalIDStr, err := r.redis.Get(ctx, mapKey)
	if err != nil {
		if err == redislib.Nil {
			return nil, fmt.Errorf("order mapping not found")
		}
		return nil, err
	}

	internalID, err := strconv.ParseInt(internalIDStr, 10, 64)
	if err != nil {
		return nil, err
	}

	return r.GetByID(ctx, internalID)
}

// Update updates an order
func (r *OrderRepository) Update(ctx context.Context, order *model.Order, oldStatus string) error {
	order.UpdatedAt = time.Now()
	orderIDStr := strconv.FormatInt(order.ID, 10)

	key := redis.OrderKey(orderIDStr)
	err := r.redis.SetJSON(ctx, key, order, 0)
	if err != nil {
		return err
	}

	// Update status index if changed
	if oldStatus != "" && oldStatus != order.Status {
		oldStatusKey := redis.OrdersByStatusKey(oldStatus)
		newStatusKey := redis.OrdersByStatusKey(order.Status)

		r.redis.SRem(ctx, oldStatusKey, orderIDStr)
		r.redis.SAdd(ctx, newStatusKey, orderIDStr)
	}

	return nil
}

// UpdateStatus updates order status
func (r *OrderRepository) UpdateStatus(ctx context.Context, orderID int64, status string) error {
	order, err := r.GetByID(ctx, orderID)
	if err != nil {
		return err
	}

	oldStatus := order.Status
	order.Status = status

	if status == "filled" {
		now := time.Now()
		order.FilledAt = &now
		order.FilledAmount = order.Amount
	}

	return r.Update(ctx, order, oldStatus)
}

// ListByUser retrieves all orders for a user
func (r *OrderRepository) ListByUser(ctx context.Context, userID string, limit int) ([]*model.Order, error) {
	userOrdersKey := redis.UserOrdersKey(userID)

	orderIDs, err := r.redis.SMembers(ctx, userOrdersKey)
	if err != nil {
		return nil, err
	}

	// Limit results
	if limit > 0 && len(orderIDs) > limit {
		orderIDs = orderIDs[:limit]
	}

	orders := make([]*model.Order, 0, len(orderIDs))
	for _, idStr := range orderIDs {
		id, _ := strconv.ParseInt(idStr, 10, 64)
		order, err := r.GetByID(ctx, id)
		if err == nil {
			orders = append(orders, order)
		}
	}

	return orders, nil
}

// ListByStatus retrieves all orders with a specific status
func (r *OrderRepository) ListByStatus(ctx context.Context, status string) ([]*model.Order, error) {
	statusKey := redis.OrdersByStatusKey(status)

	orderIDs, err := r.redis.SMembers(ctx, statusKey)
	if err != nil {
		return nil, err
	}

	orders := make([]*model.Order, 0, len(orderIDs))
	for _, idStr := range orderIDs {
		id, _ := strconv.ParseInt(idStr, 10, 64)
		order, err := r.GetByID(ctx, id)
		if err == nil {
			orders = append(orders, order)
		}
	}

	return orders, nil
}

// ListByParent retrieves all orders for a specific parent (bot or position)
func (r *OrderRepository) ListByParent(ctx context.Context, parentType string, parentID int64) ([]*model.Order, error) {
	// Get all orders for the user first (we need to scan)
	// Since we don't have a parent index, we'll filter by parent
	// For now, we'll get user's orders and filter
	// TODO: Add parent index for better performance if needed

	// This is not optimal - ideally we'd have a parent index
	// But for now we'll iterate through user's orders
	// We need userID - let's change approach

	// Better approach: Get order IDs from a pattern scan
	// But Redis doesn't allow complex queries easily

	// Alternative: Return empty for now and build index on write
	// For this implementation, we'll scan user orders
	// Since we have the parent, let's get the user first

	return nil, fmt.Errorf("not implemented - need user context")
}

// ListByParentAndUser retrieves orders by parent and user
// If limit > 0, returns only the most recent 'limit' orders
func (r *OrderRepository) ListByParentAndUser(ctx context.Context, userID string, parentType string, parentID int64, limit int) ([]*model.Order, error) {
	var orderIDs []string
	var err error

	// Use sorted set index if available (much faster for limited queries)
	if parentType == "bot" {
		botOrdersKey := redis.BotOrdersKey(parentID)
		// Get most recent orders (highest score = most recent timestamp)
		// ZRevRange gets highest scores first (most recent)
		orderIDs, err = r.redis.ZRevRange(ctx, botOrdersKey, 0, int64(limit-1))
		if err != nil {
			return nil, err
		}
	} else if parentType == "position" {
		posOrdersKey := redis.PositionOrdersKey(parentID)
		orderIDs, err = r.redis.ZRevRange(ctx, posOrdersKey, 0, int64(limit-1))
		if err != nil {
			return nil, err
		}
	}

	// Fallback: if sorted set is empty or doesn't exist, use old method
	if len(orderIDs) == 0 {
		userOrdersKey := redis.UserOrdersKey(userID)
		allOrderIDs, err := r.redis.SMembers(ctx, userOrdersKey)
		if err != nil {
			return nil, err
		}

		if len(allOrderIDs) == 0 {
			return []*model.Order{}, nil
		}

		// Use pipeline to batch fetch all orders at once
		pipe := r.redis.GetClient().Pipeline()
		cmds := make(map[string]*redislib.StringCmd)
		
		for _, idStr := range allOrderIDs {
			orderKey := redis.OrderKey(idStr)
			cmds[idStr] = pipe.Get(ctx, orderKey)
		}
		
		_, err = pipe.Exec(ctx)
		if err != nil && err != redislib.Nil {
			return nil, err
		}

		// Parse results and filter by parent
		orders := make([]*model.Order, 0)
		for _, idStr := range allOrderIDs {
			cmd := cmds[idStr]
			if cmd == nil {
				continue
			}
			
			val, err := cmd.Result()
			if err != nil {
				if err == redislib.Nil {
					continue
				}
				return nil, err
			}
			
			var order model.Order
			if err := json.Unmarshal([]byte(val), &order); err != nil {
				continue
			}
			
			if order.ParentType == parentType && order.ParentID == parentID {
				orders = append(orders, &order)
			}
		}

		// Sort and limit
		if limit > 0 && len(orders) > limit {
			sort.Slice(orders, func(i, j int) bool {
				if orders[i].CreatedAt.Equal(orders[j].CreatedAt) {
					return orders[i].ID > orders[j].ID
				}
				return orders[i].CreatedAt.After(orders[j].CreatedAt)
			})
			orders = orders[:limit]
		}

		return orders, nil
	}

	// Fetch orders using pipeline (only the ones we need)
	if len(orderIDs) == 0 {
		return []*model.Order{}, nil
	}

	pipe := r.redis.GetClient().Pipeline()
	cmds := make(map[string]*redislib.StringCmd)
	
	for _, idStr := range orderIDs {
		orderKey := redis.OrderKey(idStr)
		cmds[idStr] = pipe.Get(ctx, orderKey)
	}
	
	_, err = pipe.Exec(ctx)
	if err != nil && err != redislib.Nil {
		return nil, err
	}

	// Parse results
	orders := make([]*model.Order, 0, len(orderIDs))
	for _, idStr := range orderIDs {
		cmd := cmds[idStr]
		if cmd == nil {
			continue
		}
		
		val, err := cmd.Result()
		if err != nil {
			if err == redislib.Nil {
				continue
			}
			return nil, err
		}
		
		var order model.Order
		if err := json.Unmarshal([]byte(val), &order); err != nil {
			continue
		}
		
		orders = append(orders, &order)
	}

	return orders, nil
}

// Delete deletes an order
func (r *OrderRepository) Delete(ctx context.Context, orderID int64) error {
	order, err := r.GetByID(ctx, orderID)
	if err != nil {
		return err
	}

	orderIDStr := strconv.FormatInt(orderID, 10)
	userIDStr := order.UserID

	// Remove from Redis
	orderKey := redis.OrderKey(orderIDStr)
	err = r.redis.Del(ctx, orderKey)
	if err != nil {
		return err
	}

	// Remove from user's orders
	userOrdersKey := redis.UserOrdersKey(userIDStr)
	r.redis.SRem(ctx, userOrdersKey, orderIDStr)

	// Remove from status index
	statusKey := redis.OrdersByStatusKey(order.Status)
	r.redis.SRem(ctx, statusKey, orderIDStr)

	// Remove from pair index
	pairKey := redis.PairOrdersKey(order.Pair)
	r.redis.SRem(ctx, pairKey, orderIDStr)

	// Remove from Indodax Order ID map index
	if order.OrderID != "" {
		mapKey := redis.OrderIDMapKey(order.OrderID)
		r.redis.Del(ctx, mapKey)
	}

	// Remove from parent sorted set index
	if order.ParentType == "bot" {
		botOrdersKey := redis.BotOrdersKey(order.ParentID)
		r.redis.ZRem(ctx, botOrdersKey, orderIDStr)
	} else if order.ParentType == "position" {
		posOrdersKey := redis.PositionOrdersKey(order.ParentID)
		r.redis.ZRem(ctx, posOrdersKey, orderIDStr)
	}

	return nil
}
