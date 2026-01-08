package model

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// Coin Model
type Coin struct {
	// Basic Info
	PairID        string `json:"pair_id"`        // e.g., "btcidr"
	BaseCurrency  string `json:"base_currency"`  // e.g., "btc"
	QuoteCurrency string `json:"quote_currency"` // e.g., "idr"

	// Current Market Data
	CurrentPrice float64 `json:"current_price"` // Last traded price
	High24h      float64 `json:"high_24h"`
	Low24h       float64 `json:"low_24h"`
	Open24h      float64 `json:"open_24h"`
	Volume24h    float64 `json:"volume_24h"` // Base currency
	VolumeIDR    float64 `json:"volume_idr"` // In IDR
	Change24h    float64 `json:"change_24h"` // Percentage

	// Orderbook Data
	BestBid   float64 `json:"best_bid"`
	BestAsk   float64 `json:"best_ask"`
	BidVolume float64 `json:"bid_volume"`
	AskVolume float64 `json:"ask_volume"`

	// Gap Analysis (calculated)
	GapPercentage float64 `json:"gap_percentage"` // ((Ask - Bid) / Bid) * 100
	Spread        float64 `json:"spread"`         // Ask - Bid (absolute)

	// Timeframe Data (OHLC + Transaction Count)
	Timeframes Timeframes           `json:"timeframes"`
	LastReset  map[string]time.Time `json:"last_reset"`

	// Pump Score (calculated)
	PumpScore float64 `json:"pump_score"` // 0 to infinity

	// Volatility (calculated)
	Volatility1m float64 `json:"volatility_1m"` // Percentage

	// Metadata
	LastUpdate time.Time `json:"last_update"`
}

// Timeframes Structure
type Timeframes struct {
	OneMinute  TimeframeData `json:"1m"`
	FiveMinute TimeframeData `json:"5m"`
	FifteenMin TimeframeData `json:"15m"`
	ThirtyMin  TimeframeData `json:"30m"`
}

type TimeframeData struct {
	Open float64 `json:"open"` // Opening price when timeframe started
	High float64 `json:"high"` // Highest price in timeframe
	Low  float64 `json:"low"`  // Lowest price in timeframe
	Trx  int     `json:"trx"`  // Transaction count in timeframe
}

// Helper to Marshal Coin to Map for Redis
func (c *Coin) ToMap() (map[string]interface{}, error) {
	timeframesJSON, err := json.Marshal(c.Timeframes)
	if err != nil {
		return nil, err
	}
	lastResetJSON, err := json.Marshal(c.LastReset)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"pair_id":        c.PairID,
		"base_currency":  c.BaseCurrency,
		"quote_currency": c.QuoteCurrency,
		"current_price":  c.CurrentPrice,
		"high_24h":       c.High24h,
		"low_24h":        c.Low24h,
		"open_24h":       c.Open24h,
		"volume_24h":     c.Volume24h,
		"volume_idr":     c.VolumeIDR,
		"change_24h":     c.Change24h,
		"best_bid":       c.BestBid,
		"best_ask":       c.BestAsk,
		"bid_volume":     c.BidVolume,
		"ask_volume":     c.AskVolume,
		"gap_percentage": c.GapPercentage,
		"spread":         c.Spread,
		"timeframes":     string(timeframesJSON),
		"last_reset":     string(lastResetJSON),
		"pump_score":     c.PumpScore,
		"volatility_1m":  c.Volatility1m,
		"last_update":    c.LastUpdate.UnixMilli(),
	}, nil
}

// FromMap to reconstruct Coin from Redis
func CoinFromMap(data map[string]string) (*Coin, error) {
	c := &Coin{}
	c.PairID = data["pair_id"]
	c.BaseCurrency = data["base_currency"]
	c.QuoteCurrency = data["quote_currency"]

	c.CurrentPrice, _ = strconv.ParseFloat(data["current_price"], 64)
	c.High24h, _ = strconv.ParseFloat(data["high_24h"], 64)
	c.Low24h, _ = strconv.ParseFloat(data["low_24h"], 64)
	c.Open24h, _ = strconv.ParseFloat(data["open_24h"], 64)
	c.Volume24h, _ = strconv.ParseFloat(data["volume_24h"], 64)
	c.VolumeIDR, _ = strconv.ParseFloat(data["volume_idr"], 64)
	c.Change24h, _ = strconv.ParseFloat(data["change_24h"], 64)

	c.BestBid, _ = strconv.ParseFloat(data["best_bid"], 64)
	c.BestAsk, _ = strconv.ParseFloat(data["best_ask"], 64)
	c.BidVolume, _ = strconv.ParseFloat(data["bid_volume"], 64)
	c.AskVolume, _ = strconv.ParseFloat(data["ask_volume"], 64)

	c.GapPercentage, _ = strconv.ParseFloat(data["gap_percentage"], 64)
	c.Spread, _ = strconv.ParseFloat(data["spread"], 64)
	c.PumpScore, _ = strconv.ParseFloat(data["pump_score"], 64)
	c.Volatility1m, _ = strconv.ParseFloat(data["volatility_1m"], 64)

	if lu, ok := data["last_update"]; ok {
		ms, _ := strconv.ParseInt(lu, 10, 64)
		c.LastUpdate = time.UnixMilli(ms)
	}

	if tf, ok := data["timeframes"]; ok {
		json.Unmarshal([]byte(tf), &c.Timeframes)
	}

	if lr, ok := data["last_reset"]; ok {
		json.Unmarshal([]byte(lr), &c.LastReset)
	}

	return c, nil
}

// Ensure PairID is lowercase
func NormalizePairID(pairID string) string {
	return strings.ToLower(pairID)
}
