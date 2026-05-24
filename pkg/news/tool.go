package news

import (
	"context"
	"errors"
	"fmt"
)

// ToolName is the name of the fetch_news tool registered in the agent.
const ToolName = "fetch_news"

// ToolDescription is the tool description presented to the LLM.
const ToolDescription = "Obtiene artículos de noticias financieras y económicas recientes desde feeds RSS configurados. " +
	"Usa este tool para: (1) responder cualquier solicitud de resumen de noticias o estado general del mercado, " +
	"(2) obtener noticias actuales sobre un tema, empresa, sector o activo específico, " +
	"(3) basar análisis de activos en eventos recientes. " +
	"Para noticias generales sin tema específico, llama con keywords=[]."

// ToolParams returns the JSON Schema parameters object for the fetch_news tool.
// Returns a fresh map on each call to prevent mutation by callers.
func ToolParams() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"keywords": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Keywords para filtrar artículos por título y descripción. Incluye tickers, nombres de empresas o términos sectoriales. Usa una lista vacía ([]) para obtener todas las noticias recientes sin filtrar.",
			},
			"max_age_hours": map[string]any{
				"type":        "integer",
				"description": "Antigüedad máxima de los artículos en horas. Por defecto 48 si no se indica.",
			},
			"max_results": map[string]any{
				"type":        "integer",
				"description": "Número máximo de artículos a devolver. Por defecto 10 si no se indica.",
			},
		},
		"required": []string{},
	}
}

// Provider holds the configured RSS feeds and executes the fetch_news pipeline.
type Provider struct {
	feeds []FeedConfig
}

// NewProvider creates a Provider using the given feeds.
// If feeds is nil or empty, DefaultFeeds is used.
func NewProvider(feeds []FeedConfig) *Provider {
	if len(feeds) == 0 {
		feeds = DefaultFeeds
	}
	return &Provider{feeds: feeds}
}

// HandleFetchNews executes the full pipeline: fetch → filter → return.
// Defaults: MaxAgeHours=48, MaxResults=10 when zero.
// If all configured feeds fail, returns an empty slice and a descriptive error.
func (p *Provider) HandleFetchNews(ctx context.Context, params FetchNewsParams) ([]NewsItem, error) {
	if params.MaxAgeHours == 0 {
		params.MaxAgeHours = 48
	}
	if params.MaxResults == 0 {
		params.MaxResults = 10
	}

	items, failCount := fetchAllFeeds(ctx, p.feeds)
	if failCount == len(p.feeds) {
		return []NewsItem{}, fmt.Errorf("news: all %d configured feeds failed to load", len(p.feeds))
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return []NewsItem{}, ctx.Err()
	}

	return filterItems(items, params), nil
}
