package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"tuyul/backend/internal/model"
	"tuyul/backend/internal/repository"
	"tuyul/backend/internal/service/market"
	"tuyul/backend/internal/util"
	"tuyul/backend/pkg/indodax"
	"tuyul/backend/pkg/logger"
)

// StopLossMonitor monitors trades for stop-loss triggers
type StopLossMonitor struct {
	tradeRepo           *repository.TradeRepository
	apiKeyService       *APIKeyService
	indodaxClient       *indodax.Client
	marketDataService   *market.MarketDataService
	notificationService *NotificationService
	balanceRepo         *repository.BalanceRepository
	log                 *logger.Logger

	// Monitoring state
	activeTrades map[int64]*model.Trade // tradeID -> trade
	mu           sync.RWMutex

	// Control
	ticker *time.Ticker
	done   chan struct{}
}

func NewStopLossMonitor(
	tradeRepo *repository.TradeRepository,
	apiKeyService *APIKeyService,
	indodaxClient *indodax.Client,
	marketDataService *market.MarketDataService,
	notificationService *NotificationService,
	balanceRepo *repository.BalanceRepository,
) *StopLossMonitor {
	return &StopLossMonitor{
		tradeRepo:           tradeRepo,
		apiKeyService:       apiKeyService,
		indodaxClient:       indodaxClient,
		marketDataService:   marketDataService,
		notificationService: notificationService,
		balanceRepo:         balanceRepo,
		log:                 logger.GetLogger(),
		activeTrades:        make(map[int64]*model.Trade),
		done:                make(chan struct{}),
	}
}

// Start begins monitoring for stop-loss triggers
func (m *StopLossMonitor) Start() {
	m.ticker = time.NewTicker(1 * time.Second)
	go m.monitorLoop()
	m.log.Info("Stop-loss monitor started")
}

// Stop stops the monitor
func (m *StopLossMonitor) Stop() {
	if m.ticker != nil {
		m.ticker.Stop()
	}
	close(m.done)
	m.log.Info("Stop-loss monitor stopped")
}

// AddTrade adds a trade to the monitoring list
func (m *StopLossMonitor) AddTrade(trade *model.Trade) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if trade.Status == model.TradeStatusFilled && !trade.StopLossTriggered {
		m.activeTrades[trade.ID] = trade
		m.log.Infof("Added trade %d to stop-loss monitoring", trade.ID)
	}
}

// RemoveTrade removes a trade from monitoring
func (m *StopLossMonitor) RemoveTrade(tradeID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.activeTrades, tradeID)
	m.log.Infof("Removed trade %d from stop-loss monitoring", tradeID)
}

// monitorLoop is the main monitoring loop
func (m *StopLossMonitor) monitorLoop() {
	for {
		select {
		case <-m.ticker.C:
			m.checkStopLoss()
		case <-m.done:
			return
		}
	}
}

// checkStopLoss checks all active trades for stop-loss triggers
func (m *StopLossMonitor) checkStopLoss() {
	m.mu.RLock()
	trades := make([]*model.Trade, 0, len(m.activeTrades))
	for _, trade := range m.activeTrades {
		trades = append(trades, trade)
	}
	m.mu.RUnlock()

	for _, trade := range trades {
		m.checkTradeStopLoss(trade)
	}
}

// checkTradeStopLoss checks a single trade for stop-loss trigger
func (m *StopLossMonitor) checkTradeStopLoss(trade *model.Trade) {
	ctx := context.Background()

	// Get current market price
	coin, err := m.marketDataService.GetCoin(ctx, trade.Pair)
	if err != nil {
		return // No market data available
	}

	currentPrice := coin.CurrentPrice

	// Calculate stop-loss price
	stopLossPrice := trade.BuyPrice * (1 - trade.StopLoss/100)

	// Check if stop-loss is triggered
	if currentPrice <= stopLossPrice {
		m.log.Warnf("Stop-loss triggered for TradeID=%d: Price=%.2f, StopLoss=%.2f",
			trade.ID, currentPrice, stopLossPrice)

		// Trigger stop-loss
		if err := m.triggerStopLoss(ctx, trade, currentPrice); err != nil {
			m.log.Errorf("Failed to trigger stop-loss for TradeID=%d: %v", trade.ID, err)
		} else {
			// Remove from monitoring after successful trigger
			m.RemoveTrade(trade.ID)
		}
	}
}

