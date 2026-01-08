package market

import (
	"tuyul/backend/internal/model"
)

// CalculatePumpScore calculates the pump score based on multi-timeframe weighted average
func CalculatePumpScore(coin *model.Coin) float64 {
	// Formula:
	// Pump Score = (1m_pct × 1m_trx × 0.20) +
	//              (5m_pct × 5m_trx × 0.40) +
	//              (15m_pct × 15m_trx × 0.30) +
	//              (30m_pct × 30m_trx × 0.10)

	score1m := calculateTimeframeScore(coin.CurrentPrice, coin.Timeframes.OneMinute, 0.20)
	score5m := calculateTimeframeScore(coin.CurrentPrice, coin.Timeframes.FiveMinute, 0.40)
	score15m := calculateTimeframeScore(coin.CurrentPrice, coin.Timeframes.FifteenMin, 0.30)
	score30m := calculateTimeframeScore(coin.CurrentPrice, coin.Timeframes.ThirtyMin, 0.10)

	return score1m + score5m + score15m + score30m
}

func calculateTimeframeScore(currentPrice float64, tf model.TimeframeData, weight float64) float64 {
	if tf.Open == 0 {
		return 0
	}

	// Price Change %
	changePct := ((currentPrice - tf.Open) / tf.Open) * 100

	// Score
	return changePct * float64(tf.Trx) * weight
}
