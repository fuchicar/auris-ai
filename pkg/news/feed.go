package news

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/mmcdole/gofeed"
)

// FeedConfig describes a single RSS or Atom feed source.
type FeedConfig struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Language string `json:"language"`
}

// DefaultFeeds contains the pre-configured financial news sources.
// Mix of Spanish and English outlets covering macroeconomic and market news.
// All entries have been validated against the live sources via gofeed (see
// feed_test.go).
var DefaultFeeds = []FeedConfig{
	// Spanish
	{Name: "Expansión", URL: "https://e00-expansion.uecdn.es/rss/portada.xml", Language: "es"},
	{Name: "El País Economía", URL: "https://feeds.elpais.com/mrss-s/pages/ep/site/elpais.com/section/economia/portada", Language: "es"},
	{Name: "La Vanguardia Economía", URL: "https://www.lavanguardia.com/rss/economia.xml", Language: "es"},
	{Name: "ABC Economía", URL: "https://www.abc.es/rss/feeds/abc_Economia.xml", Language: "es"},
	{Name: "El Mundo Economía", URL: "https://e00-elmundo.uecdn.es/rss/economia.xml", Language: "es"},
	{Name: "El Independiente Economía", URL: "https://www.elindependiente.com/economia/feed/", Language: "es"},
	{Name: "Investing.com ES", URL: "https://es.investing.com/rss/news.rss", Language: "es"},
	// English
	{Name: "Bloomberg Markets", URL: "https://feeds.bloomberg.com/markets/news.rss", Language: "en"},
	{Name: "Financial Times", URL: "https://www.ft.com/rss/home", Language: "en"},
	{Name: "Financial Times Companies", URL: "https://www.ft.com/companies?format=rss", Language: "en"},
	{Name: "The Economist Finance", URL: "https://www.economist.com/finance-and-economics/rss.xml", Language: "en"},
	{Name: "CNBC Economy", URL: "https://www.cnbc.com/id/20910258/device/rss/rss.html", Language: "en"},
	{Name: "CNBC Markets", URL: "https://www.cnbc.com/id/10001147/device/rss/rss.html", Language: "en"},
	{Name: "MarketWatch", URL: "https://www.marketwatch.com/rss/topstories", Language: "en"},
	{Name: "Yahoo Finance", URL: "https://finance.yahoo.com/news/rssindex", Language: "en"},
	{Name: "Seeking Alpha", URL: "https://seekingalpha.com/feed.xml", Language: "en"},
	{Name: "BBC Business", URL: "https://feeds.bbci.co.uk/news/business/rss.xml", Language: "en"},
	{Name: "Investing.com Stock Market", URL: "https://www.investing.com/rss/news_25.rss", Language: "en"},
	{Name: "Investing.com Earnings", URL: "https://www.investing.com/rss/news_1062.rss", Language: "en"},
}

const feedTimeout = 5 * time.Second

// rawItem pairs a feed item with its source feed name.
type rawItem struct {
	item   *gofeed.Item
	source string
}

// fetchAllFeeds fetches all configured feeds concurrently.
// Each feed uses a per-feed context with feedTimeout. Failed feeds are logged
// and skipped. Returns all collected items (with source names) and the number
// of feeds that failed.
func fetchAllFeeds(ctx context.Context, feeds []FeedConfig) ([]rawItem, int) {
	type result struct {
		items []rawItem
		err   error
	}

	ch := make(chan result, len(feeds))
	var wg sync.WaitGroup

	for _, feed := range feeds {
		wg.Add(1)
		go func(cfg FeedConfig) {
			defer wg.Done()
			ctx5, cancel := context.WithTimeout(ctx, feedTimeout)
			defer cancel()
			fp := gofeed.NewParser()
			parsed, err := fp.ParseURLWithContext(cfg.URL, ctx5)
			if err != nil {
				log.Printf("news: feed %q: %v", cfg.Name, err)
				ch <- result{nil, err}
				return
			}
			sourceName := parsed.Title
			if sourceName == "" {
				sourceName = cfg.Name
			}
			items := make([]rawItem, len(parsed.Items))
			for i, it := range parsed.Items {
				items[i] = rawItem{item: it, source: sourceName}
			}
			ch <- result{items, nil}
		}(feed)
	}

	wg.Wait()
	close(ch)

	var allItems []rawItem
	failCount := 0
	for r := range ch {
		if r.err != nil {
			failCount++
			continue
		}
		allItems = append(allItems, r.items...)
	}
	return allItems, failCount
}
