package simulation

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"auris/pkg/config"
)

// dataFileName is the name of the simulation universe file, looked up (in
// order) in the process's current working directory and in the config
// directory before falling back to the embedded default.
const dataFileName = "simulation_data.json"

//go:embed data/simulation_data.json
var embeddedDataFS embed.FS

// instrumentDef is the JSON schema for one entry in the simulation universe
// file. It only carries generation parameters — no pre-computed price
// history — so the file stays small and the candle/quote series are derived
// deterministically at request time by generator.
type instrumentDef struct {
	Symbol            string  `json:"symbol"`
	Name              string  `json:"name"`
	Exchange          string  `json:"exchange"`
	Currency          string  `json:"currency"`
	Type              string  `json:"type"` // "stock" | "crypto"
	Country           string  `json:"country"`
	BasePrice         float64 `json:"base_price"`
	AnnualDrift       float64 `json:"annual_drift"`
	AnnualVolatility  float64 `json:"annual_volatility"`
	Beta              float64 `json:"beta"`
	DividendYield     float64 `json:"dividend_yield"`
	EPS               float64 `json:"eps"`
	SharesOutstanding float64 `json:"shares_outstanding"`
}

// universeFile is the top-level shape of the simulation data file.
type universeFile struct {
	Epoch            string          `json:"epoch"` // YYYY-MM-DD
	MarketFactorSeed string          `json:"market_factor_seed"`
	Instruments      []instrumentDef `json:"instruments"`
}

// loadUniverse resolves and parses the simulation data file: cwd override,
// then config-dir override, then the embedded default. A malformed override
// file falls back to the embedded default rather than failing outright —
// simulation mode should never be broken by a bad hand-edited file.
func loadUniverse() (universeFile, error) {
	if raw, ok := readOverride(); ok {
		var u universeFile
		if err := json.Unmarshal(raw, &u); err == nil {
			return u, nil
		}
	}
	raw, err := embeddedDataFS.ReadFile("data/" + dataFileName)
	if err != nil {
		return universeFile{}, fmt.Errorf("simulation: embedded data: %w", err)
	}
	var u universeFile
	if err := json.Unmarshal(raw, &u); err != nil {
		return universeFile{}, fmt.Errorf("simulation: embedded data: %w", err)
	}
	return u, nil
}

// readOverride looks for a user-supplied simulation_data.json, first in the
// current working directory (explicit override for testing/customization),
// then next to the config file (typically ~/.config/auris-ai/).
func readOverride() ([]byte, bool) {
	if b, err := os.ReadFile(dataFileName); err == nil {
		return b, true
	}
	if p, err := config.Path(); err == nil {
		candidate := filepath.Join(filepath.Dir(p), dataFileName)
		if b, err := os.ReadFile(candidate); err == nil {
			return b, true
		}
	}
	return nil, false
}

// parseEpoch parses the universe file's epoch date.
func parseEpoch(s string) (time.Time, error) {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("simulation: parse epoch %q: %w", s, err)
	}
	return t.UTC(), nil
}
