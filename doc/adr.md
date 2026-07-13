# Auris — Architecture Decision Records

Registro de decisiones de diseño del proyecto Auris. Cada ADR sigue el formato:

- **Status** — `accepted` (vigente), `pending` (activa pero no resuelta), `superseded` (reemplazada por otra).
- **Context** — el problema o la tensión que motiva la decisión.
- **Decision** — lo que se acordó.
- **Consequences** — efectos de la decisión (positivos y negativos).
- **Open questions** — solo si `Status == pending`.

Origen histórico: las cuatro primeras ADR (`DD-1`..`DD-4`) se cerraron el **2026-07-02** y estaban marcadas `[x]` en `TODO.md` antes de la reestructuración del 2026-07-10. `DD-5` permanece `pending` desde su apertura el 2026-07-09.

---

## DD-1 — Indicadores técnicos reciben `prices[]` directamente

- **Status**: accepted
- **Opened**: 2026-07-02
- **Closed**: 2026-07-02

### Context

Las herramientas técnicas (`calculate_sma`, `calculate_ema`, `calculate_rsi`, `calculate_macd`, `calculate_bollinger_bands`) podían operar de dos formas distintas: (a) pedir una serie de precios `prices[]` que el LLM ya obtiene vía `market_get_candles`, o (b) hacer su propia llamada al market provider. El LLM es quien encadena las llamadas, así que la pregunta es si la tool debe asumir el coste de la llamada al provider o dejar al LLM hacerlo.

### Decision

Los indicadores técnicos reciben `prices[]` directamente. El LLM encadena `market_get_candles` → `calculate_*`. Implementado así desde el inicio en `pkg/agent/math.go` (`calcSMA`/`calcEMA`/`calcRSI`/`calcMACD`/`calcBollingerBands`): la serie llega vía `args["prices"]` en el dispatch (`pkg/agent/tools.go`), sin llamar a `a.market`.

### Consequences

- Tests puros sin dependencia de market provider — más rápidos, más deterministas.
- Mayor control sobre la serie analizada (la tool no decide timeframe ni ventana).
- El LLM gasta un turno extra del bucle ReAct para la llamada a `market_get_candles` cuando el usuario no le ha dado precios; aceptado por la simplicidad del modelo.

---

## DD-2 — `portfolio_calculate_metrics` con auto-fetch de quotes

- **Status**: accepted
- **Opened**: 2026-07-02
- **Closed**: 2026-07-02

### Context

`portfolio_calculate_metrics` necesita el precio actual de cada holding para calcular valor total, P&L latente y pesos. Dos opciones: (a) auto-fetch vía `market.GetQuote` por holding (más preciso, más lento, riesgo de rate-limit), (b) aceptar un snapshot de precios pasado por el LLM (más rápido, pero exige al LLM haber hecho los `market_get_quote` previos y saber pasarlos bien).

### Decision

Comportamiento por defecto: **auto-fetch**. Parámetro opcional `quotes` (`map[string]{last, dividend_yield_ttm?, beta?}`) en el schema y dispatch de `portfolio_calculate_metrics` (`pkg/agent/tools.go`) — los símbolos incluidos en el snapshot se usan directamente y saltan el auto-fetch; el resto sigue la ruta de `market.GetQuote`/`GetFundamentals`.

### Consequences

- El caso típico (el LLM pide métricas sin preparar nada) funciona sin preparación.
- Los símbolos incluidos en el snapshot son inmunes a rate-limit (el LLM los trae "gratis" en el contexto).
- Test dedicado: `TestRegression_DD2_PortfolioMetricsQuotesSnapshotSkipsAutoFetch` en `pkg/agent/math_test.go`.

---

## DD-3 — `monte_carlo_simulation` recibe drift/volatility del LLM

- **Status**: accepted
- **Opened**: 2026-07-02
- **Closed**: 2026-07-02

### Context

La herramienta `monte_carlo_simulation` (MATH-14) necesita `drift_annual` y `volatility_annual` para alimentar el motor GBM. Podía calcularlos internamente pidiendo una serie histórica al market provider, o recibirlos como parámetros.

### Decision

