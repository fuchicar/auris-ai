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
