package market

import "tuyul/backend/internal/model"

// CalculateVolatility calculates the 1-minute volatility percentage
func CalculateVolatility(coin *model.Coin) float64 {
	tf := coin.Timeframes.OneMinute
	if tf.Open == 0 {
		return 0
	}

	// (High - Low) / Open * 100
	volatility := ((tf.High - tf.Low) / tf.Open) * 100
	return volatility
}
