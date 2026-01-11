package util

import (
	"fmt"
	"tuyul/backend/pkg/indodax"
	"tuyul/backend/pkg/logger"
)

// OrderValidationResult contains validation results
type OrderValidationResult struct {
	Valid      bool
	Amount     float64
	OrderValue float64
	Reason     string
}

// ValidateOrderAmount validates order amount against exchange minimums
// Returns validation result with adjusted amount if valid
func ValidateOrderAmount(
	botID int64,
	amount float64,
	price float64,
	pairInfo indodax.Pair,
	volumePrecision int,
	baseCurrency string,
	log *logger.Logger,
) OrderValidationResult {
	// Round amount to precision
	roundedAmount := FloorToPrecision(amount, volumePrecision)
	orderValue := roundedAmount * price

	// Validate minimum coin amount (TradeMinTradedCurrency is the minimum coin amount, e.g., 0.001 BTC, 1.0 CST)
	if roundedAmount < pairInfo.TradeMinTradedCurrency {
		reason := fmt.Sprintf("Coin amount too small - %.8f < minimum %.8f",
			roundedAmount, pairInfo.TradeMinTradedCurrency)
		log.Debugf("Bot %d: %s", botID, reason)
		return OrderValidationResult{
			Valid:  false,
			Reason: reason,
		}
	}

	// Validate minimum order value in IDR (TradeMinBaseCurrency is the minimum IDR value)
	if orderValue < float64(pairInfo.TradeMinBaseCurrency) {
		reason := fmt.Sprintf("Order value too small - %.2f IDR < minimum %d IDR",
			orderValue, pairInfo.TradeMinBaseCurrency)
		log.Debugf("Bot %d: %s (amount=%.8f * price=%.2f)", botID, reason, roundedAmount, price)
		return OrderValidationResult{
			Valid:  false,
			Reason: reason,
		}
	}

	log.Debugf("Bot %d: Order validation passed - amount=%.8f %s (min=%.8f), value=%.2f IDR (min=%d)",
		botID, roundedAmount, baseCurrency, pairInfo.TradeMinTradedCurrency,
		orderValue, pairInfo.TradeMinBaseCurrency)

	return OrderValidationResult{
		Valid:      true,
		Amount:     roundedAmount,
		OrderValue: orderValue,
	}
}

// CalculateAvailableBalance calculates available balance after reserving minimum
// Returns available balance and whether there's enough to trade
func CalculateAvailableBalance(
	idrBalance float64,
	minBalanceReserve float64,
) (availableBalance float64, hasEnough bool) {
	if idrBalance <= minBalanceReserve {
		return 0, false
	}
	return idrBalance - minBalanceReserve, true
}

// CalculatePositionSize calculates position size with balance constraints
// Returns position size in IDR and whether calculation is valid
func CalculatePositionSize(
	maxPositionSize float64,
	availableBalance float64,
) (sizeIDR float64, valid bool) {
	if availableBalance <= 0 {
		return 0, false
	}

	sizeIDR = maxPositionSize
	if availableBalance < sizeIDR {
		sizeIDR = availableBalance
	}

	return sizeIDR, true
}
