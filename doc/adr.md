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

---

## DD-7 — Modo simulación: flag de configuración en vez de proveedor seleccionable, extracción de `news.Source`

- **Status**: accepted
- **Opened**: 2026-07-13
- **Closed**: 2026-07-13

### Context

FEAT-19 pide un "modo simulación" para evaluar el agente sin API key de ningún proveedor de mercado real. Dos arquitecturas posibles:

1. **Registrar "simulación" como una entrada más en `registry.AllMarket()`**, seleccionable junto a FMP/EODHD en `ScreenProvider`/`MarketProviderManageModel`. Reutiliza toda la UI de selección existente, pero exige lógica de exclusión mutua nueva (el enunciado prohíbe combinar simulación con proveedores reales) y no hay forma limpia de "recordar" los proveedores reales configurados mientras se usa simulación sin builder ad-hoc en esas pantallas.
2. **`AurisConfig.SimulationMode bool`**, ortogonal a `cfg.Providers`/`cfg.ActiveProvider`, que cortocircuita `AppModel.buildMarketProvider()` antes de tocar la cadena real.

También hacía falta decidir cómo alimentar `fetch_news` con titulares sintéticos: `pkg/agent.Agent.news` era `*news.Provider` (tipo concreto, no interfaz), así que no había forma de inyectar un generador falso sin cambiar esa firma.

### Decision

1. Modo simulación es el flag `AurisConfig.SimulationMode bool` (mismo patrón que `EncryptStorage`: campo simple, copiado tal cual en `Load`/`Save`). `buildMarketProvider()` (`pkg/tui/app.go`) hace `if a.cfg.SimulationMode { return simulation.New() }` antes del bucle sobre `cfg.Providers` — los proveedores reales configurados **no se borran**, quedan en `cfg.Providers` sin usarse mientras el flag esté activo, así que desactivar simulación los restaura sin pedir credenciales de nuevo. `pkg/drivers/simulation` implementa `market.ProviderAPI` completo y se registra en el setup wizard (`ScreenDataMode`, nueva pantalla entre `ScreenProfile` y `ScreenProvider`) y en un toggle post-setup (`ScreenSimulationMode`, `/simulation`) — pero **no** en `pkg/registry/market.go`: no es "un proveedor más" de la cascada, es un modo que la sustituye entera. Esto es una desviación deliberada del checklist "Adding a new market driver" de `CLAUDE.md`.
2. Se extrajo `news.Source` (interfaz con la firma exacta de `HandleFetchNews` que ya tenía `*news.Provider`) en `pkg/news/tool.go`; `Agent.news`/`SetNewsProvider` pasan de `*news.Provider` a `news.Source`. `pkg/drivers/simulation.NewsSource` implementa la interfaz generando titulares sintéticos a partir de los mismos "días de movimiento grande" que ya calcula el generador de velas — no hay RSS real posible para empresas inventadas.
3. El generador de datos (`pkg/drivers/simulation/generator.go`) usa un factor de mercado diario compartido (una sola semilla por fecha de calendario) más ruido idiosincrático por instrumento, en vez de un paseo aleatorio independiente por símbolo — así el universo simulado sube y baja de forma correlacionada, como un mercado real, no como ruido sin sentido. Todo es determinista (`math/rand/v2` con semillas derivadas de `symbol|fecha`, sin estado persistido) para que la demo sea reproducible entre ejecuciones.

### Consequences

- Ningún cambio en `pkg/registry/market.go`, `ScreenProvider` ni `MarketProviderManageModel` — el driver de simulación vive completamente al margen de la cascada de proveedores reales, con su propio punto de entrada (`ScreenDataMode` en el wizard, `/simulation` después).
- `Agent.news` como interfaz es un cambio de firma pequeño pero de superficie amplia (todo sitio que construye un `Agent` y llama `SetNewsProvider` sigue compilando sin cambios, ya que `*news.Provider` satisface `news.Source` estructuralmente).
- El fichero de universo simulado (`pkg/drivers/simulation/data/simulation_data.json`) se embebe en el binario vía `go:embed` como valor por defecto — la app funciona "de fábrica" sin ningún fichero externo — pero admite override en cwd o en el directorio de configuración para personalización/tests, sin necesidad de recompilar.

