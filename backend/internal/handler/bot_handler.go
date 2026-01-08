package handler

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"time"

	"tuyul/backend/internal/model"
	"tuyul/backend/internal/repository"
	"tuyul/backend/internal/service"
	"tuyul/backend/internal/util"
	"tuyul/backend/pkg/redis"

	"github.com/gin-gonic/gin"
	redislib "github.com/redis/go-redis/v9"
)

type BotHandler struct {
	botRepo   *repository.BotRepository
	orderRepo *repository.OrderRepository
	mmService *service.MarketMakerService
	phService *service.PumpHunterService
}

func NewBotHandler(botRepo *repository.BotRepository, orderRepo *repository.OrderRepository, mmService *service.MarketMakerService, phService *service.PumpHunterService) *BotHandler {
	return &BotHandler{
		botRepo:   botRepo,
		orderRepo: orderRepo,
		mmService: mmService,
		phService: phService,
	}
}

// CreateBot handles POST /api/v1/bots
func (h *BotHandler) CreateBot(c *gin.Context) {
	var req model.BotConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.SendValidationError(c, err.Error())
		return
	}

	userID, exists := c.Get("user_id")
	if !exists {
		util.SendError(c, util.ErrUnauthorized("User not authenticated"))
		return
	}

	var bot *model.BotConfig
	var err error

	switch req.Type {
	case model.BotTypeMarketMaker:
		bot, err = h.mmService.CreateBot(c.Request.Context(), userID.(string), &req)
	case model.BotTypePumpHunter:
		bot, err = h.phService.CreateBot(c.Request.Context(), userID.(string), &req)
	default:
		err = util.ErrBadRequest("Unsupported bot type")
	}

	if err != nil {
		util.SendError(c, err)
		return
	}

	util.SendCreated(c, bot, "Bot created successfully")
}

// UpdateBot handles PUT /api/v1/bots/:id
func (h *BotHandler) UpdateBot(c *gin.Context) {
	var req model.BotConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.SendValidationError(c, err.Error())
		return
	}

	userID, exists := c.Get("user_id")
	if !exists {
		util.SendError(c, util.ErrUnauthorized("User not authenticated"))
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		util.SendError(c, util.ErrBadRequest("Invalid bot ID"))
		return
	}

	var bot *model.BotConfig
	var updateErr error

	switch req.Type {
	case model.BotTypeMarketMaker:
		bot, updateErr = h.mmService.UpdateBot(c.Request.Context(), userID.(string), id, &req)
	case model.BotTypePumpHunter:
		bot, updateErr = h.phService.UpdateBot(c.Request.Context(), userID.(string), id, &req)
	default:
		updateErr = util.ErrBadRequest("Unsupported bot type")
	}

	if updateErr != nil {
		util.SendError(c, updateErr)
		return
	}

	util.SendSuccess(c, bot)
}

// ListBots handles GET /api/v1/bots
func (h *BotHandler) ListBots(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		util.SendError(c, util.ErrUnauthorized("User not authenticated"))
		return
	}

	bots, err := h.botRepo.ListByUser(c.Request.Context(), userID.(string))
	if err != nil {
		util.SendError(c, err)
		return
	}

	util.SendSuccess(c, bots)
}

// GetBot handles GET /api/v1/bots/:id
func (h *BotHandler) GetBot(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		util.SendError(c, util.ErrUnauthorized("User not authenticated"))
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		util.SendError(c, util.ErrBadRequest("Invalid bot ID"))
		return
	}

	bot, err := h.botRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		util.SendError(c, err)
		return
	}

	if bot.UserID != userID.(string) {
		util.SendError(c, util.ErrForbidden("Access denied"))
		return
	}

	util.SendSuccess(c, bot)
}

