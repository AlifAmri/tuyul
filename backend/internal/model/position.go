package model

import (
	"time"
)

// Position status constants
const (
	PositionStatusPending = "pending"  // Order placed, waiting for false pump check
	PositionStatusBuying  = "buying"
	PositionStatusOpen    = "open"
	PositionStatusSelling = "selling"
	PositionStatusClosed  = "closed"
	PositionStatusError   = "error"
)

// Position represents a single trade position opened by Pump Hunter bot
type Position struct {
	ID          int64  `json:"id"`
	BotConfigID int64  `json:"bot_config_id"`
	UserID      string `json:"user_id"`

	// Position details
	Pair   string `json:"pair"`
	Status string `json:"status"` // buying, open, selling, closed, error

	// Entry
	InternalEntryOrderID int64     `json:"internal_entry_order_id"`
	EntryPrice           float64   `json:"entry_price"`
	EntryQuantity        float64   `json:"entry_quantity"`
	EntryAmountIDR       float64   `json:"entry_amount_idr"`
	EntryOrderID         string    `json:"entry_order_id"` // Indodax ID
	EntryOrderType       string    `json:"entry_order_type,omitempty"` // "market" or "limit"
	EntryPumpScore       float64   `json:"entry_pump_score"`
	EntryTrxCount1m      int       `json:"entry_trx_count_1m,omitempty"` // Transaction count at entry (1m)
	EntryAt              time.Time `json:"entry_at"`
	OrderPlacedAt        time.Time `json:"order_placed_at,omitempty"` // When buy order was placed

	// Exit
	InternalExitOrderID int64      `json:"internal_exit_order_id"`
	ExitPrice           *float64   `json:"exit_price,omitempty"`
	ExitQuantity        *float64   `json:"exit_quantity,omitempty"`
	ExitAmountIDR       *float64   `json:"exit_amount_idr,omitempty"`
	ExitOrderID         string     `json:"exit_order_id,omitempty"` // Indodax ID
	ExitAt              *time.Time `json:"exit_at,omitempty"`

	// Price tracking
	HighestPrice float64 `json:"highest_price"`
	LowestPrice  float64 `json:"lowest_price"`
	
	// ATH tracking (for target profit = 1%)
	LastPriceCheck time.Time `json:"last_price_check,omitempty"` // Last 1-minute check
	MinutesBelowATH int      `json:"minutes_below_ath,omitempty"` // Consecutive minutes below ATH
	
	// Exit confirmation tracking (2-minute confirmation for all exits)
	ExitSignalReason string `json:"exit_signal_reason,omitempty"` // First exit signal reason
	ExitSignalMinute  int    `json:"exit_signal_minute,omitempty"` // Minute when first signal detected
	ExitConfirmCount  int    `json:"exit_confirm_count,omitempty"` // Consecutive confirmations (need 2)

	// Profit
	ProfitIDR     *float64 `json:"profit_idr,omitempty"`
	ProfitPercent *float64 `json:"profit_percent,omitempty"`

	// Close reason
	CloseReason string `json:"close_reason,omitempty"`

	// Paper trade flag
	IsPaperTrade bool `json:"is_paper_trade"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