## DD-8 — Pipeline de release: GoReleaser sobre Makefile a mano; versión `dev` + `debug.ReadBuildInfo()` como fallback del ldflags de GoReleaser

- **Status**: accepted
- **Opened**: 2026-07-13
- **Closed**: 2026-07-13

### Context

Hasta FEAT-20 el proyecto no tenía tooling de build/CI: sin Makefile, sin `.github/`, sin forma de que el binario reportara su propia versión. El usuario planea publicar el repo en GitHub próximamente y quiere usar sus herramientas gratuitas (Actions) para todo el ciclo de vida. Dos decisiones no eran obvias:

1. Cómo automatizar builds multi-plataforma y releases de GitHub: Makefile/script a mano vs. GoReleaser (herramienta de terceros gratuita/OSS, exige `.goreleaser.yaml` y una Action dedicada).
2. Cómo debe comportarse `-version` cuando el binario NO se construyó con GoReleaser (`go build`/`go install` directos, el camino documentado hoy en el README): mostrar un `"dev"` desnudo, o aprovechar el VCS stamping automático de Go 1.18+ (`runtime/debug.ReadBuildInfo()`) para mostrar el commit real sin herramienta extra. En el momento de esta decisión el repo no tenía ningún tag de git (`git describe --tags --always --dirty` caía al hash corto), así que la vía del tag real todavía no podía probarse con un release de verdad.

### Decision

1. GoReleaser (v2, `.goreleaser.yaml`), no Makefile: multi-plataforma con checksums, archivos y changelog agrupado es su caso de uso de fábrica; se invoca sin instalación permanente (`go run github.com/goreleaser/goreleaser/v2@latest`), disparado solo por push de tag (`.github/workflows/release.yml`), separado de un `ci.yml` simple que no depende de GoReleaser y corre en cada push/PR a `main`. Se descartó también disparar `release.yml` en `pull_request` (como sugiere el ejemplo oficial de la documentación de GoReleaser) porque `goreleaser release` sin `--snapshot` falla si no hay un tag real en el ref — habría dejado todas las PRs en rojo.
2. `cmd/auris/version.go` declara `version`/`commit`/`date`/`builtBy` con los nombres exactos que usa la plantilla de ldflags **por defecto** de GoReleaser, evitando un bloque `ldflags:` propio. Defaults sin GoReleaser: `"dev"`/`"none"`/`"unknown"`/`"unknown"`. Cuando `version == "dev"`, `-version` se enriquece con `debug.ReadBuildInfo()` (`vcs.revision` truncado a 7 chars, sufijo `-dirty` si `vcs.modified == "true"`); si no hay VCS info disponible, cae al `"dev"` desnudo sin error.

### Consequences

- El binario nunca miente sobre su versión: en release real lleva el tag; en build local lleva, cuando es posible, el commit real, sin exigir `-ldflags` manual.
- Acoplamiento implícito: si `version`/`commit`/`date`/`builtBy` se renombran algún día, hay que añadir un `ldflags:` explícito o re-alinear nombres con GoReleaser.
- `release.yml`/`ci.yml` quedan inactivos hasta publicar el repo en GitHub (remoto actual: Gitea/Forgejo autoalojado) — esperado, no un error de configuración.
- Primera dependencia de *tooling* externo del repo (no toca `go.mod`); validada localmente con `goreleaser check` y `goreleaser build --snapshot --clean --single-target` (requirió `GOTOOLCHAIN=auto`, ya que GoReleaser v2 exige Go ≥ 1.26.4 y el entorno de desarrollo tenía 1.25.12 instalado — el propio `go run` descargó el toolchain necesario sin tocar `go.mod`, que sigue declarando `go 1.25.0`).
