package finance

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

// --- ROI -----------------------------------------------------------------------

type RoiResult struct {
	ROIPercent float64 `json:"roi_percent"`
	ProfitLoss float64 `json:"profit_loss"`
	Summary    string  `json:"summary"`
}

func CalcROI(costBasis, currentValue float64) (RoiResult, error) {
	if err := ValidateFinite("cost_basis", costBasis); err != nil {
		return RoiResult{}, err
	}
	if err := ValidateFinite("current_value", currentValue); err != nil {
		return RoiResult{}, err
	}
	if costBasis == 0 {
		return RoiResult{}, errors.New("cost_basis cannot be zero")
	}
	pl := currentValue - costBasis
	roi := pl / costBasis * 100
	dir := "profit"
	if pl < 0 {
		dir = "loss"
	}
	return RoiResult{
		ROIPercent: Round2(roi),
		ProfitLoss: Round2(pl),
		Summary:    fmt.Sprintf("%.2f%% ROI — %s of %.2f on cost basis %.2f", roi, dir, math.Abs(pl), costBasis),
	}, nil
}

// --- CAGR ----------------------------------------------------------------------

type CagrResult struct {
	CAGRPercent float64 `json:"cagr_percent"`
	Summary     string  `json:"summary"`
}

func CalcCAGR(initialValue, finalValue, years float64) (CagrResult, error) {
	if err := ValidatePositive("initial_value", initialValue); err != nil {
		return CagrResult{}, err
	}
	if err := ValidateNonNegative("final_value", finalValue); err != nil {
		return CagrResult{}, err
	}
	if err := ValidatePositive("years", years); err != nil {
		return CagrResult{}, err
	}
	cagr := (math.Pow(finalValue/initialValue, 1/years) - 1) * 100
	return CagrResult{
		CAGRPercent: Round2(cagr),
		Summary:     fmt.Sprintf("%.2f%% CAGR over %.1f years (%.2f → %.2f)", cagr, years, initialValue, finalValue),
	}, nil
}

// --- P&L -----------------------------------------------------------------------

type PnlResult struct {
	PnLAbsolute   float64 `json:"pnl_absolute"`
	PnLPercent    float64 `json:"pnl_percent"`
	PositionValue float64 `json:"position_value"`
	Summary       string  `json:"summary"`
}

func CalcPnL(entryPrice, currentPrice, quantity float64, positionType string) (PnlResult, error) {
	if positionType != "long" && positionType != "short" {
		return PnlResult{}, fmt.Errorf("position_type must be \"long\" or \"short\", got %q", positionType)
	}
	if err := ValidatePositive("entry_price", entryPrice); err != nil {
		return PnlResult{}, err
	}
	if err := ValidatePositive("current_price", currentPrice); err != nil {
		return PnlResult{}, err
	}
	if err := ValidateFinite("quantity", quantity); err != nil {
		return PnlResult{}, err
	}
	qty := math.Abs(quantity)
	if qty == 0 {
		return PnlResult{}, errors.New("quantity cannot be zero")
	}
	var pnl float64
	if positionType == "long" {
		pnl = (currentPrice - entryPrice) * qty
	} else {
		pnl = (entryPrice - currentPrice) * qty
	}
	posValue := currentPrice * qty
	pnlPct := pnl / (entryPrice * qty) * 100
	dir := "profit"
	if pnl < 0 {
		dir = "loss"
	}
	summary := fmt.Sprintf("%s position: %.2f%% %s (P&L: %.2f, position value: %.2f)", positionType, math.Abs(pnlPct), dir, pnl, posValue)
	if quantity < 0 {
		summary += " [WARNING: quantity was negative and has been treated as positive]"
	}
	return PnlResult{
		PnLAbsolute:   Round2(pnl),
		PnLPercent:    Round2(pnlPct),
		PositionValue: Round2(posValue),
		Summary:       summary,
	}, nil
}

// --- DCF -----------------------------------------------------------------------

type DcfResult struct {
	IntrinsicValueTotal    float64 `json:"intrinsic_value_total"`
	IntrinsicValuePerShare float64 `json:"intrinsic_value_per_share"`
	TerminalValue          float64 `json:"terminal_value"`
	PVOfCashflows          float64 `json:"pv_of_cashflows"`
	Summary                string  `json:"summary"`
}

