package util

import (
	"tuyul/backend/pkg/logger"
)

// ValidateAndNormalizeBalance validates and normalizes a single balance value
// Returns the normalized balance value
func ValidateAndNormalizeBalance(
	balance float64,
	currency string,
	maxReasonable float64,
	log *logger.Logger,
) float64 {
	// Negative balance check
	if balance < 0 {
		log.Warnf("Balance for %s was negative (%.8f), resetting to 0", currency, balance)
		return 0
	}

	// Corruption check: unreasonably large balance
	if balance > maxReasonable {
		log.Errorf("Balance for %s is unreasonably large (%.8f), resetting to 0", currency, balance)
		return 0
	}

	// Floating point cleanup for coins (not IDR)
	// Clean up tiny balances that are likely floating point errors
	if currency != "idr" && balance > 0 && balance < TinyBalanceThreshold {
		log.Debugf("Cleaning up tiny %s balance (%.18f) - setting to 0", currency, balance)
		return 0
	}

	return balance
}

// ValidateAndNormalizeBalances validates and normalizes a balances map
// Ensures all required balances exist and are valid
func ValidateAndNormalizeBalances(
	balances map[string]float64,
	requiredCurrencies []string,
	initialIDR float64,
	log *logger.Logger,
) map[string]float64 {
	// Initialize balances map if nil
	if balances == nil {
		balances = make(map[string]float64)
		log.Warnf("Balances map was nil, initializing")
	}

	// Validate and normalize each required currency
	for _, currency := range requiredCurrencies {
		currentBalance, exists := balances[currency]
		if !exists {
			// Initialize missing currency
			if currency == "idr" {
				currentBalance = initialIDR
			} else {
				currentBalance = 0
			}
			balances[currency] = currentBalance
			log.Debugf("Initialized missing %s balance to %.8f", currency, currentBalance)
		} else {
			// Validate and normalize existing balance
			maxReasonable := MaxReasonableIDRBalance
			if currency != "idr" {
				maxReasonable = MaxReasonableCoinAmount
			}
			normalized := ValidateAndNormalizeBalance(currentBalance, currency, maxReasonable, log)
			balances[currency] = normalized
		}
	}

	return balances
}