El LLM los calcula (encadenando, p.ej., `calculate_volatility` sobre los retornos diarios) y los pasa como `drift_annual`/`volatility_annual`. La tool solo ejecuta el motor GBM (`S(T) = S(0)·exp((μ-½σ²)T + σ√T·Z)`, solución cerrada, no simulación paso a paso). Implementada y cerrada: ver nota de sesión 2026-07-05 (MATH-14) en `doc/task_completed.md`.

### Consequences

- Firma del tool fijada y testeable sin market provider.
- Desacopla el motor de simulación del cálculo de parámetros — `drift`/`volatility` se vuelven reutilizables por otras tools en el futuro.
- El LLM paga un turno extra del bucle ReAct para calcularlos; aceptado por la coherencia con `calcVolatility`/`calcSharpe` ya existentes.

---

## DD-4 — Tools técnicos en el system prompt

- **Status**: accepted
- **Opened**: 2026-07-02
- **Closed**: 2026-07-02

### Context

El system prompt del agente (`pkg/agent/prompts.go`) no mencionaba explícitamente los indicadores técnicos. Riesgo: el LLM decidiera por defecto omitir `calculate_sma`/`calculate_rsi`/etc. en favor de razonamiento puramente verbal sobre las velas que ya había traído vía `market_get_candles`.

### Decision

Añadidas dos frases a la sección "Uso de herramientas" del system prompt indicando cuándo usar los indicadores técnicos (cuando el usuario pida análisis técnico, momentum, volatilidad derivada de una serie, etc.) y cuándo usar las herramientas de cartera (valoración, P&L, concentración, rebalanceo).

### Consequences

- El LLM tiene una guía explícita para elegir entre herramientas técnicas y de cartera.
- Sin cambios funcionales — solo texto del prompt.
- Nota: las dos frases se añadieron a `pkg/agent/prompts.go`; ver guía de prompts en `.claude/skills/auris-system-prompts/SKILL.md` para las restricciones de mantenerlas efectivas con modelos pequeños/cuantizados.

---

## DD-5 — Orden de cascada FMP/EODHD si el límite real de EODHD es 20 calls/día

- **Status**: pending
- **Opened**: 2026-07-09

### Context

FEAT-7 (segundo market provider, `pkg/drivers/eodhd/`) planteó dos arquitecturas posibles:

1. **Cascada con fallback FMP→EODHD**: ambos activos, FMP primario (free tier ~250 calls/día, cobertura amplia pero con huecos BME en el plan free), EODHD secundario (rescata cuando FMP no cubre el símbolo). Probado en vivo contra la API real con el token del usuario.
2. **Provider primario único configurable**: el usuario elige uno, sin cascada. Más simple pero pierde la resiliencia.

La elección entre ambas arquitecturas depende del límite real del free tier de EODHD:

- **1200 calls/día** (lo que reporta el header HTTP `x-ratelimit-limit: 1200`): EODHD solo bastaría como primario para un usuario medio; la cascada sería opcional / para cobertura US que EODHD no tenga.
- **20 calls/día** (lo que dice un email recibido por el usuario de EODHD: "Validity: 20 API calls per day + 500 welcome calls"): EODHD pasa de "secundario de cobertura" a "recurso escaso", y la cascada FMP→EODHD deja de ser una mejora opcional y se vuelve **obligatoria** (con 20 calls/día EODHD solo cubre un puñado de símbolos del usuario, no todo el universo; FMP free con ~250 calls/día sostiene el grueso).

### Decision (vigente hasta resolución)

Cascada con fallback FMP→EODHD, primario FMP, secundario EODHD, basada en el header HTTP verificado en la sesión 2026-07-09. Implementada tal cual en `pkg/agent/market_chain.go` y `pkg/tui/app.go::buildMarketProvider`.

### Consequences

- Resiliencia ante huecos de cobertura (EODHD cubre BME donde FMP free falla).
- Distribución de cuota entre dos proveedores (si EODHD tiene 1200 calls/día, la cascada es cómoda; si tiene 20, FMP lleva el grueso y EODHD se reserva para lo que FMP no cubra).
- Si el header HTTP está desactualizado y el email es correcto, el sistema sigue funcionando pero EODHD quedará casi siempre sin cuota al final del día — no se rompe, solo se degrada silenciosamente.

### Open questions

