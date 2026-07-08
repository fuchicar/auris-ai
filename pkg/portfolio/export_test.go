package portfolio

import (
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPositionRows_AllHoldingsIncludedEvenWithoutQuote(t *testing.T) {
	p := makePortfolio("AAPL", "MSFT")
	// Only AAPL has a quote; MSFT should still get a row with blank price/value/weight/unrealised.
	m, err := ComputeMetrics(p, map[string]Quote{"AAPL": {Last: 150}})
	if err != nil {
		t.Fatal(err)
	}
	rows := positionRows(p, m)
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(rows))
	}
	bySymbol := make(map[string][]string, len(rows))
	for _, r := range rows {
		bySymbol[r[0]] = r
	}
	aapl := bySymbol["AAPL"]
	if aapl[3] == "" || aapl[4] == "" { // cost_basis, last_price
		t.Errorf("AAPL row missing cost_basis/last_price: %v", aapl)
	}
	msft := bySymbol["MSFT"]
	if msft[4] != "" || msft[5] != "" || msft[6] != "" || msft[7] != "" {
		t.Errorf("MSFT row (missing quote) should have blank last_price/value/weight/unrealised, got %v", msft)
	}
	if msft[3] == "" {
		t.Errorf("MSFT row should still have cost_basis (offline-derivable), got %v", msft)
	}
}

func TestPositionRows_DedupesSymbols(t *testing.T) {
	now := time.Now()
	p := &Portfolio{ID: "t", Instruments: []Instrument{
		{ID: "1", Symbol: "AAPL", Name: "Apple", Type: InstrumentHolding, Lots: []Lot{{ID: "l1", Quantity: 5, Price: 100, Date: now}}},
		{ID: "2", Symbol: "AAPL", Name: "Apple", Type: InstrumentHolding, Lots: []Lot{{ID: "l2", Quantity: 3, Price: 110, Date: now}}},
	}}
	m, _ := ComputeMetrics(p, nil)
	rows := positionRows(p, m)
	if len(rows) != 1 {
		t.Fatalf("want 1 deduped row, got %d: %v", len(rows), rows)
	}
	if rows[0][2] != "8" { // quantity 5+3
		t.Errorf("want combined quantity 8, got %s", rows[0][2])
	}
}

func TestPositionRows_EmptyPortfolio(t *testing.T) {
	p := &Portfolio{ID: "t"}
	rows := positionRows(p, PortfolioMetrics{})
	if len(rows) != 0 {
		t.Errorf("want 0 rows, got %d", len(rows))
	}
}

func TestLotRows_OneRowPerLot(t *testing.T) {
	p := makePortfolio("AAPL", "MSFT")
	rows := lotRows(p)
	if len(rows) != 2 {
		t.Fatalf("want 2 rows (one lot per symbol), got %d", len(rows))
	}
	for _, r := range rows {
		if len(r) != 5 {
			t.Errorf("row should have 5 columns, got %d: %v", len(r), r)
		}
	}
}

func TestLotRows_SkipsWatchlist(t *testing.T) {
	now := time.Now()
	p := &Portfolio{ID: "t", Instruments: []Instrument{
		{ID: "1", Symbol: "AAPL", Type: InstrumentHolding, Lots: []Lot{{ID: "l1", Quantity: 1, Price: 100, Date: now}}},
		{ID: "2", Symbol: "TSLA", Type: InstrumentWatchlist},
	}}
	rows := lotRows(p)
	if len(rows) != 1 {
		t.Fatalf("want 1 row (watchlist has no lots), got %d", len(rows))
	}
	if rows[0][0] != "AAPL" {
		t.Errorf("want AAPL row, got %v", rows[0])
	}
}

func TestMetricsRow_SingleRowMirrorsFields(t *testing.T) {
	p := makePortfolio("AAPL")
	m, err := ComputeMetrics(p, map[string]Quote{"AAPL": {Last: 150}})
	if err != nil {
		t.Fatal(err)
	}
	rows := metricsRow(m)
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	if len(rows[0]) != len(metricsHeader) {
		t.Errorf("row/header length mismatch: %d vs %d", len(rows[0]), len(metricsHeader))
	}
}

func TestExportFiles_WritesAllFourFiles(t *testing.T) {
	dir := t.TempDir()
	p := makePortfolio("AAPL", "MSFT")
	p.Cash = 500
	m, err := ComputeMetrics(p, map[string]Quote{"AAPL": {Last: 150}, "MSFT": {Last: 200}})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)

	paths, err := ExportFiles(dir, p, m, now)
	if err != nil {
		t.Fatalf("ExportFiles: %v", err)
	}
	if len(paths) != 4 {
		t.Fatalf("want 4 paths, got %d: %v", len(paths), paths)
	}
	for _, path := range paths {
		if !filepath.IsAbs(path) {
			continue // paths are dir-relative when dir isn't absolute; that's fine
		}
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected file to exist: %s: %v", path, err)
		}
	}

	// JSON: unmarshal and check round-trip fidelity.
	jsonData, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatalf("read json: %v", err)
	}
	var exported PortfolioExport
	if err := json.Unmarshal(jsonData, &exported); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	if exported.Portfolio.ID != p.ID {
		t.Errorf("json Portfolio.ID: want %s, got %s", p.ID, exported.Portfolio.ID)
	}
	if exported.Metrics.TotalValue != m.TotalValue {
		t.Errorf("json Metrics.TotalValue: want %v, got %v", m.TotalValue, exported.Metrics.TotalValue)
	}

	// positions.csv: header + 2 rows.
	assertCSVRowCount(t, paths[1], 1+2)
	// lots.csv: header + 2 rows (one lot per symbol from makePortfolio).
	assertCSVRowCount(t, paths[2], 1+2)
	// metrics.csv: header + 1 row.
	assertCSVRowCount(t, paths[3], 1+1)
}

func TestExportFiles_NilPortfolioErrors(t *testing.T) {
	if _, err := ExportFiles(t.TempDir(), nil, PortfolioMetrics{}, time.Now()); err == nil {
		t.Error("want error for nil portfolio, got nil")
	}
}

func TestExportFiles_FilenamesIncludePortfolioIDAndTimestamp(t *testing.T) {
	dir := t.TempDir()
	p := &Portfolio{ID: "abc123"}
	now := time.Date(2026, 7, 8, 12, 30, 0, 0, time.UTC)
	paths, err := ExportFiles(dir, p, PortfolioMetrics{}, now)
	if err != nil {
		t.Fatal(err)
	}
	wantBase := "auris-portfolio-abc123-20260708-123000"
	if filepath.Base(paths[0]) != wantBase+".json" {
		t.Errorf("json filename: got %s", filepath.Base(paths[0]))
	}
	if filepath.Base(paths[1]) != wantBase+"-positions.csv" {
		t.Errorf("positions filename: got %s", filepath.Base(paths[1]))
	}
}

func assertCSVRowCount(t *testing.T, path string, want int) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatalf("read csv %s: %v", path, err)
	}
	if len(rows) != want {
		t.Errorf("%s: want %d rows (incl. header), got %d", filepath.Base(path), want, len(rows))
	}
}
