package news

import (
	"context"
	"errors"
	"fmt"
)

// ToolName is the name of the fetch_news tool registered in the agent.
const ToolName = "fetch_news"

// ToolDescription is the tool description presented to the LLM.
const ToolDescription = "Fetch recent financial and economic news articles from configured RSS feeds. " +
	"Use this tool to: (1) answer any request for a news summary or general market overview, " +
	"(2) get current news about a specific topic, company, sector, or asset, " +
	"(3) ground asset analysis in recent events. " +
	"For general news with no specific topic, call with keywords=[]."

// ToolParams returns the JSON Schema parameters object for the fetch_news tool.
// Returns a fresh map on each call to prevent mutation by callers.
func ToolParams() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"keywords": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Keywords to filter articles by title and description. Include tickers, company names, or sector terms. Use an empty list ([]) to retrieve all recent news without filtering.",
			},
			"max_age_hours": map[string]any{
				"type":        "integer",
				"description": "Maximum article age in hours. Defaults to 48 if not specified.",
			},
			"max_results": map[string]any{
				"type":        "integer",
				"description": "Maximum number of articles to return. Defaults to 10 if not specified.",
			},
		},
		"required": []string{},
	}
}

// Source is anything that can execute the fetch_news tool pipeline. Provider
// is the real RSS-backed implementation; pkg/drivers/simulation supplies a
// synthetic implementation used in simulation mode (see agent.SetNewsProvider).
type Source interface {
	HandleFetchNews(ctx context.Context, params FetchNewsParams) ([]NewsItem, error)
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
