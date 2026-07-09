package finance

import "math"

// Round2 rounds v to 2 decimal places.
func Round2(v float64) float64 { return math.Round(v*100) / 100 }

// Round4 rounds v to 4 decimal places.
func Round4(v float64) float64 { return math.Round(v*10000) / 10000 }

func meanFloat(xs []float64) float64 {
	sum := 0.0
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs))
}

// sampleStddev returns the sample standard deviation (denominator n-1).
// Returns 0 for slices with fewer than 2 elements.
func sampleStddev(xs []float64) float64 {
	if len(xs) < 2 {
		return 0
	}
	m := meanFloat(xs)
	variance := 0.0
	for _, x := range xs {
		d := x - m
		variance += d * d
	}
	return math.Sqrt(variance / float64(len(xs)-1))
}

// percentileInterp returns the p-th percentile using linear interpolation (Type 7).
func percentileInterp(sorted []float64, p float64) float64 {
	n := len(sorted)
	if n == 1 {
		return sorted[0]
	}
	h := p * float64(n-1)
	lo := int(math.Floor(h))
	hi := lo + 1
	if hi >= n {
		return sorted[n-1]
	}
	return sorted[lo] + (h-float64(lo))*(sorted[hi]-sorted[lo])
}
