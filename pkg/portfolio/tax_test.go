package portfolio

import (
	"errors"
	"testing"
	"time"
)

func TestCalculateTaxPnL_NilPortfolio(t *testing.T) {
	_, err := CalculateTaxPnL(nil, time.Time{}, time.Time{})
	if err == nil {
		t.Fatal("expected error for nil portfolio")
	}
}

func TestCalculateTaxPnL_NoTransactions_EmptyResult(t *testing.T) {
	p := &Portfolio{ID: "test"}
	r, err := CalculateTaxPnL(p, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Lots) != 0 || len(r.BySymbol) != 0 {
		t.Errorf("expected empty result, got %+v", r)
	}
	if r.TotalPnL != 0 || r.TotalShortTermPnL != 0 || r.TotalLongTermPnL != 0 {
		t.Errorf("expected zero totals, got %+v", r)
	}
}

func TestCalculateTaxPnL_TransactionsButNoSells_EmptyResult(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	p := &Portfolio{ID: "test", Transactions: []Transaction{
		{Type: TransactionBuy, Symbol: "AAPL", Quantity: 10, Price: 100, Date: base},
		{Type: TransactionDividend, Symbol: "AAPL", CashDelta: 5, Date: base.AddDate(0, 1, 0)},
		{Type: TransactionDeposit, CashDelta: 1000, Date: base},
	}}
	r, err := CalculateTaxPnL(p, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Lots) != 0 || len(r.BySymbol) != 0 {
		t.Errorf("expected empty result (no sells), got %+v", r)
	}
}

func TestCalculateTaxPnL_ShortTermClassification(t *testing.T) {
	buyDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	sellDate := buyDate.AddDate(0, 6, 0) // ~180 days, well under 365
	p := &Portfolio{ID: "test", Transactions: []Transaction{
		{
			Type: TransactionSell, Symbol: "AAPL", Quantity: 10, Price: 150, Date: sellDate,
			ConsumedLots: []LotConsumption{{LotID: "lot1", Quantity: 10, Price: 100, Date: buyDate}},
		},
	}}
	r, err := CalculateTaxPnL(p, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Lots) != 1 {
		t.Fatalf("expected 1 lot, got %d", len(r.Lots))
	}
	if r.Lots[0].Term != TaxTermShort {
		t.Errorf("expected short_term, got %s", r.Lots[0].Term)
	}
	if r.Lots[0].RealizedPnL != 500 {
		t.Errorf("expected realized pnl 500, got %v", r.Lots[0].RealizedPnL)
	}
	if r.TotalShortTermPnL != 500 || r.TotalLongTermPnL != 0 {
		t.Errorf("expected short=500 long=0, got short=%v long=%v", r.TotalShortTermPnL, r.TotalLongTermPnL)
	}
}

func TestCalculateTaxPnL_LongTermClassification(t *testing.T) {
	buyDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	sellDate := buyDate.AddDate(2, 0, 0) // 2 years later, well over 365 days
	p := &Portfolio{ID: "test", Transactions: []Transaction{
		{
			Type: TransactionSell, Symbol: "MSFT", Quantity: 4, Price: 300, Date: sellDate,
			ConsumedLots: []LotConsumption{{LotID: "lot1", Quantity: 4, Price: 200, Date: buyDate}},
		},
	}}
	r, err := CalculateTaxPnL(p, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Lots) != 1 {
		t.Fatalf("expected 1 lot, got %d", len(r.Lots))
	}
	if r.Lots[0].Term != TaxTermLong {
		t.Errorf("expected long_term, got %s", r.Lots[0].Term)
	}
	if r.TotalLongTermPnL != 400 || r.TotalShortTermPnL != 0 {
		t.Errorf("expected long=400 short=0, got long=%v short=%v", r.TotalLongTermPnL, r.TotalShortTermPnL)
	}
}

