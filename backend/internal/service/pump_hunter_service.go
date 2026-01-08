package service

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"tuyul/backend/internal/model"
	"tuyul/backend/internal/repository"
	"tuyul/backend/internal/service/market"
	"tuyul/backend/internal/util"
	"tuyul/backend/pkg/indodax"
	"tuyul/backend/pkg/logger"
)

type PumpSignal struct {
	Coin      *model.Coin
	Score     float64
	Timestamp time.Time
}

type PumpHunterInstance struct {
	Config        *model.BotConfig
	TradeClient   TradeClient
	OpenPositions map[int64]*model.Position
	SignalBuffer  map[string]*PumpSignal
	DailyLoss     float64
	LastLossTime  time.Time
	StopChan      chan struct{}
	mu            sync.RWMutex
	signalMu      sync.Mutex
}

type PumpHunterService struct {
	botRepo             *repository.BotRepository
	posRepo             *repository.PositionRepository
	orderRepo           *repository.OrderRepository
	apiKeyService       *APIKeyService
	marketDataService   *market.MarketDataService
	orderMonitor        *OrderMonitor
	notificationService *NotificationService
	indodaxClient       *indodax.Client
	log                 *logger.Logger

	instances map[int64]*PumpHunterInstance
	mu        sync.RWMutex
}

func NewPumpHunterService(
	botRepo *repository.BotRepository,
	posRepo *repository.PositionRepository,
	orderRepo *repository.OrderRepository,
	apiKeyService *APIKeyService,
	marketDataService *market.MarketDataService,
	orderMonitor *OrderMonitor,
	notificationService *NotificationService,
	indodaxClient *indodax.Client,
) *PumpHunterService {
	s := &PumpHunterService{
		botRepo:             botRepo,
		posRepo:             posRepo,
		orderRepo:           orderRepo,
		apiKeyService:       apiKeyService,
		marketDataService:   marketDataService,
		orderMonitor:        orderMonitor,
		notificationService: notificationService,
		indodaxClient:       indodaxClient,
		log:                 logger.GetLogger(),
		instances:           make(map[int64]*PumpHunterInstance),
	}

	// Register for coin updates
	marketDataService.OnUpdate(s.handleCoinUpdate)

	// Register for order updates
	orderMonitor.AddOrderHandler(s.handleOrderUpdate)

	return s
}

// StartBot starts a pump hunter bot
func (s *PumpHunterService) StartBot(ctx context.Context, userID string, botID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. Get bot config
	bot, err := s.botRepo.GetByID(ctx, botID)
	if err != nil {
		return err
	}
	if bot.UserID != userID {
		return util.ErrForbidden("Access denied")
	}
	if bot.Type != model.BotTypePumpHunter {
		return util.ErrBadRequest("Not a pump hunter bot")
	}
	if bot.Status == model.BotStatusRunning {
		return nil
	}

	inst := &PumpHunterInstance{
		Config:        bot,
		OpenPositions: make(map[int64]*model.Position),
		SignalBuffer:  make(map[string]*PumpSignal),
		StopChan:      make(chan struct{}),
	}

	// 2. Setup Trade Client
	if bot.IsPaperTrading {
		inst.TradeClient = NewPaperTradeClient(bot.Balances, func(order *model.Order) {
			s.handleOrderFilled(inst, order)
		})
	} else {
		key, err := s.apiKeyService.GetDecrypted(ctx, userID)
		if err != nil {
			return util.ErrBadRequest("Valid API key not found")
		}
		inst.TradeClient = NewLiveTradeClient(s.indodaxClient, key.Key, key.Secret)

		// Subscribe to order updates
		if err := s.orderMonitor.SubscribeUserOrders(ctx, userID); err != nil {
			s.log.Warnf("Failed to subscribe to order updates for user %s: %v", userID, err)
		}
	}

	// 3. Load active positions from DB
	activePos, err := s.posRepo.ListActiveByBot(ctx, botID)
	if err == nil {
		for _, pos := range activePos {
			inst.OpenPositions[pos.ID] = pos
		}
	}

	// 4. Start background monitoring for exits (time-based, score-based)
	go s.runBot(inst)

	s.instances[botID] = inst
	// 4. Initial balance sync (allocates IDR and ensures isolated state)
	if err := s.syncBalance(ctx, inst); err != nil {
		s.log.Warnf("Failed to initial sync balance for bot %d: %v", botID, err)
	}

	bot.Status = model.BotStatusRunning
	inst.Config.Status = model.BotStatusRunning // Update in-memory status
	s.botRepo.UpdateStatus(ctx, botID, model.BotStatusRunning, nil)

	// Notify via WebSocket
	s.notificationService.NotifyBotUpdate(ctx, userID, model.WSBotUpdatePayload{
		BotID:          botID,
		Status:         model.BotStatusRunning,
		TotalTrades:    bot.TotalTrades,
		WinningTrades:  bot.WinningTrades,
		WinRate:        bot.WinRate(),
		TotalProfitIDR: bot.TotalProfitIDR,
		Balances:       bot.Balances,
	})

	s.log.Infof("Pump Hunter Bot %d started", botID)
	return nil
}

