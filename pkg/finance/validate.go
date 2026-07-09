package finance

import (
	"fmt"
	"math"
)

// Plausibility bounds for numeric tool inputs (REF-3). These exist to catch
// corrupted/hallucinated data (NaN, ±Inf, absurd magnitudes) before it
// reaches a Calc* function — not to second-guess legitimate financial
// extremes. A 1000%+ return is unusual but real (an asset can multiply 10x
// or more), so the return bound is deliberately generous; annualised rates
// get a tighter bound since no real-world rate assumption approaches it even
// in hyperinflation scenarios.
const (
	// minPlausibleReturn / maxPlausibleReturn bound a periodic or cumulative
	// return expressed as a decimal fraction (e.g. 0.05 = 5%). The floor is
	// the hard "can't lose more than the whole position" bound (-100%); the
	// ceiling comfortably admits a 10x-100x move (+10,000%).
	minPlausibleReturn = -1.0
	maxPlausibleReturn = 100.0

	// minPlausibleRate / maxPlausibleRate bound an annualised rate (discount
	// rate, risk-free rate, growth-rate assumption) expressed as a decimal
	// fraction.
	minPlausibleRate = -1.0
	maxPlausibleRate = 10.0
)

// ValidateFinite rejects NaN and ±Inf. It makes no claim about sign or
// magnitude — use it for domain-agnostic values (EPS, EBITDA, currency
// amounts) that can legitimately be negative or arbitrarily large/small.
func ValidateFinite(name string, v float64) error {
	if math.IsNaN(v) {
		return fmt.Errorf("%s must be a finite number, got NaN", name)
	}
	if math.IsInf(v, 0) {
		return fmt.Errorf("%s must be a finite number, got Inf", name)
	}
	return nil
}

// ValidateFiniteAll applies ValidateFinite to every element of vs.
func ValidateFiniteAll(name string, vs []float64) error {
	for i, v := range vs {
		if err := ValidateFinite(fmt.Sprintf("%s[%d]", name, i), v); err != nil {
			return err
		}
	}
	return nil
}

// ValidatePositive requires v to be finite and strictly greater than zero.
// Use it for prices and other must-be-positive monetary inputs.
func ValidatePositive(name string, v float64) error {
	if err := ValidateFinite(name, v); err != nil {
		return err
	}
	if v <= 0 {
		return fmt.Errorf("%s must be greater than zero, got %g", name, v)
	}
	return nil
}

// ValidatePositiveAll applies ValidatePositive to every element of vs. Use it
// for prices []float64 series (volatility, max drawdown, technical
// indicators).
func ValidatePositiveAll(name string, vs []float64) error {
	for i, v := range vs {
		if err := ValidatePositive(fmt.Sprintf("%s[%d]", name, i), v); err != nil {
			return err
		}
	}
	return nil
}

// ValidateNonNegative requires v to be finite and >= zero. Use it where zero
// is a legitimate value (or an explicit "unset" sentinel) but negative isn't.
func ValidateNonNegative(name string, v float64) error {
	if err := ValidateFinite(name, v); err != nil {
		return err
	}
	if v < 0 {
		return fmt.Errorf("%s must be greater than or equal to zero, got %g", name, v)
	}
	return nil
}

// ValidateNonNegativeAll applies ValidateNonNegative to every element of vs.
func ValidateNonNegativeAll(name string, vs []float64) error {
	for i, v := range vs {
		if err := ValidateNonNegative(fmt.Sprintf("%s[%d]", name, i), v); err != nil {
			return err
		}
	}
	return nil
}

// ValidateReturn requires v to be finite and within the plausible range for a
// periodic/cumulative return expressed as a decimal fraction (e.g. 0.05 =
// 5%). The range is deliberately generous — see the package-level constants.
func ValidateReturn(name string, v float64) error {
	if err := ValidateFinite(name, v); err != nil {
		return err
	}
	if v < minPlausibleReturn || v > maxPlausibleReturn {
		return fmt.Errorf("%s=%g is outside the plausible range [%.0f%%, %.0f%%]",
			name, v, minPlausibleReturn*100, maxPlausibleReturn*100)
	}
	return nil
}

// ValidateReturnSlice applies ValidateReturn to every element of vs.
func ValidateReturnSlice(name string, vs []float64) error {
	for i, v := range vs {
		if err := ValidateReturn(fmt.Sprintf("%s[%d]", name, i), v); err != nil {
			return err
		}
	}
	return nil
}

// ValidateReturnPercent applies the same plausibility bound as
// ValidateReturn, but for values expressed in percent units (e.g. -20 means
// -20%) rather than decimal fractions.
func ValidateReturnPercent(name string, v float64) error {
	if err := ValidateFinite(name, v); err != nil {
		return err
	}
	min, max := minPlausibleReturn*100, maxPlausibleReturn*100
	if v < min || v > max {
		return fmt.Errorf("%s=%g is outside the plausible range [%.0f%%, %.0f%%]", name, v, min, max)
	}
	return nil
}

// ValidateReturnPercentSlice applies ValidateReturnPercent to every element of vs.
func ValidateReturnPercentSlice(name string, vs []float64) error {
	for i, v := range vs {
		if err := ValidateReturnPercent(fmt.Sprintf("%s[%d]", name, i), v); err != nil {
			return err
		}
	}
	return nil
}

// ValidateRate requires v to be finite and within the plausible range for an
// annualised rate (discount rate, risk-free rate, growth-rate assumption)
// expressed as a decimal fraction.
func ValidateRate(name string, v float64) error {
	if err := ValidateFinite(name, v); err != nil {
		return err
	}
	if v < minPlausibleRate || v > maxPlausibleRate {
		return fmt.Errorf("%s=%g is outside the plausible range [%.0f%%, %.0f%%]",
			name, v, minPlausibleRate*100, maxPlausibleRate*100)
	}
	return nil
}
