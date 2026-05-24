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

// DefaultFeeds contains the pre-configured Spanish financial news sources.
var DefaultFeeds = []FeedConfig{
	{Name: "Expansión", URL: "https://e00-expansion.uecdn.es/rss/portada.xml", Language: "es"},
	{Name: "El País Economía", URL: "https://feeds.elpais.com/mrss-s/pages/ep/site/elpais.com/section/economia/portada", Language: "es"},
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