// CreateBot creates a new pump hunter bot
func (s *PumpHunterService) CreateBot(ctx context.Context, userID string, req *model.BotConfigRequest) (*model.BotConfig, error) {
	// 1. Validate
	if req.Type != model.BotTypePumpHunter {
		return nil, util.ErrBadRequest("Invalid bot type for Pump Hunter service")
	}

	if req.EntryRules == nil || req.ExitRules == nil || req.RiskManagement == nil {
		return nil, util.ErrBadRequest("Pump Hunter rules and risk management are required")
	}

	// 2. Separate logic for paper vs live
	var apiKeyID *int64
	if !req.IsPaperTrading {
		if req.APIKeyID == nil {
			return nil, util.ErrBadRequest("API key is required for live trading")
		}
		// Verify API key ownership and validity
		_, err := s.apiKeyService.GetDecrypted(ctx, userID)
		if err != nil {
			return nil, util.NewAppError(400, util.ErrCodeAPIKeyInvalid, "Valid API key not found for user")
		}
		apiKeyID = req.APIKeyID
	}

	// 3. Create bot config
	bot := &model.BotConfig{
		UserID:            userID,
		Name:              req.Name,
		Type:              model.BotTypePumpHunter,
		Pair:              "ALL", // Pump hunter scans all pairs
		IsPaperTrading:    req.IsPaperTrading,
		APIKeyID:          apiKeyID,
		EntryRules:        req.EntryRules,
		ExitRules:         req.ExitRules,
		RiskManagement:    req.RiskManagement,
		InitialBalanceIDR: req.InitialBalanceIDR,
		Balances:          make(map[string]float64),
	}

	// 4. Initialize balances properly
	// For Pump Hunter: always set IDR balance, coin balances will be managed per position
	if req.IsPaperTrading {
		// Paper trading: set IDR balance from InitialBalanceIDR
		bot.Balances["idr"] = req.InitialBalanceIDR
		s.log.Infof("Pump Hunter bot created (paper): IDR=%.2f", req.InitialBalanceIDR)
	} else {
		// Live trading: set IDR balance (will sync from Indodax on start)
		bot.Balances["idr"] = req.InitialBalanceIDR
		s.log.Infof("Pump Hunter bot created (live): IDR=%.2f (will sync from Indodax on start)", 
			req.InitialBalanceIDR)
	}
	// Note: Pump Hunter doesn't have a single coin balance since it trades multiple pairs
	// Coin balances are managed per position

	// 5. Save to repository
	if err := s.botRepo.Create(ctx, bot); err != nil {
		return nil, err
	}

	s.log.Infof("Pump Hunter bot created successfully: ID=%d, Name=%s, PaperTrading=%v", 
		bot.ID, bot.Name, bot.IsPaperTrading)

	return bot, nil
}

// ListBots lists all pump hunter bots for a user
func (s *PumpHunterService) ListBots(ctx context.Context, userID string) ([]*model.BotConfig, error) {
	allBots, err := s.botRepo.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	var phBots []*model.BotConfig
	for _, b := range allBots {
		if b.Type == model.BotTypePumpHunter {
			phBots = append(phBots, b)
		}
	}
	return phBots, nil
}

// GetBot gets a pump hunter bot by ID
func (s *PumpHunterService) GetBot(ctx context.Context, userID string, botID int64) (*model.BotConfig, error) {
	bot, err := s.botRepo.GetByID(ctx, botID)
	if err != nil {
		return nil, err
	}
	if bot.UserID != userID || bot.Type != model.BotTypePumpHunter {
		return nil, util.ErrNotFound("Bot not found")
	}
	return bot, nil
}

// UpdateBot updates a bot configuration
func (s *PumpHunterService) UpdateBot(ctx context.Context, userID string, botID int64, req *model.BotConfigRequest) (*model.BotConfig, error) {
	bot, err := s.GetBot(ctx, userID, botID)
	if err != nil {
		return nil, err
	}

	s.mu.RLock()
	if _, ok := s.instances[botID]; ok {
		s.mu.RUnlock()
		return nil, util.ErrBadRequest("Cannot update a running bot. Stop it first.")
	}
	s.mu.RUnlock()

	// Update fields
	bot.Name = req.Name
	bot.IsPaperTrading = req.IsPaperTrading
	bot.APIKeyID = req.APIKeyID
	bot.EntryRules = req.EntryRules
	bot.ExitRules = req.ExitRules
	bot.RiskManagement = req.RiskManagement

	if err := s.botRepo.Update(ctx, bot, ""); err != nil {
		return nil, err
	}

	return bot, nil
}

