# Auris

**Auris** es un asesor financiero con IA para la terminal. Combina datos de mercado en tiempo real con IA conversacional para que puedas analizar instrumentos, ejecutar cálculos financieros y gestionar carteras — todo desde tu terminal.

Backends LLM soportados: **Ollama** (local), **Google Gemini**, **Anthropic Claude**, **OpenAI** y **MiniMax**.  
Datos de mercado: **Financial Modeling Prep (FMP)** y **EOD Historical Data (EODHD)**, combinados en cascada con fallback.

---

## Características

- Chat agente interactivo con acceso a datos de mercado en vivo
- Búsqueda de instrumentos entre acciones, ETFs, forex, futuros y criptomonedas
- Cotizaciones en tiempo real, velas históricas, fundamentales y acciones corporativas
- Calculadoras financieras integradas: ROI, CAGR, ratio de Sharpe, VaR, DCF, volatilidad y más
- Agregación de noticias financieras desde feeds RSS/Atom configurables
- Gestión de carteras con seguimiento de lotes y P&L
- Configuración cifrada en disco (AES-256-GCM + Argon2id)
- Sesiones de conversación persistentes
- Interfaz en inglés y castellano (detectado automáticamente por el locale)
- Temas claro y oscuro

---

## Requisitos

| Dependencia | Versión |
|---|---|
| Go | ≥ 1.25 |
| Clave API de FMP | [financialmodelingprep.com](https://financialmodelingprep.com) |
| Proveedor LLM | Ollama (local) o clave API para Gemini / Claude / OpenAI / MiniMax |

---

## Instalación

### Compilar desde el código fuente

```bash
git clone <url-del-repositorio>
cd auris-ai
go build -o auris ./cmd/auris
./auris
```

### Con Nix

Consulta la sección [Nix y NixOS](#nix-y-nixos) más abajo.

### Instalar en el PATH

Una vez compilado, copia el binario a un directorio de tu `$PATH`:

```bash
go install ./cmd/auris
```

### Binarios precompilados

Los releases etiquetados (`vX.Y.Z`) publican binarios precompilados para Linux, macOS y Windows (amd64/arm64) vía GitHub Releases, generados con [GoReleaser](https://goreleaser.com). Ejecuta `auris -version` para comprobar de qué build proviene un binario concreto.

---

## Inicio rápido

En el primer arranque, Auris ejecuta un asistente de configuración guiado:

1. **Idioma** — detectado automáticamente; preguntado si hay ambigüedad
2. **Tema** — claro u oscuro (y variantes)
3. **Contraseña** — usada para cifrar tus claves API en disco
4. **Perfil financiero** — 10 preguntas que personalizan los consejos de la IA
5. **Proveedor de mercado** — selecciona FMP e introduce tu clave API
6. **Proveedores de IA** — selecciona uno o más (Ollama, Gemini, Claude, OpenAI, MiniMax) y configura cada uno
7. **Modelo por defecto** — elige el par proveedor/modelo usado por defecto en el chat

En los arranques posteriores, solo se solicita la contraseña para desbloquear la configuración.

Para volver a ejecutar el asistente de configuración en cualquier momento:

```bash
auris -setup
```

---

## Uso

```
auris [opciones]

Opciones:
  -setup          Fuerza el asistente de configuración aunque ya exista un config
  -debug <ruta>   Añade logs de diagnóstico del agente al fichero indicado
  -version        Imprime la información de versión y termina
```

### Variables de entorno

| Variable | Por defecto | Descripción |
|---|---|---|
| `AURIS_OLLAMA_NUM_CTX` | `32768` | Tamaño de la ventana de contexto enviada a Ollama. Aumentar para modelos grandes (p.ej. `131072`). |
| `LC_ALL` / `LANG` | sistema | Determina el idioma de la interfaz (`en` o `es`). |

---

## Proveedores

### Proveedores LLM

| Proveedor | Notas |
|---|---|
| **Ollama** | Inferencia local. Endpoint por defecto: `http://localhost:11434`. Admite URL base y clave API personalizadas para instancias remotas. |
| **Google Gemini** | Requiere una clave API de Gemini. |
| **Anthropic Claude** | Requiere una clave API de Anthropic. Admite URL base personalizada. |
| **OpenAI** | Requiere una clave API de OpenAI. También utilizable contra cualquier endpoint compatible con OpenAI mediante URL base personalizada. |
| **MiniMax** | Requiere una clave API de MiniMax. |

### Proveedores de datos de mercado

| Proveedor | Notas |
|---|---|
| **Financial Modeling Prep** | Requiere una clave API de FMP (gratuita o de pago). El plan gratuito cubre la mayoría de funcionalidades (cobertura principalmente US). |
| **EOD Historical Data (EODHD)** | Segundo proveedor opcional usado como fallback en la cascada; añade cobertura no-US (p. ej. BME/Madrid). |

---

## Nix y NixOS

El repositorio incluye un `shell.nix` (entorno de desarrollo) y un `default.nix` (compilación reproducible del paquete).

### Entorno de desarrollo

Entra en un shell con Go y todas las herramientas de desarrollo disponibles:

```bash
nix-shell
```

Si usas [direnv](https://direnv.net), el entorno se activa automáticamente al entrar en el directorio del proyecto:

```bash
direnv allow
```

El shell proporciona: `go`, `gopls`, `golangci-lint`, `gotools`, `git`.

### Compilar con Nix

```bash
nix-build
./result/bin/auris
```

### Instalar en tu perfil de usuario Nix

```bash
nix-env -if .
auris
```

Para desinstalar:

```bash
nix-env -e auris
```

### Configuración del sistema NixOS

Añade Auris como paquete del sistema importando la derivación directamente. En tu `configuration.nix` de NixOS:

```nix
{ config, pkgs, ... }:
let
  auris = pkgs.callPackage (builtins.fetchGit {
    url  = "<url-del-repositorio>";
    ref  = "main";
  } + "/default.nix") {};
in {
  environment.systemPackages = [ auris ];
}
```

### home-manager

```nix
{ config, pkgs, ... }:
let
  auris = pkgs.callPackage (builtins.fetchGit {
    url = "<url-del-repositorio>";
    ref = "main";
  } + "/default.nix") {};
in {
  home.packages = [ auris ];
}
```

### Fijar la versión de nixpkgs

El Nix clásico resuelve nixpkgs desde el canal del sistema. Para fijar una versión concreta y obtener builds totalmente reproducibles, crea un fichero de pin `nixpkgs.json` o considera migrar a `flake.nix` (el `default.nix` y el `shell.nix` de este repositorio están diseñados para ser importables desde un flake sin duplicar código).

---

## Desarrollo

```bash
# Compilar todos los paquetes
go build ./...

# Análisis estático
go vet ./...

# Ejecutar todos los tests (los tests de integración requieren claves API)
go test ./... -timeout 120s

# Ejecutar un test específico
go test ./pkg/drivers/fmp/... -run TestGetQuote_AAPL -v

# Ejecutar solo tests unitarios (sin credenciales)
go test ./pkg/drivers/fmp/... -run TestInterfaceCompliance -v
```

Las credenciales para los tests de integración se leen desde ficheros en el directorio `test_data/` de cada driver (ver `CLAUDE.md` para más detalles). Los tests se saltan automáticamente cuando las credenciales no están presentes.

---

## Ubicación del fichero de configuración

| Plataforma | Ruta |
|---|---|
| Linux | `~/.config/auris/auris.json` |
| macOS | `~/Library/Application Support/auris/auris.json` |

Las claves API se cifran en disco. El subdirectorio `sessions/` junto al fichero de configuración almacena el historial de conversaciones.
