package market

import (
	"tuyul/backend/internal/model"
)

// CalculateGap calculates the bid-ask gap and spread
func CalculateGap(coin *model.Coin) {
	if coin.BestBid > 0 {
		coin.GapPercentage = ((coin.BestAsk - coin.BestBid) / coin.BestBid) * 100
		coin.Spread = coin.BestAsk - coin.BestBid
	}
}

// ShouldIncludeInGapAnalysis checks if the coin meets liquidity requirements
func ShouldIncludeInGapAnalysis(coin *model.Coin) bool {
	const MIN_VOLUME_IDR = 10_000_000 // 10M IDR as per blueprint
	return coin.VolumeIDR >= MIN_VOLUME_IDR
}