- Verificar el email de EODHD: ¿es phishing, bienvenida genérica de un tier distinto, o informativo real del plan free?
- Si se confirma 20 calls/día, evaluar si la decisión debe endurecerse (p.ej. marcar el secundario como "solo a partir del segundo fallo consecutivo del primario" para racionar) o quedarse como está.
- Acciones derivadas no resueltas: ninguna tarea abierta en `TODO.md` derivada de DD-5.

---

## DD-6 — Cifrado de carteras/sesiones: salt independiente, clave a nivel de paquete, re-cifrado inmediato

- **Status**: accepted
- **Opened**: 2026-07-13
- **Closed**: 2026-07-13

### Context

FEAT-16 extiende el cifrado AES-256-GCM que ya protegía las API keys (`pkg/config/crypto.go`) a carteras (`pkg/portfolio/`) y sesiones de chat (`pkg/config/session.go`), ninguna de las cuales tenía cifrado alguno. Tres decisiones de diseño no eran obvias a partir del código existente:

1. **Salt**: `config.Save` regenera el salt del KDF de credenciales (`diskKDF.Salt`) en cada llamada — reutilizarlo para el cifrado de almacenamiento habría cambiado la clave derivada en cada guardado no relacionado, rompiendo archivos ya cifrados.
2. **Threading de la clave**: `SavePortfolio`/`LoadPortfolio`/`SaveSession`/`LoadSession` se llaman desde muchos puntos (`app.go`, `screen_portfolio_*.go`, `pkg/agent/tools.go`) sin parámetro de passphrase/clave. Añadirlo a todas las firmas habría sido un diff grande e invasivo.
3. **Qué hacer con los archivos existentes al activar/desactivar el toggle**: re-cifrar todo de inmediato (síncrono) vs. aplicar solo a partir del siguiente guardado natural de cada archivo.

### Decision

1. `AurisConfig.StorageSalt` es un campo independiente del salt de credenciales, generado una sola vez (al activar el cifrado por primera vez) y estable entre guardados — nunca se regenera en un `Save` no relacionado con el toggle.
2. La clave activa vive en estado a nivel de paquete (`config.SetStorageKey`/`StorageKey`), siguiendo la misma convención ya usada para overrides de test (`portfoliosDirOverride`/`SetPortfoliosDirForTest`) en lugar de pasar la clave por parámetro en cada llamada. `nil` = cifrado desactivado (comportamiento previo, sin cambios).
3. Activar/desactivar el toggle, o cambiar la passphrase con el cifrado activo, **re-cifra/descifra inmediatamente** todos los archivos existentes (`portfolio.ReencryptAllPortfolios`/`config.ReencryptAllSessions`, invocados por `AppModel.setStorageEncryption` y por la transición de `ScreenChangePassphrase`) — operación síncrona iniciada por el usuario, no un proceso en background. El algoritmo es idempotente ante reintentos: para cada archivo prueba primero la clave antigua y, si falla y hay clave nueva, prueba la nueva antes de darse por vencido (el archivo puede ya estar migrado de un intento parcial anterior).

Además, el cifrado de almacenamiento es **opt-out, no opt-in**: los setups nuevos activan `EncryptStorage` por defecto (`ScreenPassphrase`, solo alcanzable en el asistente de primer arranque — los usuarios recurrentes pasan por `ScreenUnlock`, así que el valor por defecto es seguro de aplicar sin condición ahí).

La detección de archivo cifrado vs. plano usa un sobre explícito (`config.EncryptedEnvelope`, `{"encrypted":true,"data":"..."}`) en vez de "intenta parsear JSON y si falla asume cifrado" — un archivo plano legado no tiene esas claves, así que `Encrypted` queda en `false` y cae al parseo directo, sin necesidad de una pasada de migración separada.

### Consequences

- Los archivos de carteras/sesiones cifrados y en claro conviven sin problema durante y después de una migración parcial — no hay estado "roto" intermedio.
- El estado de clave a nivel de paquete es una dependencia oculta (hay que recordar llamar `SetStorageKey` tras desbloquear/cambiar passphrase/toggle) — aceptable en una app TUI de un solo proceso y un solo usuario, documentado en `CLAUDE.md`.
- Un fallo a mitad del batch de re-cifrado deja algunos archivos migrados y otros no; no hay rollback transaccional (desproporcionado para una feature Tier C) — pero el reintento es seguro gracias al diseño idempotente.
