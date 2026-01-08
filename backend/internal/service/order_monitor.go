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
	"tuyul/backend/pkg/indodax"
	"tuyul/backend/pkg/logger"
)

// OrderMonitor monitors order updates via Private WebSocket
type OrderMonitor struct {
	tradeRepo           *repository.TradeRepository
	orderRepo           *repository.OrderRepository
	apiKeyService       *APIKeyService
	notificationService *NotificationService
	indodaxClient       *indodax.Client
	log                 *logger.Logger

	// User-specific WebSocket clients
	wsClients map[string]*indodax.PrivateWSClient
	mu        sync.RWMutex

	// Callbacks for Copilot
	onBuyFilled  func(trade *model.Trade, filledAmount float64)
	onSellFilled func(trade *model.Trade, filledAmount float64, avgPrice float64)

	// Generic handlers for bots
	orderHandlers []func(userID string, order *indodax.OrderUpdate)

	done chan struct{}
}

func NewOrderMonitor(
	tradeRepo *repository.TradeRepository,
	orderRepo *repository.OrderRepository,
	apiKeyService *APIKeyService,
	notificationService *NotificationService,
	indodaxClient *indodax.Client,
) *OrderMonitor {
	return &OrderMonitor{
		tradeRepo:           tradeRepo,
		orderRepo:           orderRepo,
		apiKeyService:       apiKeyService,
		notificationService: notificationService,
		indodaxClient:       indodaxClient,
		log:                 logger.GetLogger(),
		wsClients:           make(map[string]*indodax.PrivateWSClient),
		done:                make(chan struct{}),
	}
}

// SetBuyFilledCallback sets the callback for buy order fills
func (m *OrderMonitor) SetBuyFilledCallback(cb func(trade *model.Trade, filledAmount float64)) {
	m.onBuyFilled = cb
}

// SetSellFilledCallback sets the callback for sell order fills
func (m *OrderMonitor) SetSellFilledCallback(cb func(trade *model.Trade, filledAmount float64, avgPrice float64)) {
	m.onSellFilled = cb
}

// AddOrderHandler adds a generic order update handler
func (m *OrderMonitor) AddOrderHandler(handler func(userID string, order *indodax.OrderUpdate)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.orderHandlers = append(m.orderHandlers, handler)
}

// SubscribeUserOrders subscribes to order updates for a specific user
func (m *OrderMonitor) SubscribeUserOrders(ctx context.Context, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if already subscribed
	if _, exists := m.wsClients[userID]; exists {
		return nil
	}

	// Get user's API credentials
	credentials, err := m.apiKeyService.GetDecrypted(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get API credentials: %w", err)
	}

	// Create Private WebSocket client
	wsClient := indodax.NewPrivateWSClient(m.indodaxClient, credentials.Key, credentials.Secret)

	// Set order update handler
	wsClient.SetOrderUpdateHandler(func(order *indodax.OrderUpdate) {
		m.handleOrderUpdate(userID, order)
	})

	// Set error handler
	wsClient.SetErrorHandler(func(err error) {
		m.log.Errorf("Private WS error for user %s: %v", userID, err)
	})

	// Connect
	if err := wsClient.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect private websocket: %w", err)
	}

	m.wsClients[userID] = wsClient
	m.log.Infof("Subscribed to order updates for user %s", userID)

	return nil
}

// UnsubscribeUserOrders unsubscribes from order updates for a user
func (m *OrderMonitor) UnsubscribeUserOrders(userID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if wsClient, exists := m.wsClients[userID]; exists {
		wsClient.Close()
		delete(m.wsClients, userID)
		m.log.Infof("Unsubscribed from order updates for user %s", userID)
	}
}

