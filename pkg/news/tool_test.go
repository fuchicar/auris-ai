package news

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestToolParams_KeywordsOptional verifies that keywords is NOT required.
// Models must be able to call fetch_news({}) to get general news without specifying keywords.
// If keywords were required, models would refuse to call the tool for "Lee las noticias de hoy"
// because they don't know what keywords to invent.
func TestToolParams_KeywordsOptional(t *testing.T) {
	params := ToolParams()
	req, ok := params["required"]
	if !ok {
		t.Fatal("ToolParams missing 'required' field")
	}
	reqSlice, ok := req.([]string)
	if !ok {
		t.Fatalf("'required' is not []string: %T", req)
	}
	for _, r := range reqSlice {
		if r == "keywords" {
			t.Fatal("keywords must NOT be required: models must be able to call fetch_news with keywords=[] for general news")
		}
	}
}

// TestToolParams_EmptyKeywordsDescribed verifies that the keywords description
// explains that an empty list returns all recent news without filtering.
func TestToolParams_EmptyKeywordsDescribed(t *testing.T) {
	params := ToolParams()
	props, _ := params["properties"].(map[string]any)
	kw, _ := props["keywords"].(map[string]any)
	desc, _ := kw["description"].(string)
	if !strings.Contains(desc, "[]") && !strings.Contains(strings.ToLower(desc), "vac") {
		t.Error("keywords description must explain that an empty list ([]) returns all recent news without filtering")
	}
}

func TestToolParams_KeywordsSchema(t *testing.T) {
	params := ToolParams()
	props, _ := params["properties"].(map[string]any)
	kw, ok := props["keywords"].(map[string]any)
	if !ok {
		t.Fatal("keywords property missing")
	}
	if kw["type"] != "array" {
		t.Errorf("keywords type = %v, want 'array'", kw["type"])
	}
}

func TestNewProvider_DefaultFeeds(t *testing.T) {
	p := NewProvider(nil)
	if len(p.feeds) != len(DefaultFeeds) {
		t.Errorf("expected %d feeds, got %d", len(DefaultFeeds), len(p.feeds))
	}
}

func TestNewProvider_CustomFeeds(t *testing.T) {
	feeds := []FeedConfig{{Name: "test", URL: "http://x.com", Language: "en"}}
	p := NewProvider(feeds)
	if len(p.feeds) != 1 {
		t.Errorf("expected 1 feed, got %d", len(p.feeds))
	}
}

func TestHandleFetchNews_AllFeedsFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "fail", http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := NewProvider([]FeedConfig{
		{Name: "bad1", URL: srv.URL + "/feed1.xml"},
		{Name: "bad2", URL: srv.URL + "/feed2.xml"},
	})
	items, err := p.HandleFetchNews(context.Background(), FetchNewsParams{Keywords: []string{"x"}})
	if err == nil {
		t.Error("expected error when all feeds fail, got nil")
	}
	if len(items) != 0 {
		t.Errorf("expected empty slice, got %d items", len(items))
	}
}

func TestHandleFetchNews_Defaults(t *testing.T) {
	// Build RSS with 15 recent items (1 hour ago each, unique titles).
	recentDate := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC1123Z)
	var itemsBuilder strings.Builder
	for i := range 15 {
		fmt.Fprintf(&itemsBuilder, `<item><title>Item %d</title><description>body</description><pubDate>%s</pubDate><link>http://x.com/%d</link></item>`, i, recentDate, i)
	}
	rss := fmt.Sprintf(`<?xml version="1.0"?><rss version="2.0"><channel><title>Test</title>%s</channel></rss>`, itemsBuilder.String())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprint(w, rss)
	}))
	defer srv.Close()

	p := NewProvider([]FeedConfig{{Name: "t", URL: srv.URL}})
	// MaxResults=0 should default to 10
	got, err := p.HandleFetchNews(context.Background(), FetchNewsParams{Keywords: []string{}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 10 {
		t.Errorf("expected default MaxResults=10, got %d items", len(got))
	}
}