func TestCalculateTaxPnL_ExactBoundary_365Days_IsShortTerm(t *testing.T) {
	buyDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	sellDate := buyDate.AddDate(0, 0, 365) // exactly 365 days
	p := &Portfolio{ID: "test", Transactions: []Transaction{
		{
			Type: TransactionSell, Symbol: "AAPL", Quantity: 1, Price: 110, Date: sellDate,
			ConsumedLots: []LotConsumption{{LotID: "lot1", Quantity: 1, Price: 100, Date: buyDate}},
		},
	}}
	r, err := CalculateTaxPnL(p, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Lots[0].Term != TaxTermShort {
		t.Errorf("expected exactly-365-days to be short_term, got %s", r.Lots[0].Term)
	}
	if r.Lots[0].HoldingDays != 365 {
		t.Errorf("expected HoldingDays 365, got %d", r.Lots[0].HoldingDays)
	}
}

func TestCalculateTaxPnL_366Days_IsLongTerm(t *testing.T) {
	buyDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	sellDate := buyDate.AddDate(0, 0, 366)
	p := &Portfolio{ID: "test", Transactions: []Transaction{
		{
			Type: TransactionSell, Symbol: "AAPL", Quantity: 1, Price: 110, Date: sellDate,
			ConsumedLots: []LotConsumption{{LotID: "lot1", Quantity: 1, Price: 100, Date: buyDate}},
		},
	}}
	r, err := CalculateTaxPnL(p, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Lots[0].Term != TaxTermLong {
		t.Errorf("expected 366-days to be long_term, got %s", r.Lots[0].Term)
	}
}

func TestCalculateTaxPnL_SingleSaleMultipleConsumedLots_SplitAcrossTerms(t *testing.T) {
	sellDate := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	p := &Portfolio{ID: "test", Transactions: []Transaction{
		{
			Type: TransactionSell, Symbol: "AAPL", Quantity: 15, Price: 150, Date: sellDate,
			ConsumedLots: []LotConsumption{
				// bought 2 years before sale: long-term
				{LotID: "lot-old", Quantity: 10, Price: 100, Date: sellDate.AddDate(-2, 0, 0)},
				// bought 1 month before sale: short-term
				{LotID: "lot-new", Quantity: 5, Price: 140, Date: sellDate.AddDate(0, -1, 0)},
			},
		},
	}}
	r, err := CalculateTaxPnL(p, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Lots) != 2 {
		t.Fatalf("expected 2 lot details, got %d", len(r.Lots))
	}
	if len(r.BySymbol) != 1 {
		t.Fatalf("expected 1 symbol summary, got %d", len(r.BySymbol))
	}
	sym := r.BySymbol[0]
	if sym.LongTermPnL != 500 { // 10 * (150-100)
		t.Errorf("expected long term pnl 500, got %v", sym.LongTermPnL)
	}
	if sym.ShortTermPnL != 50 { // 5 * (150-140)
		t.Errorf("expected short term pnl 50, got %v", sym.ShortTermPnL)
	}
	if sym.LongTermLots != 1 || sym.ShortTermLots != 1 {
		t.Errorf("expected 1 long lot and 1 short lot, got long=%d short=%d", sym.LongTermLots, sym.ShortTermLots)
	}
}

func TestCalculateTaxPnL_GroupedBySymbol_MultipleSymbols(t *testing.T) {
	buyDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	sellDate := buyDate.AddDate(0, 3, 0)
	p := &Portfolio{ID: "test", Transactions: []Transaction{
		{
			Type: TransactionSell, Symbol: "AAPL", Quantity: 10, Price: 120, Date: sellDate,
			ConsumedLots: []LotConsumption{{LotID: "a1", Quantity: 10, Price: 100, Date: buyDate}},
		},
		{
			Type: TransactionSell, Symbol: "MSFT", Quantity: 5, Price: 300, Date: sellDate,
			ConsumedLots: []LotConsumption{{LotID: "m1", Quantity: 5, Price: 250, Date: buyDate}},
		},
	}}
	r, err := CalculateTaxPnL(p, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.BySymbol) != 2 {
		t.Fatalf("expected 2 symbols, got %d", len(r.BySymbol))
	}
	// Sorted alphabetically.
	if r.BySymbol[0].Symbol != "AAPL" || r.BySymbol[1].Symbol != "MSFT" {
		t.Errorf("expected AAPL before MSFT, got %s then %s", r.BySymbol[0].Symbol, r.BySymbol[1].Symbol)
	}
}

func TestCalculateTaxPnL_GrandTotalsSumBySymbolSubtotals(t *testing.T) {
	buyDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	sellDate := buyDate.AddDate(0, 3, 0)
	p := &Portfolio{ID: "test", Transactions: []Transaction{
		{
			Type: TransactionSell, Symbol: "AAPL", Quantity: 10, Price: 120, Date: sellDate,
			ConsumedLots: []LotConsumption{{LotID: "a1", Quantity: 10, Price: 100, Date: buyDate}},
		},
		{
			Type: TransactionSell, Symbol: "MSFT", Quantity: 5, Price: 300, Date: sellDate,
			ConsumedLots: []LotConsumption{{LotID: "m1", Quantity: 5, Price: 250, Date: buyDate}},
		},
	}}
	r, err := CalculateTaxPnL(p, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	var sumShort, sumLong float64
	for _, s := range r.BySymbol {
		sumShort += s.ShortTermPnL
		sumLong += s.LongTermPnL
	}
	if sumShort != r.TotalShortTermPnL {
		t.Errorf("sum of by-symbol short term (%v) != total short term (%v)", sumShort, r.TotalShortTermPnL)
	}
	if sumLong != r.TotalLongTermPnL {
		t.Errorf("sum of by-symbol long term (%v) != total long term (%v)", sumLong, r.TotalLongTermPnL)
	}
	if r.TotalPnL != r.TotalShortTermPnL+r.TotalLongTermPnL {
		t.Errorf("total pnl (%v) != short+long (%v)", r.TotalPnL, r.TotalShortTermPnL+r.TotalLongTermPnL)
	}
}

func TestCalculateTaxPnL_DateRangeFilter_ExcludesSalesOutsideWindow(t *testing.T) {
	buyDate := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	saleInRange := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	saleOutOfRange := time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC)
	p := &Portfolio{ID: "test", Transactions: []Transaction{
		{
			Type: TransactionSell, Symbol: "AAPL", Quantity: 1, Price: 110, Date: saleInRange,
			ConsumedLots: []LotConsumption{{LotID: "a1", Quantity: 1, Price: 100, Date: buyDate}},
		},
		{
			Type: TransactionSell, Symbol: "AAPL", Quantity: 1, Price: 120, Date: saleOutOfRange,
			ConsumedLots: []LotConsumption{{LotID: "a2", Quantity: 1, Price: 100, Date: buyDate}},
		},
	}}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	r, err := CalculateTaxPnL(p, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Lots) != 1 {
		t.Fatalf("expected 1 lot in range, got %d", len(r.Lots))
	}
	if r.Lots[0].LotID != "a1" {
		t.Errorf("expected in-range lot a1, got %s", r.Lots[0].LotID)
	}
	if r.From == "" || r.To == "" {
		t.Errorf("expected From/To to be echoed back, got From=%q To=%q", r.From, r.To)
	}
}

func TestCalculateTaxPnL_OnlyFromBound(t *testing.T) {
	buyDate := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	saleEarly := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	saleLate := time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC)
	p := &Portfolio{ID: "test", Transactions: []Transaction{
		{Type: TransactionSell, Symbol: "AAPL", Quantity: 1, Price: 110, Date: saleEarly,
			ConsumedLots: []LotConsumption{{LotID: "early", Quantity: 1, Price: 100, Date: buyDate}}},
		{Type: TransactionSell, Symbol: "AAPL", Quantity: 1, Price: 120, Date: saleLate,
			ConsumedLots: []LotConsumption{{LotID: "late", Quantity: 1, Price: 100, Date: buyDate}}},
	}}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r, err := CalculateTaxPnL(p, from, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Lots) != 1 || r.Lots[0].LotID != "late" {
		t.Fatalf("expected only 'late' lot, got %+v", r.Lots)
	}
	if r.From == "" {
		t.Errorf("expected From to be echoed back")
	}
	if r.To != "" {
		t.Errorf("expected To to remain empty (unbounded), got %q", r.To)
	}
}

func TestCalculateTaxPnL_OnlyToBound(t *testing.T) {
	buyDate := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	saleEarly := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	saleLate := time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC)
	p := &Portfolio{ID: "test", Transactions: []Transaction{
		{Type: TransactionSell, Symbol: "AAPL", Quantity: 1, Price: 110, Date: saleEarly,
			ConsumedLots: []LotConsumption{{LotID: "early", Quantity: 1, Price: 100, Date: buyDate}}},
		{Type: TransactionSell, Symbol: "AAPL", Quantity: 1, Price: 120, Date: saleLate,
			ConsumedLots: []LotConsumption{{LotID: "late", Quantity: 1, Price: 100, Date: buyDate}}},
	}}
	to := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r, err := CalculateTaxPnL(p, time.Time{}, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Lots) != 1 || r.Lots[0].LotID != "early" {
		t.Fatalf("expected only 'early' lot, got %+v", r.Lots)
	}
}

