package service

import (
	"context"
	"fmt"
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

type MarketMakerService struct {
	botRepo             *repository.BotRepository
	orderRepo           *repository.OrderRepository
	apiKeyService       *APIKeyService
	marketDataService   *market.MarketDataService
	subManager          *market.SubscriptionManager
	orderMonitor        *OrderMonitor
	notificationService *NotificationService
	indodaxClient       *indodax.Client
	log                 *logger.Logger

	// Runtime bots
	instances map[int64]*BotInstance
	mu        sync.RWMutex
}

// BotInstance represents a running bot in memory
type BotInstance struct {
	Config       *model.BotConfig
	TradeClient  TradeClient
	StopChan     chan struct{}
	TickerChan   chan market.OrderBookTicker
	ActiveOrder  *model.Order
	CurrentBid   float64
	CurrentAsk   float64
	BaseCurrency string // e.g. "btc" in "btcidr"
	LastBuyPrice float64 // Track last buy price for profit calculation
	TotalCoinBought float64 // Track total coins bought (for average price calculation)
	TotalCostIDR float64 // Track total cost in IDR (for average price calculation)
}

func NewMarketMakerService(
	botRepo *repository.BotRepository,
	orderRepo *repository.OrderRepository,
	apiKeyService *APIKeyService,
	marketDataService *market.MarketDataService,
	subManager *market.SubscriptionManager,
	orderMonitor *OrderMonitor,
	notificationService *NotificationService,
	indodaxClient *indodax.Client,
) *MarketMakerService {
	s := &MarketMakerService{
		botRepo:             botRepo,
		orderRepo:           orderRepo,
		apiKeyService:       apiKeyService,
		marketDataService:   marketDataService,
		subManager:          subManager,
		orderMonitor:        orderMonitor,
		notificationService: notificationService,
		indodaxClient:       indodaxClient,
		log:                 logger.GetLogger(),
		instances:           make(map[int64]*BotInstance),
	}

	// Register order update handler for live bots
	orderMonitor.AddOrderHandler(s.handleLiveOrderUpdate)

	return s
}

// CreateBot creates a new market maker bot configuration
func (s *MarketMakerService) CreateBot(ctx context.Context, userID string, req *model.BotConfigRequest) (*model.BotConfig, error) {
	// 1. Validate parameters
	if err := s.validateBotConfig(ctx, req); err != nil {
		return nil, err
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

	// 3. Get pair info to determine base currency
	pairInfo, ok := s.marketDataService.GetPairInfo(req.Pair)
	if !ok {
		return nil, util.ErrBadRequest("Invalid or unsupported pair")
	}

	// 4. Create bot config
	bot := &model.BotConfig{
		UserID:                     userID,
		Name:                       req.Name,
		Type:                       model.BotTypeMarketMaker,
		Pair:                       req.Pair,
		IsPaperTrading:             req.IsPaperTrading,
		APIKeyID:                   apiKeyID,
		InitialBalanceIDR:          req.InitialBalanceIDR,
		OrderSizeIDR:               req.OrderSizeIDR,
		MinGapPercent:              req.MinGapPercent,
		RepositionThresholdPercent: req.RepositionThresholdPercent,
		MaxLossIDR:                 req.MaxLossIDR,
		Status:                     model.BotStatusStopped,
		CreatedAt:                  time.Now(),
		UpdatedAt:                  time.Now(),
	}

	// 5. Initialize balances properly
	bot.Balances = make(map[string]float64)
	
	if req.IsPaperTrading {
		// Paper trading: set IDR balance and coin balance (coin = 0)
		bot.Balances["idr"] = req.InitialBalanceIDR
		bot.Balances[pairInfo.BaseCurrency] = 0
		s.log.Infof("Bot created (paper): IDR=%.2f, %s=0", req.InitialBalanceIDR, pairInfo.BaseCurrency)
	} else {
		// Live trading: set IDR balance only, coin = 0
		// Note: For live trading, actual balance will be synced from Indodax when bot starts
		// But we initialize with the allocated amount for tracking
		bot.Balances["idr"] = req.InitialBalanceIDR
		bot.Balances[pairInfo.BaseCurrency] = 0
		s.log.Infof("Bot created (live): IDR=%.2f (will sync from Indodax on start), %s=0", 
			req.InitialBalanceIDR, pairInfo.BaseCurrency)
	}

	// 6. Save to repository
	if err := s.botRepo.Create(ctx, bot); err != nil {
		s.log.Errorf("Failed to create bot: %v", err)
		return nil, util.ErrInternalServer("Failed to create bot")
	}

	s.log.Infof("Bot created successfully: ID=%d, Name=%s, Type=%s, Pair=%s, PaperTrading=%v", 
		bot.ID, bot.Name, bot.Type, bot.Pair, bot.IsPaperTrading)

	return bot, nil
}

// GetBot retrieves a bot by ID and verifies ownership
func (s *MarketMakerService) GetBot(ctx context.Context, userID string, botID int64) (*model.BotConfig, error) {
	bot, err := s.botRepo.GetByID(ctx, botID)
	if err != nil {
		return nil, util.ErrNotFound("Bot not found")
	}

	if bot.UserID != userID {
		return nil, util.ErrForbidden("Access denied")
	}

	return bot, nil
}

// ListBots lists all bots for a user
func (s *MarketMakerService) ListBots(ctx context.Context, userID string) ([]*model.BotConfig, error) {
	return s.botRepo.ListByUser(ctx, userID)
}

// UpdateBot updates a bot configuration
func (s *MarketMakerService) UpdateBot(ctx context.Context, userID string, botID int64, req *model.BotConfigRequest) (*model.BotConfig, error) {
	bot, err := s.GetBot(ctx, userID, botID)
	if err != nil {
		return nil, err
	}

	if bot.Status == model.BotStatusRunning {
		return nil, util.ErrBadRequest("Cannot update a running bot. Stop it first.")
	}

	// Update fields
	bot.Name = req.Name
	bot.Pair = req.Pair
	bot.OrderSizeIDR = req.OrderSizeIDR
	bot.MinGapPercent = req.MinGapPercent
	bot.RepositionThresholdPercent = req.RepositionThresholdPercent
	bot.MaxLossIDR = req.MaxLossIDR
	bot.IsPaperTrading = req.IsPaperTrading
	bot.APIKeyID = req.APIKeyID
	bot.InitialBalanceIDR = req.InitialBalanceIDR

	if err := s.botRepo.Update(ctx, bot, ""); err != nil {
		return nil, err
	}

	return bot, nil
}

// DeleteBot deletes a bot
func (s *MarketMakerService) DeleteBot(ctx context.Context, userID string, botID int64) error {
	bot, err := s.GetBot(ctx, userID, botID)
	if err != nil {
		return err
	}

	if bot.Status == model.BotStatusRunning {
		return util.ErrBadRequest("Cannot delete a running bot. Stop it first.")
	}

	// Delete all orders associated with this bot
	orders, err := s.orderRepo.ListByParentAndUser(ctx, userID, "bot", botID)
	if err != nil {
		s.log.Warnf("Failed to list orders for bot %d: %v", botID, err)
	} else {
		s.log.Infof("Deleting %d orders for bot %d", len(orders), botID)
		for _, order := range orders {
			if err := s.orderRepo.Delete(ctx, order.ID); err != nil {
				s.log.Warnf("Failed to delete order %d for bot %d: %v", order.ID, botID, err)
			}
		}
	}

	return s.botRepo.Delete(ctx, botID)
}

// StartBot starts a bot instance
func (s *MarketMakerService) StartBot(ctx context.Context, userID string, botID int64) error {
	bot, err := s.GetBot(ctx, userID, botID)
	if err != nil {
		return err
	}

	if bot.Status == model.BotStatusRunning {
		return util.ErrBadRequest("Bot is already running")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. Create instance
	inst := &BotInstance{
		Config:     bot,
		StopChan:   make(chan struct{}),
		TickerChan: make(chan market.OrderBookTicker, 10),
	}

	// 2. Setup Trade Client
	if bot.IsPaperTrading {
		inst.TradeClient = NewPaperTradeClient(bot.Balances, func(order *model.Order) {
			s.handleFilled(inst, order)
		})
	} else {
		// Get API keys
		key, err := s.apiKeyService.GetDecrypted(ctx, userID)
		if err != nil {
			return util.ErrBadRequest("Valid API key not found")
		}
		inst.TradeClient = NewLiveTradeClient(s.indodaxClient, key.Key, key.Secret)
		// Subscribe to order updates for this user
		if err := s.orderMonitor.SubscribeUserOrders(ctx, userID); err != nil {
			s.log.Warnf("Failed to subscribe to order updates for user %s: %v", userID, err)
		}
	}

	// 3. Set Base Currency
	pairInfo, ok := s.marketDataService.GetPairInfo(bot.Pair)
	if !ok {
		return util.ErrBadRequest("Invalid pair")
	}
	
	// Derive base currency from pair ID (more reliable than API field)
	// For pairs like "cstidr", base currency is "cst" (everything before "idr")
	if strings.HasSuffix(bot.Pair, "idr") {
		inst.BaseCurrency = strings.TrimSuffix(bot.Pair, "idr")
	} else if strings.Contains(bot.Pair, "_") {
		// For pairs like "btc_usdt", take the first part
		parts := strings.Split(bot.Pair, "_")
		inst.BaseCurrency = parts[0]
	} else {
		// Fallback to API field if format is unknown
		inst.BaseCurrency = pairInfo.BaseCurrency
	}
	
	if inst.BaseCurrency == "idr" || inst.BaseCurrency == "" {
		s.log.Errorf("Bot %d: Invalid BaseCurrency '%s' for pair %s (derived from pair ID)", botID, inst.BaseCurrency, bot.Pair)
		return util.ErrBadRequest(fmt.Sprintf("Invalid base currency: %s", inst.BaseCurrency))
	}
	s.log.Debugf("Bot %d: BaseCurrency set to '%s' for pair %s (API had: %s)", botID, inst.BaseCurrency, bot.Pair, pairInfo.BaseCurrency)

	// 3.5. Initialize balances if needed (for paper trading)
	if bot.IsPaperTrading {
		s.log.Infof("Bot %d: Initializing balances for paper trading (InitialBalanceIDR=%.2f)", botID, bot.InitialBalanceIDR)
		
		// Initialize balances map if nil
		if bot.Balances == nil {
			bot.Balances = make(map[string]float64)
		}
		
		// Get current balances
		currentIDR := bot.Balances["idr"]
		currentCoin := bot.Balances[inst.BaseCurrency]
		
		s.log.Infof("Bot %d: Current balances before reset - IDR=%.2f, %s=%.8f", 
			botID, currentIDR, inst.BaseCurrency, currentCoin)
		
		// For paper trading, always validate and reset if corrupted
		// Reset if: negative, zero (when initial > 0), or unreasonably large
		needsReset := false
		if currentIDR < 0 {
			s.log.Warnf("Bot %d: IDR balance is negative (%.2f), resetting", botID, currentIDR)
			needsReset = true
		} else if bot.InitialBalanceIDR > 0 && (currentIDR == 0 || currentIDR > bot.InitialBalanceIDR*10 || currentIDR > 1000000000) {
			s.log.Warnf("Bot %d: IDR balance looks corrupted (%.2f), resetting to initial (%.2f)", 
				botID, currentIDR, bot.InitialBalanceIDR)
			needsReset = true
		}
		
		if currentCoin < 0 || currentCoin > 1000000 {
			s.log.Warnf("Bot %d: %s balance looks corrupted (%.8f), resetting to 0", 
				botID, inst.BaseCurrency, currentCoin)
			needsReset = true
		}
		
		// Always ensure both keys exist and have valid values
		if needsReset || bot.Balances["idr"] == 0 {
			bot.Balances["idr"] = bot.InitialBalanceIDR
			bot.Balances[inst.BaseCurrency] = 0
			s.log.Warnf("Bot %d: RESET balances to initial - IDR=%.2f, %s=0", 
				botID, bot.InitialBalanceIDR, inst.BaseCurrency)
		} else {
			// Just ensure coin balance exists
			if _, exists := bot.Balances[inst.BaseCurrency]; !exists {
				bot.Balances[inst.BaseCurrency] = 0
			}
		}
		
		s.log.Infof("Bot %d: Final balances after initialization - IDR=%.2f, %s=%.8f", 
			botID, bot.Balances["idr"], inst.BaseCurrency, bot.Balances[inst.BaseCurrency])
		
		// Save corrected balances to Redis
		if err := s.botRepo.UpdateBalance(ctx, botID, bot.Balances); err != nil {
			s.log.Errorf("Bot %d: Failed to save corrected balances to Redis: %v", botID, err)
		} else {
			s.log.Infof("Bot %d: Successfully saved corrected balances to Redis", botID)
		}
	}

	s.instances[botID] = inst

	// 3.6. Cancel any existing active orders for this bot (safety measure)
	if bot.IsPaperTrading {
		// For paper trading, we can't cancel via API, but we can clear the active order reference
		// This prevents old corrupted orders from affecting the bot
		inst.ActiveOrder = nil
		s.log.Infof("Bot %d: Cleared any existing active order references", botID)
	}

	// 4. Subscribe to ticker
	s.log.Infof("Bot %d: Subscribing to ticker for pair %s", botID, bot.Pair)
	err = s.subManager.Subscribe(bot.Pair, func(ticker market.OrderBookTicker) {
		s.log.Debugf("Bot %d: Received ticker update via subscription - bid=%.2f ask=%.2f", botID, ticker.BestBid, ticker.BestAsk)
		select {
		case inst.TickerChan <- ticker:
		default:
			// Full channel, skip ticker
			s.log.Warnf("Bot %d: Ticker channel full, skipping update", botID)
		}
	})
	if err != nil {
		s.log.Errorf("Bot %d: Failed to subscribe to ticker: %v", botID, err)
		delete(s.instances, botID)
		return util.ErrInternalServer(fmt.Sprintf("Failed to subscribe to ticker: %v", err))
	}
	s.log.Infof("Bot %d: Successfully subscribed to ticker for pair %s", botID, bot.Pair)

	// 5. Initial balance sync for live bots
	if !bot.IsPaperTrading {
		if err := s.syncBalance(ctx, inst); err != nil {
			s.log.Warnf("Failed to initial sync balance for bot %d: %v", botID, err)
		}
	}

	// 3. Update status in DB
	if err := s.botRepo.UpdateStatus(ctx, botID, model.BotStatusRunning, nil); err != nil {
		s.subManager.Unsubscribe(bot.Pair, nil) // Should pass exact handler but we'll optimize later
		delete(s.instances, botID)
		return err
	}
	// Update in-memory status to match database
	inst.Config.Status = model.BotStatusRunning

	// 4. Start event loop
	go s.runBot(inst)

	s.log.Infof("Bot %d started successfully for pair %s", botID, bot.Pair)

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

	return nil
}

// StopBot stops a bot instance
func (s *MarketMakerService) StopBot(ctx context.Context, userID string, botID int64) error {
	bot, err := s.GetBot(ctx, userID, botID)
	if err != nil {
		return err
	}

	if bot.Status != model.BotStatusRunning {
		return util.ErrBadRequest("Bot is not running")
	}

	s.mu.Lock()
	inst, ok := s.instances[botID]
	if !ok {
		s.mu.Unlock()
		// Force update status if instance is gone but status says running
		s.botRepo.UpdateStatus(ctx, botID, model.BotStatusStopped, nil)
		return nil
	}
	delete(s.instances, botID)
	s.mu.Unlock()

	// 1. Cancel any active/open orders
	if inst.ActiveOrder != nil && inst.ActiveOrder.Status == "open" {
		s.log.Infof("Bot %d: Cancelling active order %s (ID: %s)", botID, inst.ActiveOrder.Side, inst.ActiveOrder.OrderID)
		if err := inst.TradeClient.CancelOrder(ctx, inst.Config.Pair, inst.ActiveOrder.OrderID, inst.ActiveOrder.Side); err != nil {
			s.log.Warnf("Bot %d: Failed to cancel active order %s: %v", botID, inst.ActiveOrder.OrderID, err)
		} else {
			s.orderRepo.UpdateStatus(ctx, inst.ActiveOrder.ID, "cancelled")
			inst.ActiveOrder.Status = "cancelled"
			s.notificationService.NotifyOrderUpdate(ctx, userID, inst.ActiveOrder)
		}
		inst.ActiveOrder = nil
	}

	// Also check for any other open orders for this bot
	openOrders, err := s.orderRepo.ListByParentAndUser(ctx, userID, "bot", botID)
	if err == nil {
		for _, order := range openOrders {
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

	// 2. Signal stop
	close(inst.StopChan)

	// 3. Unsubscribe (needs exact handler, for now we will fix later)
	// s.subManager.Unsubscribe(bot.Pair, handler)

	// 4. Update status in DB
	err = s.botRepo.UpdateStatus(ctx, botID, model.BotStatusStopped, nil)
	// Update in-memory status to match database
	if inst != nil {
		inst.Config.Status = model.BotStatusStopped
	}

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

func (s *MarketMakerService) runBot(inst *BotInstance) {
	s.log.Infof("Starting event loop for bot %d", inst.Config.ID)
	defer s.log.Infof("Event loop stopped for bot %d", inst.Config.ID)

	for {
		select {
		case <-inst.StopChan:
			return
		case ticker := <-inst.TickerChan:
			s.handleTicker(inst, ticker)
		}
	}
}

func (s *MarketMakerService) handleTicker(inst *BotInstance, ticker market.OrderBookTicker) {
	s.log.Debugf("Bot %d received ticker: bid=%.2f ask=%.2f", inst.Config.ID, ticker.BestBid, ticker.BestAsk)

	// 2. Update prices first (needed for decision making)
	inst.CurrentBid = ticker.BestBid
	inst.CurrentAsk = ticker.BestAsk

	// 3. Check minimum gap
	spreadPercent := (ticker.BestAsk - ticker.BestBid) / ticker.BestBid * 100
	if spreadPercent < inst.Config.MinGapPercent {
		s.log.Debugf("Bot %d: Spread too tight (%.4f%% < %.4f%%), skipping", inst.Config.ID, spreadPercent, inst.Config.MinGapPercent)
		return // Spread too tight
	}

	s.log.Debugf("Bot %d: Gap OK (%.4f%% >= %.4f%%), processing orders", inst.Config.ID, spreadPercent, inst.Config.MinGapPercent)

	// 1. Check volatility - only skip SELL if volatile AND we're at a loss
	coin, err := s.marketDataService.GetCoin(context.Background(), inst.Config.Pair)
	if err == nil && coin.Volatility1m > 2.0 {
		// Determine if we would be selling
		coinBalance := inst.Config.Balances[inst.BaseCurrency]
		if coinBalance > 0 {
			// We have coins, so we would be selling
			// Check if we're at profit or loss
			if inst.LastBuyPrice > 0 {
				// Compare current ask price (what we'd sell at) vs buy price
				currentSellPrice := ticker.BestAsk
				profitPercent := ((currentSellPrice - inst.LastBuyPrice) / inst.LastBuyPrice) * 100
				
				if profitPercent < 0 {
					// We're at a loss - skip selling during high volatility
					s.log.Debugf("Bot %d: Too volatile (%.2f%%) and at loss (%.2f%%), skipping SELL", 
						inst.Config.ID, coin.Volatility1m, profitPercent)
					return
				} else {
					// We're at profit - volatility is fine, proceed with selling
					s.log.Debugf("Bot %d: Volatile (%.2f%%) but at profit (%.2f%%), proceeding with SELL", 
						inst.Config.ID, coin.Volatility1m, profitPercent)
				}
			} else {
				// No buy price tracked yet - skip selling during high volatility to be safe
				s.log.Debugf("Bot %d: Too volatile (%.2f%%) for SELL (no buy price tracked), skipping", 
					inst.Config.ID, coin.Volatility1m)
				return
			}
		} else {
			// We would be buying - volatility is OK, proceed
			s.log.Debugf("Bot %d: Volatile (%.2f%%) but buying is OK, proceeding", inst.Config.ID, coin.Volatility1m)
		}
	}

	// 4. Process orders
	if inst.ActiveOrder == nil {
		s.placeNewOrder(inst)
	} else {
		s.checkReposition(inst)
	}
}

func (s *MarketMakerService) placeNewOrder(inst *BotInstance) {
	s.log.Debugf("Bot %d: placeNewOrder called", inst.Config.ID)

	// Ensure balances map is initialized
	if inst.Config.Balances == nil {
		inst.Config.Balances = make(map[string]float64)
		inst.Config.Balances["idr"] = inst.Config.InitialBalanceIDR
		inst.Config.Balances[inst.BaseCurrency] = 0
		s.log.Warnf("Bot %d: Balances map was nil, reinitialized", inst.Config.ID)
	}

	// Determine side based on inventory
	idrBalance := inst.Config.Balances["idr"]
	if idrBalance < 0 {
		idrBalance = 0
		inst.Config.Balances["idr"] = 0
		s.log.Warnf("Bot %d: IDR balance was negative, reset to 0", inst.Config.ID)
	}

	coinBalance, exists := inst.Config.Balances[inst.BaseCurrency]
	if !exists {
		coinBalance = 0
		inst.Config.Balances[inst.BaseCurrency] = 0
	}
	if coinBalance < 0 {
		coinBalance = 0
		inst.Config.Balances[inst.BaseCurrency] = 0
		s.log.Warnf("Bot %d: %s balance was negative, reset to 0", inst.Config.ID, inst.BaseCurrency)
	}

	s.log.Debugf("Bot %d: Balance check - IDR=%.2f, %s=%.8f (BaseCurrency=%s, all balances: %+v)", 
		inst.Config.ID, idrBalance, inst.BaseCurrency, coinBalance, inst.BaseCurrency, inst.Config.Balances)

	pairInfo, ok := s.marketDataService.GetPairInfo(inst.Config.Pair)
	if !ok {
		s.log.Warnf("Bot %d: Pair info not found for %s", inst.Config.ID, inst.Config.Pair)
		return
	}
	
	// Use VolumePrecision if valid, otherwise fallback to PriceRound, then default to 8
	volumePrecision := pairInfo.VolumePrecision
	if volumePrecision <= 0 {
		if pairInfo.PriceRound > 0 {
			volumePrecision = pairInfo.PriceRound
			s.log.Debugf("Bot %d: VolumePrecision is 0, using PriceRound=%d from API", inst.Config.ID, volumePrecision)
		} else {
			volumePrecision = 8 // Final fallback to 8 decimal places for crypto
			s.log.Debugf("Bot %d: VolumePrecision and PriceRound are both 0, using default 8", inst.Config.ID)
		}
	}
	
	s.log.Debugf("Bot %d: Pair info - VolumePrecision=%d, TradeMinBaseCurrency=%d, TradeMinTradedCurrency=%.2f", 
		inst.Config.ID, pairInfo.VolumePrecision, pairInfo.TradeMinBaseCurrency, pairInfo.TradeMinTradedCurrency)

	var side string
	var price, amount float64

	// Safety check: ensure coinBalance is reasonable (not corrupted)
	if coinBalance > 1000000 { // More than 1 million coins is definitely wrong
		s.log.Errorf("Bot %d: Coin balance is unreasonably large (%.8f), resetting to 0", inst.Config.ID, coinBalance)
		coinBalance = 0
		inst.Config.Balances[inst.BaseCurrency] = 0
	}
	
	// Decision logic:
	// - If we have coins: SELL ALL available coin balance (with stop-loss check)
	// - If we have IDR: BUY using OrderSizeIDR / price
	if coinBalance > 0 {
		// Have coins -> SELL ALL available balance
		// Calculate sell price first
		sellPrice := inst.CurrentAsk - s.getTickSize(pairInfo) // Competitive sell
		
		// Check stop-loss before selling (prevent selling at large loss when price keeps dropping)
		if inst.LastBuyPrice > 0 {
			// Calculate current profit/loss percentage
			profitPercent := ((sellPrice - inst.LastBuyPrice) / inst.LastBuyPrice) * 100
			
			// If loss exceeds 5%, skip selling to avoid realizing large losses
			// This prevents selling at a large loss when price keeps dropping
			// The bot will wait for price recovery or hit the total MaxLossIDR limit
			if profitPercent < -5.0 {
				s.log.Debugf("Bot %d: Skipping SELL - at loss (%.2f%%) and price may recover, holding", 
					inst.Config.ID, profitPercent)
				return
			}
			
			s.log.Debugf("Bot %d: SELL check - buyPrice=%.2f, sellPrice=%.2f, profit=%.2f%%", 
				inst.Config.ID, inst.LastBuyPrice, sellPrice, profitPercent)
		}
		
		side = "sell"
		price = sellPrice
		amount = coinBalance // Sell all available coins
		
		// Safety check: don't sell if amount is unreasonably large (corruption protection)
		if amount > 1000000 {
			s.log.Errorf("Bot %d: Refusing to place SELL order - amount %.8f is unreasonably large", inst.Config.ID, amount)
			return
		}
		
		s.log.Debugf("Bot %d: Placing SELL order - price=%.2f amount=%.8f (all available)", inst.Config.ID, price, amount)
	} else if idrBalance >= inst.Config.OrderSizeIDR {
		// Have IDR -> BUY
		side = "buy"
		price = inst.CurrentBid + s.getTickSize(pairInfo) // Competitive buy
		amount = inst.Config.OrderSizeIDR / price
		
		// Safety check: don't buy more than we can afford
		maxAffordable := idrBalance / price
		if amount > maxAffordable {
			s.log.Warnf("Bot %d: Attempted to buy %.8f but can only afford %.8f, capping", inst.Config.ID, amount, maxAffordable)
			amount = maxAffordable
		}
		
		s.log.Debugf("Bot %d: Placing BUY order - price=%.2f amount=%.8f (size=%.2f IDR)", inst.Config.ID, price, amount, inst.Config.OrderSizeIDR)
	} else {
		// Insufficient balance
		s.log.Debugf("Bot %d: Insufficient balance - IDR=%.2f < %.2f, %s=%.8f", 
			inst.Config.ID, idrBalance, inst.Config.OrderSizeIDR, inst.BaseCurrency, coinBalance)
		return
	}

	// Round amount and validate
	
	s.log.Debugf("Bot %d: Before rounding - amount=%.8f, volumePrecision=%d", inst.Config.ID, amount, volumePrecision)
	amount = util.FloorToPrecision(amount, volumePrecision)
	s.log.Debugf("Bot %d: After rounding - amount=%.8f", inst.Config.ID, amount)
	
	// Validate minimum order value (in IDR)
	// Since we trade based on IDR order size, we only need to check TradeMinTradedCurrency
	orderValueIDR := amount * price
	if orderValueIDR < pairInfo.TradeMinTradedCurrency {
		s.log.Debugf("Bot %d: Amount validation failed - order value=%.2f IDR < min traded currency=%.2f IDR (amount=%.8f * price=%.2f)", 
			inst.Config.ID, orderValueIDR, pairInfo.TradeMinTradedCurrency, amount, price)
		return
	}
	
	s.log.Debugf("Bot %d: Order validation passed - amount=%.8f, price=%.2f, value=%.2f IDR (min=%.2f IDR)", 
		inst.Config.ID, amount, price, orderValueIDR, pairInfo.TradeMinTradedCurrency)

	s.log.Infof("Bot %d: Placing %s order - pair=%s price=%.2f amount=%.8f", inst.Config.ID, side, inst.Config.Pair, price, amount)

	// Place order
	ctx := context.Background()
	res, err := inst.TradeClient.Trade(ctx, side, inst.Config.Pair, price, amount, "limit")
	if err != nil {
		s.log.Errorf("Failed to place %s order for bot %d: %v", side, inst.Config.ID, err)
		return
	}

	s.log.Infof("Bot %d: Successfully placed %s order - OrderID=%d", inst.Config.ID, side, res.OrderID)

	// Create order model
	order := &model.Order{
		UserID:       inst.Config.UserID,
		ParentID:     inst.Config.ID,
		ParentType:   "bot",
		OrderID:      fmt.Sprintf("%d", res.OrderID),
		Pair:         inst.Config.Pair,
		Side:         side,
		Status:       "open",
		Price:        price,
		Amount:       amount,
		IsPaperTrade: inst.Config.IsPaperTrading,
	}

	if err := s.orderRepo.Create(ctx, order); err != nil {
		s.log.Errorf("Failed to save order for bot %d: %v", inst.Config.ID, err)
		return
	}

	inst.ActiveOrder = order
	s.log.Infof("Bot %d placed %s order: %.8f @ %.2f", inst.Config.ID, side, amount, price)
	
	// Notify order creation via WebSocket
	s.notificationService.NotifyOrderUpdate(ctx, inst.Config.UserID, order)
	
	// Also notify bot update with current stats and balances
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

func (s *MarketMakerService) checkReposition(inst *BotInstance) {
	order := inst.ActiveOrder
	var currentMarketPrice float64
	if order.Side == "buy" {
		currentMarketPrice = inst.CurrentBid
	} else {
		currentMarketPrice = inst.CurrentAsk
	}

	diff := util.Abs(order.Price-currentMarketPrice) / order.Price * 100
	if diff > inst.Config.RepositionThresholdPercent {
		s.log.Infof("Price moved %.2f%%, repositioning %s order for bot %d", diff, order.Side, inst.Config.ID)

		ctx := context.Background()
		if err := inst.TradeClient.CancelOrder(ctx, inst.Config.Pair, order.OrderID, order.Side); err != nil {
			s.log.Errorf("Failed to cancel order for bot %d: %v", inst.Config.ID, err)
			return
		}

		s.orderRepo.UpdateStatus(ctx, order.ID, "cancelled")
		order.Status = "cancelled"
		
		// Notify order cancellation via WebSocket
		s.notificationService.NotifyOrderUpdate(ctx, inst.Config.UserID, order)
		
		inst.ActiveOrder = nil
	}
}

func (s *MarketMakerService) handleFilled(inst *BotInstance, filledOrder *model.Order) {
	ctx := context.Background()

	// If the order doesn't have a database ID, look it up by OrderID (for paper trading)
	if filledOrder.ID == 0 && filledOrder.OrderID != "" {
		dbOrder, err := s.orderRepo.GetByOrderID(ctx, filledOrder.OrderID)
		if err != nil {
			s.log.Errorf("Bot %d: Failed to find order by OrderID %s: %v", inst.Config.ID, filledOrder.OrderID, err)
			return
		}
		// Update filledOrder with database order details
		filledOrder.ID = dbOrder.ID
		filledOrder.Pair = dbOrder.Pair
		filledOrder.Side = dbOrder.Side
		filledOrder.Price = dbOrder.Price
		filledOrder.Amount = dbOrder.Amount
		s.log.Debugf("Bot %d: Found order in database - ID=%d, OrderID=%s", inst.Config.ID, filledOrder.ID, filledOrder.OrderID)
	}

	// Ensure balances map is initialized
	if inst.Config.Balances == nil {
		inst.Config.Balances = make(map[string]float64)
		inst.Config.Balances["idr"] = inst.Config.InitialBalanceIDR
		inst.Config.Balances[inst.BaseCurrency] = 0
	}

	// Use FilledAmount if available, otherwise use Amount
	filledAmount := filledOrder.FilledAmount
	if filledAmount == 0 {
		filledAmount = filledOrder.Amount
	}

	// Safety check: reject unreasonably large amounts (likely corrupted orders)
	if filledAmount > 1000000 {
		s.log.Errorf("Bot %d: Rejecting order fill - amount %.8f is unreasonably large (order likely corrupted)", 
			inst.Config.ID, filledAmount)
		return
	}
	
	// Safety check: reject unreasonably large prices
	if filledOrder.Price > 1000000000 {
		s.log.Errorf("Bot %d: Rejecting order fill - price %.2f is unreasonably large (order likely corrupted)", 
			inst.Config.ID, filledOrder.Price)
		return
	}

	// Calculate total cost/value
	totalValue := filledAmount * filledOrder.Price
	
	// Safety check: reject unreasonably large total values
	if totalValue > 10000000000 { // More than 10 billion IDR
		s.log.Errorf("Bot %d: Rejecting order fill - total value %.2f is unreasonably large (order likely corrupted)", 
			inst.Config.ID, totalValue)
		return
	}

	s.log.Debugf("Bot %d: handleFilled - side=%s filledAmount=%.8f price=%.2f totalValue=%.2f, before: IDR=%.2f %s=%.8f", 
		inst.Config.ID, filledOrder.Side, filledAmount, filledOrder.Price, totalValue,
		inst.Config.Balances["idr"], inst.BaseCurrency, inst.Config.Balances[inst.BaseCurrency])

	// 1. Update balance
	if filledOrder.Side == "buy" {
		// Buy: spend IDR, receive coins
		inst.Config.Balances["idr"] -= totalValue
		if inst.Config.Balances["idr"] < 0 {
			s.log.Warnf("Bot %d: IDR balance went negative (%.2f), resetting to 0", inst.Config.ID, inst.Config.Balances["idr"])
			inst.Config.Balances["idr"] = 0
		}
		inst.Config.Balances[inst.BaseCurrency] += filledAmount
		// Track buy price and cost for profit calculation (weighted average)
		inst.TotalCoinBought += filledAmount
		inst.TotalCostIDR += totalValue
		inst.LastBuyPrice = inst.TotalCostIDR / inst.TotalCoinBought // Average buy price
		s.log.Debugf("Bot %d: After BUY - IDR=%.2f %s=%.8f (avg buy price: %.2f, total coins: %.8f, total cost: %.2f)", 
			inst.Config.ID, inst.Config.Balances["idr"], inst.BaseCurrency, inst.Config.Balances[inst.BaseCurrency], 
			inst.LastBuyPrice, inst.TotalCoinBought, inst.TotalCostIDR)
	} else {
		// Sell: spend coins, receive IDR
		if inst.Config.Balances[inst.BaseCurrency] < filledAmount {
			s.log.Warnf("Bot %d: Attempted to sell %.8f %s but only have %.8f, capping to available", 
				inst.Config.ID, filledAmount, inst.BaseCurrency, inst.Config.Balances[inst.BaseCurrency])
			filledAmount = inst.Config.Balances[inst.BaseCurrency]
			totalValue = filledAmount * filledOrder.Price
		}
		inst.Config.Balances[inst.BaseCurrency] -= filledAmount
		if inst.Config.Balances[inst.BaseCurrency] < 0 {
			inst.Config.Balances[inst.BaseCurrency] = 0
		}
		inst.Config.Balances["idr"] += totalValue
		
		// Update tracking: reduce coins and cost proportionally
		if inst.TotalCoinBought > 0 {
			// Calculate proportion of coins being sold
			sellRatio := filledAmount / inst.TotalCoinBought
			if sellRatio > 1.0 {
				sellRatio = 1.0 // Cap at 100% if somehow we're selling more than we bought
			}
			// Reduce total cost proportionally
			costOfSoldCoins := inst.TotalCostIDR * sellRatio
			inst.TotalCostIDR -= costOfSoldCoins
			inst.TotalCoinBought -= filledAmount
			if inst.TotalCoinBought < 0 {
				inst.TotalCoinBought = 0
			}
			// Recalculate average buy price for remaining coins
			if inst.TotalCoinBought > 0 {
				inst.LastBuyPrice = inst.TotalCostIDR / inst.TotalCoinBought
			} else {
				inst.LastBuyPrice = 0
				inst.TotalCostIDR = 0
			}
		}
		
		s.log.Debugf("Bot %d: After SELL - IDR=%.2f %s=%.8f (added %.2f IDR, remaining coins: %.8f, remaining cost: %.2f)", 
			inst.Config.ID, inst.Config.Balances["idr"], inst.BaseCurrency, inst.Config.Balances[inst.BaseCurrency],
			totalValue, inst.TotalCoinBought, inst.TotalCostIDR)

		// 2. Calculate and update profit
		inst.Config.TotalTrades++
		profit := s.calculateProfit(inst, filledOrder, filledAmount)
		inst.Config.TotalProfitIDR += profit
		if profit > 0 {
			inst.Config.WinningTrades++
		}
		
		s.log.Infof("Bot %d: SELL profit calculated - sellPrice=%.2f, avgBuyPrice=%.2f, amount=%.8f, profit=%.2f IDR, totalProfit=%.2f IDR", 
			inst.Config.ID, filledOrder.Price, inst.LastBuyPrice, filledAmount, profit, inst.Config.TotalProfitIDR)

		// 3. Circuit breaker check
		if inst.Config.TotalProfitIDR < -inst.Config.MaxLossIDR {
			s.log.Warnf("Bot %d reached max loss limit: %.2f", inst.Config.ID, inst.Config.TotalProfitIDR)
			s.StopBot(ctx, inst.Config.UserID, inst.Config.ID)
			return
		}
	}

	// Save balance and stats
	s.botRepo.UpdateBalance(ctx, inst.Config.ID, inst.Config.Balances)
	s.botRepo.UpdateStats(ctx, inst.Config.ID, inst.Config.TotalTrades, inst.Config.WinningTrades, inst.Config.TotalProfitIDR)

	// Update order status and filled amount in database
	if filledOrder.ID > 0 {
		// Get the order from database to update it
		dbOrder, err := s.orderRepo.GetByID(ctx, filledOrder.ID)
		if err != nil {
			s.log.Errorf("Bot %d: Failed to get order %d for update: %v", inst.Config.ID, filledOrder.ID, err)
		} else {
			// Update filled amount and status
			dbOrder.FilledAmount = filledAmount
			dbOrder.Status = "filled"
			now := time.Now()
			dbOrder.FilledAt = &now
			
			// Save updated order
			if err := s.orderRepo.Update(ctx, dbOrder, "open"); err != nil {
				s.log.Errorf("Bot %d: Failed to update order %d: %v", inst.Config.ID, filledOrder.ID, err)
			} else {
				s.log.Debugf("Bot %d: Order filled - ID=%d, filledAmount=%.8f (original=%.8f)", 
					inst.Config.ID, filledOrder.ID, filledAmount, filledOrder.Amount)
				// Update filledOrder reference for WebSocket notification
				filledOrder.FilledAmount = filledAmount
				filledOrder.Status = "filled"
				filledOrder.FilledAt = &now
			}
		}
	} else {
		s.log.Warnf("Bot %d: Cannot update order status - order has no database ID (OrderID=%s)", 
			inst.Config.ID, filledOrder.OrderID)
		filledOrder.Status = "filled"
		filledOrder.FilledAmount = filledAmount
	}
	inst.ActiveOrder = nil

	s.log.Infof("Bot %d order filled: %s %.8f @ %.2f", inst.Config.ID, filledOrder.Side, filledAmount, filledOrder.Price)

	// Notify order update via WebSocket
	s.notificationService.NotifyOrderUpdate(ctx, inst.Config.UserID, filledOrder)

	// Notify bot update with current stats and balances
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

func (s *MarketMakerService) handleLiveOrderUpdate(userID string, order *indodax.OrderUpdate) {
	if strings.ToLower(order.Status) != "filled" {
		return
	}

	// Check all running bots for this user
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, inst := range s.instances {
		if inst.Config.UserID == userID && inst.ActiveOrder != nil && inst.ActiveOrder.OrderID == order.OrderID {
			// Found the bot!
			s.handleFilled(inst, inst.ActiveOrder)
			break
		}
	}
}

func (s *MarketMakerService) getTickSize(pair indodax.Pair) float64 {
	// Simple tick size implementation
	// In production, fetch from price_increments or calculate based on precision
	return 1.0 / util.Pow10(pair.PricePrecision)
}

func (s *MarketMakerService) syncBalance(ctx context.Context, inst *BotInstance) error {
	// For live bots, we check real IDR on Indodax to ensure we don't allocate more than exists
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

	// 1. Determine IDR Allocation (Cap to initial_balance_idr)
	idrAllocation := inst.Config.InitialBalanceIDR
	if realIDR < idrAllocation {
		idrAllocation = realIDR
	}

	// 2. Maintain Virtual Balance Isolation
	// We do NOT sync coin balances from Indodax because the bot should only
	// sell what it bought itself.
	if inst.Config.Balances == nil {
		inst.Config.Balances = make(map[string]float64)
	}

	// Always sync IDR allocation on start
	inst.Config.Balances["idr"] = idrAllocation

	// BaseCurrency balance stays as it is in Redis (our virtual record of what the bot bought)
	// If it's a first-time start, it will be 0.
	if _, ok := inst.Config.Balances[inst.BaseCurrency]; !ok {
		inst.Config.Balances[inst.BaseCurrency] = 0
	}

	s.log.Infof("Bot %d balance synced: IDR=%.2f (Allocated=%.2f), %s=%.8f (Isolated)",
		inst.Config.ID, realIDR, idrAllocation, inst.BaseCurrency, inst.Config.Balances[inst.BaseCurrency])

	return s.botRepo.UpdateBalance(ctx, inst.Config.ID, inst.Config.Balances)
}

func (s *MarketMakerService) calculateProfit(inst *BotInstance, sellOrder *model.Order, sellAmount float64) float64 {
	// Calculate profit: (SellPrice - BuyPrice) * Amount - Fees
	// Fees: 0.1% on buy + 0.1% on sell = 0.2% total
	const feePercent = 0.002 // 0.2% total fees
	
	if inst.LastBuyPrice == 0 {
		// No buy price tracked, can't calculate profit accurately
		// This shouldn't happen in normal flow, but handle gracefully
		s.log.Warnf("Bot %d: Cannot calculate profit - no buy price tracked", inst.Config.ID)
		return 0.0
	}
	
	// Gross profit before fees
	grossProfit := (sellOrder.Price - inst.LastBuyPrice) * sellAmount
	
	// Calculate fees
	buyCost := inst.LastBuyPrice * sellAmount
	sellRevenue := sellOrder.Price * sellAmount
	fees := (buyCost + sellRevenue) * feePercent
	
	// Net profit
	netProfit := grossProfit - fees
	
	return netProfit
}

func (s *MarketMakerService) validateBotConfig(ctx context.Context, req *model.BotConfigRequest) error {
	// Validate pair
	if req.Pair == "" {
		return util.ErrBadRequest("Pair is required")
	}

	// Check if pair exists in market metadata
	_, ok := s.marketDataService.GetPairInfo(req.Pair)
	if !ok {
		return util.ErrBadRequest(fmt.Sprintf("Invalid or unsupported pair: %s", req.Pair))
	}

	// Validate numeric parameters
	if req.InitialBalanceIDR < 50000 {
		return util.ErrBadRequest("Initial balance must be at least 50,000 IDR")
	}
	if req.OrderSizeIDR < 10000 {
		return util.ErrBadRequest("Order size must be at least 10,000 IDR")
	}
	if req.OrderSizeIDR > req.InitialBalanceIDR {
		return util.ErrBadRequest("Order size cannot exceed initial balance")
	}
	if req.MinGapPercent < 0 || req.MinGapPercent > 10 {
		return util.ErrBadRequest("Min gap percent must be between 0 and 10")
	}
	if req.RepositionThresholdPercent < 0 || req.RepositionThresholdPercent > 5 {
		return util.ErrBadRequest("Reposition threshold must be between 0 and 5")
	}
	if req.MaxLossIDR <= 0 {
		return util.ErrBadRequest("Max loss IDR must be greater than 0")
	}

	return nil
}
