package handler

import (
	"net/http"
	"strconv"
	"time"

	"tuyul/backend/internal/service/market"
	"tuyul/backend/internal/util"
	"tuyul/backend/pkg/redis"

	"github.com/gin-gonic/gin"
)

type MarketHandler struct {
	marketService *market.MarketDataService
}

func NewMarketHandler(marketService *market.MarketDataService) *MarketHandler {
	return &MarketHandler{
		marketService: marketService,
	}
}

// GetSummary returns all market summaries
func (h *MarketHandler) GetSummary(c *gin.Context) {
	limitStr := c.DefaultQuery("limit", "0")
	limit, _ := strconv.Atoi(limitStr)
	if limit > 200 {
		limit = 200
	}

	minVolStr := c.DefaultQuery("min_volume", "0")
	minVol, _ := strconv.ParseFloat(minVolStr, 64)

	coins, err := h.marketService.GetSortedCoins(c.Request.Context(), redis.PumpScoreRankKey(), limit, minVol)
	if err != nil {
		util.SendError(c, err)
		return
	}

	util.SendSuccess(c, gin.H{
		"markets":     coins,
		"count":       len(coins),
		"last_update": time.Now(),
	})
}

// GetPumpScores returns markets sorted by pump score
func (h *MarketHandler) GetPumpScores(c *gin.Context) {
	limitStr := c.DefaultQuery("limit", "0")
	limit, _ := strconv.Atoi(limitStr)

	minVolStr := c.DefaultQuery("min_volume", "0")
	minVol, _ := strconv.ParseFloat(minVolStr, 64)

	coins, err := h.marketService.GetSortedCoins(c.Request.Context(), redis.PumpScoreRankKey(), limit, minVol)
	if err != nil {
		util.SendError(c, err)
		return
	}

	util.SendSuccess(c, gin.H{
		"scores": coins,
		"count":  len(coins),
	})
}

// GetGaps returns markets sorted by gap percentage (High to Low)
func (h *MarketHandler) GetGaps(c *gin.Context) {
	limitStr := c.DefaultQuery("limit", "0")
	limit, _ := strconv.Atoi(limitStr)

	// Lower default volume to see more results during dev/testing
	minVolStr := c.DefaultQuery("min_volume", "0") // 0 IDR
	minVol, _ := strconv.ParseFloat(minVolStr, 64)

	// Fetch coins sorted by gap (ZRevRange gets highest scores first)
	coins, err := h.marketService.GetSortedCoins(c.Request.Context(), redis.GapRankKey(), limit, minVol)
	if err != nil {
		util.SendError(c, err)
		return
	}

	util.SendSuccess(c, gin.H{
		"gaps":  coins,
		"count": len(coins),
	})
}

func (h *MarketHandler) GetPairDetail(c *gin.Context) {
	pairID := c.Param("pair")
	coin, err := h.marketService.GetCoin(c.Request.Context(), pairID)
	if err != nil {
		util.SendCustomError(c, http.StatusNotFound, util.ErrCodeNotFound, "Pair not found")
		return
	}

	util.SendSuccess(c, coin)
}

// SyncMetadata manually triggers a metadata refresh from Indodax
func (h *MarketHandler) SyncMetadata(c *gin.Context) {
	if err := h.marketService.RefreshMetadata(); err != nil {
		util.SendError(c, util.ErrInternalServer("Failed to sync metadata: "+err.Error()))
		return
	}

	util.SendSuccess(c, gin.H{
		"message": "Metadata synced successfully from Indodax",
		"time":    time.Now(),
	})
}
