package handler

import (
	"strconv"
	"time"

	"tuyul/backend/internal/model"
	"tuyul/backend/internal/repository"
	"tuyul/backend/internal/service"
	"tuyul/backend/internal/util"

	"github.com/gin-gonic/gin"
)

type BotHandler struct {
	botRepo   *repository.BotRepository
	mmService *service.MarketMakerService
	phService *service.PumpHunterService
}

func NewBotHandler(botRepo *repository.BotRepository, mmService *service.MarketMakerService, phService *service.PumpHunterService) *BotHandler {
	return &BotHandler{
		botRepo:   botRepo,
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