func CalcDCF(freeCashFlows []float64, discountRate, terminalGrowthRate, sharesOutstanding float64) (DcfResult, error) {
	if len(freeCashFlows) == 0 {
		return DcfResult{}, errors.New("free_cash_flows must not be empty")
	}
	if err := ValidateFiniteAll("free_cash_flows", freeCashFlows); err != nil {
		return DcfResult{}, err
	}
	if err := ValidateRate("discount_rate", discountRate); err != nil {
		return DcfResult{}, err
	}
	if err := ValidateRate("terminal_growth_rate", terminalGrowthRate); err != nil {
		return DcfResult{}, err
	}
	if err := ValidateNonNegative("shares_outstanding", sharesOutstanding); err != nil {
		return DcfResult{}, err
	}
	if discountRate <= terminalGrowthRate {
		return DcfResult{}, errors.New("discount_rate must be greater than terminal_growth_rate to avoid infinite terminal value")
	}
	allNegative := true
	for _, fcf := range freeCashFlows {
		if fcf > 0 {
			allNegative = false
			break
		}
	}
	if allNegative {
		return DcfResult{}, errors.New("all free_cash_flows are negative or zero; DCF intrinsic value would be meaningless — provide at least one positive FCF")
	}
	pvFCF := 0.0
	for i, fcf := range freeCashFlows {
		pvFCF += fcf / math.Pow(1+discountRate, float64(i+1))
	}
	n := len(freeCashFlows)
	lastFCF := freeCashFlows[n-1]
	tv := lastFCF * (1 + terminalGrowthRate) / (discountRate - terminalGrowthRate)
	pvTV := tv / math.Pow(1+discountRate, float64(n))
	total := pvFCF + pvTV
	perShare := 0.0
	if sharesOutstanding > 0 {
		perShare = total / sharesOutstanding
	}
	summary := fmt.Sprintf("DCF intrinsic value: %.2f (%.2f/share); PV of FCFs: %.2f, PV of terminal value: %.2f",
		total, perShare, pvFCF, pvTV)
	if lastFCF < 0 {
		summary += " [WARNING: last FCF is negative, making the terminal value negative — results may not be economically meaningful]"
	}
	return DcfResult{
		IntrinsicValueTotal:    Round2(total),
		IntrinsicValuePerShare: Round2(perShare),
		TerminalValue:          Round2(tv),
		PVOfCashflows:          Round2(pvFCF),
		Summary:                summary,
	}, nil
}

// --- Multiples -----------------------------------------------------------------

type MultiplesResult struct {
	PER          *float64 `json:"per,omitempty"`
	PBV          *float64 `json:"pbv,omitempty"`
	EVEbitda     *float64 `json:"ev_ebitda,omitempty"`
	EVRevenue    *float64 `json:"ev_revenue,omitempty"`
	PriceToSales *float64 `json:"price_to_sales,omitempty"`
	Summary      string   `json:"summary"`
}

func CalcMultiples(price, eps, bookValuePerShare, ebitda, enterpriseValue, revenue float64) (MultiplesResult, error) {
	if err := ValidatePositive("price", price); err != nil {
		return MultiplesResult{}, err
	}
	if err := ValidateFinite("eps", eps); err != nil {
		return MultiplesResult{}, err
	}
	if err := ValidateFinite("book_value_per_share", bookValuePerShare); err != nil {
		return MultiplesResult{}, err
	}
	if err := ValidateFinite("ebitda", ebitda); err != nil {
		return MultiplesResult{}, err
	}
	if err := ValidateFinite("enterprise_value", enterpriseValue); err != nil {
		return MultiplesResult{}, err
	}
	if err := ValidateFinite("revenue", revenue); err != nil {
		return MultiplesResult{}, err
	}
	r := MultiplesResult{}
	var parts []string
	if eps != 0 {
		r.PER = f64ptr(price / eps)
		parts = append(parts, fmt.Sprintf("P/E=%.2f", *r.PER))
	}
	if bookValuePerShare != 0 {
		r.PBV = f64ptr(price / bookValuePerShare)
		parts = append(parts, fmt.Sprintf("P/BV=%.2f", *r.PBV))
	}
	if ebitda != 0 {
		r.EVEbitda = f64ptr(enterpriseValue / ebitda)
		parts = append(parts, fmt.Sprintf("EV/EBITDA=%.2f", *r.EVEbitda))
	}
	if revenue != 0 {
		r.EVRevenue = f64ptr(enterpriseValue / revenue)
		r.PriceToSales = f64ptr(price / revenue)
		parts = append(parts, fmt.Sprintf("EV/Rev=%.2f P/S=%.2f", *r.EVRevenue, *r.PriceToSales))
	}
	if len(parts) == 0 {
		r.Summary = "no multiples computed: all denominators are zero"
	} else {
		r.Summary = strings.Join(parts, ", ")
	}
	return r, nil
}

func f64ptr(v float64) *float64 { r := Round2(v); return &r }

// --- Price / Free Cash Flow -----------------------------------------------------

