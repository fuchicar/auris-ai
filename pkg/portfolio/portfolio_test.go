package portfolio

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ─── FIFO tests ───────────────────────────────────────────────────────────────

func TestApplyFIFOSell_PartialLot(t *testing.T) {
	lots := []Lot{
		{ID: "1", Quantity: 10, Price: 100, Date: time.Now()},
	}
	res, err := ApplyFIFOSell(lots, 4, 120)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := 4 * (120.0 - 100.0) // 80
	if res.RealizedPnL != want {
		t.Errorf("RealizedPnL = %.8f, want %.8f", res.RealizedPnL, want)
	}
	if len(res.RemainingLots) != 1 {
		t.Fatalf("want 1 remaining lot, got %d", len(res.RemainingLots))
	}
	if res.RemainingLots[0].Quantity != 6 {
		t.Errorf("remaining qty = %v, want 6", res.RemainingLots[0].Quantity)
	}
}

func TestApplyFIFOSell_MultiLot(t *testing.T) {
	lots := []Lot{
		{ID: "1", Quantity: 10, Price: 100, Date: time.Now().Add(-2 * time.Hour)},
		{ID: "2", Quantity: 5, Price: 120, Date: time.Now().Add(-time.Hour)},
	}
	// Sell 12 shares: all 10 from lot1, then 2 from lot2
	res, err := ApplyFIFOSell(lots, 12, 150)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantPnL := 10*(150.0-100.0) + 2*(150.0-120.0) // 500 + 60 = 560
	if res.RealizedPnL != wantPnL {
		t.Errorf("RealizedPnL = %.8f, want %.8f", res.RealizedPnL, wantPnL)
	}
	if len(res.RemainingLots) != 1 {
		t.Fatalf("want 1 remaining lot, got %d", len(res.RemainingLots))
	}
	if res.RemainingLots[0].Quantity != 3 {
		t.Errorf("remaining qty = %v, want 3", res.RemainingLots[0].Quantity)
	}
}

func TestApplyFIFOSell_FullConsumption(t *testing.T) {
	lots := []Lot{
		{ID: "1", Quantity: 5, Price: 50, Date: time.Now()},
	}
	res, err := ApplyFIFOSell(lots, 5, 80)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.RealizedPnL != 5*(80.0-50.0) {
		t.Errorf("RealizedPnL = %.8f, want 150", res.RealizedPnL)
	}
	if len(res.RemainingLots) != 0 {
		t.Errorf("expected no remaining lots, got %d", len(res.RemainingLots))
	}
}

func TestApplyFIFOSell_Insufficient(t *testing.T) {
	lots := []Lot{
		{ID: "1", Quantity: 3, Price: 100, Date: time.Now()},
	}
	_, err := ApplyFIFOSell(lots, 5, 120)
	if err == nil {
		t.Fatal("expected error for insufficient lots, got nil")
	}
}

func TestApplyFIFOSell_FractionalShares(t *testing.T) {
	lots := []Lot{
		{ID: "1", Quantity: 0.11888, Price: 200, Date: time.Now()},
	}
	res, err := ApplyFIFOSell(lots, 0.05, 250)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantPnL := 0.05 * (250.0 - 200.0) // 2.5
	if res.RealizedPnL != wantPnL {
		t.Errorf("RealizedPnL = %.8f, want %.8f", res.RealizedPnL, wantPnL)
	}
}

// ─── Save/Load round-trip ─────────────────────────────────────────────────────

func TestSaveLoadPortfolio(t *testing.T) {
	// Redirect portfolios to a temp directory.
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "portfolios")

	origDir := portfoliosDirOverride
	portfoliosDirOverride = dir
	t.Cleanup(func() { portfoliosDirOverride = origDir })

	p := NewPortfolio("Test Portfolio")
	p.Description = "A test portfolio"
	p.AIProvider = "ollama"
	p.AIModel = "llama3.2"
	p.Instruments = []Instrument{
		{
			ID:     "i1",
			Symbol: "AAPL",
			Name:   "Apple Inc.",
			Type:   InstrumentHolding,
			Lots: []Lot{
				{ID: "l1", Quantity: 10, Price: 150, Date: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)},
			},
		},
	}
	p.RealizedPnL = 250.0

	if err := SavePortfolio(p); err != nil {
		t.Fatalf("SavePortfolio: %v", err)
	}

	loaded, err := LoadPortfolio(p.ID)
	if err != nil {
		t.Fatalf("LoadPortfolio: %v", err)
	}
	if loaded == nil {
		t.Fatal("LoadPortfolio returned nil")
	}
	if loaded.Name != p.Name {
		t.Errorf("Name = %q, want %q", loaded.Name, p.Name)
	}
	if loaded.RealizedPnL != p.RealizedPnL {
		t.Errorf("RealizedPnL = %v, want %v", loaded.RealizedPnL, p.RealizedPnL)
	}
	if len(loaded.Instruments) != 1 || loaded.Instruments[0].Symbol != "AAPL" {
		t.Errorf("instruments mismatch: %+v", loaded.Instruments)
	}

	// ListPortfolios should return the one portfolio.
	list, err := ListPortfolios()
	if err != nil {
		t.Fatalf("ListPortfolios: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("ListPortfolios len = %d, want 1", len(list))
	}

	// DeletePortfolio should remove it.
	if err := DeletePortfolio(p.ID); err != nil {
		t.Fatalf("DeletePortfolio: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, p.ID+".json")); !os.IsNotExist(err) {
		t.Error("file still exists after delete")
	}
}

func TestLoadPortfolio_NotFound(t *testing.T) {
	tmp := t.TempDir()

	origDir := portfoliosDirOverride
	portfoliosDirOverride = filepath.Join(tmp, "portfolios")
	t.Cleanup(func() { portfoliosDirOverride = origDir })

	p, err := LoadPortfolio("nonexistent")
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if p != nil {
		t.Error("expected nil portfolio")
	}
}
