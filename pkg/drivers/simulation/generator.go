package simulation

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"math"
	"math/rand/v2"
	"strings"
	"time"

	"auris/pkg/market"
)

const (
	tradingDaysPerYear    = 252.0
	marketFactorDailyVol  = 0.008  // daily stdev of the shared "market sentiment" factor
	intrabarBucketsPerDay = 2340.0 // ~6.5h session sampled every 10s, used to scale live-quote wiggle
)

// generator derives all simulated market data from a fixed universe of
// instrument parameters plus a shared epoch/seed. Every method is pure and
// deterministic: the same (symbol, date) always yields the same numbers, so
// repeated demo runs and tests are reproducible.
type generator struct {
	epoch       time.Time
	marketSeed  string
	instruments []instrumentDef
}

func newGenerator(u universeFile, epoch time.Time) *generator {
	return &generator{epoch: epoch, marketSeed: u.MarketFactorSeed, instruments: u.Instruments}
}

// find looks up an instrument by symbol, case-insensitively.
func (g *generator) find(symbol string) (instrumentDef, bool) {
	for _, inst := range g.instruments {
		if strings.EqualFold(inst.Symbol, symbol) {
			return inst, true
		}
	}
	return instrumentDef{}, false
}

func (i instrumentDef) assetType() market.AssetType {
	switch i.Type {
	case "crypto":
		return market.AssetTypeCrypto
	case "etf":
		return market.AssetTypeETF
	default:
		return market.AssetTypeStock
	}
}

func (i instrumentDef) toInstrument() market.Instrument {
	return market.Instrument{
		Symbol:   i.Symbol,
		Name:     i.Name,
		Exchange: i.Exchange,
		Currency: i.Currency,
		Type:     i.assetType(),
	}
}

// ── deterministic seeding ───────────────────────────────────────────────────

// seedFromString derives a reproducible 128-bit seed from an arbitrary key,
// split into the two uint64 halves rand/v2's PCG source expects.
func seedFromString(s string) (uint64, uint64) {
	h := fnv.New128a()
	_, _ = h.Write([]byte(s))
	sum := h.Sum(nil)
	return binary.BigEndian.Uint64(sum[0:8]), binary.BigEndian.Uint64(sum[8:16])
}

// deterministicRand returns a PRNG seeded solely by the given key parts, so
// the exact same draw sequence is produced every time for the same inputs.
func deterministicRand(parts ...string) *rand.Rand {
	s1, s2 := seedFromString(strings.Join(parts, "|"))
	return rand.New(rand.NewPCG(s1, s2))
}