// GetBotSummary handles GET /api/v1/bots/:id/summary
func (h *BotHandler) GetBotSummary(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		util.SendError(c, util.ErrUnauthorized("User not authenticated"))
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		util.SendError(c, util.ErrBadRequest("Invalid bot ID"))
		return
	}

	bot, err := h.botRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		util.SendError(c, err)
		return
	}

	if bot.UserID != userID.(string) {
		util.SendError(c, util.ErrForbidden("Access denied"))
		return
	}

	summary := model.BotSummary{
		BotID:          bot.ID,
		Type:           bot.Type,
		Status:         bot.Status,
		Pair:           bot.Pair,
		TotalTrades:    bot.TotalTrades,
		WinningTrades:  bot.WinningTrades,
		LosingTrades:   bot.TotalTrades - bot.WinningTrades,
		TotalProfitIDR: bot.TotalProfitIDR,
	}

	if bot.TotalTrades > 0 {
		summary.WinRate = float64(bot.WinningTrades) / float64(bot.TotalTrades) * 100
		summary.AverageProfit = bot.TotalProfitIDR / float64(bot.TotalTrades)
	}

	// Calculate uptime if running
	if bot.Status == model.BotStatusRunning && bot.UpdatedAt.IsZero() == false {
		summary.Uptime = time.Since(bot.UpdatedAt).String()
	}

	util.SendSuccess(c, summary)
}

// DeleteBot handles DELETE /api/v1/bots/:id
func (h *BotHandler) DeleteBot(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		util.SendError(c, util.ErrUnauthorized("User not authenticated"))
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		util.SendError(c, util.ErrBadRequest("Invalid bot ID"))
		return
	}

	bot, err := h.botRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		util.SendError(c, err)
		return
	}

	if bot.UserID != userID.(string) {
		util.SendError(c, util.ErrForbidden("Access denied"))
		return
	}

	var deleteErr error
	switch bot.Type {
	case model.BotTypeMarketMaker:
		deleteErr = h.mmService.DeleteBot(c.Request.Context(), userID.(string), id)
	case model.BotTypePumpHunter:
		deleteErr = h.phService.DeleteBot(c.Request.Context(), userID.(string), id)
	default:
		deleteErr = h.botRepo.Delete(c.Request.Context(), id)
	}

	if deleteErr != nil {
		util.SendError(c, deleteErr)
		return
	}

	util.SendSuccess(c, gin.H{"message": "Bot deleted successfully"})
}

// StartBot handles POST /api/v1/bots/:id/start
func (h *BotHandler) StartBot(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		util.SendError(c, util.ErrUnauthorized("User not authenticated"))
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		util.SendError(c, util.ErrBadRequest("Invalid bot ID"))
		return
	}

	bot, err := h.botRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		util.SendError(c, err)
		return
	}

	var startErr error
	switch bot.Type {
	case model.BotTypeMarketMaker:
		startErr = h.mmService.StartBot(c.Request.Context(), userID.(string), id)
	case model.BotTypePumpHunter:
		startErr = h.phService.StartBot(c.Request.Context(), userID.(string), id)
	default:
		startErr = util.ErrBadRequest("Unsupported bot type")
	}

	if startErr != nil {
		util.SendError(c, startErr)
		return
	}

	util.SendSuccess(c, gin.H{"message": "Bot started successfully"})
}

// StopBot handles POST /api/v1/bots/:id/stop
func (h *BotHandler) StopBot(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		util.SendError(c, util.ErrUnauthorized("User not authenticated"))
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		util.SendError(c, util.ErrBadRequest("Invalid bot ID"))
		return
	}

	bot, err := h.botRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		util.SendError(c, err)
		return
	}

	var stopErr error
	switch bot.Type {
	case model.BotTypeMarketMaker:
		stopErr = h.mmService.StopBot(c.Request.Context(), userID.(string), id)
	case model.BotTypePumpHunter:
		stopErr = h.phService.StopBot(c.Request.Context(), userID.(string), id)
	default:
		stopErr = util.ErrBadRequest("Unsupported bot type")
	}

	if stopErr != nil {
		util.SendError(c, stopErr)
		return
	}
	util.SendSuccess(c, gin.H{"message": "Bot stopped successfully"})
}