func TestCalculateTaxPnL_InvalidRange_ToBeforeFrom(t *testing.T) {
	p := &Portfolio{ID: "test"}
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err := CalculateTaxPnL(p, from, to)
	if !errors.Is(err, ErrPeriodInvalidRange) {
		t.Fatalf("expected ErrPeriodInvalidRange, got %v", err)
	}
}

func TestCalculateTaxPnL_LotsSortedBySaleDateThenSymbol(t *testing.T) {
	buyDate := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	saleLater := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	saleEarlier := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	p := &Portfolio{ID: "test", Transactions: []Transaction{
		{Type: TransactionSell, Symbol: "MSFT", Quantity: 1, Price: 110, Date: saleLater,
			ConsumedLots: []LotConsumption{{LotID: "later-msft", Quantity: 1, Price: 100, Date: buyDate}}},
		{Type: TransactionSell, Symbol: "AAPL", Quantity: 1, Price: 110, Date: saleEarlier,
			ConsumedLots: []LotConsumption{{LotID: "earlier-aapl", Quantity: 1, Price: 100, Date: buyDate}}},
		{Type: TransactionSell, Symbol: "GOOG", Quantity: 1, Price: 110, Date: saleEarlier,
			ConsumedLots: []LotConsumption{{LotID: "earlier-goog", Quantity: 1, Price: 100, Date: buyDate}}},
	}}
	r, err := CalculateTaxPnL(p, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Lots) != 3 {
		t.Fatalf("expected 3 lots, got %d", len(r.Lots))
	}
	// Earlier sale date first; among same date, alphabetical by symbol.
	if r.Lots[0].LotID != "earlier-aapl" || r.Lots[1].LotID != "earlier-goog" || r.Lots[2].LotID != "later-msft" {
		t.Errorf("unexpected sort order: %s, %s, %s", r.Lots[0].LotID, r.Lots[1].LotID, r.Lots[2].LotID)
	}
}
