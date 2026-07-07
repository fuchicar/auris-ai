package market

import "time"

type AssetType string

const (
	AssetTypeStock  AssetType = "stock"
	AssetTypeETF    AssetType = "etf"
	AssetTypeFuture AssetType = "future"
	AssetTypeForex  AssetType = "forex"
	AssetTypeCrypto AssetType = "crypto"
)

type Instrument struct {
	Symbol       string
	ISIN         string
	Name         string
	Exchange     string
	Currency     string
	Type         AssetType
	TradingHours TradingHours
}

type TradingHours struct {
	Open     time.Time
	Close    time.Time
	Timezone string
}

type Candle struct {
	Time   time.Time
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64
}

type Tick struct {
	Time   time.Time
	Price  float64
	Volume float64
	Side   string // "buy" | "sell" | "unknown"
}

type Quote struct {
	Time time.Time
	Bid  float64
	Ask  float64
	Last float64
	// ChangePercent is the percentage change vs the previous close (e.g.
	// 1.23 for +1.23%). It is 0 when the underlying driver does not supply
	// this figure — callers must treat 0 as "unknown", not "unchanged",
	// unless they've separately confirmed the driver populates it.
	ChangePercent float64
}

type OrderBookLevel struct {
	Price  float64
	Volume float64
}

type OrderBook struct {
	Time time.Time
	Bids []OrderBookLevel
	Asks []OrderBookLevel
}

type Timeframe string

const (
	Timeframe1m  Timeframe = "1m"
	Timeframe5m  Timeframe = "5m"
	Timeframe15m Timeframe = "15m"
	Timeframe1h  Timeframe = "1h"
	Timeframe4h  Timeframe = "4h"
	Timeframe1d  Timeframe = "1d"
)

type CorporateAction struct {
	Date        time.Time
	Type        string // "dividend" | "split" | "rights"
	Value       float64
	Description string
}

type Fundamental struct {
	Symbol           string
	PERatio          float64
	EPS              float64
	MarketCap        float64
	DividendYieldTTM float64 // TTM dividend yield as a fraction (e.g. 0.025 = 2.5 %)
	Beta             float64 // beta vs broad market benchmark; 0 if unknown
}
