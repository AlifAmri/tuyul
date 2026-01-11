package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"tuyul/backend/internal/model"
	"tuyul/backend/internal/repository"
	"tuyul/backend/internal/util"
	"tuyul/backend/pkg/indodax"
	"tuyul/backend/pkg/logger"
)

// GenerateClientOrderID generates a unique client order ID for a bot order
func GenerateClientOrderID(botID int64, pair, side string) string {
	return fmt.Sprintf("bot%d-%s-%s-%d", botID, pair, strings.ToLower(side), time.Now().UnixMilli())
}

// CreateTradeClient creates a trade client based on trading mode
// Returns the trade client and any error
func CreateTradeClient(
	ctx context.Context,
	isPaperTrading bool,
	balances map[string]float64,
	apiKeyService *APIKeyService,
	indodaxClient *indodax.Client,
	userID string,
	onFilled func(order *model.Order),
) (TradeClient, error) {
	if isPaperTrading {
		return NewPaperTradeClient(balances, onFilled), nil
	}

	// Get API key for live trading
	key, err := apiKeyService.GetDecrypted(ctx, userID)
	if err != nil {
		return nil, util.ErrBadRequest("Valid API key not found")
	}

	return NewLiveTradeClient(indodaxClient, key.Key, key.Secret), nil
}

// StopBotWithError stops a bot and sets error status (for live bots only)
// This is a shared utility that can be used by both Market Maker and Pump Hunter
func StopBotWithError(
	botID int64,
	userID string,
	errorMsg string,
	botRepo *repository.BotRepository,
	instances map[int64]interface{},
	getStopChan func(int64) (chan struct{}, bool),
	log *logger.Logger,
	notificationService *NotificationService,
) {
	// Get stop channel from instance
	stopChan, ok := getStopChan(botID)
	if !ok {
		log.Warnf("Bot %d: Instance not found when trying to stop with error", botID)
		return
	}

	// Close stop channel to stop the bot loop
	select {
	case <-stopChan:
		// Already closed
	default:
		close(stopChan)
	}

	// Update status to error
	bgCtx := context.Background()
	errMsg := errorMsg
	if err := botRepo.UpdateStatus(bgCtx, botID, model.BotStatusError, &errMsg); err != nil {
		log.Errorf("Failed to update bot %d status to error: %v", botID, err)
	}

	// Notify via WebSocket
	go func() {
		notifyCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		bot, _ := botRepo.GetByID(notifyCtx, botID)
		if bot != nil {
			notificationService.NotifyBotUpdate(notifyCtx, userID, model.WSBotUpdatePayload{
				BotID:          botID,
				Status:         model.BotStatusError,
				TotalTrades:    bot.TotalTrades,
				WinningTrades:  bot.WinningTrades,
				WinRate:        bot.WinRate(),
				TotalProfitIDR: bot.TotalProfitIDR,
				Balances:       bot.Balances,
			})
		}
	}()
}
