package providers

import "errors"

var (
	// ErrNotSupported se devuelve cuando el proveedor no implementa la operación solicitada.
	ErrNotSupported = errors.New("operation not supported by this provider")

	// ErrNotFound se devuelve cuando el símbolo o recurso no existe en el proveedor.
	ErrNotFound = errors.New("symbol or resource not found")

	// ErrUnauthorized se devuelve cuando la API key es inválida o está ausente.
	ErrUnauthorized = errors.New("invalid or missing API credentials")

	// ErrRateLimit se devuelve cuando el proveedor rechaza la petición por exceso de tasa.
	ErrRateLimit = errors.New("API rate limit exceeded")

	// ErrNotConnected se devuelve cuando se invoca un método que requiere conexión activa.
	ErrNotConnected = errors.New("provider is not connected")

	// ErrSubscriptionRequired se devuelve cuando el endpoint requiere un plan de suscripción superior.
	ErrSubscriptionRequired = errors.New("endpoint requires a higher subscription plan")
)