type PfcfResult struct {
	PFCF     float64 `json:"pfcf"`
	Currency string  `json:"currency,omitempty"`
	Summary  string  `json:"summary"`
}

func CalcPFCF(price, fcfPerShare float64, currency string) (PfcfResult, error) {
	if err := ValidatePositive("price", price); err != nil {
		return PfcfResult{}, err
	}
	if err := ValidatePositive("free_cash_flow_per_share", fcfPerShare); err != nil {
		return PfcfResult{}, err
	}
	pfcf := Round2(price / fcfPerShare)
	return PfcfResult{
		PFCF:     pfcf,
		Currency: currency,
		Summary:  fmt.Sprintf("P/FCF=%.2f", pfcf),
	}, nil
}

// --- PEG ratio -------------------------------------------------------------------

type PegResult struct {
	PEG            float64 `json:"peg"`
	Interpretation string  `json:"interpretation"`
	Summary        string  `json:"summary"`
}

func CalcPEG(peRatio, growthRatePercent float64) (PegResult, error) {
	if err := ValidatePositive("pe_ratio", peRatio); err != nil {
		return PegResult{}, err
	}
	if err := ValidateFinite("growth_rate_percent", growthRatePercent); err != nil {
		return PegResult{}, err
	}
	if growthRatePercent == 0 {
		return PegResult{}, errors.New("growth_rate_percent cannot be zero")
	}
	peg := Round2(peRatio / growthRatePercent)
	interp := "reasonable"
	switch {
	case peg < 1:
		interp = "undervalued"
	case peg > 2:
		interp = "overvalued"
	}
	summary := fmt.Sprintf("PEG=%.2f (%s)", peg, interp)
	if growthRatePercent < 0 {
		summary += " — WARNING: negative growth rate makes PEG uninterpretable as a valuation signal"
	}
	return PegResult{
		PEG:            peg,
		Interpretation: interp,
		Summary:        summary,
	}, nil
}

// --- Dividend yield ---------------------------------------------------------------

type DividendYieldResult struct {
	AnnualDividend float64 `json:"annual_dividend"`
	YieldPercent   float64 `json:"yield_percent"`
	Summary        string  `json:"summary"`
}

func CalcDividendYield(price, annualDividendPerShare float64, quarterlyDividends []float64) (DividendYieldResult, error) {
	if err := ValidatePositive("price", price); err != nil {
		return DividendYieldResult{}, err
	}
	if err := ValidateNonNegative("annual_dividend_per_share", annualDividendPerShare); err != nil {
		return DividendYieldResult{}, err
	}
	if err := ValidateNonNegativeAll("quarterly_dividends", quarterlyDividends); err != nil {
		return DividendYieldResult{}, err
	}
	annual := annualDividendPerShare
	source := "annual_dividend_per_share"
	if len(quarterlyDividends) > 0 {
		if len(quarterlyDividends) != 4 {
			return DividendYieldResult{}, errors.New("quarterly_dividends must contain exactly 4 values")
		}
		annual = 0
		for _, d := range quarterlyDividends {
			annual += d
		}
		source = "trailing twelve months (sum of quarterly_dividends)"
	}
	if annual <= 0 {
		return DividendYieldResult{}, errors.New("must provide annual_dividend_per_share or quarterly_dividends")
	}
	y := Round4(annual / price * 100)
	return DividendYieldResult{
		AnnualDividend: Round4(annual),
		YieldPercent:   y,
		Summary:        fmt.Sprintf("%.4f%% dividend yield, based on %s", y, source),
	}, nil
}

// --- Dividend growth ---------------------------------------------------------------

type DividendGrowthResult struct {
	CAGRPercent float64 `json:"cagr_percent"`
	Summary     string  `json:"summary"`
}

func CalcDividendGrowth(dividends []float64) (DividendGrowthResult, error) {
	if len(dividends) < 2 {
		return DividendGrowthResult{}, errors.New("dividends must contain at least 2 chronological values")
	}
	if err := ValidateNonNegativeAll("dividends", dividends); err != nil {
		return DividendGrowthResult{}, err
	}
	first, last := dividends[0], dividends[len(dividends)-1]
	if first <= 0 {
		return DividendGrowthResult{}, errors.New("dividends[0] must be greater than zero")
	}
	years := float64(len(dividends) - 1)
	cagr := (math.Pow(last/first, 1/years) - 1) * 100
	return DividendGrowthResult{
		CAGRPercent: Round2(cagr),
		Summary:     fmt.Sprintf("%.2f%% dividend CAGR over %d periods (%.4f → %.4f)", cagr, len(dividends)-1, first, last),
	}, nil
}