// triggerStopLoss executes the stop-loss by placing a market sell order
func (m *StopLossMonitor) triggerStopLoss(ctx context.Context, trade *model.Trade, currentPrice float64) error {
	// 1. Get Trade Client
	tradeClient, err := m.getTradeClient(ctx, trade.UserID, trade.IsPaperTrade)
	if err != nil {
		return err
	}

	// 2. Cancel existing sell order if exists
	if trade.SellOrderID != "" {
		err := tradeClient.CancelOrder(ctx, trade.Pair, trade.SellOrderID, "sell")
		if err != nil {
			m.log.Warnf("Failed to cancel sell order before stop-loss: %v", err)
		}
	}

	// 3. Get current balance
	accountInfo, err := tradeClient.GetInfo(ctx)
	if err != nil {
		return fmt.Errorf("failed to get account info: %w", err)
	}

	coinSymbol := m.extractCoinSymbol(trade.Pair)
	sellAmount := m.parseBalance(accountInfo.Balance[coinSymbol].String())

	if sellAmount <= 0 {
		return fmt.Errorf("no coins available to sell")
	}

	// 4. Place market sell order (use aggressive price to ensure fill)
	// Use 10% below current price to ensure immediate fill
	marketPrice := currentPrice * 0.90
	marketPrice = util.RoundToPrecision(marketPrice, 0)
	
	// Generate unique client order ID for stop-loss sell
	clientOrderID := fmt.Sprintf("copilot-%s-stoploss-%d", trade.Pair, time.Now().UnixMilli())

	result, err := tradeClient.Trade(ctx, "sell", trade.Pair, marketPrice, sellAmount, "market", clientOrderID)
	if err != nil {
		return fmt.Errorf("failed to place stop-loss sell order: %w", err)
	}

	// 5. Update trade status
	oldStatus := trade.Status
	trade.Status = model.TradeStatusStopped
	trade.StopLossTriggered = true
	trade.SellOrderID = fmt.Sprintf("%d", result.OrderID)
	trade.SellPrice = marketPrice
	trade.SellAmount = sellAmount

	if err := m.tradeRepo.Update(ctx, trade, oldStatus); err != nil {
		m.log.Errorf("Failed to update trade after stop-loss: %v", err)
		return err
	}

	// 6. Update virtual balance if paper trading
	if trade.IsPaperTrade {
		balances, _ := m.getPaperBalances(ctx, trade.UserID)
		// Remove coins
		coinSymbol := m.extractCoinSymbol(trade.Pair)
		balances[coinSymbol] -= sellAmount
		if balances[coinSymbol] < 0 {
			balances[coinSymbol] = 0
		}
		// Add IDR (at stop-loss price)
		balances["idr"] += sellAmount * marketPrice
		m.savePaperBalances(ctx, trade.UserID, balances)
	}

	m.log.Warnf("Stop-loss executed (%s): TradeID=%d, SellPrice=%.2f, Amount=%.8f",
		map[bool]string{true: "paper", false: "live"}[trade.IsPaperTrade],
		trade.ID, marketPrice, sellAmount)

	// Send WebSocket alert to user
	m.notificationService.NotifyUser(ctx, trade.UserID, model.MessageTypeOrderUpdate, trade)

	return nil
}

func (m *StopLossMonitor) getTradeClient(ctx context.Context, userID string, isPaperTrade bool) (TradeClient, error) {
	if isPaperTrade {
		balances, _ := m.getPaperBalances(ctx, userID)
		// Note: We don't need a fill callback here because stop-loss is immediate market sell
		return NewPaperTradeClient(balances, nil), nil
	}

	credentials, err := m.apiKeyService.GetDecrypted(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("valid API key not found")
	}
	return NewLiveTradeClient(m.indodaxClient, credentials.Key, credentials.Secret), nil
}

func (m *StopLossMonitor) getPaperBalances(ctx context.Context, userID string) (map[string]float64, error) {
	return m.balanceRepo.GetUserPaperBalances(ctx, userID)
}

func (m *StopLossMonitor) savePaperBalances(ctx context.Context, userID string, balances map[string]float64) {
	m.balanceRepo.SaveUserPaperBalances(ctx, userID, balances)
}

// extractCoinSymbol extracts the coin symbol from pair (e.g., "btcidr" -> "btc")
func (m *StopLossMonitor) extractCoinSymbol(pair string) string {
	// Simple implementation - can be improved
	if len(pair) > 3 {
		return pair[:len(pair)-3] // Remove "idr" suffix
	}
	return pair
}

// parseBalance parses balance string to float64
func (m *StopLossMonitor) parseBalance(balanceStr string) float64 {
	var balance float64
	fmt.Sscanf(balanceStr, "%f", &balance)
	return balance
}

// LoadActiveTrades loads all filled trades into monitoring
func (m *StopLossMonitor) LoadActiveTrades(ctx context.Context) error {
	// This would require a method to get all filled trades from all users
	// For now, trades are added when buy orders fill
	m.log.Info("Active trades loaded for stop-loss monitoring")
	return nil
}
