package market

import (
	"tuyul/backend/internal/model"
)

// CalculateGap calculates the bid-ask gap and spread
// Gap is set to 0 if:
// - Current price is under 20, OR
// - Volume 24h is 0 or less than 1 billion IDR
func CalculateGap(coin *model.Coin) {
	const MIN_VOLUME_FOR_GAP = 1_000_000_000 // 1 billion IDR

	// Skip gap calculation if price is too low or volume is insufficient
	if coin.CurrentPrice < 20 || coin.VolumeIDR == 0 || coin.VolumeIDR < MIN_VOLUME_FOR_GAP {
		coin.GapPercentage = 0
		coin.Spread = 0
		return
	}

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
