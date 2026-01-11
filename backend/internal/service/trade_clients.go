package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"tuyul/backend/internal/model"
	"tuyul/backend/pkg/indodax"
)

// LiveTradeClient implements TradeClient for real trading
type LiveTradeClient struct {
	client    *indodax.Client
	apiKey    string
	apiSecret string
}

func NewLiveTradeClient(client *indodax.Client, apiKey, apiSecret string) *LiveTradeClient {
	return &LiveTradeClient{
		client:    client,
		apiKey:    apiKey,
		apiSecret: apiSecret,
	}
}

func (c *LiveTradeClient) GetInfo(ctx context.Context) (*indodax.GetInfoReturn, error) {
	return c.client.GetInfo(ctx, c.apiKey, c.apiSecret)
}

func (c *LiveTradeClient) Trade(ctx context.Context, side, pair string, price, amount float64, type_ string, clientOrderID string) (*indodax.TradeReturn, error) {
	// Convert pair format from internal format (e.g., "cstidr") to Indodax format (e.g., "cst_idr")
	indodaxPair := convertPairToIndodaxFormat(pair)
	
	// If no clientOrderID provided, generate one (fallback)
	// Format: {pair}-{side}-{timestamp}
	if clientOrderID == "" {
		clientOrderID = fmt.Sprintf("%s-%s-%d", pair, strings.ToLower(side), time.Now().UnixMilli())
	}
	
	// Handle market vs limit orders according to Indodax API:
	// - Market BUY: use IDR amount (amount parameter contains IDR), no price
	// - Market SELL: use Coin amount (amount parameter contains coin), no price
	// - Limit BUY: use Coin amount (amount parameter contains coin), with price
	// - Limit SELL: use Coin amount (amount parameter contains coin), with price
	req := indodax.TradeRequest{
		Pair:          indodaxPair,
		Type:          side,
		OrderType:     type_,
		ClientOrderID: clientOrderID,
	}
	
	if type_ == "market" {
		// Market orders: no price parameter
		req.Price = 0
		if side == "buy" {
			// Market buy: use IDR amount
			req.IDR = amount
			req.Coin = 0
		} else {
			// Market sell: use coin amount
			req.Coin = amount
			req.IDR = 0
		}
	} else {
		// Limit orders: use price and coin amount
		req.Price = price
		req.Coin = amount
		req.IDR = 0
	}
	
	fmt.Printf("[TRADE_DEBUG] Sending trade request with ClientOrderID: %s (pair=%s, side=%s, type=%s, price=%.2f, coin=%.8f, idr=%.2f)\n", 
		clientOrderID, indodaxPair, side, type_, req.Price, req.Coin, req.IDR)
	
	result, err := c.client.Trade(ctx, c.apiKey, c.apiSecret, req)
	if err != nil {
		return nil, err
	}
	
	fmt.Printf("[TRADE_DEBUG] Indodax response - OrderID: %d, ClientOrderID: %s\n", 
		result.OrderID, result.ClientOrderID)
	
	return result, nil
}

// convertPairToIndodaxFormat converts internal pair format to Indodax format
// Internal: "cstidr" -> Indodax: "cst_idr"
// Internal: "btc_idr" -> Indodax: "btc_idr" (already correct)
func convertPairToIndodaxFormat(pair string) string {
	// If already has underscore, return as is
	if strings.Contains(pair, "_") {
		return pair
	}
	// Convert "cstidr" to "cst_idr"
	if strings.HasSuffix(pair, "idr") {
		base := strings.TrimSuffix(pair, "idr")
		return base + "_idr"
	}
	// If format is unknown, return as is (let Indodax handle the error)
	return pair
}

func (c *LiveTradeClient) CancelOrder(ctx context.Context, pair string, orderID string, side string) error {
	// Convert pair format for Indodax
	indodaxPair := convertPairToIndodaxFormat(pair)
	
	// Try to cancel by ClientOrderID first (orderID format: "cstidr-buy-1767968961234")
	// If it's a numeric ID (old format), use numeric cancel
	var id int64
	_, err := fmt.Sscanf(orderID, "%d", &id)
	if err == nil {
		// Numeric ID, use old cancelOrder method
		_, err := c.client.CancelOrder(ctx, c.apiKey, c.apiSecret, indodaxPair, id, side)
		return err
	}
	
	// ClientOrderID format, use new cancelByClientOrderId method
	_, err = c.client.CancelByClientOrderID(ctx, c.apiKey, c.apiSecret, indodaxPair, orderID, side)
	return err
}

func (c *LiveTradeClient) GetOrder(ctx context.Context, pair string, orderID string) (*indodax.OrderInfo, error) {
	// Try to parse as numeric ID first (old format)
	var id int64
	_, err := fmt.Sscanf(orderID, "%d", &id)
	if err == nil {
		// Numeric ID, use getOrder method
		indodaxPair := convertPairToIndodaxFormat(pair)
		return c.client.GetOrder(ctx, c.apiKey, c.apiSecret, indodaxPair, id)
	}
	
	// ClientOrderID format, use getOrderByClientOrderId method
	return c.client.GetOrderByClientOrderID(ctx, c.apiKey, c.apiSecret, orderID)
}

// PaperTradeClient implements TradeClient for simulated trading
type PaperTradeClient struct {
	balances map[string]float64
	onFilled func(order *model.Order)
}

func NewPaperTradeClient(balances map[string]float64, onFilled func(order *model.Order)) *PaperTradeClient {
	return &PaperTradeClient{
		balances: balances,
		onFilled: onFilled,
	}
}

func (c *PaperTradeClient) GetInfo(ctx context.Context) (*indodax.GetInfoReturn, error) {
	// Simulate GetInfo from virtual balances
	balance := make(map[string]indodax.BalanceValue)
	for k, v := range c.balances {
		balance[k] = indodax.BalanceValue(fmt.Sprintf("%.8f", v))
	}

	return &indodax.GetInfoReturn{
		Balance: balance,
	}, nil
}

func (c *PaperTradeClient) Trade(ctx context.Context, side, pair string, price, amount float64, type_ string, clientOrderID string) (*indodax.TradeReturn, error) {
	// Generate unique paper order ID
	orderID := time.Now().UnixNano()
	
	// If no clientOrderID provided, generate one for paper trading
	if clientOrderID == "" {
		clientOrderID = fmt.Sprintf("paper-%s-%s-%d", pair, strings.ToLower(side), time.Now().UnixMilli())
	}

	// Simulate successful placement
	go func() {
		time.Sleep(5 * time.Second)
		if c.onFilled != nil {
			c.onFilled(&model.Order{
				OrderID: clientOrderID, // Use clientOrderID for consistency
				Side:    side,
				Pair:    pair,
				Price:   price,
				Amount:  amount,
				Status:  "filled",
			})
		}
	}()

	return &indodax.TradeReturn{
		OrderID:       orderID,
		ClientOrderID: clientOrderID,
	}, nil
}

func (c *PaperTradeClient) CancelOrder(ctx context.Context, pair string, orderID string, side string) error {
	// In paper trading, cancelling a pending order is immediate
	return nil
}

func (c *PaperTradeClient) GetOrder(ctx context.Context, pair string, orderID string) (*indodax.OrderInfo, error) {
	return &indodax.OrderInfo{}, nil
}
