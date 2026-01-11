package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
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

	// Cleanup routine
	cleanupStopChan chan struct{}
}

// GetBotInstance returns the bot instance if it's running (for reading current prices)
func (s *MarketMakerService) GetBotInstance(botID int64) *BotInstance {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.instances[botID]
}

// GetMarketDataService returns the market data service (for accessing market data)
func (s *MarketMakerService) GetMarketDataService() *market.MarketDataService {
	return s.marketDataService
}

// BotInstance represents a running bot in memory
type BotInstance struct {
	Config          *model.BotConfig
	TradeClient     TradeClient
	StopChan        chan struct{}
	TickerChan      chan market.OrderBookTicker
	TickerHandler   market.TickerHandler // Store the handler so we can unsubscribe
	ActiveOrder     *model.Order
	CurrentBid      float64
	CurrentAsk      float64
	BaseCurrency    string        // e.g. "btc" in "btcidr"
	LastBuyPrice    float64       // Track last buy price for profit calculation
	TotalCoinBought float64       // Track total coins bought (for average price calculation)
	TotalCostIDR    float64       // Track total cost in IDR (for average price calculation)
	LastOrderTime   time.Time     // Track last order placement/cancellation for rate limiting
	PairInfo        *indodax.Pair // Cached pair info to avoid repeated lookups

	mu sync.Mutex // Protects ActiveOrder and order operations to prevent race conditions
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
		cleanupStopChan:     make(chan struct{}),
	}

	// Register order update handler for live bots
	orderMonitor.AddOrderHandler(s.handleLiveOrderUpdate)

	// Start background cleanup routine
	go s.startOrderCleanupRoutine()

	return s
}

