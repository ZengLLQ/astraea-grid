package das

import "math"

// CalculateConfidence returns the statistical probability that the block data is available.
// If an adversary withholds at least 50% of any row or column to prevent 2D Reed-Solomon reconstruction,
// each random sample has probability at least 0.5 of hitting a withheld share.
// After s independent successful samples, the adversary's probability of undetected withholding is 2^(-s).
// Confidence = 1 - 2^(-s).
func CalculateConfidence(numSuccessfulSamples int) float64 {
	if numSuccessfulSamples <= 0 {
		return 0.0
	}
	if numSuccessfulSamples >= 60 {
		return 1.0 // Prevent precision underflow
	}
	return 1.0 - math.Pow(0.5, float64(numSuccessfulSamples))
}
