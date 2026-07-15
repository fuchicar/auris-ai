// Package portfolio — export.go
//
// Writes a portfolio's positions, lots and computed metrics to disk as a
// JSON file (full fidelity, for backup/reimport) and three CSV files
// (positions, lots, metrics summary — for spreadsheets). Pure with respect
// to market data: callers supply an already-computed PortfolioMetrics, same
// division of responsibility as ComputeMetrics itself (see metrics.go).
package portfolio

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/fuchicar/auris-ai/pkg/config"
)

// PortfolioExport is the full-fidelity JSON export shape: the raw portfolio
// (instruments, lots, transactions, target allocation) plus its computed
// metrics snapshot.
type PortfolioExport struct {
	Portfolio *Portfolio       `json:"portfolio"`
	Metrics   PortfolioMetrics `json:"metrics"`
}

// ExportFiles writes one JSON export and three CSVs (positions, lots,
// metrics summary) into dir, named
// auris-portfolio-<id>-<timestamp>[.json|-positions.csv|-lots.csv|-metrics.csv].
// now is injected for deterministic tests; callers pass time.Now(). Returns
// the paths written, in that order (json, positions, lots, metrics).
func ExportFiles(dir string, p *Portfolio, m PortfolioMetrics, now time.Time) ([]string, error) {
	if p == nil {
		return nil, fmt.Errorf("portfolio: ExportFiles: nil portfolio")
	}
	base := fmt.Sprintf("auris-portfolio-%s-%s", p.ID, now.Format("20060102-150405"))

	jsonPath := filepath.Join(dir, base+".json")
	if err := writeJSON(jsonPath, PortfolioExport{Portfolio: p, Metrics: m}); err != nil {
		return nil, err
	}

	positionsPath := filepath.Join(dir, base+"-positions.csv")
	if err := writeCSV(positionsPath, positionsHeader, positionRows(p, m)); err != nil {
		return nil, err
	}

	lotsPath := filepath.Join(dir, base+"-lots.csv")
	if err := writeCSV(lotsPath, lotsHeader, lotRows(p)); err != nil {
		return nil, err
	}

	metricsPath := filepath.Join(dir, base+"-metrics.csv")
	if err := writeCSV(metricsPath, metricsHeader, metricsRow(m)); err != nil {
		return nil, err
	}

	return []string{jsonPath, positionsPath, lotsPath, metricsPath}, nil
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("portfolio: ExportFiles: marshal json: %w", err)
	}
	if err := config.WriteFileAtomic(path, data, 0o600); err != nil {
		return fmt.Errorf("portfolio: ExportFiles: write json: %w", err)
	}
	return nil
}

func writeCSV(path string, header []string, rows [][]string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("portfolio: ExportFiles: create %s: %w", filepath.Base(path), err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	if err := w.Write(header); err != nil {
		return fmt.Errorf("portfolio: ExportFiles: write header %s: %w", filepath.Base(path), err)
	}
	for _, row := range rows {
		if err := w.Write(row); err != nil {
			return fmt.Errorf("portfolio: ExportFiles: write row %s: %w", filepath.Base(path), err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return fmt.Errorf("portfolio: ExportFiles: flush %s: %w", filepath.Base(path), err)
	}
	return nil
}

var positionsHeader = []string{"symbol", "name", "quantity", "cost_basis", "last_price", "value", "weight", "unrealised"}

// positionRows returns one row per distinct holding symbol, in first-seen
// portfolio order, aggregating quantity/cost basis across every Instrument
// entry that shares the symbol. Unlike PortfolioMetrics.WeightBySymbol
// (which only lists symbols it could price), every held symbol gets a row
// here; last_price/value/weight/unrealised are left blank for symbols in
// m.MissingQuotes (or when m is the zero value, e.g. an offline export).
func positionRows(p *Portfolio, m PortfolioMetrics) [][]string {
	byWeight := make(map[string]SymbolWeight, len(m.WeightBySymbol))
	for _, w := range m.WeightBySymbol {
		byWeight[w.Symbol] = w
	}

	type agg struct {
		name      string
		quantity  float64
		costBasis float64
	}
	order := make([]string, 0, len(p.Instruments))
	bySymbol := make(map[string]*agg, len(p.Instruments))
	for _, ins := range p.Instruments {
		if ins.Type != InstrumentHolding {
			continue
		}
		a, ok := bySymbol[ins.Symbol]
		if !ok {
			a = &agg{name: ins.Name}
			bySymbol[ins.Symbol] = a
			order = append(order, ins.Symbol)
		}
		a.quantity += ins.TotalQuantity()
		for _, l := range ins.Lots {
			a.costBasis += l.Quantity * l.Price
		}
	}

	rows := make([][]string, 0, len(order))
	for _, symbol := range order {
		a := bySymbol[symbol]
		row := []string{symbol, a.name, formatFloat(a.quantity), formatFloat(round2(a.costBasis))}
		if w, ok := byWeight[symbol]; ok {
			row = append(row, formatFloat(w.LastPrice), formatFloat(w.Value), formatFloat(w.Weight), formatFloat(w.Unrealised))
		} else {
			row = append(row, "", "", "", "")
		}
		rows = append(rows, row)
	}
	return rows
}

var lotsHeader = []string{"symbol", "lot_id", "quantity", "price", "date"}

// lotRows returns one row per Lot across all holding instruments, in
// portfolio/lot order.
func lotRows(p *Portfolio) [][]string {
	var rows [][]string
	for _, ins := range p.Instruments {
		if ins.Type != InstrumentHolding {
			continue
		}
		for _, l := range ins.Lots {
			rows = append(rows, []string{
				ins.Symbol, l.ID, formatFloat(l.Quantity), formatFloat(l.Price), l.Date.Format(time.RFC3339),
			})
		}
	}
	return rows
}

var metricsHeader = []string{
	"holding_count", "cost_basis", "current_value", "cash", "total_value",
	"unrealised_pnl", "total_pnl_absolute", "total_pnl_percent",
	"hhi", "hhi_interpretation", "dividend_yield", "weighted_beta", "computed_at",
}

// metricsRow returns a single summary row mirroring PortfolioMetrics' scalar
// fields.
func metricsRow(m PortfolioMetrics) [][]string {
	return [][]string{{
		strconv.Itoa(m.HoldingCount),
		formatFloat(m.CostBasis),
		formatFloat(m.CurrentValue),
		formatFloat(m.Cash),
		formatFloat(m.TotalValue),
		formatFloat(m.UnrealisedPnL),
		formatFloat(m.TotalPnLAbsolute),
		formatFloat(m.TotalPnLPercent),
		formatFloat(m.HHI),
		m.HHIInterpretation,
		formatFloat(m.DividendYield),
		formatFloat(m.WeightedBeta),
		m.ComputedAt,
	}}
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