// ListPositions handles GET /api/v1/bots/:id/positions
func (h *BotHandler) ListPositions(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		util.SendError(c, util.ErrUnauthorized("User not authenticated"))
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		util.SendError(c, util.ErrBadRequest("Invalid bot ID"))
		return
	}

	bot, err := h.botRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		util.SendError(c, err)
		return
	}

	if bot.UserID != userID.(string) {
		util.SendError(c, util.ErrForbidden("Access denied"))
		return
	}

	positions, err := h.phService.ListPositions(c.Request.Context(), userID.(string), id)
	if err != nil {
		util.SendError(c, err)
		return
	}

	util.SendSuccess(c, positions)
}

// ListOrders handles GET /api/v1/bots/:id/orders
func (h *BotHandler) ListOrders(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		util.SendError(c, util.ErrUnauthorized("User not authenticated"))
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		util.SendError(c, util.ErrBadRequest("Invalid bot ID"))
		return
	}

	// Verify bot ownership
	bot, err := h.botRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		util.SendError(c, err)
		return
	}

	if bot.UserID != userID.(string) {
		util.SendError(c, util.ErrForbidden("Access denied"))
		return
	}

	// For Market Maker bots: get orders with ParentType="bot"
	// For Pump Hunter bots: get orders with ParentType="position" for all positions
	var orders []*model.Order

	if bot.Type == model.BotTypeMarketMaker {
		// Direct bot orders - fetch only 10 most recent
		orders, err = h.orderRepo.ListByParentAndUser(c.Request.Context(), userID.(string), "bot", id, 10)
	} else if bot.Type == model.BotTypePumpHunter {
		// Get all positions for this bot first
		positions, err := h.phService.ListPositions(c.Request.Context(), userID.(string), id)
		if err != nil {
			util.SendError(c, err)
			return
		}

		if len(positions) == 0 {
			orders = []*model.Order{}
		} else {
			ctx := c.Request.Context()
			// Use pipeline to get top 10 order IDs from each position's sorted set in parallel
			redisClient := h.orderRepo.GetRedisClient().GetClient()
			pipe := redisClient.Pipeline()
			cmds := make(map[int64]*redislib.StringSliceCmd)
			
			for _, pos := range positions {
				posOrdersKey := redis.PositionOrdersKey(pos.ID)
				cmds[pos.ID] = pipe.ZRevRange(ctx, posOrdersKey, 0, 9) // Get top 10 from each position
			}
			
			_, err = pipe.Exec(ctx)
			if err != nil && err != redislib.Nil {
				util.SendError(c, err)
				return
			}

			// Collect all order IDs with their timestamps
			type orderIDWithTS struct {
				orderID string
				ts      float64
			}
			allOrderIDs := make([]orderIDWithTS, 0)
			
			// Use another pipeline to get scores
			pipe2 := redisClient.Pipeline()
			scoreCmds := make(map[string]*redislib.FloatCmd)
			
			for posID, cmd := range cmds {
				orderIDs, err := cmd.Result()
				if err == nil {
					// Get scores (timestamps) for these order IDs
					for _, orderIDStr := range orderIDs {
						posOrdersKey := redis.PositionOrdersKey(posID)
						scoreCmds[orderIDStr] = pipe2.ZScore(ctx, posOrdersKey, orderIDStr)
					}
				}
			}
			
			_, err = pipe2.Exec(ctx)
			if err != nil && err != redislib.Nil {
				// Fallback to simpler method
				allOrders := make([]*model.Order, 0)
				for _, pos := range positions {
					posOrders, err := h.orderRepo.ListByParentAndUser(ctx, userID.(string), "position", pos.ID, 10)
					if err == nil {
						allOrders = append(allOrders, posOrders...)
					}
				}
				sort.Slice(allOrders, func(i, j int) bool {
					if allOrders[i].CreatedAt.Equal(allOrders[j].CreatedAt) {
						return allOrders[i].ID > allOrders[j].ID
					}
					return allOrders[i].CreatedAt.After(allOrders[j].CreatedAt)
				})
				if len(allOrders) > 10 {
					allOrders = allOrders[:10]
				}
				orders = allOrders
			} else {
				for orderIDStr, scoreCmd := range scoreCmds {
					score, err := scoreCmd.Result()
					if err == nil {
						allOrderIDs = append(allOrderIDs, orderIDWithTS{orderID: orderIDStr, ts: score})
					}
				}
			}

			// If sorted sets are empty, fallback to fetching from each position
			if len(allOrderIDs) == 0 {
				allOrders := make([]*model.Order, 0)
				for _, pos := range positions {
					posOrders, err := h.orderRepo.ListByParentAndUser(c.Request.Context(), userID.(string), "position", pos.ID, 10)
					if err == nil {
						allOrders = append(allOrders, posOrders...)
					}
				}
				// Sort and limit
				sort.Slice(allOrders, func(i, j int) bool {
					if allOrders[i].CreatedAt.Equal(allOrders[j].CreatedAt) {
						return allOrders[i].ID > allOrders[j].ID
					}
					return allOrders[i].CreatedAt.After(allOrders[j].CreatedAt)
				})
				if len(allOrders) > 10 {
					allOrders = allOrders[:10]
				}
				orders = allOrders
			} else {
				// Sort by timestamp descending and take top 10
				sort.Slice(allOrderIDs, func(i, j int) bool {
					if allOrderIDs[i].ts == allOrderIDs[j].ts {
						idI, _ := strconv.ParseInt(allOrderIDs[i].orderID, 10, 64)
						idJ, _ := strconv.ParseInt(allOrderIDs[j].orderID, 10, 64)
						return idI > idJ
					}
					return allOrderIDs[i].ts > allOrderIDs[j].ts
				})

				// Limit to top 10
				if len(allOrderIDs) > 10 {
					allOrderIDs = allOrderIDs[:10]
				}

				// Fetch the actual orders using pipeline
				pipe = h.orderRepo.GetRedisClient().Pipeline()
				orderCmds := make(map[string]*redislib.StringCmd)
				for _, oid := range allOrderIDs {
					orderKey := redis.OrderKey(oid.orderID)
					orderCmds[oid.orderID] = pipe.Get(ctx, orderKey)
				}
				
				_, err = pipe.Exec(ctx)
				if err != nil && err != redislib.Nil {
					util.SendError(c, err)
					return
				}

				orders = make([]*model.Order, 0, len(allOrderIDs))
				for _, oid := range allOrderIDs {
					cmd := orderCmds[oid.orderID]
					if cmd == nil {
						continue
					}
					val, err := cmd.Result()
					if err != nil {
						continue
					}
					var order model.Order
					if err := json.Unmarshal([]byte(val), &order); err == nil {
						orders = append(orders, &order)
					}
				}
			}
		}
	}

	if err != nil {
		util.SendError(c, err)
		return
	}

	// Sort orders by most recent first (by CreatedAt, then by ID as fallback)
	// Then limit to 10 most recent orders
	if len(orders) > 0 {
		// Sort by CreatedAt descending (most recent first), then by ID descending
		sort.Slice(orders, func(i, j int) bool {
			if orders[i].CreatedAt.Equal(orders[j].CreatedAt) {
				return orders[i].ID > orders[j].ID
			}
			return orders[i].CreatedAt.After(orders[j].CreatedAt)
		})
		
		// Limit to 10
		if len(orders) > 10 {
			orders = orders[:10]
		}
	}

	util.SendSuccess(c, orders)
}
