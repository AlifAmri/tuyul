package model

import "time"

type BotSummary struct {
	BotID          int64      `json:"bot_id"`
	Type           string     `json:"type"`
	Status         string     `json:"status"`
	Pair           string     `json:"pair,omitempty"`
	TotalTrades    int        `json:"total_trades"`
	WinningTrades  int        `json:"winning_trades"`
	LosingTrades   int        `json:"losing_trades"`
	WinRate        float64    `json:"win_rate"`
	TotalProfitIDR float64    `json:"total_profit_idr"`
	AverageProfit  float64    `json:"average_profit"`
	Uptime         string     `json:"uptime"`
	LastTradeAt    *time.Time `json:"last_trade_at,omitempty"`
	// Market data
	BuyPrice      float64 `json:"buy_price,omitempty"`      // Current bid price
	SellPrice     float64 `json:"sell_price,omitempty"`     // Current ask price
	SpreadPercent float64 `json:"spread_percent,omitempty"` // Current spread percentage
}
