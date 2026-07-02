package portfolio

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"

	"auris/pkg/config"
)

// InstrumentType distinguishes between confirmed holdings and tracked watchlist items.
type InstrumentType string

const (
	// InstrumentHolding is an instrument the user owns with recorded lots.
	InstrumentHolding InstrumentType = "holding"
	// InstrumentWatchlist is an instrument the user tracks without a position.
	InstrumentWatchlist InstrumentType = "watchlist"
)

// Lot represents a single purchase of an instrument.
type Lot struct {
	ID       string    `json:"id"`
	Quantity float64   `json:"quantity"` // fractional shares allowed
	Price    float64   `json:"price"`    // purchase price per unit
	Date     time.Time `json:"date"`
}

// Instrument is a financial instrument held in or tracked by a portfolio.
type Instrument struct {
	ID     string         `json:"id"`
	Symbol string         `json:"symbol"`
	Name   string         `json:"name"`
	Type   InstrumentType `json:"type"`
	Lots   []Lot          `json:"lots,omitempty"`
}

// TotalQuantity returns the sum of all lot quantities.
func (ins Instrument) TotalQuantity() float64 {
	var total float64
	for _, l := range ins.Lots {
		total += l.Quantity
	}
	return total
}

// Portfolio is a named collection of instruments managed by the user.
type Portfolio struct {
	ID              string       `json:"id"`
	Name            string       `json:"name"`
	Description     string       `json:"description,omitempty"`
	AIProvider      string       `json:"ai_provider"`
	AIModel         string       `json:"ai_model"`
	ActiveSessionID string       `json:"active_session_id,omitempty"`
	Instruments     []Instrument `json:"instruments,omitempty"`
	RealizedPnL     float64      `json:"realized_pnl"`
	// TargetAllocation maps symbol → target weight as a fraction (e.g. 0.3
	// for 30%). Consumed by the portfolio_suggest_rebalance tool (MATH-12,
	// not yet implemented); left unset/empty until that tool is built.
	TargetAllocation map[string]float64 `json:"target_allocation,omitempty"`
	CreatedAt        time.Time          `json:"created_at"`
	UpdatedAt        time.Time          `json:"updated_at"`
}

// SellResult holds the outcome of a FIFO sell operation.
type SellResult struct {
	RealizedPnL   float64 // positive = gain, negative = loss
	RemainingLots []Lot   // lots remaining after the sale
}

// ErrInsufficientLots is returned when a sell quantity exceeds the available lots.
var ErrInsufficientLots = errors.New("insufficient lots for requested quantity")

// ApplyFIFOSell consumes lots oldest-first for qty units sold at sellPrice.
// Returns the realised P&L and the updated lot slice. Returns ErrInsufficientLots
// if qty exceeds the total available.
func ApplyFIFOSell(lots []Lot, qty, sellPrice float64) (SellResult, error) {
	// Calculate total available.
	var available float64
	for _, l := range lots {
		available += l.Quantity
	}
	// Use a small epsilon to handle floating point rounding.
	if qty > available+1e-9 {
		return SellResult{}, fmt.Errorf("portfolio: ApplyFIFOSell: %w", ErrInsufficientLots)
	}

	remaining := make([]Lot, len(lots))
	copy(remaining, lots)

	var pnl float64
	toSell := qty

	for i := 0; i < len(remaining) && toSell > 1e-12; i++ {
		lot := &remaining[i]
		if lot.Quantity <= toSell+1e-9 {
			// Consume the entire lot.
			consumed := lot.Quantity
			pnl += consumed * (sellPrice - lot.Price)
			toSell -= consumed
			lot.Quantity = 0
		} else {
			// Partially consume this lot.
			pnl += toSell * (sellPrice - lot.Price)
			lot.Quantity -= toSell
			toSell = 0
		}
	}

	// Remove fully consumed lots (quantity ≈ 0).
	filtered := remaining[:0]
	for _, l := range remaining {
		if l.Quantity > 1e-12 {
			filtered = append(filtered, l)
		}
	}

	return SellResult{
		RealizedPnL:   math.Round(pnl*1e8) / 1e8, // avoid floating point noise
		RemainingLots: filtered,
	}, nil
}

// ─── Persistence ─────────────────────────────────────────────────────────────

// portfoliosDirOverride is used in tests to redirect file I/O to a temp directory.
var portfoliosDirOverride string

// SetPortfoliosDirForTest replaces the portfolios directory override for the
// duration of the test and returns the previous value so the test can restore
// it via t.Cleanup. Not safe to call outside tests.
func SetPortfoliosDirForTest(dir string) string {
	prev := portfoliosDirOverride
	portfoliosDirOverride = dir
	return prev
}

// PortfoliosDir returns the directory where portfolio files are stored.
func PortfoliosDir() (string, error) {
	if portfoliosDirOverride != "" {
		return portfoliosDirOverride, nil
	}
	p, err := config.Path()
	if err != nil {
		return "", fmt.Errorf("portfolio: PortfoliosDir: %w", err)
	}
	return filepath.Join(filepath.Dir(p), "portfolios"), nil
}

// NewPortfolio creates a new Portfolio with a timestamp-based ID.
func NewPortfolio(name string) *Portfolio {
	now := time.Now()
	return &Portfolio{
		ID:        now.Format("20060102-150405"),
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// newLotID generates a timestamp-based ID for a lot.
func newLotID() string {
	return time.Now().Format("20060102-150405.000000")
}

// NewInstrumentID generates a timestamp-based ID for an instrument.
func NewInstrumentID() string {
	return time.Now().Format("20060102-150405.000")
}

// NewLot creates a Lot with a generated ID.
func NewLot(qty, price float64, date time.Time) Lot {
	return Lot{
		ID:       newLotID(),
		Quantity: qty,
		Price:    price,
		Date:     date,
	}
}

// SavePortfolio writes a portfolio to disk as a JSON file.
func SavePortfolio(p *Portfolio) error {
	dir, err := PortfoliosDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("portfolio: SavePortfolio: mkdir: %w", err)
	}
	p.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("portfolio: SavePortfolio: marshal: %w", err)
	}
	path := filepath.Join(dir, p.ID+".json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("portfolio: SavePortfolio: write: %w", err)
	}
	return nil
}

// LoadPortfolio reads a portfolio from disk by its ID. Returns nil and no error
// if the file does not exist.
func LoadPortfolio(id string) (*Portfolio, error) {
	dir, err := PortfoliosDir()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, id+".json"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("portfolio: LoadPortfolio: %w", err)
	}
	var p Portfolio
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("portfolio: LoadPortfolio: unmarshal: %w", err)
	}
	return &p, nil
}

// ListPortfolios returns all portfolios from disk sorted by UpdatedAt descending.
func ListPortfolios() ([]*Portfolio, error) {
	dir, err := PortfoliosDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("portfolio: ListPortfolios: %w", err)
	}
	var portfolios []*Portfolio
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		id := e.Name()[:len(e.Name())-5]
		p, err := LoadPortfolio(id)
		if err != nil || p == nil {
			continue
		}
		portfolios = append(portfolios, p)
	}
	sort.Slice(portfolios, func(i, j int) bool {
		return portfolios[i].UpdatedAt.After(portfolios[j].UpdatedAt)
	})
	return portfolios, nil
}

// DeletePortfolio removes a portfolio file from disk.
func DeletePortfolio(id string) error {
	dir, err := PortfoliosDir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, id+".json")
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("portfolio: DeletePortfolio: %w", err)
	}
	return nil
}
