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
// Mix of Spanish and English outlets covering macroeconomic and market news,
// plus macro / institutional sources (Fed, BEA, SEC, CBO, BoE) that provide
// primary-source signals the agent uses in its recommendations.
//
// All entries have been validated against the live sources via gofeed with
// the same 5s per-feed timeout used in production (see feed_test.go).
var DefaultFeeds = []FeedConfig{
	// --- Spanish (11) -----------------------------------------------------
	// Diarios económicos
	{Name: "Expansión", URL: "https://e00-expansion.uecdn.es/rss/portada.xml", Language: "es"},
	{Name: "El País Economía", URL: "https://feeds.elpais.com/mrss-s/pages/ep/site/elpais.com/section/economia/portada", Language: "es"},
	{Name: "La Vanguardia Economía", URL: "https://www.lavanguardia.com/rss/economia.xml", Language: "es"},
	{Name: "ABC Economía", URL: "https://www.abc.es/rss/feeds/abc_Economia.xml", Language: "es"},
	{Name: "El Mundo Economía", URL: "https://e00-elmundo.uecdn.es/rss/economia.xml", Language: "es"},
	{Name: "El Independiente Economía", URL: "https://www.elindependiente.com/economia/feed/", Language: "es"},
	// Diarios / agencias (nuevos)
	{Name: "Cinco Días", URL: "https://cincodias.elpais.com/rss/cincodias/portada.xml", Language: "es"},
	{Name: "Europapress Economía", URL: "https://www.europapress.es/rss/rss.aspx?ch=0046", Language: "es"},
	{Name: "El Diario.es Economía", URL: "https://www.eldiario.es/rss/economia/", Language: "es"},
	// Investing.com ES
	{Name: "Investing.com ES", URL: "https://es.investing.com/rss/news.rss", Language: "es"},
	{Name: "Investing.com ES Economy", URL: "https://es.investing.com/rss/news_95.rss", Language: "es"},
	// --- English (35) -----------------------------------------------------
	// Financial Times
	{Name: "Financial Times", URL: "https://www.ft.com/rss/home", Language: "en"},
	{Name: "FT Companies", URL: "https://www.ft.com/companies?format=rss", Language: "en"},
	{Name: "FT Markets", URL: "https://www.ft.com/markets?format=rss", Language: "en"},
	{Name: "FT Opinion", URL: "https://www.ft.com/opinion?format=rss", Language: "en"},
	// The Economist
	{Name: "The Economist Finance", URL: "https://www.economist.com/finance-and-economics/rss.xml", Language: "en"},
	{Name: "The Economist Business", URL: "https://www.economist.com/business/rss.xml", Language: "en"},
	{Name: "The Economist Leaders", URL: "https://www.economist.com/leaders/rss.xml", Language: "en"},
	{Name: "The Economist World This Week", URL: "https://www.economist.com/the-world-this-week/rss.xml", Language: "en"},
	// CNBC
	{Name: "CNBC Economy", URL: "https://www.cnbc.com/id/20910258/device/rss/rss.html", Language: "en"},
	{Name: "CNBC Markets", URL: "https://www.cnbc.com/id/10001147/device/rss/rss.html", Language: "en"},
	{Name: "CNBC Top News", URL: "https://www.cnbc.com/id/100003114/device/rss/rss.html", Language: "en"},
	{Name: "CNBC Investing", URL: "https://www.cnbc.com/id/15839069/device/rss/rss.html", Language: "en"},
	// Otros generalistas financieros
	{Name: "MarketWatch", URL: "https://www.marketwatch.com/rss/topstories", Language: "en"},
	{Name: "Yahoo Finance", URL: "https://finance.yahoo.com/news/rssindex", Language: "en"},
	{Name: "Seeking Alpha", URL: "https://seekingalpha.com/feed.xml", Language: "en"},
	{Name: "BBC Business", URL: "https://feeds.bbci.co.uk/news/business/rss.xml", Language: "en"},
	// Investing.com EN (sub-secciones)
	{Name: "Investing.com Stock Market", URL: "https://www.investing.com/rss/news_25.rss", Language: "en"},
	{Name: "Investing.com Earnings", URL: "https://www.investing.com/rss/news_1062.rss", Language: "en"},
	{Name: "Investing.com Forex", URL: "https://www.investing.com/rss/news_1.rss", Language: "en"},
	{Name: "Investing.com Commodities", URL: "https://www.investing.com/rss/news_11.rss", Language: "en"},
	{Name: "Investing.com Economy", URL: "https://www.investing.com/rss/news_95.rss", Language: "en"},
	// Macro / institucional (núcleo para el bucle del TFM)
	{Name: "Federal Reserve Press", URL: "https://www.federalreserve.gov/feeds/press_all.xml", Language: "en"},
	{Name: "BEA News", URL: "https://apps.bea.gov/rss/rss.xml", Language: "en"},
	{Name: "SEC Press", URL: "https://www.sec.gov/news/pressreleases.rss", Language: "en"},
	{Name: "Congressional Budget Office", URL: "https://www.cbo.gov/publications/all/rss.xml", Language: "en"},
	{Name: "BoE News", URL: "https://www.bankofengland.co.uk/rss/news", Language: "en"},
	// Generalistas adicionales
	{Name: "The Guardian Business", URL: "https://www.theguardian.com/business/rss", Language: "en"},
	{Name: "Forbes Business", URL: "https://www.forbes.com/business/feed/", Language: "en"},
	{Name: "Fortune", URL: "https://fortune.com/feed/", Language: "en"},
	{Name: "NYT Business", URL: "https://rss.nytimes.com/services/xml/rss/nyt/Business.xml", Language: "en"},
	{Name: "NPR Business", URL: "https://feeds.npr.org/1006/rss.xml", Language: "en"},
	// Académico / investigación
	{Name: "Wharton Knowledge", URL: "https://knowledge.wharton.upenn.edu/feed/", Language: "en"},
	{Name: "Mises", URL: "https://mises.org/rss.xml", Language: "en"},
	{Name: "ProPublica", URL: "https://www.propublica.org/feeds/propublica/main", Language: "en"},
	// Internacional no-anglo
	{Name: "DW Business", URL: "https://rss.dw.com/xml/rss-en-bus", Language: "en"},
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