// handleOrderUpdate processes order update events from WebSocket
func (m *OrderMonitor) handleOrderUpdate(userID string, order *indodax.OrderUpdate) {
	ctx := context.Background()

	// Notify generic handlers
	m.mu.RLock()
	handlers := m.orderHandlers
	m.mu.RUnlock()
	for _, h := range handlers {
		h(userID, order)
	}

	// Parse order ID
	indodaxOrderID := order.OrderID

	// Find internal order record
	internalOrder, err := m.orderRepo.GetByOrderID(ctx, indodaxOrderID)
	if err != nil {
		// Optimization: if not found, it might be an order placed before this system restart
		// or placed externally. We'll ignore it.
		return
	}

	// Update order status in repository
	m.orderRepo.UpdateStatus(ctx, internalOrder.ID, strings.ToLower(order.Status))

	// Notify via WebSocket
	m.notificationService.NotifyOrderUpdate(ctx, userID, internalOrder)

	// Lifecycle dispatch based on ParentType
	if internalOrder.ParentType == "trade" {
		trade, err := m.tradeRepo.GetByID(ctx, internalOrder.ParentID)
		if err == nil {
			m.handleCopilotOrder(trade, internalOrder, order)
		}
	}

	// For Bots (Market Maker / Pump Hunter), the generic handlers registered
	// via AddOrderHandler will take care of it.
}

func (m *OrderMonitor) handleCopilotOrder(trade *model.Trade, internalOrder *model.Order, update *indodax.OrderUpdate) {
	filledAmount, _ := strconv.ParseFloat(update.ExecutedQty, 64)

	switch strings.ToLower(update.Status) {
	case "filled":
		if trade.BuyOrderID == update.OrderID {
			m.handleBuyOrderFilled(trade, filledAmount)
		} else if trade.SellOrderID == update.OrderID {
			avgPrice, _ := strconv.ParseFloat(update.Price, 64)
			m.handleSellOrderFilled(trade, filledAmount, avgPrice)
		}
	}
}

// handleBuyOrderFilled handles buy order fill events
func (m *OrderMonitor) handleBuyOrderFilled(trade *model.Trade, filledAmount float64) {
	ctx := context.Background()

	m.log.Infof("Buy order filled: TradeID=%d, Amount=%.8f", trade.ID, filledAmount)

	// Update trade status
	oldStatus := trade.Status
	trade.Status = model.TradeStatusFilled
	trade.BuyFilledAmount = filledAmount
	now := time.Now()
	trade.BuyFilledAt = &now

	if err := m.tradeRepo.Update(ctx, trade, oldStatus); err != nil {
		m.log.Errorf("Failed to update trade status: %v", err)
		return
	}

	// Trigger callback for auto-sell
	if m.onBuyFilled != nil {
		m.onBuyFilled(trade, filledAmount)
	}
}

// handleSellOrderFilled handles sell order fill events
func (m *OrderMonitor) handleSellOrderFilled(trade *model.Trade, filledAmount float64, avgPrice float64) {
	ctx := context.Background()

	m.log.Infof("Sell order filled: TradeID=%d, Amount=%.8f, Price=%.2f", trade.ID, filledAmount, avgPrice)

	// Calculate profit
	sellRevenue := filledAmount * avgPrice
	buySpent := trade.BuyFilledAmount * trade.BuyPrice
	profitIDR := sellRevenue - buySpent
	profitPercent := (profitIDR / buySpent) * 100

	// Update trade status
	oldStatus := trade.Status
	trade.Status = model.TradeStatusCompleted
	trade.SellFilledAmount = filledAmount
	now := time.Now()
	trade.SellFilledAt = &now
	trade.ProfitIDR = profitIDR
	trade.ProfitPercent = profitPercent

	if err := m.tradeRepo.Update(ctx, trade, oldStatus); err != nil {
		m.log.Errorf("Failed to update trade status: %v", err)
		return
	}

	// Trigger callback
	if m.onSellFilled != nil {
		m.onSellFilled(trade, filledAmount, avgPrice)
	}

	m.log.Infof("Trade completed: TradeID=%d, Profit=%.2f IDR (%.2f%%)",
		trade.ID, profitIDR, profitPercent)
}

// handleOrderCancelled handles order cancellation events
func (m *OrderMonitor) handleOrderCancelled(trade *model.Trade, orderID string) {
	m.log.Infof("Order cancelled: TradeID=%d, OrderID=%s", trade.ID, orderID)
}

// Stop stops the order monitor
func (m *OrderMonitor) Stop() {
	close(m.done)

	m.mu.Lock()
	defer m.mu.Unlock()

	// Close all WebSocket connections
	for userID, wsClient := range m.wsClients {
		wsClient.Close()
		m.log.Infof("Closed WebSocket for user %s", userID)
	}

	m.wsClients = make(map[string]*indodax.PrivateWSClient)
}