// DeleteBot deletes a pump hunter bot
func (s *PumpHunterService) DeleteBot(ctx context.Context, userID string, botID int64) error {
	s.mu.Lock()
	if _, ok := s.instances[botID]; ok {
		s.mu.Unlock()
		return util.ErrBadRequest("Cannot delete a running bot. Stop it first.")
	}
	s.mu.Unlock()

	bot, err := s.GetBot(ctx, userID, botID)
	if err != nil {
		return err
	}

	// Delete all orders and positions associated with this bot
	// First, get all positions for this bot
	positions, err := s.posRepo.ListByBot(ctx, botID)
	if err != nil {
		s.log.Warnf("Failed to list positions for bot %d: %v", botID, err)
	} else {
		// Delete orders for each position, then delete the position
		for _, pos := range positions {
			// Delete orders for this position
			posOrders, err := s.orderRepo.ListByParentAndUser(ctx, userID, "position", pos.ID)
			if err != nil {
				s.log.Warnf("Failed to list orders for position %d: %v", pos.ID, err)
			} else {
				for _, order := range posOrders {
					if err := s.orderRepo.Delete(ctx, order.ID); err != nil {
						s.log.Warnf("Failed to delete order %d for position %d: %v", order.ID, pos.ID, err)
					}
				}
			}
			
			// Delete the position itself
			if err := s.posRepo.Delete(ctx, pos.ID); err != nil {
				s.log.Warnf("Failed to delete position %d: %v", pos.ID, err)
			}
		}
		s.log.Infof("Deleted %d positions and their orders for bot %d", len(positions), botID)
	}

	// Also delete any orders directly associated with the bot (if any)
	botOrders, err := s.orderRepo.ListByParentAndUser(ctx, userID, "bot", botID)
	if err != nil {
		s.log.Warnf("Failed to list direct orders for bot %d: %v", botID, err)
	} else {
		for _, order := range botOrders {
			if err := s.orderRepo.Delete(ctx, order.ID); err != nil {
				s.log.Warnf("Failed to delete order %d for bot %d: %v", order.ID, botID, err)
			}
		}
		if len(botOrders) > 0 {
			s.log.Infof("Deleted %d direct orders for bot %d", len(botOrders), botID)
		}
	}

	return s.botRepo.Delete(ctx, bot.ID)
}

// StopBot stops a pump hunter bot
// ListPositions lists all positions for a bot
func (s *PumpHunterService) ListPositions(ctx context.Context, userID string, botID int64) ([]*model.Position, error) {
	bot, err := s.GetBot(ctx, userID, botID)
	if err != nil {
		return nil, err
	}

	return s.posRepo.ListByBot(ctx, bot.ID)
}

func (s *PumpHunterService) StopBot(ctx context.Context, userID string, botID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	inst, ok := s.instances[botID]
	if !ok {
		return nil
	}

	if inst.Config.UserID != userID {
		return util.ErrForbidden("Access denied")
	}

	// Cancel any open orders for this bot
	// Get all positions for this bot
	positions, err := s.posRepo.ListByBot(ctx, botID)
	if err == nil {
		for _, pos := range positions {
			// Get orders for this position
			posOrders, err := s.orderRepo.ListByParentAndUser(ctx, userID, "position", pos.ID)
			if err == nil {
				for _, order := range posOrders {
					if order.Status == "open" {
						s.log.Infof("Bot %d: Cancelling open order %d (ID: %s) for position %d", botID, order.ID, order.OrderID, pos.ID)
						if err := inst.TradeClient.CancelOrder(ctx, order.Pair, order.OrderID, order.Side); err != nil {
							s.log.Warnf("Bot %d: Failed to cancel order %d: %v", botID, order.ID, err)
						} else {
							s.orderRepo.UpdateStatus(ctx, order.ID, "cancelled")
							order.Status = "cancelled"
							s.notificationService.NotifyOrderUpdate(ctx, userID, order)
						}
					}
				}
			}
		}
	}

	// Also check for any direct bot orders
	botOrders, err := s.orderRepo.ListByParentAndUser(ctx, userID, "bot", botID)
	if err == nil {
		for _, order := range botOrders {
			if order.Status == "open" {
				s.log.Infof("Bot %d: Cancelling open order %d (ID: %s)", botID, order.ID, order.OrderID)
				if err := inst.TradeClient.CancelOrder(ctx, order.Pair, order.OrderID, order.Side); err != nil {
					s.log.Warnf("Bot %d: Failed to cancel order %d: %v", botID, order.ID, err)
				} else {
					s.orderRepo.UpdateStatus(ctx, order.ID, "cancelled")
					order.Status = "cancelled"
					s.notificationService.NotifyOrderUpdate(ctx, userID, order)
				}
			}
		}
	}

	close(inst.StopChan)
	delete(s.instances, botID)

	inst.Config.Status = model.BotStatusStopped
	err = s.botRepo.UpdateStatus(ctx, botID, model.BotStatusStopped, nil)

	// Notify via WebSocket
	stoppedBot, _ := s.botRepo.GetByID(ctx, botID)
	s.notificationService.NotifyBotUpdate(ctx, userID, model.WSBotUpdatePayload{
		BotID:          botID,
		Status:         model.BotStatusStopped,
		TotalTrades:    stoppedBot.TotalTrades,
		WinningTrades:  stoppedBot.WinningTrades,
		WinRate:        stoppedBot.WinRate(),
		TotalProfitIDR: stoppedBot.TotalProfitIDR,
		Balances:       stoppedBot.Balances,
	})

	return err
}

