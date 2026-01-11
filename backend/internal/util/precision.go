package util

import (
	"tuyul/backend/pkg/indodax"
)

// GetVolumePrecision returns the volume precision for a pair
// Uses VolumePrecision if valid, otherwise falls back to PriceRound, then default to 8
func GetVolumePrecision(pairInfo indodax.Pair) int {
	volumePrecision := pairInfo.VolumePrecision
	if volumePrecision == 0 {
		if pairInfo.PriceRound > 0 {
			volumePrecision = pairInfo.PriceRound
		} else {
			volumePrecision = 8 // Final fallback to 8 decimal places for crypto
		}
	}
	return volumePrecision
}

// GetTickSize returns the tick size (minimum price increment) for a pair
// Tries to get from marketDataService first, then falls back to price precision
type PriceIncrementGetter interface {
	GetPriceIncrement(pairID string) (float64, bool)
}

func GetTickSize(pair indodax.Pair, incrementGetter PriceIncrementGetter) float64 {
	// Try to get actual price increment from market data service first
	if incrementGetter != nil {
		if increment, ok := incrementGetter.GetPriceIncrement(pair.ID); ok && increment > 0 {
			return increment
		}
	}

	// Fallback: calculate from price precision
	// For IDR pairs (whole numbers), PricePrecision is typically 0, so tick = 1
	return 1.0 / Pow10(pair.PricePrecision)
}
