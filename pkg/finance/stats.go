package finance

import (
	"errors"
	"fmt"
	"sort"
)

// --- Descriptive statistics ----------------------------------------------------

type StatsResult struct {
	Mean         float64 `json:"mean"`
	Median       float64 `json:"median"`
	StdDev       float64 `json:"std_dev"`
	Min          float64 `json:"min"`
	Max          float64 `json:"max"`
	Percentile25 float64 `json:"percentile_25"`
	Percentile75 float64 `json:"percentile_75"`
	Count        int     `json:"count"`
	Summary      string  `json:"summary"`
	ComputedAt   string  `json:"computed_at"`
}

func CalcStats(values []float64, label string) (StatsResult, error) {
	if len(values) == 0 {
		return StatsResult{}, errors.New("values must not be empty")
	}
	if err := ValidateFiniteAll("values", values); err != nil {
		return StatsResult{}, err
	}
	s := make([]float64, len(values))
	copy(s, values)
	sort.Float64s(s)
	n := len(s)
	m := meanFloat(s)
	sd := sampleStddev(s)
	median := percentileInterp(s, 0.5)
	p25 := percentileInterp(s, 0.25)
	p75 := percentileInterp(s, 0.75)
	return StatsResult{
		Mean:         Round4(m),
		Median:       Round4(median),
		StdDev:       Round4(sd),
		Min:          Round4(s[0]),
		Max:          Round4(s[n-1]),
		Percentile25: Round4(p25),
		Percentile75: Round4(p75),
		Count:        n,
		Summary:      fmt.Sprintf("%s (n=%d): mean=%.4f, median=%.4f, std=%.4f, range=[%.4f, %.4f]", label, n, m, median, sd, s[0], s[n-1]),
	}, nil
}
