package util

// Trading safety thresholds
// These constants define maximum reasonable values to detect balance corruption
// and prevent invalid orders due to data corruption or calculation errors

const (
	// MaxReasonableCoinAmount is the maximum reasonable coin amount (1 million coins)
	// Used to detect balance corruption in coin balances
	MaxReasonableCoinAmount = 1000000.0

	// MaxReasonableIDRBalance is the maximum reasonable IDR balance (1 billion IDR)
	// Used to detect balance corruption in IDR balances
	MaxReasonableIDRBalance = 1000000000.0

	// MaxReasonableTotalValue is the maximum reasonable total order value (10 billion IDR)
	// Used to detect corruption in order value calculations
	MaxReasonableTotalValue = 10000000000.0

	// MaxReasonablePrice is the maximum reasonable price (1 billion IDR)
	// Used to detect price corruption in order data
	MaxReasonablePrice = 1000000000.0

	// MinOrderValueIDR is the minimum order value in IDR (10,000 IDR)
	// Used for validation across all trading services
	MinOrderValueIDR = 10000.0

	// MinInitialBalanceIDR is the minimum initial balance for Market Maker bot (50,000 IDR)
	MinInitialBalanceIDR = 50000.0

	// TinyBalanceThreshold is the threshold below which coin balances are considered dust
	// and should be cleaned up (0.00000001)
	TinyBalanceThreshold = 0.00000001
)