func (s *PumpHunterService) runBot(inst *PumpHunterInstance) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	// Ticker for signal processing (priority ranking)
	signalTicker := time.NewTicker(1 * time.Second)
	defer signalTicker.Stop()

	// Ticker for max loss check
	maxLossTicker := time.NewTicker(5 * time.Second)
	defer maxLossTicker.Stop()

	for {
		select {
		case <-inst.StopChan:
			return
		case <-ticker.C:
			s.monitorExits(inst)
		case <-signalTicker.C:
			s.processSignals(inst)
		case <-maxLossTicker.C:
			// Periodic check for max loss limit
			inst.mu.RLock()
			config := inst.Config
			inst.mu.RUnlock()
			
			if config.TotalProfitIDR <= -config.MaxLossIDR {
				s.log.Warnf("Bot %d reached total max loss limit (%.2f <= -%.2f), stopping bot", 
					config.ID, config.TotalProfitIDR, config.MaxLossIDR)
				ctx := context.Background()
				go func() {
					if err := s.StopBot(ctx, config.UserID, config.ID); err != nil {
						s.log.Errorf("Failed to stop bot %d after max loss: %v", config.ID, err)
					}
				}()
				return
			}
		}
	}
}

func (s *PumpHunterService) handleCoinUpdate(coin *model.Coin) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, inst := range s.instances {
		if s.checkEntryConditions(inst, coin) {
			s.bufferSignal(inst, coin)
		}
	}
}

func (s *PumpHunterService) bufferSignal(inst *PumpHunterInstance, coin *model.Coin) {
	inst.signalMu.Lock()
	defer inst.signalMu.Unlock()

	// Add or update signal in buffer
	// If same coin exists, keep the one with higher score
	if existing, ok := inst.SignalBuffer[coin.PairID]; ok {
		if coin.PumpScore > existing.Score {
			existing.Score = coin.PumpScore
			existing.Coin = coin
			existing.Timestamp = time.Now()
		}
	} else {
		inst.SignalBuffer[coin.PairID] = &PumpSignal{
			Coin:      coin,
			Score:     coin.PumpScore,
			Timestamp: time.Now(),
		}
	}
}

func (s *PumpHunterService) processSignals(inst *PumpHunterInstance) {
	inst.signalMu.Lock()
	if len(inst.SignalBuffer) == 0 {
		inst.signalMu.Unlock()
		return
	}

	// Copy and clear buffer
	signals := make([]*PumpSignal, 0, len(inst.SignalBuffer))
	for _, sig := range inst.SignalBuffer {
		signals = append(signals, sig)
	}
	inst.SignalBuffer = make(map[string]*PumpSignal)
	inst.signalMu.Unlock()

	// Sort signals by score descending (Priority Logic)
	// Larger score = higher priority
	for i := 0; i < len(signals); i++ {
		for j := i + 1; j < len(signals); j++ {
			if signals[j].Score > signals[i].Score {
				signals[i], signals[j] = signals[j], signals[i]
			}
		}
	}

	// Process signals up to available slots
	for _, sig := range signals {
		// Re-check conditions (especially MaxPositions) before opening
		if s.checkEntryConditions(inst, sig.Coin) {
			s.openPosition(inst, sig.Coin)
		}
	}
}

