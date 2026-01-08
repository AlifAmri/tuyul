package model

import (
	"time"
)

// Trade status constants
const (
	TradeStatusPending   = "pending"   // Buy order placed, not filled
	TradeStatusFilled    = "filled"    // Buy order completely filled, auto-sell placed
	TradeStatusCompleted = "completed" // Sell order completely filled
	TradeStatusCancelled = "cancelled" // Buy order cancelled before filled
	TradeStatusStopped   = "stopped"   // Stop-loss triggered
	TradeStatusError     = "error"     // Something went wrong
)

// Trade model represents a single copilot trade
type Trade struct {
	ID     int64  `json:"id"`
	UserID string `json:"user_id"`
	Pair   string `json:"pair"`

	// Buy order
	InternalBuyOrderID int64      `json:"internal_buy_order_id"`
	BuyOrderID         string     `json:"buy_order_id"` // Indodax ID
	BuyPrice           float64    `json:"buy_price"`
	BuyAmount          float64    `json:"buy_amount"`
	BuyAmountIDR       float64    `json:"buy_amount_idr"`
	BuyFilledAmount    float64    `json:"buy_filled_amount"`
	BuyFilledAt        *time.Time `json:"buy_filled_at,omitempty"`

	// Sell order
	InternalSellOrderID int64      `json:"internal_sell_order_id"`
	SellOrderID         string     `json:"sell_order_id"` // Indodax ID
	SellPrice           float64    `json:"sell_price"`
	SellAmount          float64    `json:"sell_amount"`
	SellFilledAmount    float64    `json:"sell_filled_amount"`
	SellFilledAt        *time.Time `json:"sell_filled_at,omitempty"`

	// Parameters
	TargetProfit float64 `json:"target_profit"`
	StopLoss     float64 `json:"stop_loss"`

	// Profit
	ProfitIDR     float64 `json:"profit_idr"`
	ProfitPercent float64 `json:"profit_percent"`

	// Flags
	StopLossTriggered bool `json:"stop_loss_triggered"`
	ManualSell        bool `json:"manual_sell"`

	// Status
	Status       string `json:"status"` // pending, filled, completed, cancelled, stopped, error
	ErrorMessage string `json:"error_message,omitempty"`
	IsPaperTrade bool   `json:"is_paper_trade"`

	// Timestamps
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CancelledAt *time.Time `json:"cancelled_at,omitempty"`
}

// TradeRequest represents the payload to create a new trade
type TradeRequest struct {
	Pair         string  `json:"pair" binding:"required"`
	BuyingPrice  float64 `json:"buying_price" binding:"required,gt=0"`
	VolumeIDR    float64 `json:"volume_idr" binding:"required,gte=10000"`
	TargetProfit float64 `json:"target_profit" binding:"required,gt=0"`
	StopLoss     float64 `json:"stop_loss" binding:"required,gt=0"`
	IsPaperTrade bool    `json:"is_paper_trade"`
}
