package agent

import (
	"fmt"
	"math"
)

// Plausibility bounds for numeric tool inputs (REF-3). These exist to catch
// corrupted/hallucinated data (NaN, ±Inf, absurd magnitudes) before it
// reaches a calc* function — not to second-guess legitimate financial
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

// validateFinite rejects NaN and ±Inf. It makes no claim about sign or
// magnitude — use it for domain-agnostic values (EPS, EBITDA, currency
// amounts) that can legitimately be negative or arbitrarily large/small.
func validateFinite(name string, v float64) error {
	if math.IsNaN(v) {
		return fmt.Errorf("%s must be a finite number, got NaN", name)
	}
	if math.IsInf(v, 0) {
		return fmt.Errorf("%s must be a finite number, got Inf", name)
	}
	return nil
}

// validateFiniteAll applies validateFinite to every element of vs.
func validateFiniteAll(name string, vs []float64) error {
	for i, v := range vs {
		if err := validateFinite(fmt.Sprintf("%s[%d]", name, i), v); err != nil {
			return err
		}
	}
	return nil
}

// validatePositive requires v to be finite and strictly greater than zero.
// Use it for prices and other must-be-positive monetary inputs.
func validatePositive(name string, v float64) error {
	if err := validateFinite(name, v); err != nil {
		return err
	}
	if v <= 0 {
		return fmt.Errorf("%s must be greater than zero, got %g", name, v)
	}
	return nil
}

// validatePositiveAll applies validatePositive to every element of vs. Use it
// for prices []float64 series (volatility, max drawdown, technical
// indicators).
func validatePositiveAll(name string, vs []float64) error {
	for i, v := range vs {
		if err := validatePositive(fmt.Sprintf("%s[%d]", name, i), v); err != nil {
			return err
		}
	}
	return nil
}

// validateNonNegative requires v to be finite and >= zero. Use it where zero
// is a legitimate value (or an explicit "unset" sentinel) but negative isn't.
func validateNonNegative(name string, v float64) error {
	if err := validateFinite(name, v); err != nil {
		return err
	}
	if v < 0 {
		return fmt.Errorf("%s must be greater than or equal to zero, got %g", name, v)
	}
	return nil
}

// validateNonNegativeAll applies validateNonNegative to every element of vs.
func validateNonNegativeAll(name string, vs []float64) error {
	for i, v := range vs {
		if err := validateNonNegative(fmt.Sprintf("%s[%d]", name, i), v); err != nil {
			return err
		}
	}
	return nil
}

// validateReturn requires v to be finite and within the plausible range for a
// periodic/cumulative return expressed as a decimal fraction (e.g. 0.05 =
// 5%). The range is deliberately generous — see the package-level constants.
func validateReturn(name string, v float64) error {
	if err := validateFinite(name, v); err != nil {
		return err
	}
	if v < minPlausibleReturn || v > maxPlausibleReturn {
		return fmt.Errorf("%s=%g is outside the plausible range [%.0f%%, %.0f%%]",
			name, v, minPlausibleReturn*100, maxPlausibleReturn*100)
	}
	return nil
}

// validateReturnSlice applies validateReturn to every element of vs.
func validateReturnSlice(name string, vs []float64) error {
	for i, v := range vs {
		if err := validateReturn(fmt.Sprintf("%s[%d]", name, i), v); err != nil {
			return err
		}
	}
	return nil
}

// validateReturnPercent applies the same plausibility bound as
// validateReturn, but for values expressed in percent units (e.g. -20 means
// -20%) rather than decimal fractions.
func validateReturnPercent(name string, v float64) error {
	if err := validateFinite(name, v); err != nil {
		return err
	}
	min, max := minPlausibleReturn*100, maxPlausibleReturn*100
	if v < min || v > max {
		return fmt.Errorf("%s=%g is outside the plausible range [%.0f%%, %.0f%%]", name, v, min, max)
	}
	return nil
}

// validateReturnPercentSlice applies validateReturnPercent to every element of vs.
func validateReturnPercentSlice(name string, vs []float64) error {
	for i, v := range vs {
		if err := validateReturnPercent(fmt.Sprintf("%s[%d]", name, i), v); err != nil {
			return err
		}
	}
	return nil
}

// validateRate requires v to be finite and within the plausible range for an
// annualised rate (discount rate, risk-free rate, growth-rate assumption)
// expressed as a decimal fraction.
func validateRate(name string, v float64) error {
	if err := validateFinite(name, v); err != nil {
		return err
	}
	if v < minPlausibleRate || v > maxPlausibleRate {
		return fmt.Errorf("%s=%g is outside the plausible range [%.0f%%, %.0f%%]",
			name, v, minPlausibleRate*100, maxPlausibleRate*100)
	}
	return nil
}
