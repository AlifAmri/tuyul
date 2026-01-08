package model

import (
	"encoding/json"
	"time"
)

// Bot status constants
const (
	BotStatusStopped  = "stopped"
	BotStatusStarting = "starting"
	BotStatusRunning  = "running"
	BotStatusError    = "error"
)

// Bot type constants
const (
	BotTypeMarketMaker = "market_maker"
	BotTypePumpHunter  = "pump_hunter"
)

// BotConfig represents a trading bot configuration
type BotConfig struct {
	ID     int64  `json:"id"`
	UserID string `json:"user_id"`
	Name   string `json:"name"`
	Type   string `json:"type"` // market_maker, pump_hunter
	Pair   string `json:"pair"`

	// Trading mode
	IsPaperTrading bool   `json:"is_paper_trading"`
	APIKeyID       *int64 `json:"api_key_id,omitempty"` // null for paper trading

	// Market Maker parameters
	InitialBalanceIDR          float64 `json:"initial_balance_idr"`
	OrderSizeIDR               float64 `json:"order_size_idr"`
	MinGapPercent              float64 `json:"min_gap_percent"`
	RepositionThresholdPercent float64 `json:"reposition_threshold_percent"`
	MaxLossIDR                 float64 `json:"max_loss_idr"`

	// Virtual balance (JSONB in DB)
	Balances map[string]float64 `json:"balances"`

	// Pump Hunter parameters
	EntryRules     *PumpHunterEntryRules     `json:"entry_rules,omitempty"`
	ExitRules      *PumpHunterExitRules      `json:"exit_rules,omitempty"`
	RiskManagement *PumpHunterRiskManagement `json:"risk_management,omitempty"`

	// Statistics
	TotalTrades    int     `json:"total_trades"`
	WinningTrades  int     `json:"winning_trades"`
	TotalProfitIDR float64 `json:"total_profit_idr"`

	// Status
	Status       string  `json:"status"` // stopped, starting, running, error
	ErrorMessage *string `json:"error_message,omitempty"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// BotConfigRequest represents the request to create/update a bot
type BotConfigRequest struct {
	Name           string `json:"name" binding:"required"`
	Type           string `json:"type" binding:"required,oneof=market_maker pump_hunter"`
	Pair           string `json:"pair" binding:"required_if=Type market_maker"` // Only required for market_maker
	IsPaperTrading bool   `json:"is_paper_trading"`
	APIKeyID       *int64 `json:"api_key_id"`

	// Market Maker parameters
	InitialBalanceIDR          float64 `json:"initial_balance_idr" binding:"required,gt=0"`
	OrderSizeIDR               float64 `json:"order_size_idr" binding:"required,gt=0"`
	MinGapPercent              float64 `json:"min_gap_percent" binding:"required,gte=0"`
	RepositionThresholdPercent float64 `json:"reposition_threshold_percent" binding:"required,gte=0"`
	MaxLossIDR                 float64 `json:"max_loss_idr" binding:"required_if=Type market_maker"`

	// Pump Hunter parameters
	EntryRules     *PumpHunterEntryRules     `json:"entry_rules"`
	ExitRules      *PumpHunterExitRules      `json:"exit_rules"`
	RiskManagement *PumpHunterRiskManagement `json:"risk_management"`
}

type PumpHunterEntryRules struct {
	MinPumpScore          float64  `json:"min_pump_score"`
	MinTimeframesPositive int      `json:"min_timeframes_positive"`
	Min24hVolumeIDR       float64  `json:"min_24h_volume_idr"`
	MinPriceIDR           float64  `json:"min_price_idr"`
	ExcludedPairs         []string `json:"excluded_pairs"`
	AllowedPairs          []string `json:"allowed_pairs"`
}

type PumpHunterExitRules struct {
	TargetProfitPercent    float64 `json:"target_profit_percent"`
	StopLossPercent        float64 `json:"stop_loss_percent"`
	TrailingStopEnabled    bool    `json:"trailing_stop_enabled"`
	TrailingStopPercent    float64 `json:"trailing_stop_percent"`
	MaxHoldMinutes         int     `json:"max_hold_minutes"`
	ExitOnPumpScoreDrop    bool    `json:"exit_on_pump_score_drop"`
	PumpScoreDropThreshold float64 `json:"pump_score_drop_threshold"`
}

type PumpHunterRiskManagement struct {
	MaxPositionIDR           float64 `json:"max_position_idr"`
	MaxConcurrentPositions   int     `json:"max_concurrent_positions"`
	DailyLossLimitIDR        float64 `json:"daily_loss_limit_idr"`
	CooldownAfterLossMinutes int     `json:"cooldown_after_loss_minutes"`
	MinBalanceIDR            float64 `json:"min_balance_idr"`
}

// Order represents a trading order
type Order struct {
	ID           int64   `json:"id"`
	UserID       string  `json:"user_id"`
	ParentID     int64   `json:"parent_id"`   // ID of Trade, Position, or BotConfig
	ParentType   string  `json:"parent_type"` // "trade", "position", "bot"
	OrderID      string  `json:"order_id"`    // Indodax order ID or paper order ID
	Pair         string  `json:"pair"`
	Side         string  `json:"side"`   // buy, sell
	Status       string  `json:"status"` // open, filled, cancelled
	Price        float64 `json:"price"`
	Amount       float64 `json:"amount"`
	FilledAmount float64 `json:"filled_amount"`
	IsPaperTrade bool    `json:"is_paper_trade"`

	// Timestamps
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	FilledAt  *time.Time `json:"filled_at,omitempty"`
}

// ToJSON converts balances map to JSON string
func (b *BotConfig) ToJSON() (string, error) {
	data, err := json.Marshal(b.Balances)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// FromJSON parses JSON string to balances map
func (b *BotConfig) FromJSON(jsonStr string) error {
	return json.Unmarshal([]byte(jsonStr), &b.Balances)
}

// GetBalance returns the balance for a specific currency
func (b *BotConfig) GetBalance(currency string) float64 {
	if b.Balances == nil {
		return 0
	}
	return b.Balances[currency]
}

// SetBalance sets the balance for a specific currency
func (b *BotConfig) SetBalance(currency string, amount float64) {
	if b.Balances == nil {
		b.Balances = make(map[string]float64)
	}
	b.Balances[currency] = amount
}

// WinRate calculates the win rate percentage
func (b *BotConfig) WinRate() float64 {
	if b.TotalTrades == 0 {
		return 0
	}
	return float64(b.WinningTrades) / float64(b.TotalTrades) * 100
}
