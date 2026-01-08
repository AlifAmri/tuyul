package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"tuyul/backend/internal/model"
	"tuyul/backend/internal/repository"
	"tuyul/backend/internal/service/market"
	"tuyul/backend/internal/util"
	"tuyul/backend/pkg/indodax"
	"tuyul/backend/pkg/logger"
)

type CopilotService struct {
	tradeRepo         *repository.TradeRepository
	orderRepo         *repository.OrderRepository
	balanceRepo       *repository.BalanceRepository
	apiKeyService     *APIKeyService
	marketDataService *market.MarketDataService
	orderMonitor      *OrderMonitor
	indodaxClient     *indodax.Client
	log               *logger.Logger
}

func NewCopilotService(
	tradeRepo *repository.TradeRepository,
	orderRepo *repository.OrderRepository,
	balanceRepo *repository.BalanceRepository,
	apiKeyService *APIKeyService,
	marketDataService *market.MarketDataService,
	orderMonitor *OrderMonitor,
	indodaxClient *indodax.Client,
) *CopilotService {
	s := &CopilotService{
		tradeRepo:         tradeRepo,
		orderRepo:         orderRepo,
		balanceRepo:       balanceRepo,
		apiKeyService:     apiKeyService,
		marketDataService: marketDataService,
		orderMonitor:      orderMonitor,
		indodaxClient:     indodaxClient,
		log:               logger.GetLogger(),
	}

	// Register Copilot callbacks to OrderMonitor for real trades
	orderMonitor.SetBuyFilledCallback(s.handleBuyOrderFilled)
	orderMonitor.SetSellFilledCallback(s.handleSellOrderFilled)

	return s
}

