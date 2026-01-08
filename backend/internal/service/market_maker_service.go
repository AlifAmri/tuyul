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

	// 3. Create bot config
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

	// Initialize base currency balance to 0 so it shows up in UI
	if pairInfo, ok := s.marketDataService.GetPairInfo(req.Pair); ok {
		bot.Balances[pairInfo.BaseCurrency] = 0
	}

	// 4. Save to repository
	if err := s.botRepo.Create(ctx, bot); err != nil {
		s.log.Errorf("Failed to create bot: %v", err)
		return nil, util.ErrInternalServer("Failed to create bot")
	}

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
	inst.BaseCurrency = pairInfo.BaseCurrency

	s.instances[botID] = inst

	// 4. Subscribe to ticker
	err = s.subManager.Subscribe(bot.Pair, func(ticker market.OrderBookTicker) {
		select {
		case inst.TickerChan <- ticker:
		default:
			// Full channel, skip ticker
		}
	})
	if err != nil {
		delete(s.instances, botID)
		return util.ErrInternalServer(fmt.Sprintf("Failed to subscribe to ticker: %v", err))
	}

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

	// 4. Start event loop
	go s.runBot(inst)

	s.log.Infof("Bot %d started successfully for pair %s", botID, bot.Pair)

	// Notify via WebSocket
	s.notificationService.NotifyBotUpdate(ctx, userID, model.WSBotUpdatePayload{
		BotID:  botID,
		Status: model.BotStatusRunning,
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

	// 1. Signal stop
	close(inst.StopChan)

	// 2. Unsubscribe (needs exact handler, for now we will fix later)
	// s.subManager.Unsubscribe(bot.Pair, handler)

	// 3. Update status in DB
	err = s.botRepo.UpdateStatus(ctx, botID, model.BotStatusStopped, nil)

	// Notify via WebSocket
	s.notificationService.NotifyBotUpdate(ctx, userID, model.WSBotUpdatePayload{
		BotID:  botID,
		Status: model.BotStatusStopped,
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
	// 1. Check volatility
	coin, err := s.marketDataService.GetCoin(context.Background(), inst.Config.Pair)
	if err == nil && coin.Volatility1m > 2.0 {
		return // Too volatile, pause
	}

	// 2. Update prices
	inst.CurrentBid = ticker.BestBid
	inst.CurrentAsk = ticker.BestAsk

	// 3. Check minimum gap
	spreadPercent := (ticker.BestAsk - ticker.BestBid) / ticker.BestBid * 100
	if spreadPercent < inst.Config.MinGapPercent {
		return // Spread too tight
	}

	// 4. Process orders
	if inst.ActiveOrder == nil {
		s.placeNewOrder(inst)
	} else {
		s.checkReposition(inst)
	}
}

func (s *MarketMakerService) placeNewOrder(inst *BotInstance) {
	// Determine side based on inventory
	idrBalance := inst.Config.Balances["idr"]
	coinBalance := inst.Config.Balances[inst.BaseCurrency]

	pairInfo, ok := s.marketDataService.GetPairInfo(inst.Config.Pair)
	if !ok {
		return
	}

	var side string
	var price, amount float64

	if coinBalance >= float64(pairInfo.TradeMinBaseCurrency) {
		// Have coins -> SELL ALL
		side = "sell"
		price = inst.CurrentAsk - s.getTickSize(pairInfo) // Competitive sell
		amount = coinBalance
	} else if idrBalance >= inst.Config.OrderSizeIDR {
		// Have IDR -> BUY
		side = "buy"
		price = inst.CurrentBid + s.getTickSize(pairInfo) // Competitive buy
		amount = inst.Config.OrderSizeIDR / price
	} else {
		// Insufficient balance
		return
	}

	// Round amount and validate
	amount = util.FloorToPrecision(amount, pairInfo.VolumePrecision)
	if amount < float64(pairInfo.TradeMinBaseCurrency) || amount*price < pairInfo.TradeMinTradedCurrency {
		return
	}

	// Place order
	ctx := context.Background()
	res, err := inst.TradeClient.Trade(ctx, side, inst.Config.Pair, price, amount, "limit")
	if err != nil {
		s.log.Errorf("Failed to place %s order for bot %d: %v", side, inst.Config.ID, err)
		return
	}

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
		inst.ActiveOrder = nil
	}
}

func (s *MarketMakerService) handleFilled(inst *BotInstance, filledOrder *model.Order) {
	ctx := context.Background()

	// 1. Update balance
	if filledOrder.Side == "buy" {
		inst.Config.Balances["idr"] -= filledOrder.Amount * filledOrder.Price
		inst.Config.Balances[inst.BaseCurrency] += filledOrder.Amount
	} else {
		inst.Config.Balances[inst.BaseCurrency] -= filledOrder.Amount
		inst.Config.Balances["idr"] += filledOrder.Amount * filledOrder.Price

		// 2. Update stats and profit
		inst.Config.TotalTrades++
		// Simplified profit calculation for now (assumes last buy price was lower)
		// For market maker, profit is ideally (SellPrice - BuyPrice) * Amount
		// Since we handle both sequentially, we can calculate based on the spread captured
		// For now just track it roughly
		profit := s.calculateProfit(filledOrder)
		inst.Config.TotalProfitIDR += profit
		if profit > 0 {
			inst.Config.WinningTrades++
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

	s.orderRepo.UpdateStatus(ctx, filledOrder.ID, "filled")
	inst.ActiveOrder = nil

	s.log.Infof("Bot %d order filled: %s %.8f @ %.2f", inst.Config.ID, filledOrder.Side, filledOrder.Amount, filledOrder.Price)

	// Notify via WebSocket
	s.notificationService.NotifyBotUpdate(ctx, inst.Config.UserID, model.WSBotUpdatePayload{
		BotID:          inst.Config.ID,
		Status:         inst.Config.Status,
		TotalTrades:    inst.Config.TotalTrades,
		WinningTrades:  inst.Config.WinningTrades,
		TotalProfitIDR: inst.Config.TotalProfitIDR,
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

func (s *MarketMakerService) calculateProfit(order *model.Order) float64 {
	// Crude profit calculation: assuming 0.1% fee on each side (total 0.2%)
	// And assuming we captured the spread.
	// Actually we should track the 'cost basis' for a true calculation.
	// For now, let's just return a placeholder or simple logic
	return 0.0 // Placeholder
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
