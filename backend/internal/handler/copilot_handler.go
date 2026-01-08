package handler

import (
	"strconv"

	"tuyul/backend/internal/model"
	"tuyul/backend/internal/service"
	"tuyul/backend/internal/util"

	"github.com/gin-gonic/gin"
)

type CopilotHandler struct {
	copilotService *service.CopilotService
}

func NewCopilotHandler(copilotService *service.CopilotService) *CopilotHandler {
	return &CopilotHandler{
		copilotService: copilotService,
	}
}

// PlaceTrade handles POST /api/v1/copilot/trade
func (h *CopilotHandler) PlaceTrade(c *gin.Context) {
	var req model.TradeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.SendValidationError(c, err.Error())
		return
	}

	// Get user ID from context (set by auth middleware)
	userID, exists := c.Get("user_id")
	if !exists {
		util.SendError(c, util.ErrUnauthorized("User not authenticated"))
		return
	}

	trade, err := h.copilotService.PlaceBuyOrder(c.Request.Context(), userID.(string), &req)
	if err != nil {
		util.SendError(c, err)
		return
	}

	util.SendCreated(c, trade, "Buy order placed successfully")
}

// GetTrades handles GET /api/v1/copilot/trades
func (h *CopilotHandler) GetTrades(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		util.SendError(c, util.ErrUnauthorized("User not authenticated"))
		return
	}

	// Parse pagination parameters
	offsetStr := c.DefaultQuery("offset", "0")
	limitStr := c.DefaultQuery("limit", "20")

	offset, _ := strconv.Atoi(offsetStr)
	limit, _ := strconv.Atoi(limitStr)

	trades, total, err := h.copilotService.ListTrades(c.Request.Context(), userID.(string), offset, limit)
	if err != nil {
		util.SendError(c, err)
		return
	}

	util.SendPaginated(c, map[string]interface{}{
		"trades": trades,
	}, util.Pagination{
		Limit:  limit,
		Offset: offset,
		Total:  total,
	})
}

// GetTrade handles GET /api/v1/copilot/trades/:id
func (h *CopilotHandler) GetTrade(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		util.SendError(c, util.ErrUnauthorized("User not authenticated"))
		return
	}

	tradeIDStr := c.Param("id")
	tradeID, err := strconv.ParseInt(tradeIDStr, 10, 64)
	if err != nil {
		util.SendError(c, util.ErrBadRequest("Invalid trade ID"))
		return
	}

	trade, err := h.copilotService.GetTrade(c.Request.Context(), userID.(string), tradeID)
	if err != nil {
		util.SendError(c, err)
		return
	}

	util.SendSuccess(c, trade)
}

// CancelTrade handles DELETE /api/v1/copilot/trades/:id
func (h *CopilotHandler) CancelTrade(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		util.SendError(c, util.ErrUnauthorized("User not authenticated"))
		return
	}

	tradeIDStr := c.Param("id")
	tradeID, err := strconv.ParseInt(tradeIDStr, 10, 64)
	if err != nil {
		util.SendError(c, util.ErrBadRequest("Invalid trade ID"))
		return
	}

	if err := h.copilotService.CancelBuyOrder(c.Request.Context(), userID.(string), tradeID); err != nil {
		util.SendError(c, err)
		return
	}

	util.SendSuccess(c, gin.H{
		"trade_id": tradeID,
		"status":   "cancelled",
	})
}

// ManualSell handles POST /api/v1/copilot/trades/:id/sell
func (h *CopilotHandler) ManualSell(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		util.SendError(c, util.ErrUnauthorized("User not authenticated"))
		return
	}

	tradeIDStr := c.Param("id")
	tradeID, err := strconv.ParseInt(tradeIDStr, 10, 64)
	if err != nil {
		util.SendError(c, util.ErrBadRequest("Invalid trade ID"))
		return
	}

	if err := h.copilotService.ManualSell(c.Request.Context(), userID.(string), tradeID); err != nil {
		util.SendError(c, err)
		return
	}

	util.SendSuccess(c, gin.H{
		"trade_id": tradeID,
		"message":  "Manual sell order placed",
	})
}
