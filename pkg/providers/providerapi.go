package providers

import (
	"context"
	"time"
)

type ProviderAPI interface {
	// Name returns the short human-readable display name of the provider.
	Name() string

	// DocsURL returns the URL to the provider's API key documentation page.
	DocsURL() string

	// Description returns a prose description of the provider.
	Description() string

	// -------------------------
	// Autenticación y sesión
	// -------------------------

	// Connect inicializa la conexión y autentica con el proveedor.
	Connect(ctx context.Context) error

	// Disconnect cierra la sesión y libera recursos.
	Disconnect(ctx context.Context) error

	// RefreshToken renueva las credenciales de sesión si están próximas a expirar.
	RefreshToken(ctx context.Context) error

	// -------------------------
	// Descubrimiento de instrumentos
	// -------------------------

	// SearchInstrument busca instrumentos por nombre, ticker o ISIN.
	SearchInstrument(ctx context.Context, query string) ([]Instrument, error)

	// GetInstrument devuelve los metadatos completos de un símbolo concreto.
	GetInstrument(ctx context.Context, symbol string) (Instrument, error)

	// ListInstruments devuelve todos los instrumentos disponibles, opcionalmente filtrados por tipo.
	ListInstruments(ctx context.Context, assetType AssetType) ([]Instrument, error)

	// -------------------------
	// Datos históricos
	// -------------------------

	// GetCandles devuelve velas OHLCV para un símbolo, rango temporal y granularidad.
	GetCandles(ctx context.Context, symbol string, from, to time.Time, tf Timeframe) ([]Candle, error)

	// GetTicks devuelve datos de tick a tick para un símbolo y rango temporal.
	GetTicks(ctx context.Context, symbol string, from, to time.Time) ([]Tick, error)

	// GetCorporateActions devuelve splits, dividendos y otros eventos corporativos.
	GetCorporateActions(ctx context.Context, symbol string, from, to time.Time) ([]CorporateAction, error)

	// -------------------------
	// Snapshot (consulta puntual)
	// -------------------------

	// GetQuote devuelve el bid/ask/last en el momento de la consulta.
	GetQuote(ctx context.Context, symbol string) (Quote, error)

	// GetOrderBook devuelve el estado actual del libro de órdenes.
	GetOrderBook(ctx context.Context, symbol string, depth int) (OrderBook, error)

	// -------------------------
	// Streaming (tiempo real)
	// -------------------------

	// SubscribeQuotes suscribe al feed de quotes en tiempo real.
	// Los updates se emiten por el canal devuelto hasta que se cancele el contexto.
	SubscribeQuotes(ctx context.Context, symbol string) (<-chan Quote, error)

	// SubscribeTrades suscribe al feed de operaciones ejecutadas en tiempo real.
	SubscribeTrades(ctx context.Context, symbol string) (<-chan Tick, error)

	// SubscribeOrderBook suscribe a las actualizaciones del libro de órdenes.
	SubscribeOrderBook(ctx context.Context, symbol string, depth int) (<-chan OrderBook, error)

	// -------------------------
	// Fundamentales
	// -------------------------

	// GetFundamentals devuelve los datos fundamentales de un instrumento.
	GetFundamentals(ctx context.Context, symbol string) (Fundamental, error)

	// -------------------------
	// Gestión de la conexión
	// -------------------------

	// Ping comprueba que la conexión con el proveedor sigue activa.
	Ping(ctx context.Context) error

	// IsConnected informa del estado actual de la conexión.
	IsConnected() bool
}
