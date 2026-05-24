package news

import "time"

// NewsItem is a single article returned by the fetch_news tool.
type NewsItem struct {
	Title       string    `json:"title"`
	Summary     string    `json:"summary"`
	Source      string    `json:"source"`
	URL         string    `json:"url"`
	PublishedAt time.Time `json:"published_at"`
}

// FetchNewsParams holds the input parameters for the fetch_news tool call.
type FetchNewsParams struct {
	Keywords    []string `json:"keywords"`
	MaxAgeHours int      `json:"max_age_hours"`
	MaxResults  int      `json:"max_results"`
}