// CreateBot creates a new market maker bot configuration
func (s *MarketMakerService) CreateBot(ctx context.Context, userID string, req *model.BotConfigRequest) (*model.BotConfig, error) {
	// 1. Validate parameters
	if err := s.validateBotConfig(ctx, req); err != nil {
		return nil, err
	}

	// 1.5. Check for duplicate bot (same type, pair, and mode)
	exists, err := s.botRepo.ExistsByTypePairMode(ctx, userID, model.BotTypeMarketMaker, req.Pair, req.IsPaperTrading, 0)
	if err != nil {
		return nil, err
	}
	if exists {
		modeStr := "paper"
		if !req.IsPaperTrading {
			modeStr = "live"
		}
		return nil, util.ErrBadRequest(fmt.Sprintf("A %s trading bot for pair %s already exists. Each pair can only have one bot per mode (paper/live).", modeStr, req.Pair))
	}

	// 2. Separate logic for paper vs live
	var apiKeyID *int64
	if !req.IsPaperTrading {
		// Verify user has a valid API key (single API key per user model)
		_, err := s.apiKeyService.GetDecrypted(ctx, userID)
		if err != nil {
			return nil, util.NewAppError(400, util.ErrCodeAPIKeyInvalid, "Valid API key is required for live trading. Please add your API key in Settings.")
		}
		// For backward compatibility, set apiKeyID to 1 if not provided
		// In the future, we can remove apiKeyID entirely since we have one key per user
		if req.APIKeyID != nil {
			apiKeyID = req.APIKeyID
		} else {
			// Default to 1 for single API key per user
			apiKeyIDValue := int64(1)
			apiKeyID = &apiKeyIDValue
		}
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

	// Check for duplicate bot if pair or mode is being changed (exclude current bot)
	if bot.Pair != req.Pair || bot.IsPaperTrading != req.IsPaperTrading {
		exists, err := s.botRepo.ExistsByTypePairMode(ctx, userID, model.BotTypeMarketMaker, req.Pair, req.IsPaperTrading, botID)
		if err != nil {
			return nil, err
		}
		if exists {
			modeStr := "paper"
			if !req.IsPaperTrading {
				modeStr = "live"
			}
			return nil, util.ErrBadRequest(fmt.Sprintf("A %s trading bot for pair %s already exists. Each pair can only have one bot per mode (paper/live).", modeStr, req.Pair))
		}
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
	orders, err := s.orderRepo.ListByParentAndUser(ctx, userID, "bot", botID, 0) // 0 = no limit, get all
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

	// Check if instance already exists (quick check with read lock)
	s.mu.RLock()
	_, exists := s.instances[botID]
	s.mu.RUnlock()
	if exists {
		return util.ErrBadRequest("Bot instance already exists")
	}

	// 1. Create instance (no lock needed yet)
	inst := &BotInstance{
		Config:     bot,
		StopChan:   make(chan struct{}),
		TickerChan: make(chan market.OrderBookTicker, 10),
		// Restore tracking fields from database
		TotalCoinBought: bot.TotalCoinBought,
		TotalCostIDR:    bot.TotalCostIDR,
		LastBuyPrice:    bot.LastBuyPrice,
	}
	s.log.Debugf("Bot %d: Restored tracking - TotalCoinBought=%.8f, TotalCostIDR=%.2f, LastBuyPrice=%.2f",
		botID, inst.TotalCoinBought, inst.TotalCostIDR, inst.LastBuyPrice)

	// 2. Setup Trade Client
	if bot.IsPaperTrading {
		inst.TradeClient = NewPaperTradeClient(bot.Balances, func(order *model.Order) {
			// For paper trading, the full amount is always filled at once
			s.handleFilled(inst, order, order.Amount)
		})
	} else {
		// Get API keys
		key, err := s.apiKeyService.GetDecrypted(ctx, userID)
		if err != nil {
			return util.ErrBadRequest("Valid API key not found")
		}
		inst.TradeClient = NewLiveTradeClient(s.indodaxClient, key.Key, key.Secret)
		// Verify subscription exists - REQUIRED for live trading
		// Subscription should be established on API boot or when API key is created
		if !s.orderMonitor.IsSubscribed(userID) {
			s.log.Errorf("User %s is not subscribed to order updates. Subscription is required for live trading.", userID)
			return fmt.Errorf("cannot start live bot: user is not subscribed to order updates. Please ensure your API key is configured correctly")
		}
	}

	// 3. Set Base Currency and cache PairInfo
	pairInfo, ok := s.marketDataService.GetPairInfo(bot.Pair)
	if !ok {
		return util.ErrBadRequest("Invalid pair")
	}
	inst.PairInfo = &pairInfo // Cache pair info to avoid repeated lookups

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
		s.log.Debugf("Bot %d: Initializing balances for paper trading (InitialBalanceIDR=%.2f)", botID, bot.InitialBalanceIDR)

		// Initialize balances map if nil
		if bot.Balances == nil {
			bot.Balances = make(map[string]float64)
		}

		// Get current balances
		currentIDR := bot.Balances["idr"]
		currentCoin := bot.Balances[inst.BaseCurrency]

		s.log.Debugf("Bot %d: Current balances before reset - IDR=%.2f, %s=%.8f",
			botID, currentIDR, inst.BaseCurrency, currentCoin)

		// For paper trading, always validate and reset if corrupted
		// Reset if: negative, zero (when initial > 0), or unreasonably large
		needsReset := false
		if currentIDR < 0 {
			s.log.Warnf("Bot %d: IDR balance is negative (%.2f), resetting", botID, currentIDR)
			needsReset = true
		} else if bot.InitialBalanceIDR > 0 && (currentIDR == 0 || currentIDR > bot.InitialBalanceIDR*10 || currentIDR > util.MaxReasonableIDRBalance) {
			s.log.Warnf("Bot %d: IDR balance looks corrupted (%.2f), resetting to initial (%.2f)",
				botID, currentIDR, bot.InitialBalanceIDR)
			needsReset = true
		}

		if currentCoin < 0 || currentCoin > util.MaxReasonableCoinAmount {
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

		s.log.Debugf("Bot %d: Final balances after initialization - IDR=%.2f, %s=%.8f",
			botID, bot.Balances["idr"], inst.BaseCurrency, bot.Balances[inst.BaseCurrency])

		// Save corrected balances to Redis
		if err := s.botRepo.UpdateBalance(ctx, botID, bot.Balances); err != nil {
			s.log.Errorf("Bot %d: Failed to save corrected balances to Redis: %v", botID, err)
		} else {
			s.log.Debugf("Bot %d: Successfully saved corrected balances to Redis", botID)
		}
	}

	// 3.6. Restore active open orders from database
	s.log.Debugf("Bot %d: Checking for active orders in database to restore...", botID)
	openOrders, err := s.orderRepo.ListByParentAndUser(ctx, userID, "bot", botID, 10) // Get up to 10 most recent
	if err == nil && len(openOrders) > 0 {
		// Find the most recent open order
		for _, order := range openOrders {
			if order.Status == "open" {
				inst.ActiveOrder = order
				s.log.Debugf("Bot %d: Restored active order from database - OrderID=%s, Side=%s, Price=%.2f, Amount=%.8f, FilledAmount=%.8f",
					botID, order.OrderID, order.Side, order.Price, order.Amount, order.FilledAmount)
				break
			}
		}
	} else if err != nil {
		s.log.Warnf("Bot %d: Failed to check for active orders: %v", botID, err)
	}

	// 3.7. If live trading and we have a restored order, verify it still exists on Indodax
	if !bot.IsPaperTrading && inst.ActiveOrder != nil {
		s.log.Debugf("Bot %d: Verifying restored order %s still exists on Indodax...", botID, inst.ActiveOrder.OrderID)
		orderInfo, err := inst.TradeClient.GetOrder(ctx, inst.ActiveOrder.Pair, inst.ActiveOrder.OrderID)
		if err != nil {
			// Order lookup failed - might be filled/cancelled or invalid
			s.log.Warnf("Bot %d: Failed to verify order %s on Indodax: %v - will clear and let bot create new order",
				botID, inst.ActiveOrder.OrderID, err)
			// Update order status in database (keep for history)
			s.orderRepo.UpdateStatus(ctx, inst.ActiveOrder.ID, "cancelled")
			s.log.Debugf("Bot %d: Order verification failed, marked as cancelled in database (kept for history)", botID)
			inst.ActiveOrder = nil
		} else {
			// Check order status from Indodax
			indodaxStatus := strings.ToLower(orderInfo.Status)
			s.log.Debugf("Bot %d: Order %s status on Indodax: %s (RemainCoin=%s, OrderCoin=%s)",
				botID, inst.ActiveOrder.OrderID, indodaxStatus, orderInfo.RemainCoin, orderInfo.OrderCoin)

			if indodaxStatus == "filled" || indodaxStatus == "done" {
				// Order was filled - process the fill
				s.log.Debugf("Bot %d: Order %s was filled while bot was stopped, processing fill...", botID, inst.ActiveOrder.OrderID)
				// Use the order amount since we don't have executed qty from GetOrder
				s.handleFilled(inst, inst.ActiveOrder, inst.ActiveOrder.Amount)
			} else if indodaxStatus == "cancelled" {
				// Order was cancelled
				s.log.Debugf("Bot %d: Order %s was cancelled, clearing active order", botID, inst.ActiveOrder.OrderID)
				s.orderRepo.UpdateStatus(ctx, inst.ActiveOrder.ID, "cancelled")
				s.log.Debugf("Bot %d: Cancelled order kept in database for history", botID)
				inst.ActiveOrder = nil
			} else if indodaxStatus == "open" {
				// Order is still open - parse remaining amount to check for partial fills
				remainCoin, _ := strconv.ParseFloat(orderInfo.RemainCoin, 64)
				orderCoin, _ := strconv.ParseFloat(orderInfo.OrderCoin, 64)

				if orderCoin > 0 && remainCoin < orderCoin {
					// Partial fill detected
					executedQty := orderCoin - remainCoin
					s.log.Debugf("Bot %d: Order %s has partial fill - ExecutedQty=%.8f, RemainQty=%.8f",
						botID, inst.ActiveOrder.OrderID, executedQty, remainCoin)
					// Update the filled amount in our tracking
					if executedQty > inst.ActiveOrder.FilledAmount {
						// Process new fills that happened while bot was stopped
						s.handlePartialFill(inst, inst.ActiveOrder, executedQty, remainCoin)
					}
				}
				s.log.Debugf("Bot %d: Successfully verified and restored active order %s", botID, inst.ActiveOrder.OrderID)
			} else {
				// Unknown status - clear to be safe
				s.log.Warnf("Bot %d: Order %s has unknown status '%s', clearing", botID, inst.ActiveOrder.OrderID, indodaxStatus)
				inst.ActiveOrder = nil
			}
		}
	} else if bot.IsPaperTrading && inst.ActiveOrder != nil {
		// For paper trading, we can't verify with Indodax, so we trust the database
		// But check if the order is stale (created more than 1 hour ago)
		if inst.ActiveOrder.CreatedAt.Before(time.Now().Add(-1 * time.Hour)) {
			s.log.Warnf("Bot %d: Paper trading order %s is stale (created %v ago), clearing",
				botID, inst.ActiveOrder.OrderID, time.Since(inst.ActiveOrder.CreatedAt))
			inst.ActiveOrder = nil
		} else {
			s.log.Debugf("Bot %d: Restored paper trading order %s", botID, inst.ActiveOrder.OrderID)
		}
	}

	if inst.ActiveOrder == nil {
		s.log.Debugf("Bot %d: No active order to restore, bot will place new orders as needed", botID)
	}

	// 4. Subscribe to ticker
	s.log.Debugf("Bot %d: Subscribing to ticker for pair %s", botID, bot.Pair)
	tickerHandler := func(ticker market.OrderBookTicker) {
		s.log.Debugf("Bot %d: Received ticker update via subscription - bid=%.2f ask=%.2f", botID, ticker.BestBid, ticker.BestAsk)
		select {
		case inst.TickerChan <- ticker:
		default:
			// Full channel, skip ticker
			s.log.Warnf("Bot %d: Ticker channel full, skipping update", botID)
		}
	}
	inst.TickerHandler = tickerHandler
	err = s.subManager.Subscribe(bot.Pair, tickerHandler)
	if err != nil {
		s.log.Errorf("Bot %d: Failed to subscribe to ticker: %v", botID, err)
		return util.ErrInternalServer(fmt.Sprintf("Failed to subscribe to ticker: %v", err))
	}
	s.log.Debugf("Bot %d: Successfully subscribed to ticker for pair %s", botID, bot.Pair)

	// 5. Initial balance sync for live bots
	if !bot.IsPaperTrading {
		if err := s.syncBalance(ctx, inst); err != nil {
			s.log.Warnf("Failed to initial sync balance for bot %d: %v", botID, err)
		}
	}

	// 6. Update status in DB
	if err := s.botRepo.UpdateStatus(ctx, botID, model.BotStatusRunning, nil); err != nil {
		s.log.Errorf("Bot %d: Failed to update status in DB: %v, unsubscribing", botID, err)
		s.subManager.Unsubscribe(bot.Pair, tickerHandler)
		return err
	}
	// Update in-memory status to match database
	inst.Config.Status = model.BotStatusRunning

	// 7. Store instance in map AFTER all setup is successful (critical: acquire lock only here)
	s.mu.Lock()
	// Double-check instance doesn't exist (race condition protection)
	if _, exists := s.instances[botID]; exists {
		s.mu.Unlock()
		s.log.Warnf("Bot %d instance already exists, cleaning up and returning error", botID)
		s.subManager.Unsubscribe(bot.Pair, tickerHandler)
		return util.ErrBadRequest("Bot instance already exists")
	}
	s.instances[botID] = inst
	s.mu.Unlock()
	s.log.Debugf("Bot %d: Instance stored in map (total instances: %d)", botID, len(s.instances))

	// 8. Start event loop (goroutine will continue running even if StartBot returns)
	go s.runBot(inst)

	s.log.Infof("Bot %d started successfully for pair %s", botID, bot.Pair)

	// Notify via WebSocket
	// Get current prices from instance if available
	var buyPrice, sellPrice, spreadPercent float64
	if inst := s.GetBotInstance(botID); inst != nil && inst.CurrentBid > 0 && inst.CurrentAsk > 0 {
		buyPrice = inst.CurrentBid
		sellPrice = inst.CurrentAsk
		spreadPercent = (inst.CurrentAsk - inst.CurrentBid) / inst.CurrentBid * 100
	}

	s.notificationService.NotifyBotUpdate(ctx, userID, model.WSBotUpdatePayload{
		BotID:          botID,
		Status:         model.BotStatusRunning,
		TotalTrades:    bot.TotalTrades,
		WinningTrades:  bot.WinningTrades,
		WinRate:        bot.WinRate(),
		TotalProfitIDR: bot.TotalProfitIDR,
		Balances:       bot.Balances,
		BuyPrice:       buyPrice,
		SellPrice:      sellPrice,
		SpreadPercent:  spreadPercent,
	})

	return nil
}

// StopBot stops a bot instance
func (s *MarketMakerService) StopBot(ctx context.Context, userID string, botID int64) error {
	s.log.Infof("StopBot called for bot %d by user %s", botID, userID)

	// Quick check: verify bot exists and user owns it (use timeout context to avoid hanging)
	queryCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s.log.Debugf("StopBot: Getting bot %d from database", botID)
	bot, err := s.botRepo.GetByID(queryCtx, botID)
	if err != nil {
		s.log.Errorf("StopBot: Failed to get bot %d: %v", botID, err)
		return err
	}
	s.log.Debugf("StopBot: Got bot %d from database, status=%s, userID=%s, requestingUserID=%s", botID, bot.Status, bot.UserID, userID)

	if bot.UserID != userID {
		s.log.Warnf("User %s attempted to stop bot %d owned by %s", userID, botID, bot.UserID)
		return util.ErrForbidden("Access denied")
	}

	// Check if already stopped (non-blocking check)
	if bot.Status != model.BotStatusRunning {
		s.log.Debugf("StopBot: Bot %d is already stopped (status: %s), returning early", botID, bot.Status)
		// Already stopped, nothing to do
		return nil
	}
	s.log.Debugf("StopBot: Bot %d status is running, proceeding to stop", botID)

	// First, try to find the instance with RLock (non-blocking read)
	s.log.Debugf("StopBot: Attempting to acquire read lock for bot %d", botID)
	s.mu.RLock()
	s.log.Debugf("StopBot: Read lock acquired for bot %d", botID)
	s.log.Debugf("Bot %d: Checking instances map (current count: %d)", botID, len(s.instances))

	// Log all instance IDs for debugging
	instanceIDs := make([]int64, 0, len(s.instances))
	for id := range s.instances {
		instanceIDs = append(instanceIDs, id)
	}
	s.log.Debugf("Bot %d: Current instance IDs in map: %v", botID, instanceIDs)

	inst, ok := s.instances[botID]
	var foundInst *BotInstance
	var foundKey int64
	if !ok {
		// Try searching by Config.ID while holding read lock
		for id, i := range s.instances {
			if i.Config != nil && i.Config.ID == botID && i.Config.UserID == userID {
				s.log.Warnf("Found bot %d in instances map with key %d (Config.ID match)", botID, id)
				foundInst = i
				foundKey = id
				break
			}
		}
	}
	s.mu.RUnlock()
	s.log.Debugf("StopBot: Read lock released for bot %d", botID)

	// Now handle the instance we found (or didn't find)
	if !ok && foundInst == nil {
		// Not found - just update status
		s.log.Warnf("Bot %d not found in instances map (map had %d instances: %v), updating status anyway", botID, len(instanceIDs), instanceIDs)
		updateCtx, updateCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer updateCancel()
		if err := s.botRepo.UpdateStatus(updateCtx, botID, model.BotStatusStopped, nil); err != nil {
			s.log.Errorf("Failed to update status for bot %d: %v", botID, err)
			return err
		}
		return nil
	}

	// Use foundInst if we found it by Config.ID, otherwise use inst
	var targetInst *BotInstance
	var targetKey int64
	if foundInst != nil {
		targetInst = foundInst
		targetKey = foundKey
	} else {
		targetInst = inst
		targetKey = botID
	}

	// Close StopChan immediately (doesn't need lock)
	// Use recover to prevent panic if channel is already closed (race condition protection)
	s.log.Infof("Stopping bot %d (key %d) - closing StopChan", botID, targetKey)
	func() {
		defer func() {
			if r := recover(); r != nil {
				s.log.Warnf("Bot %d: StopChan already closed (recovered from panic: %v)", botID, r)
			}
		}()
		close(targetInst.StopChan)
	}()

	// Unsubscribe (doesn't need lock)
	if targetInst.TickerHandler != nil {
		s.log.Debugf("Bot %d: Unsubscribing from ticker for pair %s", botID, bot.Pair)
		s.subManager.Unsubscribe(bot.Pair, targetInst.TickerHandler)
	}

	// Now acquire write lock only to delete from map
	s.log.Debugf("StopBot: Attempting to acquire write lock to delete bot %d from map", botID)
	s.mu.Lock()
	s.log.Debugf("StopBot: Write lock acquired for bot %d", botID)
	delete(s.instances, targetKey)
	s.mu.Unlock()
	s.log.Infof("Bot %d removed from instances map", botID)

	// Update status in DB immediately (use timeout context to avoid hanging)
	updateCtx, updateCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer updateCancel()
	err = s.botRepo.UpdateStatus(updateCtx, botID, model.BotStatusStopped, nil)
	if err != nil {
		s.log.Errorf("Failed to update status for bot %d: %v", botID, err)
		return err
	}
	s.log.Debugf("Bot %d status updated to stopped in database", botID)

	// Update in-memory status to match database
	if targetInst != nil {
		targetInst.Config.Status = model.BotStatusStopped
	}

	// 5. Notify via WebSocket asynchronously (don't block response)
	go func() {
		notifyCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		stoppedBot, _ := s.botRepo.GetByID(notifyCtx, botID)
		if stoppedBot != nil {
			// Get current prices from instance if available (before it's removed)
			var buyPrice, sellPrice, spreadPercent float64
			if inst := s.GetBotInstance(botID); inst != nil && inst.CurrentBid > 0 && inst.CurrentAsk > 0 {
				buyPrice = inst.CurrentBid
				sellPrice = inst.CurrentAsk
				spreadPercent = (inst.CurrentAsk - inst.CurrentBid) / inst.CurrentBid * 100
			}

			s.notificationService.NotifyBotUpdate(notifyCtx, userID, model.WSBotUpdatePayload{
				BotID:          botID,
				Status:         model.BotStatusStopped,
				TotalTrades:    stoppedBot.TotalTrades,
				WinningTrades:  stoppedBot.WinningTrades,
				WinRate:        stoppedBot.WinRate(),
				TotalProfitIDR: stoppedBot.TotalProfitIDR,
				Balances:       stoppedBot.Balances,
				BuyPrice:       buyPrice,
				SellPrice:      sellPrice,
				SpreadPercent:  spreadPercent,
			})
		}
	}()

	// 6. Cancel orders asynchronously in background (don't block the response)
	go func() {
		cancelCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Cancel any active/open orders and restore locked funds
		if targetInst.ActiveOrder != nil && targetInst.ActiveOrder.Status == "open" {
			s.log.Debugf("Bot %d: Cancelling active order %s (ID: %s)", botID, targetInst.ActiveOrder.Side, targetInst.ActiveOrder.OrderID)

			// Save order details before cancellation for balance restoration
			orderSide := targetInst.ActiveOrder.Side
			orderPrice := targetInst.ActiveOrder.Price
			orderAmount := targetInst.ActiveOrder.Amount

			if err := targetInst.TradeClient.CancelOrder(cancelCtx, bot.Pair, targetInst.ActiveOrder.OrderID, targetInst.ActiveOrder.Side); err != nil {
				// "Order not found" is expected if order was already filled/cancelled
				if util.IsOrderNotFoundError(err) {
					s.log.Debugf("Bot %d: Active order %s already filled/cancelled (not found)", botID, targetInst.ActiveOrder.OrderID)
				} else {
					s.log.Warnf("Bot %d: Failed to cancel active order %s: %v", botID, targetInst.ActiveOrder.OrderID, err)
				}
			} else {
				// Order successfully cancelled - restore locked funds
				targetInst.mu.Lock()
				if orderSide == "sell" {
					// Return locked coins
					targetInst.Config.Balances[targetInst.BaseCurrency] += orderAmount
					s.log.Debugf("Bot %d: Restored %.8f %s from cancelled SELL order on stop",
						botID, orderAmount, targetInst.BaseCurrency)
				} else { // buy
					// Return locked IDR
					returnedIDR := orderAmount * orderPrice
					targetInst.Config.Balances["idr"] += returnedIDR
					s.log.Debugf("Bot %d: Restored %.2f IDR from cancelled BUY order on stop",
						botID, returnedIDR)
				}

				// Save updated balance to database
				if err := s.botRepo.UpdateBalance(cancelCtx, botID, targetInst.Config.Balances); err != nil {
					s.log.Warnf("Bot %d: Failed to save restored balance after stop: %v", botID, err)
				}
				targetInst.mu.Unlock()

				s.orderRepo.UpdateStatus(cancelCtx, targetInst.ActiveOrder.ID, "cancelled")
				targetInst.ActiveOrder.Status = "cancelled"
				s.notificationService.NotifyOrderUpdate(cancelCtx, userID, targetInst.ActiveOrder)
				s.log.Debugf("Bot %d: Active order %d cancelled and kept in database for history", botID, targetInst.ActiveOrder.ID)
			}
		}

		// Also check for any other open orders for this bot
		openOrders, err := s.orderRepo.ListByParentAndUser(cancelCtx, userID, "bot", botID, 0)
		if err == nil {
			for _, order := range openOrders {
				if order.Status == "open" {
					s.log.Debugf("Bot %d: Cancelling open order %d (ID: %s)", botID, order.ID, order.OrderID)
					if err := targetInst.TradeClient.CancelOrder(cancelCtx, order.Pair, order.OrderID, order.Side); err != nil {
						// "Order not found" is expected if order was already filled/cancelled
						if util.IsOrderNotFoundError(err) {
							s.log.Debugf("Bot %d: Order %d (ID: %s) already filled/cancelled (not found)", botID, order.ID, order.OrderID)
						} else {
							s.log.Warnf("Bot %d: Failed to cancel order %d: %v", botID, order.ID, err)
						}
					} else {
						// Order successfully cancelled - restore locked funds
						targetInst.mu.Lock()
						if order.Side == "sell" {
							// Return locked coins
							targetInst.Config.Balances[targetInst.BaseCurrency] += order.Amount
							s.log.Debugf("Bot %d: Restored %.8f %s from cancelled SELL order %d on stop",
								botID, order.Amount, targetInst.BaseCurrency, order.ID)
						} else { // buy
							// Return locked IDR
							returnedIDR := order.Amount * order.Price
							targetInst.Config.Balances["idr"] += returnedIDR
							s.log.Debugf("Bot %d: Restored %.2f IDR from cancelled BUY order %d on stop",
								botID, returnedIDR, order.ID)
						}

						// Save updated balance to database
						if err := s.botRepo.UpdateBalance(cancelCtx, botID, targetInst.Config.Balances); err != nil {
							s.log.Warnf("Bot %d: Failed to save restored balance after cancelling order %d: %v", botID, order.ID, err)
						}
						targetInst.mu.Unlock()

						s.orderRepo.UpdateStatus(cancelCtx, order.ID, "cancelled")
						order.Status = "cancelled"
						s.notificationService.NotifyOrderUpdate(cancelCtx, userID, order)
						s.log.Infof("Bot %d: Order %d cancelled and kept in database for history", botID, order.ID)
					}
				}
			}
		}
	}()

	return nil
}

// Note: Error checking functions (isAPIKeyError, isCriticalTradingError, isOrderNotFoundError)
// have been replaced with shared utilities from util package:
// - util.IsAPIKeyError
// - util.IsCriticalTradingError
// - util.IsOrderNotFoundError

// stopBotWithError stops a bot and sets error status (for live bots only)
func (s *MarketMakerService) stopBotWithError(botID int64, userID string, errorMsg string) {
	s.mu.RLock()
	inst, ok := s.instances[botID]
	s.mu.RUnlock()

	if !ok {
		s.log.Warnf("Bot %d: Instance not found when trying to stop with error", botID)
		return
	}

	if inst.Config.IsPaperTrading {
		// Don't stop paper trading bots for API key errors
		return
	}

	s.log.Errorf("Bot %d: API key error detected, stopping bot: %s", botID, errorMsg)

	// Close stop channel to stop the bot loop
	select {
	case <-inst.StopChan:
		// Already closed
	default:
		close(inst.StopChan)
	}

	// Update status to error
	bgCtx := context.Background()
	errMsg := errorMsg
	if err := s.botRepo.UpdateStatus(bgCtx, botID, model.BotStatusError, &errMsg); err != nil {
		s.log.Errorf("Failed to update bot %d status to error: %v", botID, err)
	}

	// Update in-memory status
	s.mu.Lock()
	if inst, ok := s.instances[botID]; ok {
		inst.Config.Status = model.BotStatusError
		inst.Config.ErrorMessage = &errMsg
	}
	s.mu.Unlock()

	// Unsubscribe from ticker
	if inst.TickerHandler != nil {
		s.subManager.Unsubscribe(inst.Config.Pair, inst.TickerHandler)
	}

	// Remove from instances map
	s.mu.Lock()
	delete(s.instances, botID)
	s.mu.Unlock()

	// Notify via WebSocket
	go func() {
		notifyCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		bot, _ := s.botRepo.GetByID(notifyCtx, botID)
		if bot != nil {
			s.notificationService.NotifyBotUpdate(notifyCtx, userID, model.WSBotUpdatePayload{
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

func (s *MarketMakerService) runBot(inst *BotInstance) {
	s.log.Infof("Starting event loop for bot %d", inst.Config.ID)
	defer s.log.Infof("Event loop stopped for bot %d", inst.Config.ID)

	for {
		select {
		case <-inst.StopChan:
			s.log.Infof("Bot %d: StopChan closed, exiting runBot loop", inst.Config.ID)
			return
		case ticker := <-inst.TickerChan:
			s.handleTicker(inst, ticker)
		}
	}
}

func (s *MarketMakerService) handleTicker(inst *BotInstance, ticker market.OrderBookTicker) {
	s.log.Debugf("Bot %d received ticker: bid=%.2f ask=%.2f", inst.Config.ID, ticker.BestBid, ticker.BestAsk)

	// 1. Update prices first (needed for decision making)
	inst.CurrentBid = ticker.BestBid
	inst.CurrentAsk = ticker.BestAsk

	// 2. Check and initialize virtual balance first (before any order decisions)
	s.validateAndNormalizeBalances(inst)

	// 3. Calculate spread (will be used for both update and decision)
	spreadPercent := (ticker.BestAsk - ticker.BestBid) / ticker.BestBid * 100

	// 3.5. ALWAYS send bot update with current prices/spread (regardless of gap or trading activity)
	ctx := context.Background()
	s.notificationService.NotifyBotUpdate(ctx, inst.Config.UserID, model.WSBotUpdatePayload{
		BotID:          inst.Config.ID,
		Status:         inst.Config.Status,
		TotalTrades:    inst.Config.TotalTrades,
		WinningTrades:  inst.Config.WinningTrades,
		TotalProfitIDR: inst.Config.TotalProfitIDR,
		Balances:       inst.Config.Balances,
		WinRate:        inst.Config.WinRate(),
		BuyPrice:       ticker.BestBid,
		SellPrice:      ticker.BestAsk,
		SpreadPercent:  spreadPercent,
	})

	// 4. Check minimum gap
	if spreadPercent < inst.Config.MinGapPercent {
		s.log.Debugf("Bot %d: Spread too tight (%.4f%% < %.4f%%), skipping", inst.Config.ID, spreadPercent, inst.Config.MinGapPercent)
		return // Spread too tight
	}

	s.log.Debugf("Bot %d: Gap OK (%.4f%% >= %.4f%%), processing orders", inst.Config.ID, spreadPercent, inst.Config.MinGapPercent)

	// 4. Check volatility - only skip SELL if volatile AND we're at a loss
	coin, err := s.marketDataService.GetCoin(context.Background(), inst.Config.Pair)
	if err == nil && coin.Volatility1m > 2.0 {
		// Determine if we would be selling (use virtual balance)
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

	// 5. Process orders
	if inst.ActiveOrder == nil {
		s.placeNewOrder(inst, ticker)
	} else {
		s.checkReposition(inst, ticker)
	}
}

func (s *MarketMakerService) placeNewOrder(inst *BotInstance, ticker market.OrderBookTicker) {
	s.log.Debugf("Bot %d: placeNewOrder called", inst.Config.ID)

	// Lock to prevent race conditions when checking/setting ActiveOrder
	inst.mu.Lock()
	defer inst.mu.Unlock()

	// Rate limiting: Don't place orders too frequently (debounce)
	timeSinceLastOrder := time.Since(inst.LastOrderTime)
	if timeSinceLastOrder < 2*time.Second {
		s.log.Debugf("Bot %d: Debouncing - only %.1fs since last order action (minimum 2s)",
			inst.Config.ID, timeSinceLastOrder.Seconds())
		return
	}

	// Safety check: Don't place a new order if there's already an active order
	if inst.ActiveOrder != nil {
		// Check if the order is still open or pending WebSocket confirmation
		if inst.ActiveOrder.Status == "open" || inst.ActiveOrder.Status == "pending_ws_confirm" {
			s.log.Debugf("Bot %d: Skipping placeNewOrder - active order exists (ID: %s, side: %s, price: %.2f, status: %s)",
				inst.Config.ID, inst.ActiveOrder.OrderID, inst.ActiveOrder.Side, inst.ActiveOrder.Price, inst.ActiveOrder.Status)
			return
		}
		// If order status is not "open" or "pending_ws_confirm", clear it and proceed
		s.log.Debugf("Bot %d: Active order exists but status is '%s' (not open/pending), clearing and placing new order",
			inst.Config.ID, inst.ActiveOrder.Status)
		inst.ActiveOrder = nil
	}

	// Validate and normalize balances
	s.validateAndNormalizeBalances(inst)

	// Get balances after validation
	idrBalance := inst.Config.Balances["idr"]
	coinBalance := inst.Config.Balances[inst.BaseCurrency]

	s.log.Debugf("Bot %d: Balance check - IDR=%.2f, %s=%.8f (BaseCurrency=%s, all balances: %+v)",
		inst.Config.ID, idrBalance, inst.BaseCurrency, coinBalance, inst.BaseCurrency, inst.Config.Balances)

	// Ensure pair info is available
	if inst.PairInfo == nil {
		s.log.Warnf("Bot %d: Pair info not available for %s", inst.Config.ID, inst.Config.Pair)
		return
	}
	pairInfo := inst.PairInfo

	// Get volume precision (using shared utility)
	volumePrecision := util.GetVolumePrecision(*pairInfo)

	s.log.Debugf("Bot %d: Pair info - VolumePrecision=%d, TradeMinBaseCurrency=%d, TradeMinTradedCurrency=%.2f",
		inst.Config.ID, volumePrecision, pairInfo.TradeMinBaseCurrency, pairInfo.TradeMinTradedCurrency)

	var side string
	var price, amount float64

	// Note: Balance corruption check is now in validateAndNormalizeBalances

	// Decision logic:
	// - If we have coins: SELL ALL available coin balance (with stop-loss check)
	// - If we have IDR: BUY using OrderSizeIDR / price

	// First check if coin balance is tradeable (not dust)
	hasTradableCoins := false
	if coinBalance > 0 {
		roundedCoinBalance := util.FloorToPrecision(coinBalance, volumePrecision)
		if roundedCoinBalance > 0 && roundedCoinBalance >= pairInfo.TradeMinTradedCurrency {
			hasTradableCoins = true
		} else {
			s.log.Debugf("Bot %d: Coin balance %.8f %s is dust (below minimum %.8f) - will place BUY order instead",
				inst.Config.ID, coinBalance, inst.BaseCurrency, pairInfo.TradeMinTradedCurrency)
		}
	}

	if hasTradableCoins {
		// Have coins -> SELL ALL available balance
		// Check orderbook depth before placing sell order
		estimatedSellValueIDR := coinBalance * inst.CurrentAsk
		if !s.checkOrderbookDepth(ticker, "sell", estimatedSellValueIDR, inst.Config.MinGapPercent) {
			s.log.Debugf("Bot %d: Skipping SELL - insufficient orderbook depth", inst.Config.ID)
			return
		}

		// Calculate competitive sell price
		sellPrice, err := s.calculateCompetitivePrice(inst, ticker, "sell")
		if err != nil {
			s.log.Warnf("Bot %d: Failed to calculate competitive price for SELL: %v", inst.Config.ID, err)
			return
		}

		// Validate profit before selling
		shouldSkip, reason := s.validateSellProfit(inst, sellPrice)
		if shouldSkip {
			s.log.Debugf("Bot %d: Skipping SELL - %s", inst.Config.ID, reason)
			return
		}

		if inst.LastBuyPrice > 0 {
			profitPercent := ((sellPrice - inst.LastBuyPrice) / inst.LastBuyPrice) * 100
			s.log.Debugf("Bot %d: SELL check - buyPrice=%.2f, sellPrice=%.2f, profit=%.2f%%",
				inst.Config.ID, inst.LastBuyPrice, sellPrice, profitPercent)
		}

		// Round coin balance for order placement
		roundedCoinBalance := util.FloorToPrecision(coinBalance, volumePrecision)

		side = "sell"
		price = sellPrice

		amount = roundedCoinBalance // Use rounded amount (already validated > 0)

		// Safety check: don't sell if amount is unreasonably large (corruption protection)
		if amount > util.MaxReasonableCoinAmount {
			s.log.Errorf("Bot %d: Refusing to place SELL order - amount %.8f is unreasonably large", inst.Config.ID, amount)
			return
		}

		s.log.Debugf("Bot %d: Placing SELL order - price=%.2f amount=%.8f (all available)", inst.Config.ID, price, amount)
	} else if idrBalance >= inst.Config.OrderSizeIDR {
		// Have IDR -> BUY
		// Check orderbook depth before placing buy order
		if !s.checkOrderbookDepth(ticker, "buy", inst.Config.OrderSizeIDR, inst.Config.MinGapPercent) {
			s.log.Debugf("Bot %d: Skipping BUY - insufficient orderbook depth or bid gap too large", inst.Config.ID)
			return
		}

		side = "buy"

		// Calculate competitive buy price
		buyPrice, err := s.calculateCompetitivePrice(inst, ticker, "buy")
		if err != nil {
			s.log.Warnf("Bot %d: Failed to calculate competitive price for BUY: %v", inst.Config.ID, err)
			return
		}
		price = buyPrice

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

	// Validate order amount (using shared utility)
	s.log.Debugf("Bot %d: Before validation - amount=%.8f, volumePrecision=%d", inst.Config.ID, amount, volumePrecision)

	validation := util.ValidateOrderAmount(
		inst.Config.ID,
		amount,
		price,
		*pairInfo,
		volumePrecision,
		inst.BaseCurrency,
		s.log,
	)

	if !validation.Valid {
		s.log.Debugf("Bot %d: Order validation failed - %s", inst.Config.ID, validation.Reason)
		return
	}

	// Use validated amount
	amount = validation.Amount
	s.log.Debugf("Bot %d: After validation - amount=%.8f, orderValue=%.2f IDR", inst.Config.ID, amount, validation.OrderValue)

	// Double-check: Don't place if active order exists (defense against race conditions)
	if inst.ActiveOrder != nil && inst.ActiveOrder.Status == "open" {
		s.log.Warnf("Bot %d: Aborting placeNewOrder - active order detected during order placement (ID: %s, side: %s, price: %.2f)",
			inst.Config.ID, inst.ActiveOrder.OrderID, inst.ActiveOrder.Side, inst.ActiveOrder.Price)
		return
	}

	// Place order
	ctx := context.Background()

	// Generate unique client order ID (using shared utility)
	clientOrderID := GenerateClientOrderID(inst.Config.ID, inst.Config.Pair, side)

	// CRITICAL: Create placeholder order BEFORE API call to prevent race conditions
	// This ensures the safety check blocks concurrent placeNewOrder calls
	placeholderOrder := &model.Order{
		UserID:       inst.Config.UserID,
		ParentID:     inst.Config.ID,
		ParentType:   "bot",
		OrderID:      clientOrderID, // Temporary ID until we get response
		Pair:         inst.Config.Pair,
		Side:         side,
		Status:       "pending", // Mark as pending until API confirms
		Price:        price,
		Amount:       amount,
		IsPaperTrade: inst.Config.IsPaperTrading,
	}
	inst.ActiveOrder = placeholderOrder
	s.log.Debugf("Bot %d: Set ActiveOrder to pending BEFORE API call to prevent race", inst.Config.ID)

	res, err := inst.TradeClient.Trade(ctx, side, inst.Config.Pair, price, amount, "limit", clientOrderID)
	if err != nil {
		s.log.Errorf("Failed to place %s order for bot %d: %v", side, inst.Config.ID, err)

		// IMPORTANT: Clear ActiveOrder on failure so next ticker can retry
		inst.ActiveOrder = nil
		s.log.Debugf("Bot %d: Cleared ActiveOrder after API failure", inst.Config.ID)

		// Handle API error (rate limiting, critical errors, etc.)
		s.handleAPIError(inst, err, "Trade")
		return
	}

	// Update last order time for rate limiting
	inst.LastOrderTime = time.Now()

	// Store the ClientOrderID (for both WebSocket matching and cancellation)
	// If ClientOrderID is empty (shouldn't happen), fallback to numeric ID
	orderID := res.ClientOrderID
	if orderID == "" {
		orderID = fmt.Sprintf("%d", res.OrderID)
	}

	// Update placeholder order with actual response data
	placeholderOrder.OrderID = orderID
	placeholderOrder.Status = "open"

	if err := s.orderRepo.Create(ctx, placeholderOrder); err != nil {
		s.log.Errorf("Failed to save order for bot %d: %v", inst.Config.ID, err)
		// Don't return - order was placed on exchange, keep it in memory
	}
	s.log.Infof("Bot %d placed %s order: %.8f @ %.2f (OrderID: %s)", inst.Config.ID, side, amount, price, orderID)

	// CRITICAL: Deduct balance immediately when order is placed
	// This locks the funds and prevents overselling/overspending
	if side == "buy" {
		// Lock IDR for buy order
		totalCost := amount * price
		inst.Config.Balances["idr"] -= totalCost
		if inst.Config.Balances["idr"] < 0 {
			inst.Config.Balances["idr"] = 0
		}
		s.log.Debugf("Bot %d: Locked %.2f IDR for BUY order (new balance: %.2f IDR)",
			inst.Config.ID, totalCost, inst.Config.Balances["idr"])
	} else if side == "sell" {
		// Lock coins for sell order
		inst.Config.Balances[inst.BaseCurrency] -= amount
		if inst.Config.Balances[inst.BaseCurrency] < 0 {
			inst.Config.Balances[inst.BaseCurrency] = 0
		}
		s.log.Debugf("Bot %d: Locked %.8f %s for SELL order (new balance: %.8f %s)",
			inst.Config.ID, amount, inst.BaseCurrency, inst.Config.Balances[inst.BaseCurrency], inst.BaseCurrency)
	}

	// Save updated balance to database
	if err := s.botRepo.UpdateBalance(ctx, inst.Config.ID, inst.Config.Balances); err != nil {
		s.log.Warnf("Bot %d: Failed to save balance after order placement: %v", inst.Config.ID, err)
	}

	// Notify order creation via WebSocket
	s.notificationService.NotifyOrderUpdate(ctx, inst.Config.UserID, placeholderOrder)

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

func (s *MarketMakerService) checkReposition(inst *BotInstance, ticker market.OrderBookTicker) {
	// Lock to prevent race conditions when checking/modifying ActiveOrder
	inst.mu.Lock()
	defer inst.mu.Unlock()

	order := inst.ActiveOrder
	if order == nil {
		return
	}

	// Note: We now ALLOW repositioning of partially filled orders
	// The remaining balance after partial fills will be used for new orders
	// This is handled naturally by the placeNewOrder logic which checks available balance

	// Rate limiting: Don't cancel/reposition too frequently (debounce)
	timeSinceLastOrder := time.Since(inst.LastOrderTime)
	if timeSinceLastOrder < 2*time.Second {
		s.log.Debugf("Bot %d: Debouncing reposition - only %.1fs since last order action (minimum 2s)",
			inst.Config.ID, timeSinceLastOrder.Seconds())
		return
	}

	ctx := context.Background()
	var shouldCancel bool
	var reason string
	var expectedPrice float64

	// Calculate current spread
	spreadPercent := (inst.CurrentAsk - inst.CurrentBid) / inst.CurrentBid * 100

	if order.Side == "buy" {
		// For BUY: If spread is below gap target, cancel immediately (even if partially filled)
		// We can't profitably sell later if spread is too tight
		if spreadPercent < inst.Config.MinGapPercent {
			s.log.Debugf("Bot %d: Spread too tight (%.4f%% < %.4f%%), cancelling BUY order immediately",
				inst.Config.ID, spreadPercent, inst.Config.MinGapPercent)
			shouldCancel = true
			reason = fmt.Sprintf("Spread too tight (%.4f%% < %.4f%%) - cancelling to avoid unprofitable cycle",
				spreadPercent, inst.Config.MinGapPercent)
			// Proceed to cancellation logic below
		} else {
			// Spread is OK - proceed with normal BUY repositioning logic
			// Calculate expected competitive price
			calculatedPrice, err := s.calculateCompetitivePrice(inst, ticker, "buy")
			if err != nil {
				s.log.Warnf("Bot %d: Failed to calculate competitive price for BUY in checkReposition: %v", inst.Config.ID, err)
				return
			}
			expectedPrice = calculatedPrice

			// Allow small tolerance for price matching (handles floating point precision)
			priceDiff := order.Price - expectedPrice
			if priceDiff < 0 {
				priceDiff = -priceDiff
			}

			if priceDiff > 0.01 { // 0.01 IDR tolerance
				// Price doesn't match - cancel to reposition
				shouldCancel = true
				reason = fmt.Sprintf("Our bid (%.2f) != expected price (%.2f) - should match market",
					order.Price, expectedPrice)
			} else {
				// Price matches - but check if depth is still sufficient
				// If depth became insufficient, cancel the order (risky to keep in thin market)
				if !s.checkOrderbookDepth(ticker, "buy", inst.Config.OrderSizeIDR, inst.Config.MinGapPercent) {
					shouldCancel = true
					reason = fmt.Sprintf("Our bid (%.2f) matches expected price, but orderbook depth insufficient - cancelling risky order",
						order.Price)
				}
				// If price matches AND depth is sufficient, keep the order (shouldCancel stays false)
			}
		}
	} else {
		// For SELL: If spread is below gap target, check profit before deciding
		if spreadPercent < inst.Config.MinGapPercent {
			// Check profit before deciding
			shouldSkip, skipReason := s.validateSellProfit(inst, order.Price)
			if shouldSkip {
				// Profit doesn't meet requirements - cancel
				s.log.Debugf("Bot %d: Spread too tight (%.4f%% < %.4f%%) and %s, cancelling SELL order",
					inst.Config.ID, spreadPercent, inst.Config.MinGapPercent, skipReason)
				shouldCancel = true
				reason = fmt.Sprintf("Spread too tight (%.4f%% < %.4f%%) and %s - cancelling",
					spreadPercent, inst.Config.MinGapPercent, skipReason)
			} else if inst.LastBuyPrice > 0 {
				// Profit meets requirements - keep the order to lock in profit
				profitPercent := ((order.Price - inst.LastBuyPrice) / inst.LastBuyPrice) * 100
				s.log.Debugf("Bot %d: Spread too tight (%.4f%% < %.4f%%) but profit (%.2f%%) > MinGap (%.4f%%), keeping SELL order",
					inst.Config.ID, spreadPercent, inst.Config.MinGapPercent, profitPercent, inst.Config.MinGapPercent)
				return // Don't cancel - we want to lock in profit
			} else {
				// No buy price tracked - cancel to be safe
				s.log.Debugf("Bot %d: Spread too tight (%.4f%% < %.4f%%) and no buy price tracked, cancelling SELL order",
					inst.Config.ID, spreadPercent, inst.Config.MinGapPercent)
				shouldCancel = true
				reason = fmt.Sprintf("Spread too tight (%.4f%% < %.4f%%) and no buy price tracked - cancelling",
					spreadPercent, inst.Config.MinGapPercent)
			}
		} else {
			// Spread is OK - proceed with normal SELL repositioning logic
			// Calculate expected competitive price
			calculatedPrice, err := s.calculateCompetitivePrice(inst, ticker, "sell")
			if err != nil {
				s.log.Warnf("Bot %d: Failed to calculate competitive price for SELL in checkReposition: %v", inst.Config.ID, err)
				return
			}
			expectedPrice = calculatedPrice

			// Allow small tolerance for price matching (handles floating point precision)
			priceDiff := order.Price - expectedPrice
			if priceDiff < 0 {
				priceDiff = -priceDiff
			}

			if priceDiff > 0.01 {
				// Price doesn't match - cancel to reposition
				shouldCancel = true
				reason = fmt.Sprintf("Our ask (%.2f) != expected price (%.2f) - should match market",
					order.Price, expectedPrice)
			}
			// If price matches, keep the order (no depth check - we need liquidity fast for selling)
		}
	}

	if !shouldCancel {
		s.log.Debugf("Bot %d: Order still at competitive price - %s order price=%.2f matches expected price=%.2f",
			inst.Config.ID, order.Side, order.Price, expectedPrice)
		return
	}

	// Order price doesn't match expected competitive price - cancel and delete
	s.log.Infof("Bot %d: %s order price mismatch: %s. Cancelling and deleting order", inst.Config.ID, order.Side, reason)

	if err := inst.TradeClient.CancelOrder(ctx, inst.Config.Pair, order.OrderID, order.Side); err != nil {
		// Handle API error (rate limiting, order not found, critical errors, etc.)
		if s.handleAPIError(inst, err, "CancelOrder") {
			// Rate limited - will retry
			return
		}

		// Order not found - wait for WebSocket confirmation
		if util.IsOrderNotFoundError(err) {
			s.log.Infof("Bot %d: Order %s not found on exchange - likely filled or cancelled, waiting for WebSocket confirmation", inst.Config.ID, order.OrderID)

			// Balance was already adjusted when order was placed
			// Just wait for WebSocket to confirm if filled or cancelled
			inst.ActiveOrder.Status = "pending_ws_confirm"
			if err := s.orderRepo.UpdateStatus(ctx, order.ID, "pending_ws_confirm"); err != nil {
				s.log.Warnf("Bot %d: Failed to update order status: %v", inst.Config.ID, err)
			}
		}
		return
	}

	// Order cancelled successfully - update status and keep in database for history
	s.orderRepo.UpdateStatus(ctx, order.ID, "cancelled")
	order.Status = "cancelled"

	// Update last order time for rate limiting
	inst.LastOrderTime = time.Now()

	// Notify order cancellation via WebSocket
	s.notificationService.NotifyOrderUpdate(ctx, inst.Config.UserID, order)

	// Clear active order - don't place new order, wait for next ticker signal
	inst.ActiveOrder = nil
	s.log.Debugf("Bot %d: Order cancelled and kept in database for history, waiting for next ticker signal", inst.Config.ID)
}

func (s *MarketMakerService) handleFilled(inst *BotInstance, filledOrder *model.Order, executedQty float64) {
	// Lock to prevent race conditions when modifying ActiveOrder and balances
	inst.mu.Lock()
	defer inst.mu.Unlock()

	ctx := context.Background()

	// Log order fill immediately
	s.log.Debugf("MarketMaker Bot %d: Processing %s order COMPLETELY FILLED - OrderID=%s, Pair=%s, Price=%.2f, ExecutedQty=%.8f",
		inst.Config.ID, filledOrder.Side, filledOrder.OrderID, filledOrder.Pair, filledOrder.Price, executedQty)

	// Check if bot is still running - log a warning if stopped, but still process the fill
	// This is important for live trading where fills are real and must be tracked
	s.mu.RLock()
	_, exists := s.instances[inst.Config.ID]
	botStatus := inst.Config.Status
	s.mu.RUnlock()

	if !exists || botStatus != model.BotStatusRunning {
		s.log.Warnf("Bot %d: Processing order fill for stopped bot (exists=%v, status=%s, orderID=%s) - this is normal for orders placed before stopping",
			inst.Config.ID, exists, botStatus, filledOrder.OrderID)
		// Continue processing - we need to update balances and track the fill even if bot is stopped
		// This is especially important for live trading where fills are real trades
	}

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

	// Calculate delta: amount filled since last update (handle remaining fill for partial orders)
	previouslyFilled := filledOrder.FilledAmount
	newlyFilled := executedQty - previouslyFilled

	if newlyFilled <= 0 {
		s.log.Warnf("Bot %d: Complete fill detected but no new fill delta (executedQty=%.8f, previouslyFilled=%.8f) - order already processed",
			inst.Config.ID, executedQty, previouslyFilled)
		// Still clear ActiveOrder since order is done
		inst.ActiveOrder = nil
		return
	}

	s.log.Debugf("Bot %d: Processing final fill delta - %.8f %s (total executed: %.8f, previously: %.8f)",
		inst.Config.ID, newlyFilled, inst.BaseCurrency, executedQty, previouslyFilled)

	// Safety check: reject unreasonably large amounts (likely corrupted orders)
	if newlyFilled > util.MaxReasonableCoinAmount {
		s.log.Errorf("Bot %d: Rejecting order fill - newlyFilled %.8f is unreasonably large (order likely corrupted)",
			inst.Config.ID, newlyFilled)
		inst.ActiveOrder = nil
		return
	}

	// Safety check: reject unreasonably large prices
	if filledOrder.Price > util.MaxReasonablePrice {
		s.log.Errorf("Bot %d: Rejecting order fill - price %.2f is unreasonably large (order likely corrupted)",
			inst.Config.ID, filledOrder.Price)
		inst.ActiveOrder = nil
		return
	}

	// Calculate total cost/value for the NEW fill delta
	totalValue := newlyFilled * filledOrder.Price

	// Safety check: reject unreasonably large total values
	if totalValue > util.MaxReasonableTotalValue {
		s.log.Errorf("Bot %d: Rejecting order fill - total value %.2f is unreasonably large (order likely corrupted)",
			inst.Config.ID, totalValue)
		inst.ActiveOrder = nil
		return
	}

	s.log.Debugf("Bot %d: handleFilled - side=%s newlyFilled=%.8f price=%.2f totalValue=%.2f, before: IDR=%.2f %s=%.8f",
		inst.Config.ID, filledOrder.Side, newlyFilled, filledOrder.Price, totalValue,
		inst.Config.Balances["idr"], inst.BaseCurrency, inst.Config.Balances[inst.BaseCurrency])

	// CRITICAL: Balance was ALREADY deducted when order was placed!
	// We only need to ADD what we received from the fill
	// - BUY: We already deducted IDR, now we ADD coins
	// - SELL: We already deducted coins, now we ADD IDR

	// 1. Update balance with what we RECEIVED from fill
	if filledOrder.Side == "buy" {
		// BUY: IDR was already deducted when order placed, now add the coins we received
		inst.Config.Balances[inst.BaseCurrency] += newlyFilled
		s.log.Debugf("Bot %d: BUY filled - received %.8f %s (IDR was already locked when order placed)",
			inst.Config.ID, newlyFilled, inst.BaseCurrency)
		// Track buy price and cost for profit calculation (weighted average)
		inst.TotalCoinBought += newlyFilled
		inst.TotalCostIDR += totalValue
		inst.LastBuyPrice = inst.TotalCostIDR / inst.TotalCoinBought // Average buy price
		// Sync to bot config for persistence
		inst.Config.TotalCoinBought = inst.TotalCoinBought
		inst.Config.TotalCostIDR = inst.TotalCostIDR
		inst.Config.LastBuyPrice = inst.LastBuyPrice
		s.log.Debugf("Bot %d: After final BUY fill - IDR=%.2f %s=%.8f (avg buy price: %.2f, total coins: %.8f, total cost: %.2f)",
			inst.Config.ID, inst.Config.Balances["idr"], inst.BaseCurrency, inst.Config.Balances[inst.BaseCurrency],
			inst.LastBuyPrice, inst.TotalCoinBought, inst.TotalCostIDR)

		// Save tracking to database
		if err := s.botRepo.UpdateTracking(ctx, inst.Config.ID, inst.TotalCoinBought, inst.TotalCostIDR, inst.LastBuyPrice); err != nil {
			s.log.Warnf("Bot %d: Failed to save tracking to database: %v", inst.Config.ID, err)
		}
	} else {
		// SELL: Coins were already deducted when order placed, now add the IDR we received
		inst.Config.Balances["idr"] += totalValue
		s.log.Debugf("Bot %d: SELL filled - received %.2f IDR (coins were already locked when order placed)",
			inst.Config.ID, totalValue)

		// 2. Calculate profit BEFORE updating tracking (we need the buy price before it's reduced)
		// Only calculate profit if we have buy tracking (coins bought during this run)
		inst.Config.TotalTrades++
		var profit float64
		var buyPriceUsed float64

		if inst.TotalCoinBought > 0 {
			// We have tracking - calculate profit using average buy price
			buyPriceUsed = inst.TotalCostIDR / inst.TotalCoinBought
			profit = (filledOrder.Price - buyPriceUsed) * newlyFilled
			inst.Config.TotalProfitIDR += profit
			if profit > 0 {
				inst.Config.WinningTrades++
			}
			s.log.Infof("Bot %d: Final SELL profit calculated - sellPrice=%.2f, avgBuyPrice=%.2f, amount=%.8f, profit=%.2f IDR, totalProfit=%.2f IDR",
				inst.Config.ID, filledOrder.Price, buyPriceUsed, newlyFilled, profit, inst.Config.TotalProfitIDR)
		} else {
			// No buy tracking - these coins were from before this run (or manual deposit)
			// Still process the sell (update balances), but skip profit calculation
			s.log.Warnf("Bot %d: Selling %.8f %s but no buy price tracked (coins from previous run or manual deposit) - skipping profit calculation",
				inst.Config.ID, newlyFilled, inst.BaseCurrency)
			s.log.Infof("Bot %d: Final SELL executed - sellPrice=%.2f, amount=%.8f, profit=0.00 IDR (no tracking), totalProfit=%.2f IDR",
				inst.Config.ID, filledOrder.Price, newlyFilled, inst.Config.TotalProfitIDR)
		}

		// 3. Update tracking: reduce coins and cost proportionally (AFTER profit calculation)
		if inst.TotalCoinBought > 0 {
			// Calculate proportion of coins being sold
			sellRatio := newlyFilled / inst.TotalCoinBought
			if sellRatio > 1.0 {
				sellRatio = 1.0 // Cap at 100% if somehow we're selling more than we bought
			}
			// Reduce total cost proportionally
			costOfSoldCoins := inst.TotalCostIDR * sellRatio
			inst.TotalCostIDR -= costOfSoldCoins
			inst.TotalCoinBought -= newlyFilled
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
		// Sync to bot config for persistence
		inst.Config.TotalCoinBought = inst.TotalCoinBought
		inst.Config.TotalCostIDR = inst.TotalCostIDR
		inst.Config.LastBuyPrice = inst.LastBuyPrice

		s.log.Debugf("Bot %d: After final SELL - IDR=%.2f %s=%.8f (added %.2f IDR, remaining coins: %.8f, remaining cost: %.2f)",
			inst.Config.ID, inst.Config.Balances["idr"], inst.BaseCurrency, inst.Config.Balances[inst.BaseCurrency],
			totalValue, inst.TotalCoinBought, inst.TotalCostIDR)

		// Save tracking to database
		if err := s.botRepo.UpdateTracking(ctx, inst.Config.ID, inst.TotalCoinBought, inst.TotalCostIDR, inst.LastBuyPrice); err != nil {
			s.log.Warnf("Bot %d: Failed to save tracking to database: %v", inst.Config.ID, err)
		}

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
			dbOrder.FilledAmount = executedQty
			dbOrder.Status = "filled"
			now := time.Now()
			dbOrder.FilledAt = &now

			// Save updated order
			if err := s.orderRepo.Update(ctx, dbOrder, "open"); err != nil {
				s.log.Errorf("Bot %d: Failed to update order %d: %v", inst.Config.ID, filledOrder.ID, err)
			} else {
				s.log.Debugf("Bot %d: Order filled - ID=%d, executedQty=%.8f (original=%.8f)",
					inst.Config.ID, filledOrder.ID, executedQty, filledOrder.Amount)
				// Update filledOrder reference for WebSocket notification
				filledOrder.FilledAmount = executedQty
				filledOrder.Status = "filled"
				filledOrder.FilledAt = &now
			}
		}
	} else {
		s.log.Warnf("Bot %d: Cannot update order status - order has no database ID (OrderID=%s)",
			inst.Config.ID, filledOrder.OrderID)
		filledOrder.Status = "filled"
		filledOrder.FilledAmount = executedQty
	}
	inst.ActiveOrder = nil

	s.log.Infof("MarketMaker Bot %d: Order FILLED successfully - Side=%s, OrderID=%s, ExecutedQty=%.8f, Price=%.2f, TotalValue=%.2f IDR, NewBalances: IDR=%.2f, %s=%.8f",
		inst.Config.ID, filledOrder.Side, filledOrder.OrderID, executedQty, filledOrder.Price, executedQty*filledOrder.Price,
		inst.Config.Balances["idr"], inst.BaseCurrency, inst.Config.Balances[inst.BaseCurrency])

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
		BuyPrice:       inst.CurrentBid,
		SellPrice:      inst.CurrentAsk,
		SpreadPercent:  (inst.CurrentAsk - inst.CurrentBid) / inst.CurrentBid * 100,
	})
}

func (s *MarketMakerService) handlePartialFill(inst *BotInstance, order *model.Order, executedQty, unfilledQty float64) {
	// Lock to prevent race conditions when modifying ActiveOrder and balances
	inst.mu.Lock()
	defer inst.mu.Unlock()

	ctx := context.Background()

	s.log.Infof("Bot %d: PARTIAL FILL - Order %s, Side=%s, Executed: %.8f, Unfilled: %.8f (%.1f%% complete)",
		inst.Config.ID, order.OrderID, order.Side, executedQty, unfilledQty,
		(executedQty/(executedQty+unfilledQty))*100)

	// Calculate delta: amount filled since last update
	previouslyFilled := order.FilledAmount
	newlyFilled := executedQty - previouslyFilled

	if newlyFilled <= 0 {
		s.log.Debugf("Bot %d: No new fill delta (executedQty=%.8f, previouslyFilled=%.8f)",
			inst.Config.ID, executedQty, previouslyFilled)
		return
	}

	s.log.Debugf("Bot %d: Processing NEW partial fill delta - %.8f %s (total executed: %.8f, previously: %.8f)",
		inst.Config.ID, newlyFilled, inst.BaseCurrency, executedQty, previouslyFilled)

	// Update order's filled amount in database but keep status as "open"
	if order.ID > 0 {
		dbOrder, err := s.orderRepo.GetByID(ctx, order.ID)
		if err == nil {
			dbOrder.FilledAmount = executedQty
			// Keep status as "open" for partial fills
			if err := s.orderRepo.Update(ctx, dbOrder, "open"); err != nil {
				s.log.Errorf("Bot %d: Failed to update partial fill: %v", inst.Config.ID, err)
			} else {
				s.log.Debugf("Bot %d: Updated partial fill in database - FilledAmount=%.8f", inst.Config.ID, executedQty)
			}
		}
	}

	// Update balances immediately with the NEW fill delta
	// Ensure balances map is initialized
	if inst.Config.Balances == nil {
		inst.Config.Balances = make(map[string]float64)
		inst.Config.Balances["idr"] = inst.Config.InitialBalanceIDR
		inst.Config.Balances[inst.BaseCurrency] = 0
	}

	totalValue := newlyFilled * order.Price

	s.log.Debugf("Bot %d: Before partial fill update - IDR=%.2f %s=%.8f",
		inst.Config.ID, inst.Config.Balances["idr"], inst.BaseCurrency, inst.Config.Balances[inst.BaseCurrency])

	// CRITICAL: Balance was ALREADY deducted when order was placed!
	// For partial fills, we only add what we RECEIVED
	if order.Side == "buy" {
		// BUY partial: IDR was already locked, now add the coins we received
		inst.Config.Balances[inst.BaseCurrency] += newlyFilled
		s.log.Debugf("Bot %d: BUY partial fill - received %.8f %s (IDR was already locked)",
			inst.Config.ID, newlyFilled, inst.BaseCurrency)
		// Track buy price and cost for profit calculation (weighted average)
		inst.TotalCoinBought += newlyFilled
		inst.TotalCostIDR += totalValue
		inst.LastBuyPrice = inst.TotalCostIDR / inst.TotalCoinBought
		// Sync to bot config for persistence
		inst.Config.TotalCoinBought = inst.TotalCoinBought
		inst.Config.TotalCostIDR = inst.TotalCostIDR
		inst.Config.LastBuyPrice = inst.LastBuyPrice

		// Save tracking to database
		if err := s.botRepo.UpdateTracking(ctx, inst.Config.ID, inst.TotalCoinBought, inst.TotalCostIDR, inst.LastBuyPrice); err != nil {
			s.log.Warnf("Bot %d: Failed to save tracking to database: %v", inst.Config.ID, err)
		}
	} else {
		// SELL partial: Coins were already locked, now add the IDR we received
		inst.Config.Balances["idr"] += totalValue
		s.log.Debugf("Bot %d: SELL partial fill - received %.2f IDR (coins were already locked)",
			inst.Config.ID, totalValue)

		// Calculate partial profit if we have buy tracking
		if inst.TotalCoinBought > 0 {
			buyPriceUsed := inst.TotalCostIDR / inst.TotalCoinBought
			profit := (order.Price - buyPriceUsed) * newlyFilled
			inst.Config.TotalProfitIDR += profit

			s.log.Infof("Bot %d: Partial SELL fill processed - sold %.8f %s for %.2f IDR, profit=%.2f IDR (sellPrice=%.2f, avgBuyPrice=%.2f)",
				inst.Config.ID, newlyFilled, inst.BaseCurrency, totalValue, profit, order.Price, buyPriceUsed)

			// Update tracking: reduce coins and cost proportionally
			sellRatio := newlyFilled / inst.TotalCoinBought
			if sellRatio > 1.0 {
				sellRatio = 1.0
			}
			costOfSoldCoins := inst.TotalCostIDR * sellRatio
			inst.TotalCostIDR -= costOfSoldCoins
			inst.TotalCoinBought -= newlyFilled
			if inst.TotalCoinBought < 0 {
				inst.TotalCoinBought = 0
			}
			if inst.TotalCoinBought > 0 {
				inst.LastBuyPrice = inst.TotalCostIDR / inst.TotalCoinBought
			} else {
				inst.LastBuyPrice = 0
				inst.TotalCostIDR = 0
			}
			// Sync to bot config for persistence
			inst.Config.TotalCoinBought = inst.TotalCoinBought
			inst.Config.TotalCostIDR = inst.TotalCostIDR
			inst.Config.LastBuyPrice = inst.LastBuyPrice

			// Save tracking to database
			if err := s.botRepo.UpdateTracking(ctx, inst.Config.ID, inst.TotalCoinBought, inst.TotalCostIDR, inst.LastBuyPrice); err != nil {
				s.log.Warnf("Bot %d: Failed to save tracking to database: %v", inst.Config.ID, err)
			}
		} else {
			s.log.Infof("Bot %d: Partial SELL fill processed - sold %.8f %s for %.2f IDR (no buy tracking)",
				inst.Config.ID, newlyFilled, inst.BaseCurrency, totalValue)
		}
	}

	s.log.Debugf("Bot %d: After partial fill update - IDR=%.2f %s=%.8f",
		inst.Config.ID, inst.Config.Balances["idr"], inst.BaseCurrency, inst.Config.Balances[inst.BaseCurrency])

	// Update ActiveOrder's FilledAmount to track progress
	order.FilledAmount = executedQty

	// Save updated balances to database
	if err := s.botRepo.UpdateBalance(ctx, inst.Config.ID, inst.Config.Balances); err != nil {
		s.log.Errorf("Bot %d: Failed to save updated balances: %v", inst.Config.ID, err)
	}

	// Keep ActiveOrder intact - allow repositioning logic to decide
	s.log.Infof("Bot %d: Keeping order %s active (unfilled: %.8f %s), repositioning logic will handle it",
		inst.Config.ID, order.OrderID, unfilledQty, inst.BaseCurrency)

	// Notify via WebSocket about partial fill
	s.notificationService.NotifyOrderUpdate(ctx, inst.Config.UserID, order)
	s.notificationService.NotifyBotUpdate(ctx, inst.Config.UserID, model.WSBotUpdatePayload{
		BotID:          inst.Config.ID,
		Status:         inst.Config.Status,
		BuyPrice:       inst.CurrentBid,
		SellPrice:      inst.CurrentAsk,
		SpreadPercent:  (inst.CurrentAsk - inst.CurrentBid) / inst.CurrentBid * 100,
		Balances:       inst.Config.Balances,
		TotalProfitIDR: inst.Config.TotalProfitIDR,
		TotalTrades:    inst.Config.TotalTrades,
		WinningTrades:  inst.Config.WinningTrades,
		WinRate:        inst.Config.WinRate(),
	})
}

func (s *MarketMakerService) handleLiveOrderUpdate(userID string, order *indodax.OrderUpdate) {
	status := strings.ToLower(order.Status)

	// Handle CANCELLED orders - restore balance if we deducted pessimistically
	if status == "cancelled" || status == "canceled" {
		s.handleCancelledOrder(userID, order)
		return
	}

	// Indodax sends "FILL" or "DONE" for filled orders
	if status != "filled" && status != "fill" && status != "done" {
		return
	}

	// Parse quantities from WebSocket
	executedQty, _ := strconv.ParseFloat(order.ExecutedQty, 64)
	unfilledQty, _ := strconv.ParseFloat(order.UnfilledQty, 64)
	origQty, _ := strconv.ParseFloat(order.OrigQty, 64)
	price, _ := strconv.ParseFloat(order.Price, 64)

	// Check if order is completely filled
	isCompletelyFilled := (unfilledQty == 0) || (executedQty >= origQty)

	// Log full incoming data
	orderJSON, _ := json.Marshal(order)
	s.log.Infof("[WS_ORDER_UPDATE] MarketMaker: Received order update - Status=%s, OrigQty=%.8f, ExecutedQty=%.8f, UnfilledQty=%.8f, Price=%.2f, CompletelyFilled=%v, UserID=%s",
		status, origQty, executedQty, unfilledQty, price, isCompletelyFilled, userID)
	s.log.Debugf("[WS_ORDER_UPDATE] MarketMaker: Full order data: %s", string(orderJSON))

	// Check all running bots for this user
	s.mu.RLock()
	defer s.mu.RUnlock()

	// WebSocket sends ClientOrderID (e.g., "cstidr-buy-1767968961234")
	// We store the same ClientOrderID in our database
	// Simple direct match!
	wsClientOrderID := order.ClientOrderID // Use ClientOrderID, not OrderID!

	found := false
	for _, inst := range s.instances {
		if inst.Config.UserID != userID {
			continue
		}

		// Log matching attempt with detailed info
		if inst.ActiveOrder != nil {
			s.log.Debugf("[WS_ORDER_UPDATE] MarketMaker: Checking bot %d - ActiveOrder.OrderID=%s (status=%s) vs WS.ClientOrderID=%s (WS.OrderID=%s)",
				inst.Config.ID, inst.ActiveOrder.OrderID, inst.ActiveOrder.Status, wsClientOrderID, order.OrderID)
		} else {
			s.log.Debugf("[WS_ORDER_UPDATE] MarketMaker: Bot %d has no ActiveOrder", inst.Config.ID)
		}

		// Direct match using ClientOrderID
		if inst.ActiveOrder != nil && inst.ActiveOrder.OrderID == wsClientOrderID {
			// Found the bot!
			s.log.Infof("[WS_ORDER_UPDATE] MarketMaker: ✅ MATCHED order %s to bot %d (pair=%s, side=%s, price=%.2f, CompletelyFilled=%v)",
				wsClientOrderID, inst.Config.ID, inst.Config.Pair, order.Side, price, isCompletelyFilled)

			if isCompletelyFilled {
				// Order completely filled - process remaining fill
				s.log.Infof("Bot %d: Order %s COMPLETELY FILLED - ExecutedQty=%.8f",
					inst.Config.ID, wsClientOrderID, executedQty)
				s.handleFilled(inst, inst.ActiveOrder, executedQty)
			} else {
				// Partial fill - update status but keep order active
				s.log.Infof("Bot %d: Order %s PARTIALLY FILLED - ExecutedQty=%.8f, UnfilledQty=%.8f (%.1f%% filled)",
					inst.Config.ID, wsClientOrderID, executedQty, unfilledQty, (executedQty/origQty)*100)
				s.handlePartialFill(inst, inst.ActiveOrder, executedQty, unfilledQty)
			}

			found = true
			break
		}

		// Fallback: Look up order from database if ActiveOrder doesn't match
		// This handles cases where ActiveOrder was cleared but order still exists
		ctx := context.Background()
		dbOrder, err := s.orderRepo.GetByOrderID(ctx, wsClientOrderID)
		if err == nil && dbOrder.ParentID == inst.Config.ID && dbOrder.ParentType == "bot" {
			s.log.Infof("[WS_ORDER_UPDATE] MarketMaker: ✅ MATCHED order %s to bot %d via database lookup (pair=%s, side=%s, CompletelyFilled=%v)",
				wsClientOrderID, inst.Config.ID, dbOrder.Pair, dbOrder.Side, isCompletelyFilled)

			if isCompletelyFilled {
				// Order completely filled
				s.handleFilled(inst, dbOrder, executedQty)
			} else {
				// Partial fill - process incrementally
				s.log.Infof("Bot %d: Partial fill for order from database - ExecutedQty=%.8f, UnfilledQty=%.8f",
					inst.Config.ID, executedQty, unfilledQty)
				s.handlePartialFill(inst, dbOrder, executedQty, unfilledQty)
			}

			found = true
			break
		}
	}

	if !found {
		s.log.Warnf("[WS_ORDER_UPDATE] MarketMaker: ❌ Filled order %s NOT matched to any active bot order for user %s - order may be from stopped bot or external",
			order.OrderID, userID)
	}
}

func (s *MarketMakerService) handleCancelledOrder(userID string, order *indodax.OrderUpdate) {
	// Parse quantities
	executedQty, _ := strconv.ParseFloat(order.ExecutedQty, 64)
	origQty, _ := strconv.ParseFloat(order.OrigQty, 64)
	unfilledQty := origQty - executedQty

	wsClientOrderID := order.ClientOrderID

	s.log.Infof("[WS_ORDER_UPDATE] MarketMaker: Received CANCELLED order update - OrderID=%s, OrigQty=%.8f, ExecutedQty=%.8f, UnfilledQty=%.8f",
		wsClientOrderID, origQty, executedQty, unfilledQty)

	// Check all running bots for this user
	s.mu.RLock()
	defer s.mu.RUnlock()

	found := false
	for _, inst := range s.instances {
		if inst.Config.UserID != userID {
			continue
		}

		// Lock bot instance for safe access to ActiveOrder and Balances
		inst.mu.Lock()
		defer inst.mu.Unlock()

		// Check if this is the active order
		if inst.ActiveOrder != nil && inst.ActiveOrder.OrderID == wsClientOrderID {
			s.log.Infof("[WS_ORDER_UPDATE] MarketMaker: ✅ MATCHED cancelled order %s to bot %d (ActiveOrder)",
				wsClientOrderID, inst.Config.ID)

			// Save order reference before any operations
			cancelledOrder := inst.ActiveOrder

			// RESTORE locked funds from cancelled order
			// When we placed the order, we deducted the balance
			// Now that it's cancelled, we need to return what wasn't filled
			if cancelledOrder.Side == "sell" {
				// Return unfilled coins that were locked
				inst.Config.Balances[inst.BaseCurrency] += unfilledQty
				s.log.Infof("Bot %d: SELL order cancelled - restored %.8f %s (unfilled coins returned)",
					inst.Config.ID, unfilledQty, inst.BaseCurrency)
			} else if cancelledOrder.Side == "buy" {
				// Return unfilled IDR that was locked
				returnedIDR := unfilledQty * cancelledOrder.Price
				inst.Config.Balances["idr"] += returnedIDR
				s.log.Infof("Bot %d: BUY order cancelled - restored %.2f IDR (unfilled amount returned)",
					inst.Config.ID, returnedIDR)
			}

			// Save updated balance
			ctx := context.Background()
			if err := s.botRepo.UpdateBalance(ctx, inst.Config.ID, inst.Config.Balances); err != nil {
				s.log.Warnf("Bot %d: Failed to save balance after cancellation: %v", inst.Config.ID, err)
			}

			// Clear ActiveOrder so bot can place new orders
			inst.ActiveOrder = nil

			// Determine final status based on whether order was partially filled
			finalStatus := "cancelled"
			if executedQty > 0 {
				finalStatus = "partial"
				s.log.Infof("Bot %d: Order %s was PARTIALLY FILLED (%.8f/%.8f = %.1f%%) before cancellation",
					inst.Config.ID, cancelledOrder.OrderID, executedQty, origQty, (executedQty/origQty)*100)
			}

			// Update order status in database (keep the record for history)
			if err := s.orderRepo.UpdateStatus(ctx, cancelledOrder.ID, finalStatus); err != nil {
				s.log.Warnf("Bot %d: Failed to update cancelled order status in DB: %v", inst.Config.ID, err)
			} else {
				s.log.Debugf("Bot %d: Order %s status updated to '%s' in database (kept for history)",
					inst.Config.ID, cancelledOrder.OrderID, finalStatus)
			}

			// Notify frontend
			cancelledOrder.Status = finalStatus
			s.notificationService.NotifyOrderUpdate(ctx, userID, cancelledOrder)

			found = true
			break
		}

		// Fallback: Look up order from database if ActiveOrder doesn't match
		ctx := context.Background()
		dbOrder, err := s.orderRepo.GetByOrderID(ctx, wsClientOrderID)
		if err == nil && dbOrder.ParentID == inst.Config.ID && dbOrder.ParentType == "bot" {
			s.log.Infof("[WS_ORDER_UPDATE] MarketMaker: ✅ MATCHED cancelled order %s to bot %d via database lookup",
				wsClientOrderID, inst.Config.ID)

			// RESTORE locked funds from cancelled order
			if dbOrder.Side == "sell" {
				// Return unfilled coins that were locked
				inst.Config.Balances[inst.BaseCurrency] += unfilledQty
				s.log.Infof("Bot %d: SELL order cancelled - restored %.8f %s (unfilled coins returned)",
					inst.Config.ID, unfilledQty, inst.BaseCurrency)
			} else if dbOrder.Side == "buy" {
				// Return unfilled IDR that was locked
				returnedIDR := unfilledQty * dbOrder.Price
				inst.Config.Balances["idr"] += returnedIDR
				s.log.Infof("Bot %d: BUY order cancelled - restored %.2f IDR (unfilled amount returned)",
					inst.Config.ID, returnedIDR)
			}

			// Save updated balance
			if err := s.botRepo.UpdateBalance(ctx, inst.Config.ID, inst.Config.Balances); err != nil {
				s.log.Warnf("Bot %d: Failed to save balance after cancellation: %v", inst.Config.ID, err)
			}

			// Clear ActiveOrder if it matches
			if inst.ActiveOrder != nil && inst.ActiveOrder.OrderID == wsClientOrderID {
				inst.ActiveOrder = nil
			}

			// Determine final status based on whether order was partially filled
			finalStatus := "cancelled"
			if executedQty > 0 {
				finalStatus = "partial"
				s.log.Infof("Bot %d: Order %s was PARTIALLY FILLED (%.8f/%.8f = %.1f%%) before cancellation",
					inst.Config.ID, dbOrder.OrderID, executedQty, origQty, (executedQty/origQty)*100)
			}

			// Update order status in database (keep the record for history)
			if err := s.orderRepo.UpdateStatus(ctx, dbOrder.ID, finalStatus); err != nil {
				s.log.Warnf("Bot %d: Failed to update cancelled order status in DB: %v", inst.Config.ID, err)
			} else {
				s.log.Debugf("Bot %d: Order %s status updated to '%s' in database (kept for history)",
					inst.Config.ID, dbOrder.OrderID, finalStatus)
			}

			// Notify frontend
			dbOrder.Status = finalStatus
			s.notificationService.NotifyOrderUpdate(ctx, userID, dbOrder)

			found = true
			break
		}
	}

	if !found {
		// Database fallback: Check if order belongs to a stopped bot
		ctx := context.Background()
		dbOrder, err := s.orderRepo.GetByOrderID(ctx, wsClientOrderID)
		if err == nil && dbOrder.ParentType == "bot" {
			// Found the order in database - it belongs to a stopped bot
			bot, err := s.botRepo.GetByID(ctx, dbOrder.ParentID)
			if err == nil {
				s.log.Infof("[WS_ORDER_UPDATE] MarketMaker: ✅ MATCHED cancelled order %s to STOPPED bot %d via database",
					wsClientOrderID, bot.ID)

				// Only restore balance if order was still open/pending (not already cancelled)
				if dbOrder.Status == "open" || dbOrder.Status == "pending_ws_confirm" {
					// Derive base currency from pair
					baseCurrency := ""
					if strings.HasSuffix(dbOrder.Pair, "idr") {
						baseCurrency = strings.TrimSuffix(dbOrder.Pair, "idr")
					} else if strings.Contains(dbOrder.Pair, "_") {
						parts := strings.Split(dbOrder.Pair, "_")
						baseCurrency = parts[0]
					} else {
						baseCurrency = dbOrder.Pair
					}

					// RESTORE locked funds from cancelled order
					if dbOrder.Side == "sell" {
						// Return unfilled coins that were locked
						bot.Balances[baseCurrency] += unfilledQty
						s.log.Infof("Bot %d (STOPPED): SELL order cancelled - restored %.8f %s (unfilled coins returned)",
							bot.ID, unfilledQty, baseCurrency)
					} else if dbOrder.Side == "buy" {
						// Return unfilled IDR that was locked
						returnedIDR := unfilledQty * dbOrder.Price
						bot.Balances["idr"] += returnedIDR
						s.log.Infof("Bot %d (STOPPED): BUY order cancelled - restored %.2f IDR (unfilled amount returned)",
							bot.ID, returnedIDR)
					}

					// Save updated balance
					if err := s.botRepo.UpdateBalance(ctx, bot.ID, bot.Balances); err != nil {
						s.log.Warnf("Bot %d: Failed to save balance after cancellation: %v", bot.ID, err)
					}
				} else {
					s.log.Debugf("Bot %d: Order %s already had status '%s', no balance restoration needed",
						bot.ID, wsClientOrderID, dbOrder.Status)
				}

				// Determine final status based on whether order was partially filled
				finalStatus := "cancelled"
				if executedQty > 0 {
					finalStatus = "partial"
					s.log.Infof("Bot %d (STOPPED): Order %s was PARTIALLY FILLED (%.8f/%.8f = %.1f%%) before cancellation",
						bot.ID, dbOrder.OrderID, executedQty, origQty, (executedQty/origQty)*100)
				}

				// Update order status in database
				if err := s.orderRepo.UpdateStatus(ctx, dbOrder.ID, finalStatus); err != nil {
					s.log.Warnf("Bot %d: Failed to update cancelled order status in DB: %v", bot.ID, err)
				} else {
					s.log.Infof("Bot %d (STOPPED): Order %s status updated to '%s' in database (kept for history)",
						bot.ID, dbOrder.OrderID, finalStatus)
				}

				// Notify frontend
				dbOrder.Status = finalStatus
				s.notificationService.NotifyOrderUpdate(ctx, userID, dbOrder)

				found = true
			}
		}

		if !found {
			s.log.Warnf("[WS_ORDER_UPDATE] MarketMaker: ❌ Cancelled order %s NOT matched to any bot for user %s - may be external or manually placed order",
				order.OrderID, userID)
		}
	}
}

// calculateCompetitivePrice calculates the competitive price for placing or checking an order
// Returns the price that should be used based on whether we're the only buyer/seller
func (s *MarketMakerService) calculateCompetitivePrice(
	inst *BotInstance,
	ticker market.OrderBookTicker,
	side string, // "buy" or "sell"
) (float64, error) {
	if inst.PairInfo == nil {
		return 0, fmt.Errorf("pair info not available for bot %d", inst.Config.ID)
	}

	tickSize := s.getTickSize(*inst.PairInfo)
	var price float64
	var noCompetition bool

	if side == "buy" {
		noCompetition = s.isOnlyBuyer(inst, ticker)
		if noCompetition {
			price = inst.CurrentBid // Match best bid if no other buyers
			s.log.Debugf("Bot %d: calculateCompetitivePrice BUY - no other buyers → price = best bid (%.2f)",
				inst.Config.ID, price)
		} else {
			price = inst.CurrentBid + tickSize // Add tick to outbid other buyers
			s.log.Debugf("Bot %d: calculateCompetitivePrice BUY - other buyers competing → price = best bid (%.2f) + tick (%.2f) = %.2f",
				inst.Config.ID, inst.CurrentBid, tickSize, price)
		}
	} else if side == "sell" {
		noCompetition = s.isOnlySeller(inst, ticker)
		if noCompetition {
			price = inst.CurrentAsk // Match best ask if no other sellers
			s.log.Debugf("Bot %d: calculateCompetitivePrice SELL - no other sellers → price = best ask (%.2f)",
				inst.Config.ID, price)
		} else {
			price = inst.CurrentAsk - tickSize // Subtract tick to undercut other sellers
			s.log.Debugf("Bot %d: calculateCompetitivePrice SELL - other sellers competing → price = best ask (%.2f) - tick (%.2f) = %.2f",
				inst.Config.ID, inst.CurrentAsk, tickSize, price)
		}
	} else {
		return 0, fmt.Errorf("invalid side: %s (must be 'buy' or 'sell')", side)
	}

	// Round price to pair's price precision (critical for Indodax API)
	if tickSize >= 1.0 {
		// For IDR pairs (whole number prices), round to nearest increment then cast to int
		price = util.RoundToNearestIncrement(price, tickSize)
		price = float64(int64(price)) // Force to exact integer for IDR
		s.log.Debugf("Bot %d: calculateCompetitivePrice %s - after rounding to increment %.0f: %.0f",
			inst.Config.ID, side, tickSize, price)
	} else {
		price = util.RoundToPrecision(price, inst.PairInfo.PricePrecision)
		s.log.Debugf("Bot %d: calculateCompetitivePrice %s - after rounding to precision %d: %.2f",
			inst.Config.ID, side, inst.PairInfo.PricePrecision, price)
	}

	return price, nil
}

// validateSellProfit checks if a sell order should be placed based on profit requirements
// Returns (shouldSkip, reason) - if shouldSkip is true, the order should not be placed
func (s *MarketMakerService) validateSellProfit(inst *BotInstance, sellPrice float64) (bool, string) {
	if inst.LastBuyPrice <= 0 {
		// No buy price tracked - allow selling (let user decide)
		return false, ""
	}

	// Calculate current profit/loss percentage
	profitPercent := ((sellPrice - inst.LastBuyPrice) / inst.LastBuyPrice) * 100

	// If loss exceeds 5%, skip selling to avoid realizing large losses
	if profitPercent < -5.0 {
		return true, fmt.Sprintf("at loss (%.2f%%) and price may recover, holding", profitPercent)
	}

	// Also check if profit meets minimum gap requirement
	if profitPercent <= inst.Config.MinGapPercent {
		return true, fmt.Sprintf("profit (%.2f%%) <= MinGap (%.4f%%), not profitable enough",
			profitPercent, inst.Config.MinGapPercent)
	}

	return false, ""
}

// validateAndNormalizeBalances ensures balances are initialized and valid
// Uses shared balance validation utility
func (s *MarketMakerService) validateAndNormalizeBalances(inst *BotInstance) {
	requiredCurrencies := []string{"idr", inst.BaseCurrency}
	inst.Config.Balances = util.ValidateAndNormalizeBalances(
		inst.Config.Balances,
		requiredCurrencies,
		inst.Config.InitialBalanceIDR,
		s.log,
	)
}

// handleAPIError handles API errors consistently across Trade and CancelOrder calls
// Returns true if the error was handled and operation should retry, false otherwise
func (s *MarketMakerService) handleAPIError(inst *BotInstance, err error, operation string) bool {
	if err == nil {
		return false
	}

	// Handle rate limiting errors
	if strings.Contains(strings.ToLower(err.Error()), "too_many_requests") ||
		strings.Contains(strings.ToLower(err.Error()), "rate limit") {
		s.log.Warnf("Bot %d: Rate limited during %s - will retry after backoff", inst.Config.ID, operation)
		inst.LastOrderTime = time.Now() // Update to enforce debounce
		return true                     // Should retry
	}

	// Handle order not found errors (expected if order was already filled/cancelled)
	if util.IsOrderNotFoundError(err) {
		s.log.Infof("Bot %d: Order not found on exchange during %s - likely filled or cancelled, waiting for WebSocket confirmation",
			inst.Config.ID, operation)
		return false // Don't retry, wait for WebSocket
	}

	// Handle critical trading errors (API key or invalid pair)
	if util.IsCriticalTradingError(err) && !inst.Config.IsPaperTrading {
		s.stopBotWithError(inst.Config.ID, inst.Config.UserID, fmt.Sprintf("Trading error during %s: %v", operation, err))
		return false // Don't retry, bot stopped
	}

	// Other errors - log and return false (don't retry)
	s.log.Errorf("Bot %d: Error during %s: %v", inst.Config.ID, operation, err)
	return false
}

func (s *MarketMakerService) getTickSize(pair indodax.Pair) float64 {
	// Try to get actual price increment from market data service first
	if increment, ok := s.marketDataService.GetPriceIncrement(pair.ID); ok && increment > 0 {
		s.log.Debugf("getTickSize for %s: Using price increment from API = %.10f", pair.ID, increment)
		return increment
	}

	// Fallback: calculate from price precision
	// For IDR pairs (whole numbers), PricePrecision is typically 0, so tick = 1
	fallback := 1.0 / util.Pow10(pair.PricePrecision)
	s.log.Debugf("getTickSize for %s: Using fallback from PricePrecision=%d = %.10f", pair.ID, pair.PricePrecision, fallback)
	return fallback
}

// isOnlyBuyer checks if there are other BUYERS competing when we want to place a BUY order
// This is used when placing BUY orders to decide if we need to add tick to compete with other buyers
// Returns TRUE if: there are NO other buyers (we can just match current best bid)
// Returns FALSE if: there are other buyers competing (we should add tick to outbid them)
func (s *MarketMakerService) isOnlyBuyer(inst *BotInstance, ticker market.OrderBookTicker) bool {
	// Check the BID side (buyers) to see if there's competition
	if len(ticker.Bids) == 0 {
		s.log.Debugf("Bot %d: No buyers in orderbook → no competition, can place at current best bid", inst.Config.ID)
		return true
	}

	// If we don't have an active buy order, check if there are multiple buyers
	if inst.ActiveOrder == nil || inst.ActiveOrder.Side != "buy" {
		if len(ticker.Bids) >= 2 {
			bestBidPrice := ticker.Bids[0].Price
			secondBidPrice := ticker.Bids[1].Price
			// If there are 2+ different bid levels, there's buyer competition
			if bestBidPrice > secondBidPrice {
				s.log.Debugf("Bot %d: Multiple buyers competing (best bid=%.2f, 2nd bid=%.2f) → ADD TICK to outbid them",
					inst.Config.ID, bestBidPrice, secondBidPrice)
				return false // Multiple buyers, need to add tick
			}
		}
		// Only one buyer level
		s.log.Debugf("Bot %d: Only one buyer level → no competition", inst.Config.ID)
		return true
	}

	// We have an active buy order - check if it's the only one at best bid
	bestBid := ticker.Bids[0]
	bestBidPrice := bestBid.Price
	bestBidVolumeIDR := bestBid.IDRVolume

	// Check if our buy order price matches best bid
	priceDiff := inst.ActiveOrder.Price - bestBidPrice
	if priceDiff < 0 {
		priceDiff = -priceDiff
	}

	if priceDiff > 0.01 {
		// Our buy is not at best bid, there are other buyers ahead
		s.log.Debugf("Bot %d: Our buy (%.2f) NOT at best bid (%.2f) → other buyers ahead → ADD TICK",
			inst.Config.ID, inst.ActiveOrder.Price, bestBidPrice)
		return false
	}

	// Our buy is at best bid - check if it's the only one (volume match)
	ourOrderValueIDR := inst.ActiveOrder.Amount * inst.ActiveOrder.Price
	volumeDiff := ourOrderValueIDR - bestBidVolumeIDR
	if volumeDiff < 0 {
		volumeDiff = -volumeDiff
	}

	tolerance := ourOrderValueIDR * 0.01
	s.log.Debugf("Bot %d: Our buy at best bid - volume check: our=%.2f IDR, bestBid=%.2f IDR, diff=%.2f",
		inst.Config.ID, ourOrderValueIDR, bestBidVolumeIDR, volumeDiff)

	if volumeDiff <= tolerance {
		s.log.Debugf("Bot %d: ✓ Our buy is the ONLY buyer at best bid → no need to add tick",
			inst.Config.ID)
		return true // We're the only buyer, no need to be aggressive
	}

	s.log.Debugf("Bot %d: Other buyers at best bid (volume mismatch) → ADD TICK to outbid them",
		inst.Config.ID)
	return false // Other buyers at same price
}

// isOnlySeller checks if there are other SELLERS competing when we want to place a SELL order
// This is used when placing SELL orders to decide if we need to subtract tick to compete with other sellers
// Returns TRUE if: there are NO other sellers (we can just match current best ask)
// Returns FALSE if: there are other sellers competing (we should subtract tick to undercut them)
func (s *MarketMakerService) isOnlySeller(inst *BotInstance, ticker market.OrderBookTicker) bool {
	// Check the ASK side (sellers) to see if there's competition
	if len(ticker.Asks) == 0 {
		s.log.Debugf("Bot %d: No sellers in orderbook → no competition, can place at current best ask", inst.Config.ID)
		return true
	}

	// If we don't have an active sell order, check if there are multiple sellers
	if inst.ActiveOrder == nil || inst.ActiveOrder.Side != "sell" {
		if len(ticker.Asks) >= 2 {
			bestAskPrice := ticker.Asks[0].Price
			secondAskPrice := ticker.Asks[1].Price
			// If there are 2+ different ask levels, there's seller competition
			if secondAskPrice > bestAskPrice {
				s.log.Debugf("Bot %d: Multiple sellers competing (best ask=%.2f, 2nd ask=%.2f) → SUBTRACT TICK to undercut them",
					inst.Config.ID, bestAskPrice, secondAskPrice)
				return false // Multiple sellers, need to subtract tick
			}
		}
		// Only one seller level
		s.log.Debugf("Bot %d: Only one seller level → no competition", inst.Config.ID)
		return true
	}

	// We have an active sell order - check if it's the only one at best ask
	bestAsk := ticker.Asks[0]
	bestAskPrice := bestAsk.Price
	bestAskVolume := bestAsk.BaseVolume // Coin volume for asks

	// Check if our sell order price matches best ask
	priceDiff := inst.ActiveOrder.Price - bestAskPrice
	if priceDiff < 0 {
		priceDiff = -priceDiff
	}

	if priceDiff > 0.01 {
		// Our sell is not at best ask, there are other sellers ahead
		s.log.Debugf("Bot %d: Our sell (%.2f) NOT at best ask (%.2f) → other sellers ahead → SUBTRACT TICK",
			inst.Config.ID, inst.ActiveOrder.Price, bestAskPrice)
		return false
	}

	// Our sell is at best ask - check if it's the only one (volume match)
	ourOrderVolume := inst.ActiveOrder.Amount // Coin amount

	// If bestAskVolume is 0 or very small, it means our order is the only one at this price
	if bestAskVolume < util.TinyBalanceThreshold {
		s.log.Debugf("Bot %d: ✓ Best ask volume is 0 → our sell is the ONLY seller at best ask → no need to subtract tick",
			inst.Config.ID)
		return true
	}

	volumeDiff := ourOrderVolume - bestAskVolume
	if volumeDiff < 0 {
		volumeDiff = -volumeDiff
	}

	tolerance := ourOrderVolume * 0.01
	s.log.Debugf("Bot %d: Our sell at best ask - volume check: our=%.8f coins, bestAsk=%.8f coins, diff=%.8f",
		inst.Config.ID, ourOrderVolume, bestAskVolume, volumeDiff)

	if volumeDiff <= tolerance {
		s.log.Debugf("Bot %d: ✓ Our sell is the ONLY seller at best ask → no need to subtract tick",
			inst.Config.ID)
		return true // We're the only seller, no need to be aggressive
	}

	s.log.Debugf("Bot %d: Other sellers at best ask (volume mismatch) → SUBTRACT TICK to undercut them",
		inst.Config.ID)
	return false // Other sellers at same price
}

// checkOrderbookDepth checks if there's sufficient depth in the orderbook
// Returns true if depth is sufficient, false if market is too thin
func (s *MarketMakerService) checkOrderbookDepth(ticker market.OrderBookTicker, side string, orderSizeIDR float64, minGapPercent float64) bool {
	const MIN_DEPTH_LEVELS = 3       // Need at least 3 price levels
	const MIN_DEPTH_MULTIPLIER = 2.0 // Depth should be at least 2x our order size
	const MAX_BID_GAP_PERCENT = 0.5  // Max 0.5% gap between bid[0] and bid[1]

	if len(ticker.Bids) < MIN_DEPTH_LEVELS || len(ticker.Asks) < MIN_DEPTH_LEVELS {
		s.log.Debugf("Orderbook depth too thin: bids=%d asks=%d (need at least %d levels)",
			len(ticker.Bids), len(ticker.Asks), MIN_DEPTH_LEVELS)
		return false
	}

	if side == "buy" {
		// Check if gap between bid[0] and bid[1] is too large (thin market)
		if len(ticker.Bids) >= 2 {
			bidGap := ticker.Bids[0].Price - ticker.Bids[1].Price
			bidGapPercent := (bidGap / ticker.Bids[0].Price) * 100
			if bidGapPercent > MAX_BID_GAP_PERCENT {
				s.log.Debugf("Bid gap too large: bid[0]=%.2f, bid[1]=%.2f, gap=%.2f (%.4f%%) > threshold (%.2f%%)",
					ticker.Bids[0].Price, ticker.Bids[1].Price, bidGap, bidGapPercent, MAX_BID_GAP_PERCENT)
				return false
			}
		}
		// For buy orders, check depth below our bid (we'll be placing at best bid)
		// Sum up IDR volume at the first few bid levels
		totalDepthIDR := 0.0
		levelsToCheck := MIN_DEPTH_LEVELS
		if levelsToCheck > len(ticker.Bids) {
			levelsToCheck = len(ticker.Bids)
		}

		for i := 0; i < levelsToCheck; i++ {
			totalDepthIDR += ticker.Bids[i].IDRVolume
		}

		// Check if depth is sufficient (at least 2x our order size)
		minRequiredDepth := orderSizeIDR * MIN_DEPTH_MULTIPLIER
		if totalDepthIDR < minRequiredDepth {
			s.log.Debugf("Insufficient buy depth: total=%.2f IDR, required=%.2f IDR (order size=%.2f IDR)",
				totalDepthIDR, minRequiredDepth, orderSizeIDR)
			return false
		}

		s.log.Debugf("Buy depth OK: total=%.2f IDR across %d levels (order size=%.2f IDR)",
			totalDepthIDR, levelsToCheck, orderSizeIDR)
		return true
	} else {
		// For sell orders, check depth above our ask (we'll be placing at best ask)
		totalDepthIDR := 0.0
		levelsToCheck := MIN_DEPTH_LEVELS
		if levelsToCheck > len(ticker.Asks) {
			levelsToCheck = len(ticker.Asks)
		}

		for i := 0; i < levelsToCheck; i++ {
			totalDepthIDR += ticker.Asks[i].IDRVolume
		}

		minRequiredDepth := orderSizeIDR * MIN_DEPTH_MULTIPLIER
		if totalDepthIDR < minRequiredDepth {
			s.log.Debugf("Insufficient sell depth: total=%.2f IDR, required=%.2f IDR (order size=%.2f IDR)",
				totalDepthIDR, minRequiredDepth, orderSizeIDR)
			return false
		}

		s.log.Debugf("Sell depth OK: total=%.2f IDR across %d levels (order size=%.2f IDR)",
			totalDepthIDR, levelsToCheck, orderSizeIDR)
		return true
	}
}

func (s *MarketMakerService) syncBalance(ctx context.Context, inst *BotInstance) error {
	// For live bots, we check real IDR on Indodax to ensure we don't allocate more than exists
	var realIDR float64
	if !inst.Config.IsPaperTrading {
		info, err := inst.TradeClient.GetInfo(ctx)
		if err != nil {
			// If critical trading error (API key) and live trading, stop the bot
			if util.IsCriticalTradingError(err) && !inst.Config.IsPaperTrading {
				s.stopBotWithError(inst.Config.ID, inst.Config.UserID, fmt.Sprintf("Trading error: %v", err))
			}
			return fmt.Errorf("failed to fetch live balance: %w", err)
		}
		realIDR, _ = strconv.ParseFloat(info.Balance["idr"].String(), 64)
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

	// Calculate average buy price from totals (more accurate than LastBuyPrice which might be stale)
	var avgBuyPrice float64
	if inst.TotalCoinBought > 0 {
		avgBuyPrice = inst.TotalCostIDR / inst.TotalCoinBought
	} else if inst.LastBuyPrice > 0 {
		// Fallback to LastBuyPrice if totals are not available
		avgBuyPrice = inst.LastBuyPrice
	} else {
		// No buy price tracked, can't calculate profit accurately
		s.log.Warnf("Bot %d: Cannot calculate profit - no buy price tracked (TotalCoinBought=%.8f, TotalCostIDR=%.2f, LastBuyPrice=%.2f)",
			inst.Config.ID, inst.TotalCoinBought, inst.TotalCostIDR, inst.LastBuyPrice)
		return 0.0
	}

	// Gross profit before fees
	grossProfit := (sellOrder.Price - avgBuyPrice) * sellAmount

	// Calculate fees
	buyCost := avgBuyPrice * sellAmount
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
	if req.InitialBalanceIDR < util.MinInitialBalanceIDR {
		return util.ErrBadRequest(fmt.Sprintf("Initial balance must be at least %.0f IDR", util.MinInitialBalanceIDR))
	}
	if req.OrderSizeIDR < util.MinOrderValueIDR {
		return util.ErrBadRequest(fmt.Sprintf("Order size must be at least %.0f IDR", util.MinOrderValueIDR))
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

// startOrderCleanupRoutine starts a background goroutine that periodically cleans up stale and duplicate orders
func (s *MarketMakerService) startOrderCleanupRoutine() {
	ticker := time.NewTicker(1 * time.Minute) // Run every 1 minute
	defer ticker.Stop()

	s.log.Infof("Order cleanup routine started - will run every 1 minute")

	for {
		select {
		case <-ticker.C:
			s.cleanupStaleOrders()
		case <-s.cleanupStopChan:
			s.log.Infof("Order cleanup routine stopped")
			return
		}
	}
}

// cleanupStaleOrders checks all running bots and cleans up duplicate/stale orders
func (s *MarketMakerService) cleanupStaleOrders() {
	ctx := context.Background()
	s.log.Debugf("Starting order cleanup cycle")

	// Get all running bots
	s.mu.RLock()
	runningBots := make([]*BotInstance, 0, len(s.instances))
	for _, inst := range s.instances {
		runningBots = append(runningBots, inst)
	}
	s.mu.RUnlock()

	s.log.Debugf("Order cleanup: checking %d running bots", len(runningBots))

	// Process each running bot
	for _, inst := range runningBots {
		s.cleanupBotOrders(ctx, inst)
	}

	s.log.Debugf("Order cleanup cycle completed")
}

// cleanupBotOrders cleans up duplicate and stale orders for a single bot
func (s *MarketMakerService) cleanupBotOrders(ctx context.Context, inst *BotInstance) {
	botID := inst.Config.ID
	userID := inst.Config.UserID

	// Get all open orders for this bot from database
	openOrders, err := s.orderRepo.ListByParentAndUser(ctx, userID, "bot", botID, 0)
	if err != nil {
		s.log.Debugf("Bot %d: Failed to fetch orders for cleanup: %v", botID, err)
		return
	}

	// Filter to only open/pending orders
	var activeOrders []*model.Order
	for _, order := range openOrders {
		if order.Status == "open" || order.Status == "pending" || order.Status == "pending_ws_confirm" {
			activeOrders = append(activeOrders, order)
		}
	}

	if len(activeOrders) == 0 {
		s.log.Debugf("Bot %d: No open orders to clean up", botID)
		return
	}

	s.log.Debugf("Bot %d: Found %d open order(s) in database", botID, len(activeOrders))

	// Sort by CreatedAt (newest first)
	sort.Slice(activeOrders, func(i, j int) bool {
		return activeOrders[i].CreatedAt.After(activeOrders[j].CreatedAt)
	})

	// Keep the latest order
	latestOrder := activeOrders[0]
	ordersToCancel := activeOrders[1:]

	// Check if latest order is stale (> 5 minutes old)
	now := time.Now()
	staleThreshold := 5 * time.Minute
	latestOrderAge := now.Sub(latestOrder.CreatedAt)

	if latestOrderAge > staleThreshold {
		// Latest order is also stale - cancel it too
		s.log.Debugf("Bot %d: Latest order %s is stale (age: %v) - will cancel", botID, latestOrder.OrderID, latestOrderAge)
		ordersToCancel = append(ordersToCancel, latestOrder)
		latestOrder = nil
	} else {
		s.log.Debugf("Bot %d: Keeping latest order %s (age: %v)", botID, latestOrder.OrderID, latestOrderAge)
	}

	// Cancel all orders that need to be cancelled
	for _, order := range ordersToCancel {
		s.cancelOrderForCleanup(ctx, inst, order)
	}

	if latestOrder == nil {
		s.log.Debugf("Bot %d: All orders cancelled (all were stale or duplicates)", botID)
	} else {
		s.log.Debugf("Bot %d: Cleanup complete - kept order %s, cancelled %d duplicate/stale order(s)",
			botID, latestOrder.OrderID, len(ordersToCancel))
	}
}

// cancelOrderForCleanup cancels an order and restores locked funds
func (s *MarketMakerService) cancelOrderForCleanup(ctx context.Context, inst *BotInstance, order *model.Order) {
	botID := inst.Config.ID

	s.log.Debugf("Bot %d: Cancelling order %s (side=%s, price=%.2f, status=%s)",
		botID, order.OrderID, order.Side, order.Price, order.Status)

	// Cancel on Indodax if live trading
	if !inst.Config.IsPaperTrading {
		if err := inst.TradeClient.CancelOrder(ctx, order.Pair, order.OrderID, order.Side); err != nil {
			s.log.Debugf("Bot %d: Failed to cancel order %s on Indodax: %v", botID, order.OrderID, err)
			// Continue anyway - might already be cancelled
		} else {
			s.log.Debugf("Bot %d: Successfully cancelled order %s on Indodax", botID, order.OrderID)
		}
	}

	// Update status in database
	finalStatus := "cancelled"
	if order.FilledAmount > 0 {
		finalStatus = "partial"
		s.log.Debugf("Bot %d: Order %s was partially filled (%.8f/%.8f) - marking as 'partial'",
			botID, order.OrderID, order.FilledAmount, order.Amount)
	}

	if err := s.orderRepo.UpdateStatus(ctx, order.ID, finalStatus); err != nil {
		s.log.Debugf("Bot %d: Failed to update order %s status to '%s': %v", botID, order.OrderID, finalStatus, err)
	} else {
		s.log.Debugf("Bot %d: Updated order %s status to '%s' in database", botID, order.OrderID, finalStatus)
	}

	// Restore locked funds (unfilled portion)
	inst.mu.Lock()
	unfilledAmount := order.Amount - order.FilledAmount

	if order.Side == "sell" {
		inst.Config.Balances[inst.BaseCurrency] += unfilledAmount
		s.log.Debugf("Bot %d: Restored %.8f %s from cancelled SELL order (unfilled portion)",
			botID, unfilledAmount, inst.BaseCurrency)
	} else if order.Side == "buy" {
		unfilledValueIDR := unfilledAmount * order.Price
		inst.Config.Balances["idr"] += unfilledValueIDR
		s.log.Debugf("Bot %d: Restored %.2f IDR from cancelled BUY order (unfilled portion)",
			botID, unfilledValueIDR)
	}
	inst.mu.Unlock()

	// Save updated balance
	if err := s.botRepo.UpdateBalance(ctx, botID, inst.Config.Balances); err != nil {
		s.log.Debugf("Bot %d: Failed to save balance after cancelling order: %v", botID, err)
	} else {
		s.log.Debugf("Bot %d: Balance updated after cancelling order %s", botID, order.OrderID)
	}
}
