package news

import (
	"sort"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// filterItems applies date, keyword, dedup, and limit filters in order,
// then returns results sorted by PublishedAt descending.
func filterItems(items []rawItem, params FetchNewsParams) []NewsItem {
	cutoff := time.Now().UTC().Add(-time.Duration(params.MaxAgeHours) * time.Hour)

	seen := make(map[string]struct{})
	var result []NewsItem

	for _, ri := range items {
		item := ri.item

		// 1. Date filter.
		if item.PublishedParsed == nil || item.PublishedParsed.Before(cutoff) {
			continue
		}

		// 2. Keyword filter (skip if no keywords supplied).
		if len(params.Keywords) > 0 && !matchesKeywords(item.Title, item.Description, params.Keywords) {
			continue
		}

		// 3. Deduplication by normalised title.
		key := strings.ToLower(strings.TrimSpace(item.Title))
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}

		result = append(result, toNewsItem(ri))
	}

	// 4. Sort newest-first.
	sort.Slice(result, func(i, j int) bool {
		return result[i].PublishedAt.After(result[j].PublishedAt)
	})

	// 5. Limit.
	if params.MaxResults > 0 && len(result) > params.MaxResults {
		result = result[:params.MaxResults]
	}

	return result
}

func matchesKeywords(title, description string, keywords []string) bool {
	titleLow := strings.ToLower(title)
	descLow := strings.ToLower(description)
	for _, kw := range keywords {
		kw = strings.ToLower(kw)
		if strings.Contains(titleLow, kw) || strings.Contains(descLow, kw) {
			return true
		}
	}
	return false
}

func toNewsItem(ri rawItem) NewsItem {
	item := ri.item

	summary := stripHTML(item.Description)
	runes := []rune(summary)
	if len(runes) > 300 {
		summary = string(runes[:300])
	}

	var publishedAt time.Time
	if item.PublishedParsed != nil {
		publishedAt = *item.PublishedParsed
	}

	return NewsItem{
		Title:       item.Title,
		Summary:     summary,
		Source:      ri.source,
		URL:         item.Link,
		PublishedAt: publishedAt,
	}
}

// stripHTML removes HTML tags from s and returns the plain text, trimmed.
func stripHTML(s string) string {
	var b strings.Builder
	tok := html.NewTokenizer(strings.NewReader(s))
	for {
		tt := tok.Next()
		if tt == html.ErrorToken {
			break
		}
		if tt == html.TextToken {
			b.WriteString(tok.Token().Data)
		}
	}
	return strings.TrimSpace(b.String())
}