func (s *PumpHunterService) checkEntryConditions(inst *PumpHunterInstance, coin *model.Coin) bool {
	inst.mu.RLock()
	defer inst.mu.RUnlock()

	config := inst.Config
	if config.EntryRules == nil {
		return false
	}

	// 0. Risk Management Checks
	// 0.1 Circuit Breaker (Total Loss)
	if config.TotalProfitIDR <= -config.MaxLossIDR {
		s.log.Warnf("Bot %d reached total max loss limit (%.2f <= -%.2f), stopping bot", 
			config.ID, config.TotalProfitIDR, config.MaxLossIDR)
		// Stop the bot asynchronously
		ctx := context.Background()
		go func() {
			if err := s.StopBot(ctx, config.UserID, config.ID); err != nil {
				s.log.Errorf("Failed to stop bot %d after max loss: %v", config.ID, err)
			}
		}()
		return false
	}

	// 0.2 Daily Loss Limit
	if inst.DailyLoss >= config.MaxLossIDR/5 { // Example: daily limit is 20% of total max loss
		// Check if we should reset daily loss (it's a new day)
		if time.Since(inst.LastLossTime) > 24*time.Hour {
			inst.DailyLoss = 0
		} else {
			return false
		}
	}

	// 0.3 High Volatility Check
	if coin.Volatility1m > 5.0 {
		return false
	}

	// 1. Pump Score
	if coin.PumpScore < config.EntryRules.MinPumpScore {
		return false
	}

	// 2. Positive Timeframes
	positiveCount := 0
	if coin.Timeframes.OneMinute.Open > 0 && coin.CurrentPrice > coin.Timeframes.OneMinute.Open {
		positiveCount++
	}
	if coin.Timeframes.FiveMinute.Open > 0 && coin.CurrentPrice > coin.Timeframes.FiveMinute.Open {
		positiveCount++
	}
	if coin.Timeframes.FifteenMin.Open > 0 && coin.CurrentPrice > coin.Timeframes.FifteenMin.Open {
		positiveCount++
	}
	if coin.Timeframes.ThirtyMin.Open > 0 && coin.CurrentPrice > coin.Timeframes.ThirtyMin.Open {
		positiveCount++
	}
	if positiveCount < config.EntryRules.MinTimeframesPositive {
		return false
	}

	// 3. 24h Volume
	if coin.VolumeIDR < config.EntryRules.Min24hVolumeIDR {
		return false
	}

	// 4. Min Price
	if coin.CurrentPrice < config.EntryRules.MinPriceIDR {
		return false
	}

	// 5. Excluded/Allowed Pairs
	if len(config.EntryRules.AllowedPairs) > 0 {
		allowed := false
		for _, p := range config.EntryRules.AllowedPairs {
			if p == coin.PairID {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}
	for _, p := range config.EntryRules.ExcludedPairs {
		if p == coin.PairID {
			return false
		}
	}

	// 6. Max Concurrent Positions
	if len(inst.OpenPositions) >= config.RiskManagement.MaxConcurrentPositions {
		return false
	}

	// 7. Already have position
	for _, pos := range inst.OpenPositions {
		if pos.Pair == coin.PairID {
			return false
		}
	}

	// 8. Cooldown after loss
	if config.RiskManagement.CooldownAfterLossMinutes > 0 && !inst.LastLossTime.IsZero() {
		if time.Since(inst.LastLossTime) < time.Duration(config.RiskManagement.CooldownAfterLossMinutes)*time.Minute {
			return false
		}
	}

	// 9. Daily Loss Limit
	if config.RiskManagement.DailyLossLimitIDR > 0 && inst.DailyLoss >= config.RiskManagement.DailyLossLimitIDR {
		return false
	}

	return true
}

func (s *PumpHunterService) openPosition(inst *PumpHunterInstance, coin *model.Coin) {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	pairInfo, ok := s.marketDataService.GetPairInfo(coin.PairID)
	if !ok {
		return
	}

	// Calculate size
	sizeIDR := inst.Config.RiskManagement.MaxPositionIDR
	idrBalance := inst.Config.Balances["idr"]
	if idrBalance < sizeIDR {
		sizeIDR = idrBalance
	}

	// Check min balance
	if idrBalance-sizeIDR < inst.Config.RiskManagement.MinBalanceIDR {
		sizeIDR = idrBalance - inst.Config.RiskManagement.MinBalanceIDR
	}

	if sizeIDR < 50000 { // Assume min 50k IDR for safety
		return
	}

	price := coin.CurrentPrice
	amount := sizeIDR / price
	amount = util.FloorToPrecision(amount, pairInfo.VolumePrecision)

	if amount < float64(pairInfo.TradeMinBaseCurrency) || amount*price < pairInfo.TradeMinTradedCurrency {
		return
	}

	// Place Order
	ctx := context.Background()
	res, err := inst.TradeClient.Trade(ctx, "buy", coin.PairID, price, amount, "market")
	if err != nil {
		s.log.Errorf("Bot %d failed to open position on %s: %v", inst.Config.ID, coin.PairID, err)
		return
	}

	pos := &model.Position{
		BotConfigID:    inst.Config.ID,
		UserID:         inst.Config.UserID,
		Pair:           coin.PairID,
		Status:         model.PositionStatusBuying,
		EntryPrice:     price,
		EntryQuantity:  amount,
		EntryAmountIDR: sizeIDR,
		EntryOrderID:   fmt.Sprintf("%d", res.OrderID),
		EntryPumpScore: coin.PumpScore,
		EntryAt:        time.Now(),
		HighestPrice:   price,
		LowestPrice:    price,
		IsPaperTrade:   inst.Config.IsPaperTrading,
	}

	if err := s.posRepo.Create(ctx, pos); err != nil {
		s.log.Errorf("Bot %d failed to save position: %v", inst.Config.ID, err)
		return
	}

	// Save unified order record
	order := &model.Order{
		UserID:       inst.Config.UserID,
		ParentID:     pos.ID,
		ParentType:   "position",
		OrderID:      pos.EntryOrderID,
		Pair:         pos.Pair,
		Side:         "buy",
		Status:       "open",
		Price:        price,
		Amount:       amount,
		IsPaperTrade: inst.Config.IsPaperTrading,
	}

	if err := s.orderRepo.Create(ctx, order); err != nil {
		s.log.Errorf("Bot %d failed to save unified order: %v", inst.Config.ID, err)
	}

	// Update internal ID
	pos.InternalEntryOrderID = order.ID
	s.posRepo.Update(ctx, pos)

	inst.OpenPositions[pos.ID] = pos

	// Update Balance immediately for prediction
	inst.Config.Balances["idr"] -= sizeIDR
	s.botRepo.UpdateBalance(ctx, inst.Config.ID, inst.Config.Balances)

	s.log.Infof("Bot %d opened position on %s: %.8f @ %.2f", inst.Config.ID, coin.PairID, amount, price)
	
	// Notify order creation and bot update via WebSocket
	s.notificationService.NotifyOrderUpdate(ctx, inst.Config.UserID, order)
	s.notificationService.NotifyBotUpdate(ctx, inst.Config.UserID, model.WSBotUpdatePayload{
		BotID:          inst.Config.ID,
		Status:         inst.Config.Status,
		TotalTrades:    inst.Config.TotalTrades,
		WinningTrades:  inst.Config.WinningTrades,
		WinRate:        inst.Config.WinRate(),
		TotalProfitIDR: inst.Config.TotalProfitIDR,
		Balances:       inst.Config.Balances,
	})

	// Notify via WebSocket
	s.notificationService.NotifyBotUpdate(ctx, inst.Config.UserID, model.WSBotUpdatePayload{
		BotID:          inst.Config.ID,
		Status:         inst.Config.Status,
		TotalTrades:    inst.Config.TotalTrades,
		WinningTrades:  inst.Config.WinningTrades,
		WinRate:        inst.Config.WinRate(),
		TotalProfitIDR: inst.Config.TotalProfitIDR,
		Balances:       inst.Config.Balances,
	})
}

func (s *PumpHunterService) monitorExits(inst *PumpHunterInstance) {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	for _, pos := range inst.OpenPositions {
		if pos.Status != model.PositionStatusOpen {
			continue
		}

		ctx := context.Background()
		coin, err := s.marketDataService.GetCoin(ctx, pos.Pair)
		if err != nil {
			continue
		}

		// Update tracking
		if coin.CurrentPrice > pos.HighestPrice {
			pos.HighestPrice = coin.CurrentPrice
		}
		if coin.CurrentPrice < pos.LowestPrice {
			pos.LowestPrice = coin.CurrentPrice
		}

		// Check conditions
		reason := s.checkExitConditions(inst, pos, coin)
		if reason != "" {
			s.closePosition(inst, pos, coin.CurrentPrice, reason)
		}
	}
}

func (s *PumpHunterService) checkExitConditions(inst *PumpHunterInstance, pos *model.Position, coin *model.Coin) string {
	config := inst.Config.ExitRules
	profitPct := (coin.CurrentPrice - pos.EntryPrice) / pos.EntryPrice * 100

	// 1. Take Profit
	if profitPct >= config.TargetProfitPercent {
		return "take_profit"
	}

	// 2. Stop Loss
	if profitPct <= -config.StopLossPercent {
		return "stop_loss"
	}

	// 3. Trailing Stop
	if config.TrailingStopEnabled && pos.HighestPrice > pos.EntryPrice {
		dropPct := (pos.HighestPrice - coin.CurrentPrice) / pos.HighestPrice * 100
		if dropPct >= config.TrailingStopPercent {
			return "trailing_stop"
		}
	}

	// 4. Max Hold Time
	if config.MaxHoldMinutes > 0 {
		if time.Since(pos.EntryAt) > time.Duration(config.MaxHoldMinutes)*time.Minute {
			return "max_hold_time"
		}
	}

	// 5. Pump Score Drop
	if config.ExitOnPumpScoreDrop && coin.PumpScore < config.PumpScoreDropThreshold {
		return "pump_score_drop"
	}

	return ""
}

func (s *PumpHunterService) closePosition(inst *PumpHunterInstance, pos *model.Position, price float64, reason string) {
	ctx := context.Background()
	res, err := inst.TradeClient.Trade(ctx, "sell", pos.Pair, price, pos.EntryQuantity, "market")
	if err != nil {
		s.log.Errorf("Bot %d failed to close position on %s: %v", inst.Config.ID, pos.Pair, err)
		return
	}

	pos.Status = model.PositionStatusSelling
	pos.ExitOrderID = fmt.Sprintf("%d", res.OrderID)
	pos.CloseReason = reason

	// Save unified order record
	order := &model.Order{
		UserID:       inst.Config.UserID,
		ParentID:     pos.ID,
		ParentType:   "position",
		OrderID:      pos.ExitOrderID,
		Pair:         pos.Pair,
		Side:         "sell",
		Status:       "open",
		Price:        price,
		Amount:       pos.EntryQuantity,
		IsPaperTrade: inst.Config.IsPaperTrading,
	}

	if err := s.orderRepo.Create(ctx, order); err != nil {
		s.log.Errorf("Bot %d failed to save unified exit order: %v", inst.Config.ID, err)
	}

	pos.InternalExitOrderID = order.ID
	s.posRepo.Update(ctx, pos)

	s.log.Infof("Bot %d closing position on %s: reason=%s, price=%.2f", inst.Config.ID, pos.Pair, reason, price)
	
	// Notify order creation via WebSocket
	s.notificationService.NotifyOrderUpdate(ctx, inst.Config.UserID, order)
}

func (s *PumpHunterService) handleOrderFilled(inst *PumpHunterInstance, order *model.Order) {
	// Find position
	inst.mu.Lock()
	defer inst.mu.Unlock()

	var targetPos *model.Position
	for _, pos := range inst.OpenPositions {
		if pos.EntryOrderID == order.OrderID || pos.ExitOrderID == order.OrderID {
			targetPos = pos
			break
		}
	}

	if targetPos == nil {
		return
	}

	ctx := context.Background()
	if order.Side == "buy" {
		targetPos.Status = model.PositionStatusOpen
		s.posRepo.Update(ctx, targetPos)
		
		// Notify order filled via WebSocket
		order.Status = "filled"
		s.notificationService.NotifyOrderUpdate(ctx, inst.Config.UserID, order)

		// Update balance with actual if needed (sync)
		// For now we assume size was already deducted
	} else {
		// Finalize close
		order.Status = "filled"
		s.notificationService.NotifyOrderUpdate(ctx, inst.Config.UserID, order)
		s.finalizePositionClose(inst, targetPos, order.Price)
	}
}

func (s *PumpHunterService) finalizePositionClose(inst *PumpHunterInstance, pos *model.Position, exitPrice float64) {
	ctx := context.Background()

	exitAmount := exitPrice * pos.EntryQuantity
	profitIDR := exitAmount - pos.EntryAmountIDR
	profitPct := profitIDR / pos.EntryAmountIDR * 100

	now := time.Now()
	pos.Status = model.PositionStatusClosed
	pos.ExitPrice = &exitPrice
	pos.ExitQuantity = &pos.EntryQuantity
	pos.ExitAmountIDR = &exitAmount
	pos.ExitAt = &now
	pos.ProfitIDR = &profitIDR
	pos.ProfitPercent = &profitPct

	s.posRepo.Update(ctx, pos)

	// Update Bot Stats
	inst.Config.TotalTrades++
	if profitIDR > 0 {
		inst.Config.WinningTrades++
	} else {
		inst.DailyLoss += math.Abs(profitIDR)
		inst.LastLossTime = time.Now()
	}
	inst.Config.TotalProfitIDR += profitIDR

	// Update Balances
	inst.Config.Balances["idr"] += exitAmount
	s.botRepo.UpdateBalance(ctx, inst.Config.ID, inst.Config.Balances)
	s.botRepo.UpdateStats(ctx, inst.Config.ID, inst.Config.TotalTrades, inst.Config.WinningTrades, inst.Config.TotalProfitIDR)

	delete(inst.OpenPositions, pos.ID)
	s.log.Infof("Bot %d closed position on %s: profit=%.2f (%.2f%%)", inst.Config.ID, pos.Pair, profitIDR, profitPct)

	// Check if max loss limit reached after this trade
	if inst.Config.TotalProfitIDR <= -inst.Config.MaxLossIDR {
		s.log.Warnf("Bot %d reached total max loss limit (%.2f <= -%.2f), stopping bot", 
			inst.Config.ID, inst.Config.TotalProfitIDR, inst.Config.MaxLossIDR)
		// Stop the bot
		go func() {
			if err := s.StopBot(ctx, inst.Config.UserID, inst.Config.ID); err != nil {
				s.log.Errorf("Failed to stop bot %d after max loss: %v", inst.Config.ID, err)
			}
		}()
		return
	}

	// Notify via WebSocket
	s.notificationService.NotifyBotUpdate(ctx, inst.Config.UserID, model.WSBotUpdatePayload{
		BotID:          inst.Config.ID,
		Status:         inst.Config.Status,
		TotalTrades:    inst.Config.TotalTrades,
		WinningTrades:  inst.Config.WinningTrades,
		WinRate:        inst.Config.WinRate(),
		TotalProfitIDR: inst.Config.TotalProfitIDR,
		Balances:       inst.Config.Balances,
	})
}

func (s *PumpHunterService) handleOrderUpdate(userID string, order *indodax.OrderUpdate) {
	if strings.ToLower(order.Status) != "filled" {
		return
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, inst := range s.instances {
		if inst.Config.UserID == userID {
			inst.mu.RLock()
			for _, pos := range inst.OpenPositions {
				if pos.EntryOrderID == order.OrderID || pos.ExitOrderID == order.OrderID {
					inst.mu.RUnlock()

					// Convert indodax.OrderUpdate to model.Order for handleOrderFilled
					price, _ := strconv.ParseFloat(order.Price, 64)
					amount, _ := strconv.ParseFloat(order.ExecutedQty, 64)
					fillTime := time.Unix(order.TransactionTime/1000, 0)

					mOrder := &model.Order{
						OrderID:      order.OrderID,
						Side:         order.Side,
						Price:        price,
						Amount:       amount,
						Status:       "filled",
						FilledAt:     &fillTime,
						IsPaperTrade: inst.Config.IsPaperTrading,
					}
					s.handleOrderFilled(inst, mOrder)
					return
				}
			}
			inst.mu.RUnlock()
		}
	}
}

func (s *PumpHunterService) getTickSize(pair indodax.Pair) float64 {
	val, ok := s.marketDataService.GetPriceIncrement(pair.ID)
	if ok {
		return val
	}
	// Fallback to precision based
	return 1.0 / math.Pow10(pair.PricePrecision)
}

func (s *PumpHunterService) syncBalance(ctx context.Context, inst *PumpHunterInstance) error {
	var realIDR float64
	if !inst.Config.IsPaperTrading {
		info, err := inst.TradeClient.GetInfo(ctx)
		if err != nil {
			return fmt.Errorf("failed to fetch live balance: %w", err)
		}
		realIDR, _ = strconv.ParseFloat(info.Balance["idr"], 64)
	} else {
		// For paper trading, we just use what's in our virtual balance or initial
		if v, ok := inst.Config.Balances["idr"]; ok {
			realIDR = v
		} else {
			realIDR = inst.Config.InitialBalanceIDR
		}
	}

	// Determine IDR Allocation (Cap to initial_balance_idr)
	idrAllocation := inst.Config.InitialBalanceIDR
	if realIDR < idrAllocation {
		idrAllocation = realIDR
	}

	if inst.Config.Balances == nil {
		inst.Config.Balances = make(map[string]float64)
	}

	// Always sync IDR allocation on start
	inst.Config.Balances["idr"] = idrAllocation

	// We do NOT sync coin balances from Indodax because the bot should only
	// sell what it bought itself. Existing coin balances in inst.Config.Balances
	// (purchased by the bot in previous runs) are preserved.

	s.log.Infof("Bot %d balance synced: IDR=%.2f (Allocated=%.2f)", inst.Config.ID, realIDR, idrAllocation)

	return s.botRepo.UpdateBalance(ctx, inst.Config.ID, inst.Config.Balances)
}
