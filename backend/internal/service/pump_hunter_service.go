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
	PendingOrders map[int64]*model.Position // Track pending orders for false pump detection
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
	marketDataService.OnUpdate(s.HandleCoinUpdate)

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
		PendingOrders: make(map[int64]*model.Position),
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

		// Verify subscription exists - REQUIRED for live trading
		// Subscription should be established on API boot or when API key is created
		if !s.orderMonitor.IsSubscribed(userID) {
			s.log.Errorf("User %s is not subscribed to order updates. Subscription is required for live trading.", userID)
			return fmt.Errorf("cannot start live bot: user is not subscribed to order updates. Please ensure your API key is configured correctly")
		}
	}

	// 3. Load active positions from DB
	activePos, err := s.posRepo.ListActiveByBot(ctx, botID)
	if err == nil {
		for _, pos := range activePos {
			if pos.Status == model.PositionStatusPending {
				// Restore pending orders - verify they still exist
				if bot.IsPaperTrading {
					// For paper trading, check if order is stale (older than 1 hour)
					if time.Since(pos.OrderPlacedAt) > 1*time.Hour {
						s.log.Warnf("Bot %d: Pending order for position %d is stale (placed %v ago), cancelling",
							botID, pos.ID, time.Since(pos.OrderPlacedAt))
						// Cancel stale pending order (cancelPendingOrder handles its own mutex)
						s.cancelPendingOrder(inst, pos, "stale_on_restore")
						continue
					}
					s.log.Debugf("Bot %d: Restored pending paper trading position %d (OrderID=%s)",
						botID, pos.ID, pos.EntryOrderID)
				} else {
					// For live trading, verify order still exists on Indodax
					if pos.EntryOrderID != "" {
						orderInfo, err := inst.TradeClient.GetOrder(ctx, pos.Pair, pos.EntryOrderID)
						if err != nil {
							// Order not found or error - might be filled/cancelled
							// But also might be a temporary API error - add to PendingOrders anyway
							// monitorPendingOrders will check again and cancel if still not found
							s.log.Warnf("Bot %d: Pending order %s not found on Indodax: indodax API error: %v, will check again in monitorPendingOrders",
								botID, pos.EntryOrderID, err)
							// Add to PendingOrders anyway - monitorPendingOrders will verify and cancel if needed
							inst.PendingOrders[pos.ID] = pos
							s.log.Debugf("Bot %d: Added position %d (%s) to PendingOrders despite GetOrder error - will verify in monitorPendingOrders",
								botID, pos.ID, pos.Pair)
							continue
						}

						// Check order status
						indodaxStatus := strings.ToLower(orderInfo.Status)
						if indodaxStatus == "filled" || indodaxStatus == "done" {
							// Order was filled while API was down - process the fill
							s.log.Infof("Bot %d: Pending order %s was filled while API was down, processing fill...",
								botID, pos.EntryOrderID)

							// Try to find the order in database first
							var dbOrder *model.Order
							if pos.InternalEntryOrderID > 0 {
								// Use internal order ID if available
								dbOrder, _ = s.orderRepo.GetByID(ctx, pos.InternalEntryOrderID)
							}

							// If not found by internal ID, try by EntryOrderID (could be numeric or ClientOrderID)
							if dbOrder == nil && pos.EntryOrderID != "" {
								dbOrder, _ = s.orderRepo.GetByOrderID(ctx, pos.EntryOrderID)
							}

							// If still not found, try to find orders by position ID
							if dbOrder == nil {
								orders, err := s.orderRepo.ListByParentAndUser(ctx, bot.UserID, "position", pos.ID, 1)
								if err == nil && len(orders) > 0 {
									dbOrder = orders[0] // Use most recent order
									s.log.Debugf("Bot %d: Found order by position ID - OrderID=%d, IndodaxOrderID=%s",
										botID, dbOrder.ID, dbOrder.OrderID)
								}
							}

							// Create a filled order to trigger handleOrderFilled
							filledOrder := &model.Order{
								OrderID:      pos.EntryOrderID,
								Pair:         pos.Pair,
								Side:         "buy",
								Price:        pos.EntryPrice,
								Amount:       pos.EntryQuantity,
								FilledAmount: pos.EntryQuantity,
								Status:       "filled",
								IsPaperTrade: false,
							}

							// If we found the order in database, use its ID
							if dbOrder != nil {
								filledOrder.ID = dbOrder.ID
								filledOrder.OrderID = dbOrder.OrderID // Use the stored OrderID from database
								s.log.Debugf("Bot %d: Using database order ID %d for filled order", botID, dbOrder.ID)
							}

							now := time.Now()
							filledOrder.FilledAt = &now
							// Process fill (this will move position from pending to open)
							go s.handleOrderFilled(inst, filledOrder)
							continue
						} else if indodaxStatus == "cancelled" {
							// Order was cancelled externally
							s.log.Warnf("Bot %d: Pending order %s was cancelled externally, cancelling position",
								botID, pos.EntryOrderID)
							// Cancel pending order (cancelPendingOrder handles its own mutex)
							s.cancelPendingOrder(inst, pos, "order_cancelled_externally")
							continue
						} else if indodaxStatus == "open" {
							// Order still open - restore it
							s.log.Debugf("Bot %d: Verified and restored pending order %s (status: %s)",
								botID, pos.EntryOrderID, indodaxStatus)
						} else {
							// Unknown status - cancel to be safe
							s.log.Warnf("Bot %d: Pending order %s has unknown status '%s', cancelling position",
								botID, pos.EntryOrderID, indodaxStatus)
							// Cancel pending order (cancelPendingOrder handles its own mutex)
							s.cancelPendingOrder(inst, pos, "unknown_order_status")
							continue
						}
					}
				}
				// Add to pending orders after verification
				inst.PendingOrders[pos.ID] = pos
				s.log.Debugf("Bot %d: Added position %d (%s) to PendingOrders for false pump monitoring",
					botID, pos.ID, pos.Pair)
			} else if pos.Status == model.PositionStatusOpen {
				// Restore open positions - initialize tracking fields if missing
				if pos.LastPriceCheck.IsZero() {
					// Set to 1 minute ago so first check happens immediately
					pos.LastPriceCheck = time.Now().Add(-1 * time.Minute)
					s.log.Debugf("Bot %d: Initialized LastPriceCheck for restored position %d (set to 1 min ago for immediate check)", botID, pos.ID)
				}
				if pos.HighestPrice == 0 {
					pos.HighestPrice = pos.EntryPrice
					s.log.Debugf("Bot %d: Initialized HighestPrice for restored position %d", botID, pos.ID)
				}
				if pos.LowestPrice == 0 {
					pos.LowestPrice = pos.EntryPrice
					s.log.Debugf("Bot %d: Initialized LowestPrice for restored position %d", botID, pos.ID)
				}
				// Update position in DB to save initialized fields
				s.posRepo.Update(ctx, pos)
				inst.OpenPositions[pos.ID] = pos
				s.log.Infof("Bot %d: Restored open position %d for %s (status: %s)", botID, pos.ID, pos.Pair, pos.Status)

				// Check if we need to place a sell order based on current target profit
				// This handles the case where target profit was changed (e.g., from 1% to 30%)
				targetProfit := bot.ExitRules.TargetProfitPercent
				if targetProfit > 1.0 && pos.ExitOrderID == "" {
					// Target profit > 1% and no sell order exists - place limit sell order
					// This can happen if:
					// 1. Position was opened with target = 1% (waiting for ATH)
					// 2. Bot was stopped and target changed to > 1%
					// 3. Bot restarted - now we need to place sell order with new target
					sellPrice := pos.EntryPrice * (1 + targetProfit/100)
					s.log.Infof("Bot %d: Placing sell order for restored position %d with new target %.2f%% (was waiting for ATH, now target=%.2f%%)",
						botID, pos.ID, targetProfit, targetProfit)
					// Note: placeLimitSellOrder will update position status to "selling" and create order
					// We don't need to lock here since bot isn't running yet (runBot hasn't started)
					s.placeLimitSellOrder(inst, pos, sellPrice)
				} else if targetProfit == 1.0 {
					// Target = 1% - continue monitoring for ATH (existing behavior)
					s.log.Debugf("Bot %d: Restored position %d will continue ATH monitoring (target=1%%)", botID, pos.ID)
				}
			} else if pos.Status == model.PositionStatusSelling {
				// Restore positions with sell orders - verify sell order exists
				if pos.LastPriceCheck.IsZero() {
					pos.LastPriceCheck = time.Now()
					s.log.Debugf("Bot %d: Initialized LastPriceCheck for restored position %d", botID, pos.ID)
				}
				if pos.HighestPrice == 0 {
					pos.HighestPrice = pos.EntryPrice
					s.log.Debugf("Bot %d: Initialized HighestPrice for restored position %d", botID, pos.ID)
				}
				if pos.LowestPrice == 0 {
					pos.LowestPrice = pos.EntryPrice
					s.log.Debugf("Bot %d: Initialized LowestPrice for restored position %d", botID, pos.ID)
				}

				// Verify sell order exists
				if pos.ExitOrderID == "" {
					// No sell order ID - need to place one
					s.log.Warnf("Bot %d: Position %d has status 'selling' but no ExitOrderID, placing new sell order", botID, pos.ID)
					inst.mu.Lock()
					targetProfit := bot.ExitRules.TargetProfitPercent
					if targetProfit > 1.0 {
						// Strategy A: Place limit sell order
						sellPrice := pos.EntryPrice * (1 + targetProfit/100)
						s.placeLimitSellOrder(inst, pos, sellPrice)
					} else {
						// Strategy B: Change status back to open for ATH monitoring
						pos.Status = model.PositionStatusOpen
						s.posRepo.Update(ctx, pos)
						s.log.Infof("Bot %d: Position %d changed back to 'open' for ATH monitoring (target=1%%)", botID, pos.ID)
						// Broadcast status change
						s.notificationService.NotifyPositionUpdate(ctx, bot.UserID, pos)
					}
					inst.mu.Unlock()
				} else if bot.IsPaperTrading {
					// For paper trading, check order status in database
					dbOrder, err := s.orderRepo.GetByOrderID(ctx, pos.ExitOrderID)
					if err != nil {
						// Order not found in database - might have been cleaned up or never existed
						s.log.Warnf("Bot %d: Sell order %s for position %d not found in database, placing new sell order",
							botID, pos.ExitOrderID, pos.ID)
						inst.mu.Lock()
						targetProfit := bot.ExitRules.TargetProfitPercent
						if targetProfit > 1.0 {
							sellPrice := pos.EntryPrice * (1 + targetProfit/100)
							s.placeLimitSellOrder(inst, pos, sellPrice)
						} else {
							pos.Status = model.PositionStatusOpen
							s.posRepo.Update(ctx, pos)
							s.log.Infof("Bot %d: Position %d changed back to 'open' for ATH monitoring", botID, pos.ID)
							s.notificationService.NotifyPositionUpdate(ctx, bot.UserID, pos)
						}
						inst.mu.Unlock()
					} else if dbOrder.Status == "filled" {
						// Order was already filled - handle it
						s.log.Infof("Bot %d: Sell order %s for position %d was already filled, handling completion",
							botID, pos.ExitOrderID, pos.ID)
						inst.mu.Lock()
						filledOrder := &model.Order{
							ID:      dbOrder.ID,
							OrderID: dbOrder.OrderID,
							Side:    dbOrder.Side,
							Pair:    dbOrder.Pair,
							Price:   dbOrder.Price,
							Amount:  dbOrder.FilledAmount,
							Status:  "filled",
						}
						if dbOrder.FilledAmount == 0 {
							filledOrder.Amount = dbOrder.Amount // Fallback to original amount if filled amount not set
						}
						inst.mu.Unlock()
						go s.handleOrderFilled(inst, filledOrder)
						continue
					} else if dbOrder.Status == "cancelled" {
						// Order was cancelled - place new one
						s.log.Warnf("Bot %d: Sell order %s for position %d was cancelled, placing new sell order",
							botID, pos.ExitOrderID, pos.ID)
						inst.mu.Lock()
						targetProfit := bot.ExitRules.TargetProfitPercent
						if targetProfit > 1.0 {
							sellPrice := pos.EntryPrice * (1 + targetProfit/100)
							s.placeLimitSellOrder(inst, pos, sellPrice)
						} else {
							pos.Status = model.PositionStatusOpen
							s.posRepo.Update(ctx, pos)
							s.log.Infof("Bot %d: Position %d changed back to 'open' for ATH monitoring", botID, pos.ID)
							s.notificationService.NotifyPositionUpdate(ctx, bot.UserID, pos)
						}
						inst.mu.Unlock()
					} else if time.Since(dbOrder.UpdatedAt) > 1*time.Hour {
						// Order is stale (older than 1 hour) - place new one
						s.log.Warnf("Bot %d: Sell order %s for position %d is stale (updated %v ago), placing new sell order",
							botID, pos.ExitOrderID, pos.ID, time.Since(dbOrder.UpdatedAt))
						inst.mu.Lock()
						targetProfit := bot.ExitRules.TargetProfitPercent
						if targetProfit > 1.0 {
							sellPrice := pos.EntryPrice * (1 + targetProfit/100)
							s.placeLimitSellOrder(inst, pos, sellPrice)
						} else {
							pos.Status = model.PositionStatusOpen
							s.posRepo.Update(ctx, pos)
							s.log.Infof("Bot %d: Position %d changed back to 'open' for ATH monitoring", botID, pos.ID)
							s.notificationService.NotifyPositionUpdate(ctx, bot.UserID, pos)
						}
						inst.mu.Unlock()
					} else {
						// Order exists and is still open - restore it
						s.log.Debugf("Bot %d: Verified paper trading sell order %s for position %d (status: %s)",
							botID, pos.ExitOrderID, pos.ID, dbOrder.Status)
					}
				} else {
					// For live trading, verify order still exists on Indodax
					orderInfo, err := inst.TradeClient.GetOrder(ctx, pos.Pair, pos.ExitOrderID)
					if err != nil {
						// Order lookup failed - might be filled/cancelled or invalid
						s.log.Warnf("Bot %d: Failed to verify sell order %s for position %d on Indodax: indodax API error: %v - placing new sell order",
							botID, pos.ExitOrderID, pos.ID, err)
						inst.mu.Lock()
						targetProfit := bot.ExitRules.TargetProfitPercent
						if targetProfit > 1.0 {
							sellPrice := pos.EntryPrice * (1 + targetProfit/100)
							s.placeLimitSellOrder(inst, pos, sellPrice)
						} else {
							pos.Status = model.PositionStatusOpen
							s.posRepo.Update(ctx, pos)
							s.log.Infof("Bot %d: Position %d changed back to 'open' for ATH monitoring", botID, pos.ID)
							s.notificationService.NotifyPositionUpdate(ctx, bot.UserID, pos)
						}
						inst.mu.Unlock()
					} else {
						// Check order status from Indodax
						indodaxStatus := strings.ToLower(orderInfo.Status)
						if indodaxStatus == "filled" || indodaxStatus == "done" {
							// Order was filled - handle it
							s.log.Infof("Bot %d: Sell order %s for position %d was already filled, handling completion",
								botID, pos.ExitOrderID, pos.ID)
							inst.mu.Lock()
							price, _ := strconv.ParseFloat(orderInfo.Price, 64)
							amount, _ := strconv.ParseFloat(orderInfo.OrderCoin, 64)
							filledOrder := &model.Order{
								OrderID: pos.ExitOrderID,
								Side:    "sell",
								Pair:    pos.Pair,
								Price:   price,
								Amount:  amount,
								Status:  "filled",
							}
							inst.mu.Unlock()
							go s.handleOrderFilled(inst, filledOrder)
							continue
						} else if indodaxStatus == "cancelled" {
							// Order was cancelled externally - place new one
							s.log.Warnf("Bot %d: Sell order %s for position %d was cancelled externally, placing new sell order",
								botID, pos.ExitOrderID, pos.ID)
							inst.mu.Lock()
							targetProfit := bot.ExitRules.TargetProfitPercent
							if targetProfit > 1.0 {
								sellPrice := pos.EntryPrice * (1 + targetProfit/100)
								s.placeLimitSellOrder(inst, pos, sellPrice)
							} else {
								pos.Status = model.PositionStatusOpen
								s.posRepo.Update(ctx, pos)
								s.log.Infof("Bot %d: Position %d changed back to 'open' for ATH monitoring", botID, pos.ID)
								s.notificationService.NotifyPositionUpdate(ctx, bot.UserID, pos)
							}
							inst.mu.Unlock()
						} else if indodaxStatus == "open" {
							// Order still open - restore it
							s.log.Debugf("Bot %d: Verified and restored sell order %s for position %d (status: %s)",
								botID, pos.ExitOrderID, pos.ID, indodaxStatus)
						} else {
							// Unknown status - place new order to be safe
							s.log.Warnf("Bot %d: Sell order %s for position %d has unknown status '%s', placing new sell order",
								botID, pos.ExitOrderID, pos.ID, indodaxStatus)
							inst.mu.Lock()
							targetProfit := bot.ExitRules.TargetProfitPercent
							if targetProfit > 1.0 {
								sellPrice := pos.EntryPrice * (1 + targetProfit/100)
								s.placeLimitSellOrder(inst, pos, sellPrice)
							} else {
								pos.Status = model.PositionStatusOpen
								s.posRepo.Update(ctx, pos)
								s.log.Infof("Bot %d: Position %d changed back to 'open' for ATH monitoring", botID, pos.ID)
								s.notificationService.NotifyPositionUpdate(ctx, bot.UserID, pos)
							}
							inst.mu.Unlock()
						}
					}
				}

				// Update position in DB to save initialized fields
				s.posRepo.Update(ctx, pos)
				inst.OpenPositions[pos.ID] = pos
				s.log.Infof("Bot %d: Restored position %d for %s (status: %s)", botID, pos.ID, pos.Pair, pos.Status)
			}
		}
	}

	// 4. Start background monitoring for exits (time-based, score-based)
	s.log.Infof("Bot %d: Starting background monitoring - OpenPositions=%d, PendingOrders=%d",
		botID, len(inst.OpenPositions), len(inst.PendingOrders))
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

	// Validate RiskManagement fields
	if req.RiskManagement.MaxPositionIDR <= 0 {
		return nil, util.ErrBadRequest("max_position_idr must be greater than 0")
	}
	if req.RiskManagement.MaxConcurrentPositions <= 0 {
		return nil, util.ErrBadRequest("max_concurrent_positions must be greater than 0")
	}

	// 1.5. Check for duplicate bot (same type, pair, and mode)
	// For Pump Hunter, Pair is always "ALL"
	exists, err := s.botRepo.ExistsByTypePairMode(ctx, userID, model.BotTypePumpHunter, "ALL", req.IsPaperTrading, 0)
	if err != nil {
		return nil, err
	}
	if exists {
		modeStr := "paper"
		if !req.IsPaperTrading {
			modeStr = "live"
		}
		return nil, util.ErrBadRequest(fmt.Sprintf("A Pump Hunter bot in %s trading mode already exists. You can only have one Pump Hunter bot per mode (paper/live).", modeStr))
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

	// Check for duplicate bot if mode is being changed (exclude current bot)
	// For Pump Hunter, Pair is always "ALL", so we only check mode
	if bot.IsPaperTrading != req.IsPaperTrading {
		exists, err := s.botRepo.ExistsByTypePairMode(ctx, userID, model.BotTypePumpHunter, "ALL", req.IsPaperTrading, botID)
		if err != nil {
			return nil, err
		}
		if exists {
			modeStr := "paper"
			if !req.IsPaperTrading {
				modeStr = "live"
			}
			return nil, util.ErrBadRequest(fmt.Sprintf("A Pump Hunter bot in %s trading mode already exists. You can only have one Pump Hunter bot per mode (paper/live).", modeStr))
		}
	}

	// Update fields
	bot.Name = req.Name
	bot.IsPaperTrading = req.IsPaperTrading
	bot.APIKeyID = req.APIKeyID

	// Merge EntryRules (only update fields that are provided)
	if req.EntryRules != nil {
		if bot.EntryRules == nil {
			bot.EntryRules = &model.PumpHunterEntryRules{}
		}
		if req.EntryRules.MinPumpScore > 0 {
			bot.EntryRules.MinPumpScore = req.EntryRules.MinPumpScore
		}
		if req.EntryRules.MinTimeframesPositive > 0 {
			bot.EntryRules.MinTimeframesPositive = req.EntryRules.MinTimeframesPositive
		}
		if req.EntryRules.Min24hVolumeIDR > 0 {
			bot.EntryRules.Min24hVolumeIDR = req.EntryRules.Min24hVolumeIDR
		}
		if req.EntryRules.MinPriceIDR > 0 {
			bot.EntryRules.MinPriceIDR = req.EntryRules.MinPriceIDR
		}
		if req.EntryRules.ExcludedPairs != nil {
			bot.EntryRules.ExcludedPairs = req.EntryRules.ExcludedPairs
		}
		if req.EntryRules.AllowedPairs != nil {
			bot.EntryRules.AllowedPairs = req.EntryRules.AllowedPairs
		}
	}

	// Merge ExitRules (only update fields that are provided)
	if req.ExitRules != nil {
		if bot.ExitRules == nil {
			bot.ExitRules = &model.PumpHunterExitRules{}
		}
		if req.ExitRules.TargetProfitPercent > 0 {
			bot.ExitRules.TargetProfitPercent = req.ExitRules.TargetProfitPercent
		}
		if req.ExitRules.StopLossPercent > 0 {
			bot.ExitRules.StopLossPercent = req.ExitRules.StopLossPercent
		}
		bot.ExitRules.TrailingStopEnabled = req.ExitRules.TrailingStopEnabled
		if req.ExitRules.TrailingStopPercent > 0 {
			bot.ExitRules.TrailingStopPercent = req.ExitRules.TrailingStopPercent
		}
		if req.ExitRules.MaxHoldMinutes > 0 {
			bot.ExitRules.MaxHoldMinutes = req.ExitRules.MaxHoldMinutes
		}
		bot.ExitRules.ExitOnPumpScoreDrop = req.ExitRules.ExitOnPumpScoreDrop
		if req.ExitRules.PumpScoreDropThreshold > 0 {
			bot.ExitRules.PumpScoreDropThreshold = req.ExitRules.PumpScoreDropThreshold
		}
	}

	// Merge RiskManagement (only update fields that are provided, and validate)
	if req.RiskManagement != nil {
		if bot.RiskManagement == nil {
			bot.RiskManagement = &model.PumpHunterRiskManagement{}
		}
		// Only update if provided and > 0 (to avoid overwriting with 0)
		if req.RiskManagement.MaxPositionIDR > 0 {
			bot.RiskManagement.MaxPositionIDR = req.RiskManagement.MaxPositionIDR
		}
		if req.RiskManagement.MaxConcurrentPositions > 0 {
			bot.RiskManagement.MaxConcurrentPositions = req.RiskManagement.MaxConcurrentPositions
		}
		if req.RiskManagement.DailyLossLimitIDR > 0 {
			bot.RiskManagement.DailyLossLimitIDR = req.RiskManagement.DailyLossLimitIDR
		}
		if req.RiskManagement.CooldownAfterLossMinutes > 0 {
			bot.RiskManagement.CooldownAfterLossMinutes = req.RiskManagement.CooldownAfterLossMinutes
		}
		if req.RiskManagement.MinBalanceIDR > 0 {
			bot.RiskManagement.MinBalanceIDR = req.RiskManagement.MinBalanceIDR
		}

		// Validate final values (after merge)
		if bot.RiskManagement.MaxPositionIDR <= 0 {
			return nil, util.ErrBadRequest("max_position_idr must be greater than 0")
		}
		if bot.RiskManagement.MaxConcurrentPositions <= 0 {
			return nil, util.ErrBadRequest("max_concurrent_positions must be greater than 0")
		}
	}

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
			posOrders, err := s.orderRepo.ListByParentAndUser(ctx, userID, "position", pos.ID, 0) // 0 = no limit, get all
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
	botOrders, err := s.orderRepo.ListByParentAndUser(ctx, userID, "bot", botID, 0) // 0 = no limit, get all
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
	s.log.Infof("StopBot called for bot %d by user %s", botID, userID)

	s.mu.Lock()
	inst, ok := s.instances[botID]
	if !ok {
		s.mu.Unlock()
		s.log.Warnf("Bot %d not found in instances map, but updating status anyway", botID)
		// Bot instance not found, but update status anyway
		bgCtx := context.Background()
		if err := s.botRepo.UpdateStatus(bgCtx, botID, model.BotStatusStopped, nil); err != nil {
			s.log.Errorf("Failed to update status for bot %d: %v", botID, err)
			return err
		}
		return nil
	}

	if inst.Config.UserID != userID {
		s.mu.Unlock()
		s.log.Warnf("User %s attempted to stop bot %d owned by %s", userID, botID, inst.Config.UserID)
		return util.ErrForbidden("Access denied")
	}

	// Stop the bot loop immediately
	s.log.Infof("Stopping bot %d - closing StopChan and removing from instances", botID)
	close(inst.StopChan)
	delete(s.instances, botID)
	s.mu.Unlock()
	s.log.Infof("Bot %d removed from instances map", botID)

	// Update status immediately (don't wait for order cancellations)
	inst.Config.Status = model.BotStatusStopped

	// Use background context for DB operations to avoid timeout
	bgCtx := context.Background()
	err := s.botRepo.UpdateStatus(bgCtx, botID, model.BotStatusStopped, nil)
	if err != nil {
		s.log.Errorf("Failed to update status for bot %d: %v", botID, err)
		return err
	}
	s.log.Infof("Bot %d status updated to stopped in database", botID)

	// Notify via WebSocket asynchronously (don't block response)
	go func() {
		notifyCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		stoppedBot, _ := s.botRepo.GetByID(notifyCtx, botID)
		if stoppedBot != nil {
			s.notificationService.NotifyBotUpdate(notifyCtx, userID, model.WSBotUpdatePayload{
				BotID:          botID,
				Status:         model.BotStatusStopped,
				TotalTrades:    stoppedBot.TotalTrades,
				WinningTrades:  stoppedBot.WinningTrades,
				WinRate:        stoppedBot.WinRate(),
				TotalProfitIDR: stoppedBot.TotalProfitIDR,
				Balances:       stoppedBot.Balances,
			})
		}
	}()

	// Cancel orders asynchronously in background (don't block the response)
	go func() {
		cancelCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Cancel any open orders for this bot
		positions, err := s.posRepo.ListByBot(cancelCtx, botID)
		if err == nil {
			for _, pos := range positions {
				posOrders, err := s.orderRepo.ListByParentAndUser(cancelCtx, userID, "position", pos.ID, 0)
				if err == nil {
					for _, order := range posOrders {
						if order.Status == "open" {
							s.log.Infof("Bot %d: Cancelling open order %d (ID: %s) for position %d", botID, order.ID, order.OrderID, pos.ID)
							if err := inst.TradeClient.CancelOrder(cancelCtx, order.Pair, order.OrderID, order.Side); err != nil {
								s.log.Warnf("Bot %d: Failed to cancel order %d: indodax API error: %v", botID, order.ID, err)
							} else {
								s.orderRepo.UpdateStatus(cancelCtx, order.ID, "cancelled")
								order.Status = "cancelled"
								s.notificationService.NotifyOrderUpdate(cancelCtx, userID, order)
							}
						}
					}
				}
			}
		}

		// Also check for any direct bot orders
		botOrders, err := s.orderRepo.ListByParentAndUser(cancelCtx, userID, "bot", botID, 0)
		if err == nil {
			for _, order := range botOrders {
				if order.Status == "open" {
					s.log.Infof("Bot %d: Cancelling open order %d (ID: %s)", botID, order.ID, order.OrderID)
					if err := inst.TradeClient.CancelOrder(cancelCtx, order.Pair, order.OrderID, order.Side); err != nil {
						// "Order not found" is expected if order was already filled/cancelled
						if util.IsOrderNotFoundError(err) {
							s.log.Infof("Bot %d: Order %d (ID: %s) already filled/cancelled (not found)", botID, order.ID, order.OrderID)
						} else {
							s.log.Warnf("Bot %d: Failed to cancel order %d: indodax API error: %v", botID, order.ID, err)
						}
					} else {
						s.orderRepo.UpdateStatus(cancelCtx, order.ID, "cancelled")
						order.Status = "cancelled"
						s.notificationService.NotifyOrderUpdate(cancelCtx, userID, order)
					}
				}
			}
		}
	}()

	return err
}

