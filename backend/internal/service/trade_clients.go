package service

import (
	"context"
	"fmt"
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

func (c *LiveTradeClient) Trade(ctx context.Context, side, pair string, price, amount float64, type_ string) (*indodax.TradeReturn, error) {
	req := indodax.TradeRequest{
		Pair:      pair,
		Type:      side,
		Price:     price,
		Coin:      amount,
		OrderType: type_,
	}
	return c.client.Trade(ctx, c.apiKey, c.apiSecret, req)
}

func (c *LiveTradeClient) CancelOrder(ctx context.Context, pair string, orderID string, side string) error {
	var id int64
	fmt.Sscanf(orderID, "%d", &id)
	_, err := c.client.CancelOrder(ctx, c.apiKey, c.apiSecret, pair, id, side)
	return err
}

func (c *LiveTradeClient) GetOrder(ctx context.Context, pair string, orderID string) (*indodax.OrderInfo, error) {
	var id int64
	fmt.Sscanf(orderID, "%d", &id)
	return c.client.GetOrder(ctx, c.apiKey, c.apiSecret, pair, id)
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
	balance := make(map[string]string)
	for k, v := range c.balances {
		balance[k] = fmt.Sprintf("%f", v)
	}

	return &indodax.GetInfoReturn{
		Balance: balance,
	}, nil
}

func (c *PaperTradeClient) Trade(ctx context.Context, side, pair string, price, amount float64, type_ string) (*indodax.TradeReturn, error) {
	// Generate unique paper order ID
	orderID := time.Now().UnixNano()

	// Simulate successful placement
	go func() {
		time.Sleep(5 * time.Second)
		if c.onFilled != nil {
			c.onFilled(&model.Order{
				OrderID: fmt.Sprintf("%d", orderID),
				Side:    side,
				Pair:    pair,
				Price:   price,
				Amount:  amount,
				Status:  "filled",
			})
		}
	}()

	return &indodax.TradeReturn{
		OrderID: orderID,
	}, nil
}

func (c *PaperTradeClient) CancelOrder(ctx context.Context, pair string, orderID string, side string) error {
	// In paper trading, cancelling a pending order is immediate
	return nil
}

func (c *PaperTradeClient) GetOrder(ctx context.Context, pair string, orderID string) (*indodax.OrderInfo, error) {
	return &indodax.OrderInfo{}, nil
}