// PlaceBuyOrder validates and places a buy order for copilot trading
func (s *CopilotService) PlaceBuyOrder(ctx context.Context, userID string, req *model.TradeRequest) (*model.Trade, error) {
	// 1. Validate request
	if err := s.validateTradeRequest(req); err != nil {
		return nil, err
	}

	// 2. Setup Trade Client
	var tradeClient TradeClient
	if req.IsPaperTrade {
		balances, _ := s.getPaperBalances(ctx, userID)
		tradeClient = NewPaperTradeClient(balances, func(order *model.Order) {
			s.handleOrderFilled(order)
		})
	} else {
		credentials, err := s.apiKeyService.GetDecrypted(ctx, userID)
		if err != nil {
			return nil, util.NewAppError(400, util.ErrCodeAPIKeyInvalid, "API key not found or invalid")
		}
		tradeClient = NewLiveTradeClient(s.indodaxClient, credentials.Key, credentials.Secret)
	}

	// 3. Check balance
	accountInfo, err := tradeClient.GetInfo(ctx)
	if err != nil {
		return nil, util.NewAppErrorWithDetails(400, util.ErrCodeIndodaxAPI, "Failed to get account info", err.Error())
	}

	// Parse IDR balance
	idrBalance := s.parseBalance(accountInfo.Balance["idr"])
	if idrBalance < req.VolumeIDR {
		return nil, util.NewAppError(400, util.ErrCodeInsufficientBalance,
			fmt.Sprintf("Insufficient IDR balance. Available: %.2f, Required: %.2f", idrBalance, req.VolumeIDR))
	}

	// 4. Calculate amount (coin quantity to buy)
	amount := req.VolumeIDR / req.BuyingPrice

	// 5. Round to appropriate precision
	pairInfo, ok := s.marketDataService.GetPairInfo(req.Pair)
	if ok {
		amount = util.FloorToPrecision(amount, pairInfo.VolumePrecision)
	} else {
		amount = util.RoundToPrecision(amount, 8)
	}

	// 6. Place buy order
	result, err := tradeClient.Trade(ctx, "buy", req.Pair, req.BuyingPrice, amount, "limit")
	if err != nil {
		return nil, util.NewAppErrorWithDetails(400, util.ErrCodeIndodaxAPI, "Failed to place buy order", err.Error())
	}

	// 7. Create trade record (get ID first)
	now := time.Now()
	trade := &model.Trade{
		UserID:       userID,
		Pair:         req.Pair,
		BuyPrice:     req.BuyingPrice,
		BuyAmount:    amount,
		BuyAmountIDR: req.VolumeIDR,
		TargetProfit: req.TargetProfit,
		StopLoss:     req.StopLoss,
		Status:       model.TradeStatusPending,
		IsPaperTrade: req.IsPaperTrade,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.tradeRepo.Create(ctx, trade); err != nil {
		s.log.Errorf("Failed to save trade: %v", err)
		return nil, util.ErrInternalServer("Failed to save trade")
	}

	// 8. Create unified order record
	order := &model.Order{
		UserID:       userID,
		ParentID:     trade.ID,
		ParentType:   "trade",
		OrderID:      fmt.Sprintf("%d", result.OrderID),
		Pair:         req.Pair,
		Side:         "buy",
		Status:       "open",
		Price:        req.BuyingPrice,
		Amount:       amount,
		IsPaperTrade: req.IsPaperTrade,
	}

	if err := s.orderRepo.Create(ctx, order); err != nil {
		s.log.Errorf("Failed to save order: %v", err)
		// We'll continue because the order IS placed on Indodax
	}

	// 9. Update trade with order IDs
	trade.InternalBuyOrderID = order.ID
	trade.BuyOrderID = order.OrderID
	s.tradeRepo.Update(ctx, trade, model.TradeStatusPending)

	// 10. Deduct virtual balance if paper trading
	if req.IsPaperTrade {
		balances, _ := s.getPaperBalances(ctx, userID)
		balances["idr"] -= req.VolumeIDR
		s.savePaperBalances(ctx, userID, balances)
	}

	// 11. Subscribe to order updates for live trading
	if !req.IsPaperTrade {
		s.orderMonitor.SubscribeUserOrders(ctx, userID)
	}

	s.log.Infof("Buy order placed (%s): TradeID=%d, Pair=%s, Price=%.2f, Amount=%.8f",
		map[bool]string{true: "paper", false: "live"}[req.IsPaperTrade],
		trade.ID, trade.Pair, trade.BuyPrice, trade.BuyAmount)

	return trade, nil
}

// GetTrade retrieves a specific trade
func (s *CopilotService) GetTrade(ctx context.Context, userID string, tradeID int64) (*model.Trade, error) {
	trade, err := s.tradeRepo.GetByID(ctx, tradeID)
	if err != nil {
		return nil, util.ErrNotFound("Trade not found")
	}

	if trade.UserID != userID {
		return nil, util.NewAppError(403, util.ErrCodeForbidden, "Access denied")
	}

	return trade, nil
}

// ListTrades retrieves user's trades with pagination
func (s *CopilotService) ListTrades(ctx context.Context, userID string, offset, limit int) ([]*model.Trade, int64, error) {
	if limit > 100 {
		limit = 100
	}
	if limit <= 0 {
		limit = 20
	}

	return s.tradeRepo.ListByUser(ctx, userID, offset, limit)
}

// CancelBuyOrder cancels a pending buy order
func (s *CopilotService) CancelBuyOrder(ctx context.Context, userID string, tradeID int64) error {
	// 1. Get trade
	trade, err := s.GetTrade(ctx, userID, tradeID)
	if err != nil {
		return err
	}

	// 2. Validate status
	if trade.Status != model.TradeStatusPending {
		return util.NewAppError(400, util.ErrCodeBadRequest, "Can only cancel pending orders")
	}

	// 3. Get Trade Client
	tradeClient, err := s.getTradeClient(ctx, userID, trade.IsPaperTrade)
	if err != nil {
		return err
	}

	// 4. Cancel on Indodax
	err = tradeClient.CancelOrder(ctx, trade.Pair, trade.BuyOrderID, "buy")
	if err != nil {
		return util.NewAppErrorWithDetails(400, util.ErrCodeIndodaxAPI, "Failed to cancel order", err.Error())
	}

	// 5. Update trade status
	oldStatus := trade.Status
	trade.Status = model.TradeStatusCancelled
	now := time.Now()
	trade.CancelledAt = &now

	if err := s.tradeRepo.Update(ctx, trade, oldStatus); err != nil {
		s.log.Errorf("Failed to update trade status: %v", err)
		return util.ErrInternalServer("Failed to update trade")
	}

	// 6. Return virtual balance if paper trading
	if trade.IsPaperTrade {
		balances, _ := s.getPaperBalances(ctx, userID)
		balances["idr"] += trade.BuyAmountIDR
		s.savePaperBalances(ctx, userID, balances)
	}

	s.log.Infof("Buy order cancelled (%s): TradeID=%d",
		map[bool]string{true: "paper", false: "live"}[trade.IsPaperTrade], trade.ID)

	return nil
}

// validateTradeRequest validates the trade request parameters
func (s *CopilotService) validateTradeRequest(req *model.TradeRequest) error {
	if req.Pair == "" {
		return util.NewAppError(400, util.ErrCodeValidation, "Pair is required")
	}

	if req.BuyingPrice <= 0 {
		return util.NewAppError(400, util.ErrCodeValidation, "Buying price must be greater than 0")
	}

	if req.VolumeIDR < 10000 {
		return util.NewAppError(400, util.ErrCodeValidation, "Minimum volume is 10,000 IDR")
	}

	if req.TargetProfit < 0.1 || req.TargetProfit > 1000 {
		return util.NewAppError(400, util.ErrCodeValidation, "Target profit must be between 0.1% and 1000%")
	}

	if req.StopLoss <= 0 || req.StopLoss > 100 {
		return util.NewAppError(400, util.ErrCodeValidation, "Stop loss must be between 0% and 100%")
	}

	if req.StopLoss >= req.TargetProfit {
		return util.NewAppError(400, util.ErrCodeValidation, "Stop loss must be less than target profit")
	}

	return nil
}

// parseBalance parses balance string to float64
func (s *CopilotService) parseBalance(balanceStr string) float64 {
	var balance float64
	fmt.Sscanf(balanceStr, "%f", &balance)
	return balance
}

// extractCoinSymbol extracts the coin symbol from pair (e.g., "btcidr" -> "btc")
func (s *CopilotService) extractCoinSymbol(pair string) string {
	pair = strings.ToLower(pair)
	if strings.HasSuffix(pair, "idr") {
		return strings.TrimSuffix(pair, "idr")
	}
	if strings.Contains(pair, "_") {
		parts := strings.Split(pair, "_")
		if len(parts) > 0 {
			return parts[0]
		}
	}
	return pair
}

// PlaceAutoSell automatically places a sell order when buy order fills
func (s *CopilotService) PlaceAutoSell(ctx context.Context, trade *model.Trade, filledAmount float64) error {
	s.log.Infof("Placing auto-sell for TradeID=%d", trade.ID)

	// 1. Get Trade Client
	tradeClient, err := s.getTradeClient(ctx, trade.UserID, trade.IsPaperTrade)
	if err != nil {
		return err
	}

	// 2. Calculate sell price (buy price + target profit)
	sellPrice := trade.BuyPrice * (1 + trade.TargetProfit/100)

	// Round to appropriate precision
	pairInfo, ok := s.marketDataService.GetPairInfo(trade.Pair)
	if ok {
		sellPrice = util.RoundToPrecision(sellPrice, pairInfo.PricePrecision)
	} else {
		sellPrice = util.RoundToPrecision(sellPrice, 0) // Round to nearest IDR
	}

	// 3. Get current balance to determine exact sell amount
	accountInfo, err := tradeClient.GetInfo(ctx)
	if err != nil {
		s.log.Errorf("Failed to get account info for auto-sell: %v", err)
		// Use filled amount as fallback
	} else {
		// Extract coin symbol and get balance
		coinSymbol := s.extractCoinSymbol(trade.Pair)
		if balanceStr, ok := accountInfo.Balance[coinSymbol]; ok {
			balance := s.parseBalance(balanceStr)
			if balance > 0 {
				filledAmount = balance
			}
		}
	}

	// 4. Place sell order
	result, err := tradeClient.Trade(ctx, "sell", trade.Pair, sellPrice, filledAmount, "limit")
	if err != nil {
		s.log.Errorf("Failed to place auto-sell order: %v", err)
		trade.Status = model.TradeStatusError
		trade.ErrorMessage = fmt.Sprintf("Auto-sell failed: %v", err)
		s.tradeRepo.Update(ctx, trade, model.TradeStatusFilled)
		return fmt.Errorf("failed to place sell order: %w", err)
	}

	// 5. Save unified order record
	order := &model.Order{
		UserID:       trade.UserID,
		ParentID:     trade.ID,
		ParentType:   "trade",
		OrderID:      fmt.Sprintf("%d", result.OrderID),
		Pair:         trade.Pair,
		Side:         "sell",
		Status:       "open",
		Price:        sellPrice,
		Amount:       filledAmount,
		IsPaperTrade: trade.IsPaperTrade,
	}

	if err := s.orderRepo.Create(ctx, order); err != nil {
		s.log.Errorf("Failed to save auto-sell order: %v", err)
	}

	// 6. Update trade with sell order info
	trade.InternalSellOrderID = order.ID
	trade.SellOrderID = order.OrderID
	trade.SellPrice = sellPrice
	trade.SellAmount = filledAmount

	if err := s.tradeRepo.Update(ctx, trade, model.TradeStatusFilled); err != nil {
		s.log.Errorf("Failed to update trade with sell order: %v", err)
		return err
	}

	// 6. Store buy-sell mapping
	if err := s.tradeRepo.SetBuySellMap(ctx, trade.BuyOrderID, trade.SellOrderID); err != nil {
		s.log.Errorf("Failed to store buy-sell mapping: %v", err)
	}

	s.log.Infof("Auto-sell placed: TradeID=%d, SellOrderID=%s, Price=%.2f, Amount=%.8f",
		trade.ID, trade.SellOrderID, sellPrice, filledAmount)

	return nil
}

// ManualSell manually sells at market price (overrides auto-sell)
func (s *CopilotService) ManualSell(ctx context.Context, userID string, tradeID int64) error {
	// 1. Get trade
	trade, err := s.GetTrade(ctx, userID, tradeID)
	if err != nil {
		return err
	}

	// 2. Validate status
	if trade.Status != model.TradeStatusFilled {
		return util.NewAppError(400, util.ErrCodeBadRequest, "Trade must be in filled status to sell")
	}

	// 3. Get Trade Client
	tradeClient, err := s.getTradeClient(ctx, userID, trade.IsPaperTrade)
	if err != nil {
		return err
	}

	// 4. Cancel existing sell order if exists
	if trade.SellOrderID != "" {
		err := tradeClient.CancelOrder(ctx, trade.Pair, trade.SellOrderID, "sell")
		if err != nil {
			s.log.Warnf("Failed to cancel existing sell order: %v", err)
		}
	}

	// 5. Get current balance
	accountInfo, err := tradeClient.GetInfo(ctx)
	if err != nil {
		return util.NewAppErrorWithDetails(400, util.ErrCodeIndodaxAPI, "Failed to get account info", err.Error())
	}

	coinSymbol := s.extractCoinSymbol(trade.Pair)
	sellAmount := s.parseBalance(accountInfo.Balance[coinSymbol])

	if sellAmount <= 0 {
		return util.NewAppError(400, util.ErrCodeInsufficientBalance, "No coins available to sell")
	}

	// 6. Place market sell order (use current market price)
	// For market orders, we can use a very low price to ensure immediate fill
	marketPrice := trade.BuyPrice * 0.95 // 5% below buy price to ensure fill

	result, err := tradeClient.Trade(ctx, "sell", trade.Pair, marketPrice, sellAmount, "market")
	if err != nil {
		return util.NewAppErrorWithDetails(400, util.ErrCodeIndodaxAPI, "Failed to place manual sell", err.Error())
	}

	// 7. Update trade
	trade.SellOrderID = fmt.Sprintf("%d", result.OrderID)
	trade.SellPrice = marketPrice
	trade.SellAmount = sellAmount
	trade.ManualSell = true

	if err := s.tradeRepo.Update(ctx, trade, model.TradeStatusFilled); err != nil {
		s.log.Errorf("Failed to update trade: %v", err)
		return util.ErrInternalServer("Failed to update trade")
	}

	s.log.Infof("Manual sell placed (%s): TradeID=%d, Amount=%.8f",
		map[bool]string{true: "paper", false: "live"}[trade.IsPaperTrade], trade.ID, sellAmount)

	return nil
}

// Internal methodology helpers

func (s *CopilotService) getTradeClient(ctx context.Context, userID string, isPaperTrade bool) (TradeClient, error) {
	if isPaperTrade {
		balances, _ := s.getPaperBalances(ctx, userID)
		return NewPaperTradeClient(balances, func(order *model.Order) {
			s.handleOrderFilled(order)
		}), nil
	}

	credentials, err := s.apiKeyService.GetDecrypted(ctx, userID)
	if err != nil {
		return nil, util.NewAppError(400, util.ErrCodeAPIKeyInvalid, "API key not found")
	}
	return NewLiveTradeClient(s.indodaxClient, credentials.Key, credentials.Secret), nil
}

// handleOrderFilled is the callback for PaperTradeClient
func (s *CopilotService) handleOrderFilled(order *model.Order) {
	ctx := context.Background()
	// Find trade
	orderObj, err := s.orderRepo.GetByOrderID(ctx, order.OrderID)
	if err != nil {
		return
	}

	trade, err := s.tradeRepo.GetByID(ctx, orderObj.ParentID)
	if err != nil {
		// Might be a bot order or unknown
		return
	}

	if order.Side == "buy" {
		s.handleBuyOrderFilled(trade, order.Amount)
	} else {
		s.handleSellOrderFilled(trade, order.Amount, order.Price)
	}
}

func (s *CopilotService) handleBuyOrderFilled(trade *model.Trade, filledAmount float64) {
	ctx := context.Background()
	s.log.Infof("Buy order filled: TradeID=%d, Amount=%.8f", trade.ID, filledAmount)

	// Update trade
	trade.Status = model.TradeStatusFilled
	trade.BuyFilledAmount = filledAmount
	now := time.Now()
	trade.BuyFilledAt = &now
	s.tradeRepo.Update(ctx, trade, model.TradeStatusPending)

	// Update virtual balance if paper trading
	if trade.IsPaperTrade {
		balances, _ := s.getPaperBalances(ctx, trade.UserID)
		coinSymbol := s.extractCoinSymbol(trade.Pair)
		balances[coinSymbol] += filledAmount
		s.savePaperBalances(ctx, trade.UserID, balances)
	}

	// Place auto-sell
	s.PlaceAutoSell(ctx, trade, filledAmount)
}

func (s *CopilotService) handleSellOrderFilled(trade *model.Trade, filledAmount float64, avgPrice float64) {
	ctx := context.Background()
	s.log.Infof("Sell order filled: TradeID=%d, Amount=%.8f, Price=%.2f", trade.ID, filledAmount, avgPrice)

	// Calculate profit
	sellRevenue := filledAmount * avgPrice
	buySpent := trade.BuyAmountIDR // We use the IDR amount spent
	profitIDR := sellRevenue - buySpent
	profitPercent := (profitIDR / buySpent) * 100

	// Update trade
	trade.Status = model.TradeStatusCompleted
	trade.SellFilledAmount = filledAmount
	now := time.Now()
	trade.SellFilledAt = &now
	trade.ProfitIDR = profitIDR
	trade.ProfitPercent = profitPercent
	s.tradeRepo.Update(ctx, trade, model.TradeStatusFilled)

	// Update virtual balance if paper trading
	if trade.IsPaperTrade {
		balances, _ := s.getPaperBalances(ctx, trade.UserID)
		// Remove coins
		coinSymbol := s.extractCoinSymbol(trade.Pair)
		balances[coinSymbol] -= filledAmount
		if balances[coinSymbol] < 0 {
			balances[coinSymbol] = 0
		}
		// Add IDR
		balances["idr"] += sellRevenue
		s.savePaperBalances(ctx, trade.UserID, balances)
	}

	s.log.Infof("Trade completed (%s): TradeID=%d, Profit=%.2f IDR (%.2f%%)",
		map[bool]string{true: "paper", false: "live"}[trade.IsPaperTrade],
		trade.ID, profitIDR, profitPercent)
}

func (s *CopilotService) getPaperBalances(ctx context.Context, userID string) (map[string]float64, error) {
	return s.balanceRepo.GetUserPaperBalances(ctx, userID)
}

func (s *CopilotService) savePaperBalances(ctx context.Context, userID string, balances map[string]float64) {
	s.balanceRepo.SaveUserPaperBalances(ctx, userID, balances)
}
