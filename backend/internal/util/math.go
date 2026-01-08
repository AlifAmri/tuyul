package util

import (
	"math"
)

// RoundToPrecision rounds a float64 to a specific number of decimal places
func RoundToPrecision(val float64, precision int) float64 {
	ratio := math.Pow(10, float64(precision))
	return math.Round(val*ratio) / ratio
}

// RoundToIncrement rounds a float64 down to the nearest increment
func RoundToIncrement(val float64, increment float64) float64 {
	if increment <= 0 {
		return val
	}
	return math.Floor(val/increment) * increment
}

// RoundToNearestIncrement rounds a float64 to the nearest increment
func RoundToNearestIncrement(val float64, increment float64) float64 {
	if increment <= 0 {
		return val
	}
	return math.Round(val/increment) * increment
}

// FloorToPrecision floors a float64 to a specific number of decimal places
func FloorToPrecision(val float64, precision int) float64 {
	ratio := math.Pow(10, float64(precision))
	return math.Floor(val*ratio) / ratio
}

// Abs returns the absolute value of x
func Abs(x float64) float64 {
	return math.Abs(x)
}

// Pow10 returns 10^n
func Pow10(n int) float64 {
	return math.Pow(10, float64(n))
}
