package portfolio

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"auris/pkg/config"
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
	if len(res.ConsumedLots) != 1 {
		t.Fatalf("want 1 consumed lot, got %d", len(res.ConsumedLots))
	}
	if res.ConsumedLots[0].LotID != "1" || res.ConsumedLots[0].Quantity != 4 || res.ConsumedLots[0].Price != 100 {
		t.Errorf("ConsumedLots[0] = %+v, want {LotID:1 Quantity:4 Price:100}", res.ConsumedLots[0])
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
	if len(res.ConsumedLots) != 2 {
		t.Fatalf("want 2 consumed lots, got %d", len(res.ConsumedLots))
	}
	if res.ConsumedLots[0].LotID != "1" || res.ConsumedLots[0].Quantity != 10 || res.ConsumedLots[0].Price != 100 {
		t.Errorf("ConsumedLots[0] = %+v, want {LotID:1 Quantity:10 Price:100}", res.ConsumedLots[0])
	}
	if res.ConsumedLots[1].LotID != "2" || res.ConsumedLots[1].Quantity != 2 || res.ConsumedLots[1].Price != 120 {
		t.Errorf("ConsumedLots[1] = %+v, want {LotID:2 Quantity:2 Price:120}", res.ConsumedLots[1])
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
	if len(res.ConsumedLots) != 1 || res.ConsumedLots[0].Quantity != 5 {
		t.Errorf("ConsumedLots = %+v, want 1 entry with Quantity 5", res.ConsumedLots)
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

// ─── Transaction tests ─────────────────────────────────────────────────────────

func TestRecordTransaction_AppendsAndAdjustsCash(t *testing.T) {
	p := NewPortfolio("test")
	p.Cash = 100

	tx := p.RecordTransaction(Transaction{Type: TransactionBuy, Symbol: "AAPL", CashDelta: -50})

	if p.Cash != 50 {
		t.Errorf("Cash = %v, want 50", p.Cash)
	}
	if len(p.Transactions) != 1 {
		t.Fatalf("want 1 transaction, got %d", len(p.Transactions))
	}
	if tx.ID == "" {
		t.Error("expected a generated transaction ID")
	}
	if p.Transactions[0].ID != tx.ID {
		t.Error("appended transaction's ID does not match the returned transaction")
	}
}

func TestSaveLoadPortfolio_Transactions(t *testing.T) {
	tmp := t.TempDir()
	origDir := portfoliosDirOverride
	portfoliosDirOverride = filepath.Join(tmp, "portfolios")
	t.Cleanup(func() { portfoliosDirOverride = origDir })

	p := NewPortfolio("test")
	buyDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	sellDate := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	p.RecordTransaction(Transaction{
		Type: TransactionBuy, Symbol: "AAPL", Quantity: 10, Price: 150, CashDelta: -1500, Date: buyDate,
	})
	p.RecordTransaction(Transaction{
		Type: TransactionSell, Symbol: "AAPL", Quantity: 4, Price: 180, CashDelta: 720,
		RealizedPnL: 120,
		ConsumedLots: []LotConsumption{
			{LotID: "l1", Quantity: 4, Price: 150, Date: buyDate},
		},
		Date: sellDate,
	})
	p.RecordTransaction(Transaction{
		Type: TransactionDividend, Symbol: "AAPL", CashDelta: 25, Date: sellDate,
	})

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
	if len(loaded.Transactions) != 3 {
		t.Fatalf("want 3 transactions, got %d", len(loaded.Transactions))
	}
	if loaded.Cash != p.Cash {
		t.Errorf("Cash = %v, want %v", loaded.Cash, p.Cash)
	}
	sell := loaded.Transactions[1]
	if sell.Type != TransactionSell || sell.RealizedPnL != 120 {
		t.Errorf("sell transaction = %+v, want Type=sell RealizedPnL=120", sell)
	}
	if len(sell.ConsumedLots) != 1 || !sell.ConsumedLots[0].Date.Equal(buyDate) {
		t.Errorf("sell.ConsumedLots = %+v, want 1 entry dated %v", sell.ConsumedLots, buyDate)
	}
}

func TestLoadPortfolio_NoTransactionsField(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "portfolios")
	origDir := portfoliosDirOverride
	portfoliosDirOverride = dir
	t.Cleanup(func() { portfoliosDirOverride = origDir })

	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	// Simulate a portfolio file written before the Transactions field existed.
	raw := `{"id":"legacy-1","name":"Legacy","cash":100,"created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-01T00:00:00Z"}`
	if err := os.WriteFile(filepath.Join(dir, "legacy-1.json"), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadPortfolio("legacy-1")
	if err != nil {
		t.Fatalf("LoadPortfolio: %v", err)
	}
	if loaded == nil {
		t.Fatal("LoadPortfolio returned nil")
	}
	if loaded.Transactions != nil {
		t.Errorf("Transactions = %+v, want nil", loaded.Transactions)
	}
}

// ─── FEAT-16: storage encryption ───────────────────────────────────────────────

func TestSaveLoadPortfolio_Encrypted(t *testing.T) {
	tmp := t.TempDir()
	origDir := portfoliosDirOverride
	portfoliosDirOverride = filepath.Join(tmp, "portfolios")
	t.Cleanup(func() { portfoliosDirOverride = origDir })

	salt, err := config.NewSalt()
	if err != nil {
		t.Fatalf("NewSalt: %v", err)
	}
	key := config.DeriveStorageKey("pass", salt)
	config.SetStorageKey(key)
	t.Cleanup(func() { config.SetStorageKey(nil) })

	p := NewPortfolio("Encrypted Portfolio")
	p.Cash = 500
	if err := SavePortfolio(p); err != nil {
		t.Fatalf("SavePortfolio: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(portfoliosDirOverride, p.ID+".json"))
	if err != nil {
		t.Fatalf("read raw file: %v", err)
	}
	if strings.Contains(string(raw), "Encrypted Portfolio") {
		t.Error("portfolio name found in plaintext on disk, expected it to be encrypted")
	}

	loaded, err := LoadPortfolio(p.ID)
	if err != nil {
		t.Fatalf("LoadPortfolio: %v", err)
	}
	if loaded == nil || loaded.Name != "Encrypted Portfolio" {
		t.Fatalf("LoadPortfolio: got %+v", loaded)
	}
}

func TestLoadPortfolio_LegacyPlaintextWithKeySet(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "portfolios")
	origDir := portfoliosDirOverride
	portfoliosDirOverride = dir
	t.Cleanup(func() { portfoliosDirOverride = origDir })

	key := config.DeriveStorageKey("pass", []byte("0123456789abcdef"))
	config.SetStorageKey(key)
	t.Cleanup(func() { config.SetStorageKey(nil) })

	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	raw := `{"id":"legacy-2","name":"Legacy Plaintext","created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-01T00:00:00Z"}`
	if err := os.WriteFile(filepath.Join(dir, "legacy-2.json"), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadPortfolio("legacy-2")
	if err != nil {
		t.Fatalf("LoadPortfolio: %v", err)
	}
	if loaded == nil || loaded.Name != "Legacy Plaintext" {
		t.Fatalf("LoadPortfolio: got %+v", loaded)
	}
}

func TestReencryptAllPortfolios(t *testing.T) {
	tmp := t.TempDir()
	origDir := portfoliosDirOverride
	portfoliosDirOverride = filepath.Join(tmp, "portfolios")
	t.Cleanup(func() { portfoliosDirOverride = origDir })
	t.Cleanup(func() { config.SetStorageKey(nil) })

	// Start plaintext (nil key), create two portfolios. NewPortfolio's ID has
	// only second resolution, so assign distinct IDs explicitly to avoid a
	// collision between p1 and p2 created in the same test.
	config.SetStorageKey(nil)
	p1 := NewPortfolio("P1")
	p1.ID = "20260101-000001"
	p2 := NewPortfolio("P2")
	p2.ID = "20260101-000002"
	if err := SavePortfolio(p1); err != nil {
		t.Fatalf("SavePortfolio p1: %v", err)
	}
	if err := SavePortfolio(p2); err != nil {
		t.Fatalf("SavePortfolio p2: %v", err)
	}

	saltA, _ := config.NewSalt()
	keyA := config.DeriveStorageKey("pass", saltA)

	// Migrate plaintext -> keyA.
	if err := ReencryptAllPortfolios(nil, keyA); err != nil {
		t.Fatalf("ReencryptAllPortfolios (plaintext->A): %v", err)
	}
	config.SetStorageKey(keyA)
	loaded, err := LoadPortfolio(p1.ID)
	if err != nil || loaded == nil || loaded.Name != "P1" {
		t.Fatalf("LoadPortfolio after migration to keyA: %v, %+v", err, loaded)
	}

	raw, err := os.ReadFile(filepath.Join(portfoliosDirOverride, p1.ID+".json"))
	if err != nil {
		t.Fatalf("read raw: %v", err)
	}
	if strings.Contains(string(raw), "P1") {
		t.Error("portfolio name found in plaintext after migration to keyA")
	}

	// Idempotent retry: calling again with the same (nil, keyA) pair should
	// detect the files are already migrated and succeed without error.
	if err := ReencryptAllPortfolios(nil, keyA); err != nil {
		t.Fatalf("ReencryptAllPortfolios retry: %v", err)
	}

	saltB, _ := config.NewSalt()
	keyB := config.DeriveStorageKey("pass2", saltB)

	// Re-key from keyA -> keyB.
	if err := ReencryptAllPortfolios(keyA, keyB); err != nil {
		t.Fatalf("ReencryptAllPortfolios (A->B): %v", err)
	}
	config.SetStorageKey(keyB)
	loaded, err = LoadPortfolio(p2.ID)
	if err != nil || loaded == nil || loaded.Name != "P2" {
		t.Fatalf("LoadPortfolio after migration to keyB: %v, %+v", err, loaded)
	}

	// keyA should no longer decrypt the files.
	config.SetStorageKey(keyA)
	if _, err := LoadPortfolio(p1.ID); err == nil {
		t.Error("expected error loading with stale keyA after re-key to keyB, got nil")
	}

	// Decrypt back to plaintext.
	if err := ReencryptAllPortfolios(keyB, nil); err != nil {
		t.Fatalf("ReencryptAllPortfolios (B->plaintext): %v", err)
	}
	config.SetStorageKey(nil)
	loaded, err = LoadPortfolio(p1.ID)
	if err != nil || loaded == nil || loaded.Name != "P1" {
		t.Fatalf("LoadPortfolio after decrypt: %v, %+v", err, loaded)
	}
}