func truncateDay(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func isTradingDay(t time.Time) bool {
	wd := t.Weekday()
	return wd != time.Saturday && wd != time.Sunday
}

func round2(f float64) float64 {
	return math.Round(f*100) / 100
}

// ── coherent random walk ────────────────────────────────────────────────────

// marketFactorReturn is a single daily draw shared by every instrument on a
// given calendar day — it represents overall market sentiment and is what
// makes the whole simulated universe move together (correlated up/down days)
// instead of each symbol drifting independently like uncorrelated noise.
func (g *generator) marketFactorReturn(date time.Time) float64 {
	r := deterministicRand(g.marketSeed, date.Format(time.DateOnly), "market")
	return r.NormFloat64() * marketFactorDailyVol
}

// dailyReturn combines the instrument's own annualised drift, its beta-scaled
// exposure to the shared market factor, and an idiosyncratic per-symbol
// noise term into one day's simulated return.
func (g *generator) dailyReturn(inst instrumentDef, date time.Time) float64 {
	drift := inst.AnnualDrift / tradingDaysPerYear
	mf := g.marketFactorReturn(date)
	idio := deterministicRand(inst.Symbol, date.Format(time.DateOnly), "idio").NormFloat64() *
		inst.AnnualVolatility / math.Sqrt(tradingDaysPerYear)
	return drift + inst.Beta*mf + idio
}

// dayClose is one trading day's simulated open/close for an instrument.
type dayClose struct {
	date  time.Time
	open  float64
	close float64
	ret   float64
}

// closeSeries walks every trading day from the universe epoch through
// `through` (capped at "now" — simulated data never extends into the future),
// applying dailyReturn cumulatively. It is the single source of truth for an
// instrument's simulated price at any point in time; candles, quotes,
// fundamentals and corporate actions all derive from it.
func (g *generator) closeSeries(inst instrumentDef, through time.Time) []dayClose {
	epoch := truncateDay(g.epoch)
	through = truncateDay(through)
	if now := truncateDay(time.Now()); through.After(now) {
		through = now
	}
	if through.Before(epoch) {
		through = epoch
	}

	series := make([]dayClose, 0, int(through.Sub(epoch).Hours()/24)+1)
	price := inst.BasePrice
	for cur := epoch; !cur.After(through); cur = cur.AddDate(0, 0, 1) {
		if !isTradingDay(cur) {
			continue
		}
		open := price
		var ret float64
		if cur.After(epoch) {
			ret = g.dailyReturn(inst, cur)
			price *= 1 + ret
			if price < 0.01 {
				price = 0.01
			}
		}
		series = append(series, dayClose{date: cur, open: open, close: price, ret: ret})
	}
	return series
}

// buildCandle derives OHLCV for one trading day from its simulated
// open/close, with a small deterministic intraday range and volume — wider
// on days with a larger move, matching how real markets trade more heavily
// when prices swing.
func (g *generator) buildCandle(inst instrumentDef, d dayClose) market.Candle {
	r := deterministicRand(inst.Symbol, d.date.Format(time.DateOnly), "ohlc")
	upPct := 0.002 + math.Abs(d.ret)*0.5 + r.Float64()*0.004
	downPct := 0.002 + math.Abs(d.ret)*0.5 + r.Float64()*0.004
	hi := math.Max(d.open, d.close) * (1 + upPct)
	lo := math.Min(d.open, d.close) * (1 - downPct)

	baseVol := inst.SharesOutstanding * 0.0008
	if baseVol <= 0 {
		baseVol = d.close * 500 // crypto / no shares_outstanding: fall back to a price-scaled baseline
	}
	vol := baseVol * (0.6 + 0.8*r.Float64()) * (1 + math.Abs(d.ret)*4)

	return market.Candle{
		Time:   d.date,
		Open:   round2(d.open),
		High:   round2(hi),
		Low:    round2(lo),
		Close:  round2(d.close),
		Volume: round2(vol),
	}
}

func (g *generator) dailyCandles(inst instrumentDef, from, to time.Time) []market.Candle {
	from = truncateDay(from)
	series := g.closeSeries(inst, to)
	out := make([]market.Candle, 0, len(series))
	for _, d := range series {
		if d.date.Before(from) {
			continue
		}
		out = append(out, g.buildCandle(inst, d))
	}
	return out
}

func timeframeDuration(tf market.Timeframe) (time.Duration, bool) {
	switch tf {
	case market.Timeframe1m:
		return time.Minute, true
	case market.Timeframe5m:
		return 5 * time.Minute, true
	case market.Timeframe15m:
		return 15 * time.Minute, true
	case market.Timeframe1h:
		return time.Hour, true
	case market.Timeframe4h:
		return 4 * time.Hour, true
	default:
		return 0, false
	}
}

// intradayCandles subdivides each trading day's simulated open→close move
// into bars using a Brownian-bridge-style interpolation: the path starts at
// the day's open, ends exactly at its close, and wobbles more in the middle
// of the session than at either edge — a simple but plausible-looking
// intraday shape derived from data intradayCandles already has, without
// tracking any separate intraday parameters.
func (g *generator) intradayCandles(inst instrumentDef, from, to time.Time, tf market.Timeframe) ([]market.Candle, error) {
	dur, ok := timeframeDuration(tf)
	if !ok {
		return nil, fmt.Errorf("unsupported timeframe %q", tf)
	}
	series := g.closeSeries(inst, to)
	fromDay := truncateDay(from)

	var out []market.Candle
	for _, d := range series {
		if d.date.Before(fromDay) {
			continue
		}
		sessionStart := time.Date(d.date.Year(), d.date.Month(), d.date.Day(), 9, 30, 0, 0, time.UTC)
		sessionEnd := sessionStart.Add(6*time.Hour + 30*time.Minute)
		n := int(sessionEnd.Sub(sessionStart) / dur)
		if n < 1 {
			n = 1
		}

		prev := d.open
		for i := 1; i <= n; i++ {
			barTime := sessionStart.Add(time.Duration(i) * dur)
			frac := float64(i) / float64(n)
			target := d.open + (d.close-d.open)*frac
			r := deterministicRand(inst.Symbol, barTime.Format(time.RFC3339), "intraday")
			noiseScale := math.Abs(d.close-d.open)*0.4 + d.close*0.0015
			noise := r.NormFloat64() * noiseScale * math.Sqrt(frac*(1-frac)+0.05)
			barClose := target + noise

			if barTime.Before(from) || barTime.After(to) {
				prev = barClose
				continue
			}
			barOpen := prev
			hi := math.Max(barOpen, barClose) * (1 + r.Float64()*0.001)
			lo := math.Min(barOpen, barClose) * (1 - r.Float64()*0.001)
			baseVol := inst.SharesOutstanding * 0.0008
			if baseVol <= 0 {
				baseVol = d.close * 500
			}
			vol := (baseVol / float64(n)) * (0.5 + r.Float64())
			out = append(out, market.Candle{
				Time:   barTime,
				Open:   round2(barOpen),
				High:   round2(hi),
				Low:    round2(lo),
				Close:  round2(barClose),
				Volume: round2(vol),
			})
			prev = barClose
		}
	}
	return out, nil
}

// quote returns the "live" snapshot for an instrument: the last simulated
// trading day's close, plus a small deterministic wiggle bucketed every 10
// real-world seconds so a polling SubscribeQuotes feed looks alive without
// breaking the day-to-day determinism candles rely on.
func (g *generator) quote(inst instrumentDef, now time.Time) market.Quote {
	series := g.closeSeries(inst, now)
	if len(series) == 0 {
		return market.Quote{Time: now, Last: round2(inst.BasePrice)}
	}
	last := series[len(series)-1]

	bucket := now.UTC().Truncate(10 * time.Second)
	r := deterministicRand(inst.Symbol, bucket.Format(time.RFC3339), "quote")
	wiggle := r.NormFloat64() * inst.AnnualVolatility / math.Sqrt(tradingDaysPerYear*intrabarBucketsPerDay)
	price := last.close * (1 + wiggle)
	spread := 0.0005 + r.Float64()*0.0005

	changePct := last.ret * 100
	if len(series) >= 2 {
		prev := series[len(series)-2]
		changePct = (price/prev.close - 1) * 100
	}

	return market.Quote{
		Time:          now,
		Bid:           round2(price * (1 - spread)),
		Ask:           round2(price * (1 + spread)),
		Last:          round2(price),
		ChangePercent: round2(changePct),
	}
}

func (g *generator) orderBook(inst instrumentDef, now time.Time, depth int) market.OrderBook {
	q := g.quote(inst, now)
	mid := q.Last
	if depth <= 0 {
		depth = 5
	}
	r := deterministicRand(inst.Symbol, now.UTC().Truncate(10*time.Second).Format(time.RFC3339), "book")
	bids := make([]market.OrderBookLevel, 0, depth)
	asks := make([]market.OrderBookLevel, 0, depth)
	for i := 1; i <= depth; i++ {
		step := float64(i) * 0.0007
		vol := (200.0 / float64(i)) * (0.5 + r.Float64())
		bids = append(bids, market.OrderBookLevel{Price: round2(mid * (1 - step)), Volume: round2(vol)})
		asks = append(asks, market.OrderBookLevel{Price: round2(mid * (1 + step)), Volume: round2(vol)})
	}
	return market.OrderBook{Time: now, Bids: bids, Asks: asks}
}

// ticks synthesises a bounded number of trade prints across [from, to] by
// resampling quote() at evenly spaced instants — capped so a wide date range
// doesn't produce an unbounded slice.
func (g *generator) ticks(inst instrumentDef, from, to time.Time) []market.Tick {
	const maxTicks = 300
	if !to.After(from) {
		return nil
	}
	step := to.Sub(from) / maxTicks
	if step < time.Second {
		step = time.Second
	}

	out := make([]market.Tick, 0, maxTicks)
	prevPrice := g.quote(inst, from).Last
	for t := from; !t.After(to) && len(out) < maxTicks; t = t.Add(step) {
		r := deterministicRand(inst.Symbol, t.UTC().Format(time.RFC3339Nano), "tick")
		price := g.quote(inst, t).Last
		side := "buy"
		switch {
		case price < prevPrice:
			side = "sell"
		case price == prevPrice && r.Float64() < 0.5:
			side = "sell"
		}
		out = append(out, market.Tick{
			Time:   t,
			Price:  round2(price),
			Volume: round2(1 + r.Float64()*20),
			Side:   side,
		})
		prevPrice = price
	}
	return out
}

func (g *generator) fundamentals(inst instrumentDef, now time.Time) market.Fundamental {
	series := g.closeSeries(inst, now)
	price := inst.BasePrice
	if len(series) > 0 {
		price = series[len(series)-1].close
	}
	var pe float64
	if inst.EPS > 0 {
		pe = price / inst.EPS
	}
	return market.Fundamental{
		Symbol:           inst.Symbol,
		PERatio:          round2(pe),
		EPS:              inst.EPS,
		MarketCap:        round2(price * inst.SharesOutstanding),
		DividendYieldTTM: inst.DividendYield,
		Beta:             inst.Beta,
	}
}

// corporateActions emits a deterministic quarterly dividend (every ~63
// trading days, roughly one quarter) for instruments with a positive
// dividend yield. No stock splits are simulated.
func (g *generator) corporateActions(inst instrumentDef, from, to time.Time) []market.CorporateAction {
	if inst.DividendYield <= 0 {
		return nil
	}
	series := g.closeSeries(inst, to)
	var actions []market.CorporateAction
	for i, d := range series {
		if i == 0 || i%63 != 0 {
			continue
		}
		if d.date.Before(from) || d.date.After(to) {
			continue
		}
		actions = append(actions, market.CorporateAction{
			Date:        d.date,
			Type:        "dividend",
			Value:       round2(d.close * inst.DividendYield / 4),
			Description: fmt.Sprintf("Simulated quarterly dividend for %s", inst.Symbol),
		})
	}
	return actions
}