// Note: Error checking functions (isAPIKeyError, isCriticalTradingError, isOrderNotFoundError)
// have been replaced with shared utilities from util package:
// - util.IsAPIKeyError
// - util.IsCriticalTradingError
// - util.IsOrderNotFoundError

// stopBotWithError stops a bot and sets error status (for live bots only)
func (s *PumpHunterService) stopBotWithError(botID int64, userID string, errorMsg string) {
	s.mu.Lock()
	inst, ok := s.instances[botID]
	s.mu.Unlock()

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
		delete(s.instances, botID)
	}
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

func (s *PumpHunterService) runBot(inst *PumpHunterInstance) {
	s.log.Infof("Bot %d: Starting runBot loop", inst.Config.ID)
	defer s.log.Infof("Bot %d: runBot loop exited", inst.Config.ID)

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	// Ticker for signal processing (priority ranking)
	signalTicker := time.NewTicker(1 * time.Second)
	defer signalTicker.Stop()

	// Ticker for pending order monitoring (false pump detection and repositioning)
	pendingOrderTicker := time.NewTicker(10 * time.Second)
	defer pendingOrderTicker.Stop()

	// Ticker for max loss check
	maxLossTicker := time.NewTicker(5 * time.Second)
	defer maxLossTicker.Stop()

	for {
		select {
		case <-inst.StopChan:
			s.log.Infof("Bot %d: StopChan closed, exiting runBot loop", inst.Config.ID)
			return
		case <-ticker.C:
			s.monitorExits(inst)
		case <-signalTicker.C:
			s.processSignals(inst)
		case <-pendingOrderTicker.C:
			// Monitor pending orders for false pump detection and repositioning
			s.monitorPendingOrders(inst)
		case <-maxLossTicker.C:
			// Periodic check for max loss limit
			inst.mu.RLock()
			config := inst.Config
			inst.mu.RUnlock()

			// For Pump Hunter, use DailyLossLimitIDR from RiskManagement (MaxLossIDR is 0 for Pump Hunter)
			maxLossLimit := config.RiskManagement.DailyLossLimitIDR
			// Only check if maxLossLimit is set (> 0) and profit has reached the negative threshold
			// Example: profit=0, maxLoss=1000k → 0 <= -1000000 = false (can run)
			// Example: profit=-1000k, maxLoss=1000k → -1000000 <= -1000000 = true (stop)
			if maxLossLimit > 0 && config.TotalProfitIDR <= -maxLossLimit {
				s.log.Warnf("Bot %d reached total max loss limit (%.2f <= -%.2f), stopping bot",
					config.ID, config.TotalProfitIDR, maxLossLimit)
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

// HandleCoinUpdate is called by MarketDataService when coin data is updated
func (s *PumpHunterService) HandleCoinUpdate(coin *model.Coin) {
	s.mu.RLock()
	instances := make([]*PumpHunterInstance, 0, len(s.instances))
	for _, inst := range s.instances {
		instances = append(instances, inst)
	}
	s.mu.RUnlock()

	if len(instances) == 0 {
		return // No running bots
	}

	for _, inst := range instances {
		// Early exit: Skip processing if bot is in cooldown period
		config := inst.Config.RiskManagement
		if config.CooldownAfterLossMinutes > 0 && !inst.LastLossTime.IsZero() {
			cooldownDuration := time.Duration(config.CooldownAfterLossMinutes) * time.Minute
			timeSinceLoss := time.Since(inst.LastLossTime)
			if timeSinceLoss < cooldownDuration {
				// Bot is in cooldown, skip processing this coin update
				continue
			}
		}

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

	s.log.Debugf("Bot %d: Processing %d signals from buffer", inst.Config.ID, len(inst.SignalBuffer))

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
		s.log.Infof("Bot %d: Checking signal for %s (PumpScore=%.2f, Price=%.2f, VolumeIDR=%.2f)",
			inst.Config.ID, sig.Coin.PairID, sig.Score, sig.Coin.CurrentPrice, sig.Coin.VolumeIDR)
		// Re-check conditions (especially MaxPositions) before opening
		if s.checkEntryConditions(inst, sig.Coin) {
			s.log.Infof("Bot %d: Entry conditions PASSED for %s, opening position", inst.Config.ID, sig.Coin.PairID)
			s.openPosition(inst, sig.Coin)
		}
	}
}

func (s *PumpHunterService) checkEntryConditions(inst *PumpHunterInstance, coin *model.Coin) bool {
	inst.mu.RLock()
	defer inst.mu.RUnlock()

	config := inst.Config
	if config.EntryRules == nil {
		s.log.Warnf("Bot %d: EntryRules is nil for coin %s", inst.Config.ID, coin.PairID)
		return false
	}

	// 0. Risk Management Checks
	// 0.1 Circuit Breaker (Total Loss)
	// For Pump Hunter, use DailyLossLimitIDR from RiskManagement (MaxLossIDR is 0 for Pump Hunter)
	maxLossLimit := config.RiskManagement.DailyLossLimitIDR
	if maxLossLimit > 0 && config.TotalProfitIDR <= -maxLossLimit {
		s.log.Warnf("Bot %d reached total max loss limit (%.2f <= -%.2f), stopping bot",
			config.ID, config.TotalProfitIDR, maxLossLimit)
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
	// For Pump Hunter, use DailyLossLimitIDR from RiskManagement
	dailyLossLimit := config.RiskManagement.DailyLossLimitIDR
	if dailyLossLimit > 0 && inst.DailyLoss >= dailyLossLimit {
		// Check if we should reset daily loss (it's a new day)
		if time.Since(inst.LastLossTime) > 24*time.Hour {
			inst.DailyLoss = 0
			s.log.Infof("Bot %d: Daily loss limit reset (new day)", config.ID)
		} else {
			s.log.Warnf("Bot %d: Daily loss limit reached (%.2f >= %.2f), skipping entry", config.ID, inst.DailyLoss, dailyLossLimit)
			return false
		}
	}

	// 0.3 Max Concurrent Positions (count: pending, buying, open - don't count: selling, closed)
	activeCount := 0
	// Count pending orders (status: "pending")
	activeCount += len(inst.PendingOrders)
	// Count open positions with status "open" or "buying" (but NOT "selling" - those are exiting)
	for _, pos := range inst.OpenPositions {
		if pos.Status == model.PositionStatusOpen || pos.Status == model.PositionStatusBuying {
			activeCount++
		}
		// Note: PositionStatusSelling is NOT counted - those positions are exiting, not active
	}
	if activeCount >= config.RiskManagement.MaxConcurrentPositions {
		s.log.Debugf("Bot %d: Entry FAILED for %s - Max concurrent positions reached (%d >= %d) [PendingOrders=%d, OpenPositions(open/buying only)=%d]",
			inst.Config.ID, coin.PairID, activeCount, config.RiskManagement.MaxConcurrentPositions,
			len(inst.PendingOrders), func() int {
				count := 0
				for _, pos := range inst.OpenPositions {
					if pos.Status == model.PositionStatusOpen || pos.Status == model.PositionStatusBuying {
						count++
					}
				}
				return count
			}())
		return false
	}

	// 0.4 Cooldown after loss
	if config.RiskManagement.CooldownAfterLossMinutes > 0 && !inst.LastLossTime.IsZero() {
		cooldownDuration := time.Duration(config.RiskManagement.CooldownAfterLossMinutes) * time.Minute
		timeSinceLoss := time.Since(inst.LastLossTime)
		if timeSinceLoss < cooldownDuration {
			remaining := cooldownDuration - timeSinceLoss
			s.log.Debugf("Bot %d: Entry FAILED for %s - Still in cooldown period (%.0f minutes remaining)",
				inst.Config.ID, coin.PairID, remaining.Minutes())
			return false
		}
	}

	// 0.5 Excluded/Allowed Pairs
	if len(config.EntryRules.AllowedPairs) > 0 {
		allowed := false
		for _, p := range config.EntryRules.AllowedPairs {
			if p == coin.PairID {
				allowed = true
				break
			}
		}
		if !allowed {
			s.log.Debugf("Bot %d: Entry FAILED for %s - Pair not in allowed list", inst.Config.ID, coin.PairID)
			return false
		}
	}
	for _, p := range config.EntryRules.ExcludedPairs {
		if p == coin.PairID {
			return false
		}
	}

	// 0.6 Already have position (check both open and pending)
	for _, pos := range inst.OpenPositions {
		if pos.Pair == coin.PairID {
			s.log.Debugf("Bot %d: Entry FAILED for %s - Already have open position", inst.Config.ID, coin.PairID)
			return false
		}
	}
	// Also check pending orders
	for _, pos := range inst.PendingOrders {
		if pos.Pair == coin.PairID {
			s.log.Debugf("Bot %d: Entry FAILED for %s - Already have pending order", inst.Config.ID, coin.PairID)
			return false
		}
	}

	// 1. Entry Rules
	// 1.1 Pump Score
	if coin.PumpScore < config.EntryRules.MinPumpScore {
		// Silently fail - no log for pump score
		return false
	}

	// 1.2 24h Volume
	if coin.VolumeIDR < config.EntryRules.Min24hVolumeIDR {
		s.log.Debugf("Bot %d: Entry FAILED for %s - Volume too low (%.2f < %.2f)", inst.Config.ID, coin.PairID, coin.VolumeIDR, config.EntryRules.Min24hVolumeIDR)
		return false
	}

	// 1.3 Min Price
	if coin.CurrentPrice < config.EntryRules.MinPriceIDR {
		s.log.Debugf("Bot %d: Entry FAILED for %s - Price too low (%.2f < %.2f)", inst.Config.ID, coin.PairID, coin.CurrentPrice, config.EntryRules.MinPriceIDR)
		return false
	}

	// 1.4 Positive Timeframes (last check)
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
		s.log.Debugf("Bot %d: Entry FAILED for %s - Not enough positive timeframes (%d < %d)", inst.Config.ID, coin.PairID, positiveCount, config.EntryRules.MinTimeframesPositive)
		return false
	}

	s.log.Infof("Bot %d: Entry conditions PASSED for %s - All checks passed", inst.Config.ID, coin.PairID)
	return true
}

func (s *PumpHunterService) openPosition(inst *PumpHunterInstance, coin *model.Coin) {
	s.log.Infof("Bot %d: openPosition called for %s (PumpScore=%.2f, Price=%.2f)", inst.Config.ID, coin.PairID, coin.PumpScore, coin.CurrentPrice)

	inst.mu.Lock()
	defer inst.mu.Unlock()

	// Re-check max concurrent positions here to prevent race conditions
	// (checkEntryConditions might have passed, but positions could have been added between check and openPosition call)
	// Count: pending, buying, open - don't count: selling, closed
	activeCount := 0
	// Count pending orders (status: "pending")
	activeCount += len(inst.PendingOrders)
	// Count open positions with status "open" or "buying" (but NOT "selling" - those are exiting)
	for _, pos := range inst.OpenPositions {
		if pos.Status == model.PositionStatusOpen || pos.Status == model.PositionStatusBuying {
			activeCount++
		}
		// Note: PositionStatusSelling is NOT counted - those positions are exiting, not active
	}
	if activeCount >= inst.Config.RiskManagement.MaxConcurrentPositions {
		s.log.Debugf("Bot %d: Position NOT opened for %s - Max concurrent positions reached (%d >= %d) [race condition check] [PendingOrders=%d, OpenPositions(open/buying only)=%d]",
			inst.Config.ID, coin.PairID, activeCount, inst.Config.RiskManagement.MaxConcurrentPositions,
			len(inst.PendingOrders), func() int {
				count := 0
				for _, pos := range inst.OpenPositions {
					if pos.Status == model.PositionStatusOpen || pos.Status == model.PositionStatusBuying {
						count++
					}
				}
				return count
			}())
		return
	}

	// Atomic duplicate check (prevent race condition)
	for _, pos := range inst.OpenPositions {
		if pos.Pair == coin.PairID {
			s.log.Debugf("Bot %d: Position NOT opened - already have open position for %s", inst.Config.ID, coin.PairID)
			return
		}
	}
	for _, pos := range inst.PendingOrders {
		if pos.Pair == coin.PairID {
			s.log.Debugf("Bot %d: Position NOT opened - already have pending order for %s", inst.Config.ID, coin.PairID)
			return
		}
	}

	// Validate and normalize balances before opening position
	requiredCurrencies := []string{"idr"}
	inst.Config.Balances = util.ValidateAndNormalizeBalances(
		inst.Config.Balances,
		requiredCurrencies,
		inst.Config.InitialBalanceIDR,
		s.log,
	)

	pairInfo, ok := s.marketDataService.GetPairInfo(coin.PairID)
	if !ok {
		s.log.Warnf("Bot %d: Failed to get pair info for %s", inst.Config.ID, coin.PairID)
		return
	}
	// Get volume precision (using shared utility)
	volumePrecision := util.GetVolumePrecision(pairInfo)

	s.log.Debugf("Bot %d: Got pair info for %s - VolumePrecision=%d, TradeMinBaseCurrency=%d, TradeMinTradedCurrency=%.2f",
		inst.Config.ID, coin.PairID, volumePrecision, pairInfo.TradeMinBaseCurrency, pairInfo.TradeMinTradedCurrency)

	// Calculate available balance (using shared utility)
	idrBalance := inst.Config.Balances["idr"]
	minBalanceReserve := inst.Config.RiskManagement.MinBalanceIDR

	availableBalance, hasEnough := util.CalculateAvailableBalance(idrBalance, minBalanceReserve)
	if !hasEnough {
		s.log.Debugf("Bot %d: Insufficient balance to maintain minimum reserve - IDR=%.2f <= MinBalanceIDR=%.2f",
			inst.Config.ID, idrBalance, minBalanceReserve)
		return
	}

	// Calculate position size (using shared utility)
	maxPositionSize := inst.Config.RiskManagement.MaxPositionIDR
	if maxPositionSize <= 0 {
		// Fallback: If MaxPositionIDR is not set, use all available balance
		s.log.Warnf("Bot %d: MaxPositionIDR is not set (%.2f), using all available balance (%.2f) as fallback",
			inst.Config.ID, maxPositionSize, availableBalance)
		maxPositionSize = availableBalance
	}

	sizeIDR, valid := util.CalculatePositionSize(maxPositionSize, availableBalance)
	if !valid {
		s.log.Debugf("Bot %d: Cannot calculate position size - availableBalance=%.2f",
			inst.Config.ID, availableBalance)
		return
	}

	if sizeIDR <= 0 {
		s.log.Debugf("Bot %d: Calculated position size is zero or negative (%.2f), skipping entry",
			inst.Config.ID, sizeIDR)
		return
	}

	s.log.Debugf("Bot %d: Balance check - IDR=%.2f, Available=%.2f, MaxPositionIDR=%.2f, MinBalanceIDR=%.2f, FinalSizeIDR=%.2f",
		inst.Config.ID, idrBalance, availableBalance, maxPositionSize, minBalanceReserve, sizeIDR)

	// Calculate buy price (aggressive: bestBid + tick, or market if gap < 1%)
	buyPrice, orderType := s.calculateBuyPrice(coin, pairInfo)
	s.log.Debugf("Bot %d: Calculated buy price - %.2f (%s), bestBid=%.2f, bestAsk=%.2f, gap=%.2f%%",
		inst.Config.ID, buyPrice, orderType, coin.BestBid, coin.BestAsk,
		((coin.BestAsk-coin.BestBid)/coin.BestBid)*100)

	// Calculate amount
	amount := sizeIDR / buyPrice

	s.log.Debugf("Bot %d: Before rounding - amount=%.8f, volumePrecision=%d", inst.Config.ID, amount, volumePrecision)

	// Validate order amount (using shared utility)
	baseCurrency := strings.TrimSuffix(coin.PairID, "idr")
	validation := util.ValidateOrderAmount(
		inst.Config.ID,
		amount,
		buyPrice,
		pairInfo,
		volumePrecision,
		baseCurrency,
		s.log,
	)

	if !validation.Valid {
		s.log.Debugf("Bot %d: Order validation failed - %s", inst.Config.ID, validation.Reason)
		return
	}

	// Use validated amount
	amount = validation.Amount
	s.log.Debugf("Bot %d: After validation - amount=%.8f, orderValue=%.2f IDR", inst.Config.ID, amount, validation.OrderValue)

	// Place Order (aggressive buy)
	// For market buy orders: use IDR amount (sizeIDR), not coin amount
	// For limit buy orders: use coin amount (amount)
	tradeAmount := amount
	if orderType == "market" {
		// Market buy: pass IDR amount instead of coin amount
		tradeAmount = sizeIDR
		s.log.Infof("Bot %d: Placing MARKET BUY order for %s - sizeIDR=%.2f, type=%s",
			inst.Config.ID, coin.PairID, sizeIDR, orderType)
	} else {
		s.log.Infof("Bot %d: Placing LIMIT BUY order for %s - price=%.2f, amount=%.8f, sizeIDR=%.2f, type=%s",
			inst.Config.ID, coin.PairID, buyPrice, amount, sizeIDR, orderType)
	}
	ctx := context.Background()

	// Generate unique client order ID (using shared utility)
	clientOrderID := GenerateClientOrderID(inst.Config.ID, coin.PairID, "buy")

	// For market orders, price is not sent to Indodax (set to 0)
	tradePrice := buyPrice
	if orderType == "market" {
		tradePrice = 0
	}

	res, err := inst.TradeClient.Trade(ctx, "buy", coin.PairID, tradePrice, tradeAmount, orderType, clientOrderID)
	if err != nil {
		s.log.Errorf("Bot %d: Failed to open position on %s: indodax API error: %v", inst.Config.ID, coin.PairID, err)
		// If critical trading error (API key or invalid pair) and live trading, stop the bot
		if util.IsCriticalTradingError(err) && !inst.Config.IsPaperTrading {
			s.stopBotWithError(inst.Config.ID, inst.Config.UserID, fmt.Sprintf("Trading error: %v", err))
		}
		return
	}

	// Store the ClientOrderID (for both WebSocket matching and cancellation)
	// This matches MarketMaker behavior at market_maker_service.go:1230
	orderIDStr := res.ClientOrderID
	if orderIDStr == "" {
		// Fallback to numeric ID (shouldn't happen)
		orderIDStr = fmt.Sprintf("%d", res.OrderID)
	}

	s.log.Infof("Bot %d: Successfully placed BUY order for %s - OrderID=%s, Type=%s", inst.Config.ID, coin.PairID, orderIDStr, orderType)

	// Create position with status "pending" (waiting for false pump check)
	pos := &model.Position{
		BotConfigID:     inst.Config.ID,
		UserID:          inst.Config.UserID,
		Pair:            coin.PairID,
		Status:          model.PositionStatusPending, // Changed from PositionStatusBuying
		EntryPrice:      buyPrice,
		EntryQuantity:   amount,
		EntryAmountIDR:  sizeIDR,
		EntryOrderID:    orderIDStr,
		EntryOrderType:  orderType, // Track order type
		EntryPumpScore:  coin.PumpScore,
		EntryTrxCount1m: coin.Timeframes.OneMinute.Trx,
		EntryAt:         time.Now(),
		OrderPlacedAt:   time.Now(), // Track when order was placed for false pump monitoring
		HighestPrice:    buyPrice,   // Initialize ATH
		LowestPrice:     buyPrice,
		LastPriceCheck:  time.Now(),
		IsPaperTrade:    inst.Config.IsPaperTrading,
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
		OrderID:      orderIDStr, // Use the same orderIDStr (clientOrderID for paper, numeric for live)
		Pair:         pos.Pair,
		Side:         "buy",
		Status:       "open",
		Price:        buyPrice,
		Amount:       amount,
		IsPaperTrade: inst.Config.IsPaperTrading,
	}

	if err := s.orderRepo.Create(ctx, order); err != nil {
		s.log.Errorf("Bot %d failed to save unified order: %v", inst.Config.ID, err)
	}

	// Update internal ID
	pos.InternalEntryOrderID = order.ID
	s.posRepo.Update(ctx, pos)

	// Add to pending orders (for false pump monitoring)
	inst.PendingOrders[pos.ID] = pos

	// Update Balance immediately for prediction
	inst.Config.Balances["idr"] -= sizeIDR
	s.botRepo.UpdateBalance(ctx, inst.Config.ID, inst.Config.Balances)

	s.log.Infof("Bot %d opened position on %s: %.8f @ %.2f", inst.Config.ID, coin.PairID, amount, buyPrice)

	// Notify order creation, position creation, and bot update via WebSocket
	s.notificationService.NotifyOrderUpdate(ctx, inst.Config.UserID, order)
	s.notificationService.NotifyPositionUpdate(ctx, inst.Config.UserID, pos)
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

	if len(inst.OpenPositions) == 0 {
		return // No positions to monitor
	}

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
		athUpdated := false
		if coin.CurrentPrice > pos.HighestPrice {
			oldATH := pos.HighestPrice
			pos.HighestPrice = coin.CurrentPrice
			pos.MinutesBelowATH = 0 // Reset counter on new ATH
			athUpdated = true
			s.log.Debugf("Bot %d: Position %s - New ATH: %.2f (was %.2f)",
				inst.Config.ID, pos.Pair, coin.CurrentPrice, oldATH)
		} else if coin.CurrentPrice >= pos.HighestPrice && pos.MinutesBelowATH > 0 {
			// Price recovered to ATH (or above) - reset counter immediately
			// This ensures we catch recovery in real-time (every 10s) not just at 1-minute checks
			oldCounter := pos.MinutesBelowATH
			pos.MinutesBelowATH = 0
			s.log.Debugf("Bot %d: Position %s - Price recovered to ATH %.2f, resetting counter (was %d)",
				inst.Config.ID, pos.Pair, pos.HighestPrice, oldCounter)
		}
		if coin.CurrentPrice < pos.LowestPrice {
			pos.LowestPrice = coin.CurrentPrice
		}

		// Check conditions (this may update LastPriceCheck, HighestPrice, MinutesBelowATH, etc.)
		reason := s.checkExitConditions(inst, pos, coin)
		if reason != "" {
			s.closePosition(inst, pos, coin.CurrentPrice, reason)
		} else {
			// Save position updates (LastPriceCheck, HighestPrice, LowestPrice, MinutesBelowATH, etc.)
			// even if no exit signal (checkExitConditions may have updated these fields)
			s.posRepo.Update(ctx, pos)

			// Broadcast if ATH was updated (significant event)
			if athUpdated {
				s.notificationService.NotifyPositionUpdate(ctx, inst.Config.UserID, pos)
			}
		}
	}
}

func (s *PumpHunterService) checkExitConditions(inst *PumpHunterInstance, pos *model.Position, coin *model.Coin) string {
	config := inst.Config.ExitRules
	currentMinute := time.Now().Minute()

	// Check if 1 minute has passed since last check
	timeSinceLastCheck := time.Since(pos.LastPriceCheck)
	if timeSinceLastCheck < 1*time.Minute {
		// Not time to check yet, but return existing signal if confirmed
		if pos.ExitConfirmCount >= 2 && pos.ExitSignalReason != "" {
			return pos.ExitSignalReason // Already confirmed, sell
		}
		return "" // Wait for next minute
	}

	// Update last check time
	pos.LastPriceCheck = time.Now()
	s.log.Debugf("Bot %d: Checking exit conditions for %s (target=%.2f%%, price=%.2f, ATH=%.2f, timeSinceLastCheck=%.0fs)",
		inst.Config.ID, pos.Pair, config.TargetProfitPercent, coin.CurrentPrice, pos.HighestPrice, timeSinceLastCheck.Seconds())
	// Save LastPriceCheck update (will be saved again if exit signal detected, but ensure it's saved even if no signal)
	ctx := context.Background()
	s.posRepo.Update(ctx, pos)

	profitPct := (coin.CurrentPrice - pos.EntryPrice) / pos.EntryPrice * 100

	// Check all exit conditions (priority order)
	var exitReason string

	// 1. Stop Loss (highest priority)
	if profitPct <= -config.StopLossPercent {
		s.log.Debugf("Bot %d: Position %s - Stop loss triggered: profit %.2f%% <= -%.2f%% (entry: %.2f, current: %.2f)",
			inst.Config.ID, pos.Pair, profitPct, config.StopLossPercent, pos.EntryPrice, coin.CurrentPrice)
		exitReason = "stop_loss"
	} else if config.MaxHoldMinutes > 0 && time.Since(pos.EntryAt) > time.Duration(config.MaxHoldMinutes)*time.Minute {
		// 2. Max Hold Time
		exitReason = "max_hold_time"
	} else if config.TargetProfitPercent > 1.0 && profitPct >= config.TargetProfitPercent {
		// 3. Take Profit (for target > 1%, immediate limit order already placed, but check if filled)
		// Limit order should already be placed, but if somehow not, this is a fallback
		exitReason = "take_profit"
	} else if config.TrailingStopEnabled && pos.HighestPrice > pos.EntryPrice {
		// 4. Trailing Stop
		dropPct := (pos.HighestPrice - coin.CurrentPrice) / pos.HighestPrice * 100
		if dropPct >= config.TrailingStopPercent {
			exitReason = "trailing_stop"
		}
	}

	// Check pump score drop (only if no exit reason found yet)
	if exitReason == "" && config.ExitOnPumpScoreDrop && coin.PumpScore < config.PumpScoreDropThreshold {
		// 5. Pump Score Drop
		exitReason = "pump_score_drop"
	}

	// Check ATH decline for target = 1% (always check if target = 1%, regardless of trailing stop)
	if exitReason == "" && config.TargetProfitPercent == 1.0 {
		// 6. ATH decline (for target = 1%)
		s.log.Debugf("Bot %d: Checking ATH decline for %s (current=%.2f, ATH=%.2f, minutesBelowATH=%d)",
			inst.Config.ID, pos.Pair, coin.CurrentPrice, pos.HighestPrice, pos.MinutesBelowATH)
		// checkATHDecline increments MinutesBelowATH and returns true if price is below ATH
		// We need 2 consecutive minutes below ATH - checkATHDecline already requires this (returns true only when >= 2)
		// So when it returns true, we can immediately set exitReason and skip the 2-minute confirmation
		if s.checkATHDecline(pos, coin) {
			// checkATHDecline only returns true when MinutesBelowATH >= 2, so we've already confirmed 2 minutes
			// Skip the 2-minute confirmation logic and sell immediately
			s.log.Infof("Bot %d: Position %s - ATH decline confirmed: %.2f consecutive minutes below ATH %.2f (current: %.2f)",
				inst.Config.ID, pos.Pair, float64(pos.MinutesBelowATH), pos.HighestPrice, coin.CurrentPrice)
			return "ath_decline" // Already confirmed 2 minutes, sell immediately
		}
	}

	// 2-minute confirmation logic
	if exitReason != "" {
		ctx := context.Background()
		// First time seeing this exit signal
		if pos.ExitSignalReason == "" || pos.ExitSignalReason != exitReason {
			pos.ExitSignalReason = exitReason
			pos.ExitSignalMinute = currentMinute
			pos.ExitConfirmCount = 1
			s.log.Debugf("Bot %d: Exit signal detected: %s (1st confirmation, waiting for 2nd)",
				inst.Config.ID, exitReason)
			s.posRepo.Update(ctx, pos) // Save state
			// Broadcast exit signal detection
			s.notificationService.NotifyPositionUpdate(ctx, inst.Config.UserID, pos)
			return "" // Wait for next minute confirmation
		}

		// Same signal, same minute or next minute
		if pos.ExitSignalReason == exitReason {
			pos.ExitConfirmCount++
			if pos.ExitConfirmCount >= 2 {
				s.log.Infof("Bot %d: Exit signal confirmed (2nd): %s → SELLING",
					inst.Config.ID, exitReason)
				s.posRepo.Update(ctx, pos) // Save state
				// Broadcast exit signal confirmation
				s.notificationService.NotifyPositionUpdate(ctx, inst.Config.UserID, pos)
				return exitReason // Confirmed, sell now
			}
			s.log.Debugf("Bot %d: Exit signal %s confirmed %d/2",
				inst.Config.ID, exitReason, pos.ExitConfirmCount)
			s.posRepo.Update(ctx, pos) // Save state
			// Broadcast confirmation progress
			s.notificationService.NotifyPositionUpdate(ctx, inst.Config.UserID, pos)
			return "" // Wait for 2nd confirmation
		}
	} else {
		// No exit signal - reset confirmation counter
		if pos.ExitSignalReason != "" {
			s.log.Debugf("Bot %d: Exit signal %s cleared (price recovered)",
				inst.Config.ID, pos.ExitSignalReason)
			pos.ExitSignalReason = ""
			pos.ExitConfirmCount = 0
			ctx := context.Background()
			s.posRepo.Update(ctx, pos) // Save state
			// Broadcast exit signal cleared
			s.notificationService.NotifyPositionUpdate(ctx, inst.Config.UserID, pos)
		}
	}

	return ""
}

// checkATHDecline checks if price has been below ATH for 2 consecutive minutes
func (s *PumpHunterService) checkATHDecline(pos *model.Position, coin *model.Coin) bool {
	currentPrice := coin.CurrentPrice

	// Update ATH
	if currentPrice > pos.HighestPrice {
		oldATH := pos.HighestPrice
		pos.HighestPrice = currentPrice
		pos.MinutesBelowATH = 0 // Reset counter
		s.log.Debugf("Bot %d: Position %s - New ATH: %.2f (was %.2f), resetting below-ATH counter",
			pos.BotConfigID, pos.Pair, currentPrice, oldATH)
		// Note: ATH update broadcast is handled in monitorExits to avoid duplicate broadcasts
		return false // New ATH, wait
	}

	// Check if below ATH
	if currentPrice < pos.HighestPrice {
		pos.MinutesBelowATH++
		dropPct := (pos.HighestPrice - currentPrice) / pos.HighestPrice * 100
		// Return true only when we've had 2 consecutive minutes below ATH
		// This means checkExitConditions can skip the 2-minute confirmation for ATH decline
		if pos.MinutesBelowATH >= 2 {
			s.log.Debugf("Bot %d: Position %s - ATH decline confirmed: %.2f consecutive minutes below ATH %.2f (current: %.2f, drop: %.2f%%)",
				pos.BotConfigID, pos.Pair, float64(pos.MinutesBelowATH), pos.HighestPrice, currentPrice, dropPct)
			return true // 2 consecutive minutes below ATH confirmed
		}
		s.log.Debugf("Bot %d: Position %s - Price below ATH: %.2f < %.2f (%.2f%% drop), minute %d/2",
			pos.BotConfigID, pos.Pair, currentPrice, pos.HighestPrice, dropPct, pos.MinutesBelowATH)
		return false // Wait for 2nd minute
	}

	// Price equals ATH - reset counter
	if pos.MinutesBelowATH > 0 {
		s.log.Debugf("Bot %d: Position %s - Price recovered to ATH: %.2f, resetting below-ATH counter",
			pos.BotConfigID, pos.Pair, currentPrice)
	}
	pos.MinutesBelowATH = 0
	return false
}

func (s *PumpHunterService) closePosition(inst *PumpHunterInstance, pos *model.Position, price float64, reason string) {
	ctx := context.Background()

	// Generate unique client order ID
	clientOrderID := fmt.Sprintf("bot%d-%s-sell-%d", inst.Config.ID, pos.Pair, time.Now().UnixMilli())

	// For market sell orders, price is not sent to Indodax (set to 0)
	// Market sell uses coin amount (pos.EntryQuantity) - this is correct
	tradePrice := 0.0 // Market orders don't use price parameter

	res, err := inst.TradeClient.Trade(ctx, "sell", pos.Pair, tradePrice, pos.EntryQuantity, "market", clientOrderID)
	if err != nil {
		s.log.Errorf("Bot %d: Failed to close position on %s: indodax API error: %v", inst.Config.ID, pos.Pair, err)
		// If critical trading error (API key or invalid pair) and live trading, stop the bot
		if util.IsCriticalTradingError(err) && !inst.Config.IsPaperTrading {
			s.stopBotWithError(inst.Config.ID, inst.Config.UserID, fmt.Sprintf("Trading error: %v", err))
		}
		return
	}

	// Store order ID for WebSocket matching
	// For market sell orders, Indodax may send orderId in format "{pair}-market-{numericId}" in WebSocket
	// We store ClientOrderID if available, but also log numeric ID for debugging
	orderIDStr := res.ClientOrderID
	if orderIDStr == "" {
		// Fallback to numeric ID if ClientOrderID is empty (shouldn't happen)
		orderIDStr = fmt.Sprintf("%d", res.OrderID)
		s.log.Warnf("Bot %d: ClientOrderID is empty for sell order, using numeric ID %s", inst.Config.ID, orderIDStr)
	} else {
		// Log both for debugging market order matching issues
		s.log.Debugf("Bot %d: Stored ExitOrderID=%s (ClientOrderID from Trade response), Numeric OrderID=%d",
			inst.Config.ID, orderIDStr, res.OrderID)
	}

	pos.Status = model.PositionStatusSelling
	pos.ExitOrderID = orderIDStr
	pos.CloseReason = reason

	// Save unified order record
	order := &model.Order{
		UserID:       inst.Config.UserID,
		ParentID:     pos.ID,
		ParentType:   "position",
		OrderID:      orderIDStr, // Use the same orderIDStr (clientOrderID for paper, numeric for live)
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

	// Notify order creation and position update (status changed to Selling) via WebSocket
	s.notificationService.NotifyOrderUpdate(ctx, inst.Config.UserID, order)
	s.notificationService.NotifyPositionUpdate(ctx, inst.Config.UserID, pos)
}

func (s *PumpHunterService) handleOrderFilled(inst *PumpHunterInstance, order *model.Order) {
	ctx := context.Background()

	// If the order doesn't have a database ID, look it up by OrderID (for paper trading)
	if order.ID == 0 && order.OrderID != "" {
		dbOrder, err := s.orderRepo.GetByOrderID(ctx, order.OrderID)
		if err != nil {
			s.log.Errorf("Bot %d: Failed to find order by OrderID %s: %v", inst.Config.ID, order.OrderID, err)
			return
		}
		// Update order with database order details
		order.ID = dbOrder.ID
		order.Pair = dbOrder.Pair
		order.Side = dbOrder.Side
		order.Price = dbOrder.Price
		order.Amount = dbOrder.Amount
		s.log.Debugf("Bot %d: Found order in database - ID=%d, OrderID=%s", inst.Config.ID, order.ID, order.OrderID)
	}

	// Use FilledAmount if available, otherwise use Amount
	filledAmount := order.FilledAmount
	if filledAmount == 0 {
		filledAmount = order.Amount
	}

	// Find position (check both OpenPositions and PendingOrders)
	inst.mu.Lock()
	var targetPos *model.Position
	for _, pos := range inst.OpenPositions {
		if pos.EntryOrderID == order.OrderID || pos.ExitOrderID == order.OrderID {
			targetPos = pos
			break
		}
	}
	// Also check pending orders
	if targetPos == nil {
		for _, pos := range inst.PendingOrders {
			if pos.EntryOrderID == order.OrderID {
				targetPos = pos
				break
			}
		}
	}
	inst.mu.Unlock()

	if targetPos == nil {
		s.log.Warnf("Bot %d: Order filled but position not found in memory (OrderID=%s) - checking database", inst.Config.ID, order.OrderID)

		// Try to find position in database by order ID (position might have been deleted from memory but still exists in DB)
		// This can happen if order was filled after cancellation or during bot restart
		if order.ID > 0 {
			dbOrder, err := s.orderRepo.GetByID(ctx, order.ID)
			if err == nil && dbOrder.ParentType == "position" && dbOrder.ParentID > 0 {
				dbPos, err := s.posRepo.GetByID(ctx, dbOrder.ParentID)
				if err == nil && dbPos.BotConfigID == inst.Config.ID {
					s.log.Infof("Bot %d: Found position %d in database for filled order %s - restoring to memory",
						inst.Config.ID, dbPos.ID, order.OrderID)

					// Restore position to memory
					inst.mu.Lock()
					if dbPos.Status == model.PositionStatusPending || dbPos.Status == model.PositionStatusBuying {
						inst.PendingOrders[dbPos.ID] = dbPos
						targetPos = dbPos
					} else if dbPos.Status == model.PositionStatusOpen || dbPos.Status == model.PositionStatusSelling {
						inst.OpenPositions[dbPos.ID] = dbPos
						targetPos = dbPos
					}
					inst.mu.Unlock()

					if targetPos == nil {
						s.log.Warnf("Bot %d: Position %d found in database but has invalid status '%s' - cannot process fill",
							inst.Config.ID, dbPos.ID, dbPos.Status)
					}
				}
			}
		}

		if targetPos == nil {
			s.log.Warnf("Bot %d: Order filled but position not found in database either (OrderID=%s) - order may be orphaned",
				inst.Config.ID, order.OrderID)
			// Still update the order status even if position not found
		}
	}

	if targetPos != nil {
		inst.mu.Lock()
		if order.Side == "buy" {
			// Buy order filled - move from pending to open
			if targetPos.Status == model.PositionStatusPending {
				// Remove from pending orders
				delete(inst.PendingOrders, targetPos.ID)
				// Add to open positions
				inst.OpenPositions[targetPos.ID] = targetPos
			}

			// Update position status to open
			targetPos.Status = model.PositionStatusOpen

			// Update entry price to actual fill price (might differ from limit price)
			if order.Price > 0 {
				targetPos.EntryPrice = order.Price
				targetPos.HighestPrice = order.Price
				targetPos.LowestPrice = order.Price
			}

			// Initialize ATH tracking
			targetPos.LastPriceCheck = time.Now()
			targetPos.MinutesBelowATH = 0

			s.posRepo.Update(ctx, targetPos)
			inst.mu.Unlock()

			// Implement sell strategy based on target profit
			targetProfit := inst.Config.ExitRules.TargetProfitPercent
			if targetProfit > 1.0 {
				// Strategy A: Place limit sell order after 5-second delay
				// Delay allows Indodax to credit coins and stabilize order book
				s.log.Debugf("Bot %d: Waiting 5 seconds before placing sell order for position %d (%s)",
					inst.Config.ID, targetPos.ID, targetPos.Pair)
				time.Sleep(5 * time.Second)

				sellPrice := targetPos.EntryPrice * (1 + targetProfit/100)
				s.placeLimitSellOrder(inst, targetPos, sellPrice)
			} else if targetProfit == 1.0 {
				// Strategy B: Wait for ATH (monitored in checkExitConditions)
				s.log.Infof("Bot %d: Position %d opened, waiting for ATH (target=1%%)", inst.Config.ID, targetPos.ID)
			}

			// Notify position update (status changed to Open)
			s.notificationService.NotifyPositionUpdate(ctx, inst.Config.UserID, targetPos)
		} else {
			inst.mu.Unlock()
			// Finalize close
			s.finalizePositionClose(inst, targetPos, order.Price)
		}
	}

	// Update order status and filled amount in database
	if order.ID > 0 {
		// Get the order from database to update it
		dbOrder, err := s.orderRepo.GetByID(ctx, order.ID)
		if err != nil {
			s.log.Errorf("Bot %d: Failed to get order %d for update: %v", inst.Config.ID, order.ID, err)
		} else {
			// Update filled amount and status
			dbOrder.FilledAmount = filledAmount
			dbOrder.Status = "filled"
			now := time.Now()
			dbOrder.FilledAt = &now

			// Save updated order
			if err := s.orderRepo.Update(ctx, dbOrder, "open"); err != nil {
				s.log.Errorf("Bot %d: Failed to update order %d: %v", inst.Config.ID, order.ID, err)
			} else {
				s.log.Debugf("Bot %d: Order filled - ID=%d, filledAmount=%.8f (original=%.8f)",
					inst.Config.ID, order.ID, filledAmount, order.Amount)
				// Update order reference for WebSocket notification
				order.FilledAmount = filledAmount
				order.Status = "filled"
				order.FilledAt = &now
			}
		}
	} else {
		s.log.Warnf("Bot %d: Cannot update order status - order has no database ID (OrderID=%s)",
			inst.Config.ID, order.OrderID)
		order.Status = "filled"
		order.FilledAmount = filledAmount
	}

	s.log.Infof("Bot %d order filled: %s %.8f @ %.2f", inst.Config.ID, order.Side, filledAmount, order.Price)

	// Notify order update via WebSocket
	s.notificationService.NotifyOrderUpdate(ctx, inst.Config.UserID, order)
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

	if err := s.posRepo.Update(ctx, pos); err != nil {
		s.log.Errorf("Bot %d: Failed to update position %d status to closed in database: %v",
			inst.Config.ID, pos.ID, err)
		// Continue anyway - we'll still remove from OpenPositions
	} else {
		s.log.Debugf("Bot %d: Successfully updated position %d status to closed in database",
			inst.Config.ID, pos.ID)
	}

	// Update Bot Stats and remove from map (protected by mutex)
	inst.mu.Lock()
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

	// Remove from OpenPositions map
	delete(inst.OpenPositions, pos.ID)
	inst.mu.Unlock()

	s.botRepo.UpdateBalance(ctx, inst.Config.ID, inst.Config.Balances)
	s.botRepo.UpdateStats(ctx, inst.Config.ID, inst.Config.TotalTrades, inst.Config.WinningTrades, inst.Config.TotalProfitIDR)

	s.log.Infof("Bot %d closed position on %s: profit=%.2f (%.2f%%)", inst.Config.ID, pos.Pair, profitIDR, profitPct)

	// Notify position update (closed)
	s.notificationService.NotifyPositionUpdate(ctx, inst.Config.UserID, pos)

	// Check if max loss limit reached after this trade
	// For Pump Hunter, use DailyLossLimitIDR from RiskManagement (MaxLossIDR is 0 for Pump Hunter)
	maxLossLimit := inst.Config.RiskManagement.DailyLossLimitIDR
	if maxLossLimit > 0 && inst.Config.TotalProfitIDR <= -maxLossLimit {
		s.log.Warnf("Bot %d reached total max loss limit (%.2f <= -%.2f), stopping bot",
			inst.Config.ID, inst.Config.TotalProfitIDR, maxLossLimit)
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
	status := strings.ToLower(order.Status)

	// Handle CANCELLED orders - remove from pending orders and clean up
	if status == "cancelled" || status == "canceled" {
		s.handleCancelledOrder(userID, order)
		return
	}

	// Indodax sends "FILL" or "DONE" for filled orders
	if status != "filled" && status != "fill" && status != "done" {
		return
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, inst := range s.instances {
		if inst.Config.UserID == userID {
			inst.mu.RLock()

			// Check PendingOrders first (for buy fills that move position to open)
			for _, pos := range inst.PendingOrders {
				// Check both OrderID and ClientOrderID (market orders don't preserve ClientOrderID)
				// Also extract numeric ID from WebSocket orderId format for limit orders
				matched := false
				if pos.EntryOrderID == order.OrderID || pos.EntryOrderID == order.ClientOrderID ||
					pos.ExitOrderID == order.OrderID || pos.ExitOrderID == order.ClientOrderID {
					matched = true
				} else {
					// Try to extract numeric ID from WebSocket orderId format for limit orders
					// Format: "{pair}-{type}-{numericId}" (e.g., "slerfidr-limit-9623015")
					parts := strings.Split(order.OrderID, "-")
					if len(parts) >= 3 {
						numericID := parts[len(parts)-1]
						// Check if stored EntryOrderID contains this numeric ID
						if strings.Contains(pos.EntryOrderID, numericID) || strings.Contains(pos.ExitOrderID, numericID) {
							matched = true
							s.log.Debugf("[WS_ORDER_UPDATE] PumpHunter: Matched by extracted numeric ID %s from orderId %s",
								numericID, order.OrderID)
						}
					}
				}

				if matched {
					s.log.Debugf("[WS_ORDER_UPDATE] PumpHunter: Found order in PendingOrders - OrderID=%s, ClientOrderID=%s, PositionID=%d, Side=%s, StoredEntryOrderID=%s",
						order.OrderID, order.ClientOrderID, pos.ID, order.Side, pos.EntryOrderID)
					inst.mu.RUnlock()

					// Convert indodax.OrderUpdate to model.Order for handleOrderFilled
					price, _ := strconv.ParseFloat(order.Price, 64)
					amount, _ := strconv.ParseFloat(order.ExecutedQty, 64)
					fillTime := time.Unix(order.TransactionTime/1000, 0)

					mOrder := &model.Order{
						OrderID:      order.ClientOrderID, // Store ClientOrderID for matching
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

			// Check OpenPositions (for sell fills, and also for buy DONE updates after position moved)
			s.log.Debugf("[WS_ORDER_UPDATE] PumpHunter: Checking OpenPositions for order %s (ClientOrderID: %s, Side: %s) - OpenPositions count: %d",
				order.OrderID, order.ClientOrderID, order.Side, len(inst.OpenPositions))
			for _, pos := range inst.OpenPositions {
				// Check both OrderID and ClientOrderID (market orders don't preserve ClientOrderID)
				// Also extract numeric ID from WebSocket orderId format (e.g., "mtlidr-market-4577901" -> "4577901")
				matched := false
				if pos.EntryOrderID == order.OrderID || pos.EntryOrderID == order.ClientOrderID ||
					pos.ExitOrderID == order.OrderID || pos.ExitOrderID == order.ClientOrderID {
					matched = true
				} else {
					// Try to extract numeric ID from WebSocket orderId format for market orders
					// Format: "{pair}-{type}-{numericId}" (e.g., "mtlidr-market-4577901")
					// Extract the numeric part at the end
					parts := strings.Split(order.OrderID, "-")
					if len(parts) >= 3 {
						numericID := parts[len(parts)-1]
						// Check if stored ExitOrderID or EntryOrderID contains this numeric ID
						if strings.Contains(pos.ExitOrderID, numericID) || strings.Contains(pos.EntryOrderID, numericID) {
							matched = true
							s.log.Debugf("[WS_ORDER_UPDATE] PumpHunter: Matched by extracted numeric ID %s from orderId %s",
								numericID, order.OrderID)
						}
					}
				}

				// If this is a BUY order matching EntryOrderID, it's likely a duplicate DONE update
				// (the FILL update already moved it from PendingOrders to OpenPositions)
				if matched && order.Side == "BUY" && (pos.EntryOrderID == order.OrderID || pos.EntryOrderID == order.ClientOrderID) {
					s.log.Debugf("[WS_ORDER_UPDATE] PumpHunter: Buy order %s (ClientOrderID: %s) already processed - position %d is now open (duplicate DONE update)",
						order.OrderID, order.ClientOrderID, pos.ID)
					// Still process it to ensure order status is updated, but it won't change position state
				}

				if matched {
					s.log.Debugf("[WS_ORDER_UPDATE] PumpHunter: Found order in OpenPositions - OrderID=%s, ClientOrderID=%s, PositionID=%d, Side=%s, StoredExitOrderID=%s",
						order.OrderID, order.ClientOrderID, pos.ID, order.Side, pos.ExitOrderID)
					inst.mu.RUnlock()

					// Convert indodax.OrderUpdate to model.Order for handleOrderFilled
					price, _ := strconv.ParseFloat(order.Price, 64)
					amount, _ := strconv.ParseFloat(order.ExecutedQty, 64)
					fillTime := time.Unix(order.TransactionTime/1000, 0)

					mOrder := &model.Order{
						OrderID:      order.ClientOrderID, // Store ClientOrderID for matching
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

			// Log detailed debug info for troubleshooting
			if len(inst.OpenPositions) > 0 || len(inst.PendingOrders) > 0 {
				s.log.Debugf("[WS_ORDER_UPDATE] PumpHunter: Order %s (ClientOrderID=%s) not found in bot %d positions (PendingOrders=%d, OpenPositions=%d)",
					order.OrderID, order.ClientOrderID, inst.Config.ID, len(inst.PendingOrders), len(inst.OpenPositions))
				// Log stored ExitOrderIDs for debugging
				for posID, pos := range inst.OpenPositions {
					s.log.Debugf("[WS_ORDER_UPDATE] PumpHunter: Position %d - EntryOrderID=%s, ExitOrderID=%s",
						posID, pos.EntryOrderID, pos.ExitOrderID)
				}
			} else {
				s.log.Debugf("[WS_ORDER_UPDATE] PumpHunter: Order %s not found in bot %d positions (PendingOrders=%d, OpenPositions=%d)",
					order.OrderID, inst.Config.ID, len(inst.PendingOrders), len(inst.OpenPositions))
			}
			inst.mu.RUnlock()
		}
	}
}

func (s *PumpHunterService) handleCancelledOrder(userID string, order *indodax.OrderUpdate) {
	wsClientOrderID := order.ClientOrderID

	s.log.Infof("[WS_ORDER_UPDATE] PumpHunter: Received CANCELLED order update - OrderID=%s, ClientOrderID=%s",
		order.OrderID, wsClientOrderID)

	// Update order status in database to keep in sync with Indodax
	ctx := context.Background()
	dbOrder, err := s.orderRepo.GetByOrderID(ctx, wsClientOrderID)
	if err == nil {
		// Order found in database - update status to cancelled
		oldStatus := dbOrder.Status
		s.orderRepo.UpdateStatus(ctx, dbOrder.ID, "cancelled")
		s.log.Infof("[WS_ORDER_UPDATE] PumpHunter: Updated order %d status from '%s' to 'cancelled'", dbOrder.ID, oldStatus)
	} else {
		s.log.Debugf("[WS_ORDER_UPDATE] PumpHunter: Order %s not found in database (may be paper trading or already cleaned up)", wsClientOrderID)
	}

	// Position is already cleaned up by cancelPendingOrder() if we initiated the cancellation
	// This WebSocket event is just a confirmation from Indodax that the order is cancelled
	s.log.Debugf("[WS_ORDER_UPDATE] PumpHunter: Cancelled order %s processed", wsClientOrderID)
}

func (s *PumpHunterService) getTickSize(pair indodax.Pair) float64 {
	val, ok := s.marketDataService.GetPriceIncrement(pair.ID)
	if ok {
		return val
	}
	// Fallback to precision based
	return 1.0 / math.Pow10(pair.PricePrecision)
}

// calculateBuyPrice calculates buy price with gap check
// Returns: (price, orderType) where orderType is "market" or "limit"
func (s *PumpHunterService) calculateBuyPrice(coin *model.Coin, pairInfo indodax.Pair) (float64, string) {
	bestBid := coin.BestBid
	bestAsk := coin.BestAsk

	// If bid/ask not available, use current price
	if bestBid == 0 || bestAsk == 0 {
		return coin.CurrentPrice, "market"
	}

	// Calculate gap percentage
	gapPercent := ((bestAsk - bestBid) / bestBid) * 100

	// If gap is tight (<1%), use market order
	if gapPercent < 1.0 {
		// Market order - use current price (bestAsk for immediate fill)
		return coin.CurrentPrice, "market"
	}

	// Otherwise, aggressive limit order: always outbid
	tickSize := s.getTickSize(pairInfo)
	buyPrice := bestBid + tickSize
	return buyPrice, "limit"
}

// monitorPendingOrders monitors pending orders for false pump detection and repositioning
func (s *PumpHunterService) monitorPendingOrders(inst *PumpHunterInstance) {
	inst.mu.Lock()

	// Collect positions to process (we need to unlock before calling cancelPendingOrder)
	var positionsToCheck []*model.Position
	for _, pos := range inst.PendingOrders {
		positionsToCheck = append(positionsToCheck, pos)
	}
	pendingCount := len(positionsToCheck)

	inst.mu.Unlock()

	if pendingCount == 0 {
		s.log.Debugf("Bot %d: monitorPendingOrders - No pending orders to check", inst.Config.ID)
		return
	}

	s.log.Debugf("Bot %d: monitorPendingOrders - Checking %d pending orders", inst.Config.ID, pendingCount)

	now := time.Now()

	for _, pos := range positionsToCheck {
		// Re-check if position still exists (might have been cancelled by another goroutine)
		inst.mu.RLock()
		_, stillPending := inst.PendingOrders[pos.ID]
		inst.mu.RUnlock()

		if !stillPending {
			continue // Position was already cancelled
		}

		timeSinceOrder := now.Sub(pos.OrderPlacedAt)

		// Debounce: Don't reposition too frequently (max once per 10 seconds)
		if timeSinceOrder < 10*time.Second {
			s.log.Debugf("Bot %d: Position %d (%s) - Too soon to check (%.1fs < 10s), skipping",
				inst.Config.ID, pos.ID, pos.Pair, timeSinceOrder.Seconds())
			continue // Too soon, skip
		}

		// Get current market data
		ctx := context.Background()
		coin, err := s.marketDataService.GetCoin(ctx, pos.Pair)
		if err != nil {
			s.log.Debugf("Bot %d: Position %d (%s) - Failed to get market data: %v",
				inst.Config.ID, pos.ID, pos.Pair, err)
			continue
		}

		s.log.Debugf("Bot %d: Position %d (%s) - Checking false pump: PumpScore=%.2f, MinPumpScore=%.2f, TimeSinceOrder=%.1fs",
			inst.Config.ID, pos.ID, pos.Pair, coin.PumpScore, inst.Config.EntryRules.MinPumpScore, timeSinceOrder.Seconds())

		// Check 1: False pump detection (always check)
		if coin.PumpScore < inst.Config.EntryRules.MinPumpScore {
			s.log.Infof("Bot %d: Position %d (%s) - FALSE PUMP DETECTED! PumpScore=%.2f < MinPumpScore=%.2f, cancelling order",
				inst.Config.ID, pos.ID, pos.Pair, coin.PumpScore, inst.Config.EntryRules.MinPumpScore)
			s.cancelPendingOrder(inst, pos, "false_pump")
			continue
		}

		// Check 2: Calculate new buy price
		pairInfo, ok := s.marketDataService.GetPairInfo(pos.Pair)
		if !ok {
			continue
		}

		newBuyPrice, orderType := s.calculateBuyPrice(coin, pairInfo)

		// Check if price changed (always reposition to outbid)
		priceDiff := math.Abs(newBuyPrice - pos.EntryPrice)
		if priceDiff > 0.01 { // Small tolerance for floating point
			// Price changed - reposition (repositionPendingOrder handles its own mutex)
			s.repositionPendingOrder(inst, pos, newBuyPrice, orderType, coin)
			continue
		}

		// Check 3: Time-based checks (after 2 minutes)
		if timeSinceOrder > 2*time.Minute {
			// 2 minutes passed - check if still valid
			if coin.PumpScore < inst.Config.EntryRules.MinPumpScore {
				s.cancelPendingOrder(inst, pos, "false_pump_timeout")
			}
			// Otherwise, continue monitoring (pump still valid)
		}
	}
}

// repositionPendingOrder cancels old order and places new one at new price
func (s *PumpHunterService) repositionPendingOrder(
	inst *PumpHunterInstance,
	pos *model.Position,
	newPrice float64,
	orderType string, // "market" or "limit"
	coin *model.Coin,
) {
	ctx := context.Background()

	// Cancel old order (if it was a limit order)
	if pos.EntryOrderType == "limit" {
		err := inst.TradeClient.CancelOrder(ctx, pos.Pair, pos.EntryOrderID, "buy")
		if err != nil {
			// For paper trading, cancellation might not be needed
			if !inst.Config.IsPaperTrading {
				s.log.Warnf("Bot %d: Failed to cancel order for reposition: indodax API error: %v", inst.Config.ID, err)
			}
		} else {
			// Update order status
			s.orderRepo.UpdateStatus(ctx, pos.InternalEntryOrderID, "cancelled")
		}
	}

	// Place new order at new price
	clientOrderID := GenerateClientOrderID(inst.Config.ID, pos.Pair, "buy")
	res, err := inst.TradeClient.Trade(ctx, "buy", pos.Pair, newPrice, pos.EntryQuantity, orderType, clientOrderID)
	if err != nil {
		s.log.Errorf("Bot %d: Failed to reposition order: indodax API error: %v", inst.Config.ID, err)
		// Cancel position if reposition fails
		s.cancelPendingOrder(inst, pos, "reposition_failed")
		return
	}

	// Update position with new order ID
	// Store ClientOrderID for WebSocket matching (same as openPosition)
	orderIDStr := res.ClientOrderID
	if orderIDStr == "" {
		// Fallback to numeric ID if ClientOrderID is empty (shouldn't happen)
		orderIDStr = fmt.Sprintf("%d", res.OrderID)
		s.log.Warnf("Bot %d: ClientOrderID is empty during reposition, using numeric ID %s", inst.Config.ID, orderIDStr)
	}
	pos.EntryOrderID = orderIDStr
	pos.EntryPrice = newPrice // Update entry price to new limit price
	pos.EntryOrderType = orderType
	pos.OrderPlacedAt = time.Now() // Reset timer for debouncing

	// Update or create order record
	if pos.InternalEntryOrderID > 0 {
		// Update existing order
		order, err := s.orderRepo.GetByID(ctx, pos.InternalEntryOrderID)
		if err == nil {
			oldStatus := order.Status
			order.OrderID = pos.EntryOrderID
			order.Price = newPrice
			order.Status = "open"
			s.orderRepo.Update(ctx, order, oldStatus)
		} else {
			// Order not found, create new one
			order := &model.Order{
				UserID:       inst.Config.UserID,
				ParentID:     pos.ID,
				ParentType:   "position",
				OrderID:      pos.EntryOrderID,
				Pair:         pos.Pair,
				Side:         "buy",
				Status:       "open",
				Price:        newPrice,
				Amount:       pos.EntryQuantity,
				IsPaperTrade: inst.Config.IsPaperTrading,
			}
			s.orderRepo.Create(ctx, order)
			pos.InternalEntryOrderID = order.ID
		}
	} else {
		// Create new order record
		order := &model.Order{
			UserID:       inst.Config.UserID,
			ParentID:     pos.ID,
			ParentType:   "position",
			OrderID:      pos.EntryOrderID,
			Pair:         pos.Pair,
			Side:         "buy",
			Status:       "open",
			Price:        newPrice,
			Amount:       pos.EntryQuantity,
			IsPaperTrade: inst.Config.IsPaperTrading,
		}
		s.orderRepo.Create(ctx, order)
		pos.InternalEntryOrderID = order.ID
	}

	s.posRepo.Update(ctx, pos)

	s.log.Infof("Bot %d: Repositioned buy order for %s: %.2f → %.2f (%s)",
		inst.Config.ID, pos.Pair, pos.EntryPrice, newPrice, orderType)
}

// cancelPendingOrder cancels a pending order and deletes the position (false pump)
// NOTE: This function handles its own mutex locking. Callers should NOT hold inst.mu.
func (s *PumpHunterService) cancelPendingOrder(inst *PumpHunterInstance, pos *model.Position, reason string) {
	ctx := context.Background()

	// Cancel order (if it was a limit order)
	if pos.EntryOrderType == "limit" {
		err := inst.TradeClient.CancelOrder(ctx, pos.Pair, pos.EntryOrderID, "buy")
		if err != nil {
			// For paper trading, cancellation might not be needed
			if !inst.Config.IsPaperTrading {
				s.log.Warnf("Bot %d: Failed to cancel order %s: indodax API error: %v", inst.Config.ID, pos.EntryOrderID, err)
			}
		}
	}

	// Update order status
	if pos.InternalEntryOrderID > 0 {
		s.orderRepo.UpdateStatus(ctx, pos.InternalEntryOrderID, "cancelled")
	}

	// Restore balance and remove from maps (protected by mutex)
	inst.mu.Lock()
	inst.Config.Balances["idr"] += pos.EntryAmountIDR
	delete(inst.PendingOrders, pos.ID)
	delete(inst.OpenPositions, pos.ID)
	inst.mu.Unlock()

	s.botRepo.UpdateBalance(ctx, inst.Config.ID, inst.Config.Balances)

	// Delete position from database
	s.posRepo.Delete(ctx, pos.ID)

	s.log.Infof("Bot %d: Cancelled pending order for %s - %s", inst.Config.ID, pos.Pair, reason)
}

// placeLimitSellOrder places a limit sell order (for target profit > 1%)
func (s *PumpHunterService) placeLimitSellOrder(inst *PumpHunterInstance, pos *model.Position, sellPrice float64) {
	ctx := context.Background()

	// Get pair info to get volume precision for rounding
	pairInfo, ok := s.marketDataService.GetPairInfo(pos.Pair)
	if !ok {
		s.log.Errorf("Bot %d: Failed to get pair info for %s when placing sell order", inst.Config.ID, pos.Pair)
		return
	}

	// Get volume precision and round amount to correct precision
	volumePrecision := util.GetVolumePrecision(pairInfo)
	amount := util.FloorToPrecision(pos.EntryQuantity, volumePrecision)

	// Validate amount is not zero after rounding
	if amount <= 0 {
		s.log.Errorf("Bot %d: Amount %.8f rounded to zero for %s (volumePrecision=%d), cannot place sell order",
			inst.Config.ID, pos.EntryQuantity, pos.Pair, volumePrecision)
		return
	}

	// Validate amount meets minimum trade requirement
	if amount < pairInfo.TradeMinTradedCurrency {
		s.log.Errorf("Bot %d: Amount %.8f is below minimum trade requirement %.8f for %s",
			inst.Config.ID, amount, pairInfo.TradeMinTradedCurrency, pos.Pair)
		return
	}

	s.log.Debugf("Bot %d: Placing sell order for %s - Original amount=%.8f, Rounded amount=%.8f (volumePrecision=%d), Price=%.2f",
		inst.Config.ID, pos.Pair, pos.EntryQuantity, amount, volumePrecision, sellPrice)

	// Generate client order ID
	clientOrderID := GenerateClientOrderID(inst.Config.ID, pos.Pair, "sell")

	// Place limit sell order with rounded amount
	res, err := inst.TradeClient.Trade(ctx, "sell", pos.Pair, sellPrice, amount, "limit", clientOrderID)
	if err != nil {
		// Log full Indodax error message
		s.log.Errorf("Bot %d: Failed to place limit sell order for position %d (%s): indodax API error: %v",
			inst.Config.ID, pos.ID, pos.Pair, err)

		errStr := strings.ToLower(err.Error())
		// Check if error is "Insufficient balance" - this means position can't be sold
		// (likely already sold or balance changed). Close the position.
		if strings.Contains(errStr, "insufficient balance") {
			s.log.Warnf("Bot %d: Insufficient balance when placing sell order for position %d (%s) - closing position",
				inst.Config.ID, pos.ID, pos.Pair)

			// Capture position ID and pair for goroutine
			positionID := pos.ID
			positionPair := pos.Pair
			entryPrice := pos.EntryPrice

			// Close position asynchronously to avoid blocking bot startup
			go func() {
				defer func() {
					if r := recover(); r != nil {
						s.log.Errorf("Bot %d: Panic in position close goroutine for position %d: %v",
							inst.Config.ID, positionID, r)
					}
				}()

				closeCtx := context.Background()

				// Reload position from database to ensure we have latest state
				reloadedPos, err := s.posRepo.GetByID(closeCtx, positionID)
				if err != nil {
					s.log.Errorf("Bot %d: Failed to reload position %d from database: %v",
						inst.Config.ID, positionID, err)
					return
				}

				currentPrice := entryPrice // Fallback to entry price

				// Try to get current market price with timeout (non-blocking)
				priceCtx, cancel := context.WithTimeout(closeCtx, 2*time.Second)
				if coin, err := s.marketDataService.GetCoin(priceCtx, positionPair); err == nil {
					currentPrice = coin.CurrentPrice
				} else {
					s.log.Debugf("Bot %d: Could not get current price for %s, using entry price %.2f: %v",
						inst.Config.ID, positionPair, currentPrice, err)
				}
				cancel()

				// Verify position exists - check both OpenPositions and database
				// (position might not be in OpenPositions yet if this is during bot restore)
				inst.mu.RLock()
				_, existsInMap := inst.OpenPositions[positionID]
				inst.mu.RUnlock()

				// If not in map, check database to see if it's still active
				if !existsInMap {
					dbPos, err := s.posRepo.GetByID(closeCtx, positionID)
					if err != nil || dbPos.Status == model.PositionStatusClosed {
						s.log.Debugf("Bot %d: Position %d (%s) not found in OpenPositions and is closed/not found in DB - skipping",
							inst.Config.ID, positionID, positionPair)
						return
					}
					// Position exists in DB and is not closed - use it
					reloadedPos = dbPos
					s.log.Debugf("Bot %d: Position %d (%s) not in OpenPositions yet (likely during restore), using DB position",
						inst.Config.ID, positionID, positionPair)
				}

				s.log.Infof("Bot %d: Closing position %d (%s) due to insufficient balance at price %.2f",
					inst.Config.ID, positionID, positionPair, currentPrice)

				// Close position with current market price
				// Note: finalizePositionClose handles its own mutex locking
				s.finalizePositionClose(inst, reloadedPos, currentPrice)

				s.log.Infof("Bot %d: Successfully closed position %d (%s) due to insufficient balance",
					inst.Config.ID, positionID, positionPair)
			}()

			return
		}

		s.log.Errorf("Bot %d: Failed to place limit sell order: %v", inst.Config.ID, err)
		return
	}

	// Store the ClientOrderID (for both WebSocket matching and cancellation)
	// This matches MarketMaker behavior at market_maker_service.go:1230
	orderIDStr := res.ClientOrderID
	if orderIDStr == "" {
		// Fallback to numeric ID (shouldn't happen)
		orderIDStr = fmt.Sprintf("%d", res.OrderID)
	}

	// Update position status
	pos.Status = model.PositionStatusSelling
	pos.ExitOrderID = orderIDStr

	// Create order record (use rounded amount)
	order := &model.Order{
		UserID:       inst.Config.UserID,
		ParentID:     pos.ID,
		ParentType:   "position",
		OrderID:      orderIDStr, // Use the same orderIDStr (clientOrderID for paper, numeric for live)
		Pair:         pos.Pair,
		Side:         "sell",
		Status:       "open",
		Price:        sellPrice,
		Amount:       amount, // Use rounded amount, not pos.EntryQuantity
		IsPaperTrade: inst.Config.IsPaperTrading,
	}

	s.orderRepo.Create(ctx, order)
	pos.InternalExitOrderID = order.ID
	s.posRepo.Update(ctx, pos)

	// Notify via WebSocket
	s.notificationService.NotifyOrderUpdate(ctx, inst.Config.UserID, order)
	s.notificationService.NotifyPositionUpdate(ctx, inst.Config.UserID, pos)

	s.log.Infof("Bot %d: Placed limit sell order for %s at %.2f (target profit)", inst.Config.ID, pos.Pair, sellPrice)
}

func (s *PumpHunterService) syncBalance(ctx context.Context, inst *PumpHunterInstance) error {
	var realIDR float64
	if !inst.Config.IsPaperTrading {
		info, err := inst.TradeClient.GetInfo(ctx)
		if err != nil {
			s.log.Errorf("Bot %d: Failed to get account info from Indodax: indodax API error: %v", inst.Config.ID, err)
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

	// Determine IDR Allocation
	var idrAllocation float64
	if inst.Config.IsPaperTrading {
		// For paper trading, use the actual virtual balance (can grow/shrink with trades)
		idrAllocation = realIDR
	} else {
		// For live trading, cap to initial_balance_idr (safety measure)
		idrAllocation = inst.Config.InitialBalanceIDR
		if realIDR < idrAllocation {
			idrAllocation = realIDR
		}
	}

	// Validate and normalize balances (using shared utility)
	// For Pump Hunter, we only validate IDR balance since coin balances are per-position
	requiredCurrencies := []string{"idr"}
	inst.Config.Balances = util.ValidateAndNormalizeBalances(
		inst.Config.Balances,
		requiredCurrencies,
		idrAllocation,
		s.log,
	)

	// Update IDR balance (preserve current balance for paper trading, sync for live trading)
	inst.Config.Balances["idr"] = idrAllocation

	// We do NOT sync coin balances from Indodax because the bot should only
	// sell what it bought itself. Existing coin balances in inst.Config.Balances
	// (purchased by the bot in previous runs) are preserved.

	s.log.Infof("Bot %d balance synced: IDR=%.2f (Allocated=%.2f)", inst.Config.ID, realIDR, idrAllocation)

	return s.botRepo.UpdateBalance(ctx, inst.Config.ID, inst.Config.Balances)
}
