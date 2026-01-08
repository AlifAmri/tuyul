package service

import (
	"context"

	"tuyul/backend/pkg/indodax"
)

// TradeClient defines the interface for executing trades
type TradeClient interface {
	GetInfo(ctx context.Context) (*indodax.GetInfoReturn, error)
	Trade(ctx context.Context, side, pair string, price, amount float64, type_ string) (*indodax.TradeReturn, error)
	CancelOrder(ctx context.Context, pair string, orderID string, side string) error
	GetOrder(ctx context.Context, pair string, orderID string) (*indodax.OrderInfo, error)
}
