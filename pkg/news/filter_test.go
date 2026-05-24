package news

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/gofeed"
)

func makeRawItem(title, description string, age time.Duration) rawItem {
	pub := time.Now().UTC().Add(-age)
	return rawItem{
		item: &gofeed.Item{
			Title:           title,
			Description:     description,
			Link:            "https://example.com",
			PublishedParsed: &pub,
		},
		source: "TestFeed",
	}
}

var baseParams = FetchNewsParams{
	Keywords:    []string{},
	MaxAgeHours: 48,
	MaxResults:  10,
}

func TestFilterItems_DateFilter(t *testing.T) {
	items := []rawItem{
		makeRawItem("Recent", "body", 1*time.Hour),
		makeRawItem("Old", "body", 72*time.Hour),
	}
	got := filterItems(items, baseParams)
	if len(got) != 1 {
		t.Fatalf("expected 1 item, got %d", len(got))
	}
	if got[0].Title != "Recent" {
		t.Errorf("unexpected title: %q", got[0].Title)
	}
}

func TestFilterItems_KeywordFilter(t *testing.T) {
	items := []rawItem{
		makeRawItem("IBEX news", "stock market", 1*time.Hour),
		makeRawItem("Weather report", "sunny day", 1*time.Hour),
		makeRawItem("Company update", "mention of IBEX35 here", 1*time.Hour),
	}
	params := FetchNewsParams{Keywords: []string{"ibex"}, MaxAgeHours: 48, MaxResults: 10}
	got := filterItems(items, params)
	if len(got) != 2 {
		t.Fatalf("expected 2 items, got %d", len(got))
	}
}

func TestFilterItems_EmptyKeywords(t *testing.T) {
	items := []rawItem{
		makeRawItem("A", "body", 1*time.Hour),
		makeRawItem("B", "body", 2*time.Hour),
	}
	got := filterItems(items, baseParams)
	if len(got) != 2 {
		t.Errorf("expected 2 items when keywords empty, got %d", len(got))
	}
}

func TestFilterItems_Dedup(t *testing.T) {
	items := []rawItem{
		makeRawItem("IBEX Update", "body", 1*time.Hour),
		makeRawItem("ibex update", "body", 2*time.Hour),
		makeRawItem("  IBEX UPDATE  ", "body", 3*time.Hour),
	}
	got := filterItems(items, baseParams)
	if len(got) != 1 {
		t.Errorf("expected 1 item after dedup, got %d", len(got))
	}
}

func TestFilterItems_SortDescending(t *testing.T) {
	items := []rawItem{
		makeRawItem("Old", "body", 3*time.Hour),
		makeRawItem("Newest", "body", 1*time.Hour),
		makeRawItem("Middle", "body", 2*time.Hour),
	}
	got := filterItems(items, baseParams)
	if len(got) != 3 {
		t.Fatalf("expected 3 items, got %d", len(got))
	}
	if got[0].Title != "Newest" || got[1].Title != "Middle" || got[2].Title != "Old" {
		t.Errorf("unexpected order: %v", []string{got[0].Title, got[1].Title, got[2].Title})
	}
}

func TestFilterItems_Limit(t *testing.T) {
	items := make([]rawItem, 15)
	for i := range items {
		items[i] = makeRawItem(fmt.Sprintf("Item %d", i), "body", time.Duration(i+1)*time.Hour)
	}
	params := FetchNewsParams{Keywords: []string{}, MaxAgeHours: 48, MaxResults: 5}
	got := filterItems(items, params)
	if len(got) != 5 {
		t.Errorf("expected 5 items (limit), got %d", len(got))
	}
}

func TestFilterItems_NilPublishedParsed(t *testing.T) {
	items := []rawItem{
		{item: &gofeed.Item{Title: "NoPub", Description: "body", Link: "https://x.com"}, source: "S"},
		makeRawItem("WithPub", "body", 1*time.Hour),
	}
	got := filterItems(items, baseParams)
	if len(got) != 1 {
		t.Errorf("expected 1 item (nil PublishedParsed discarded), got %d", len(got))
	}
	if got[0].Title != "WithPub" {
		t.Errorf("unexpected title: %q", got[0].Title)
	}
}

func TestStripHTML(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"<p>Hello <b>world</b></p>", "Hello world"},
		{"<a href=\"x\">link</a>", "link"},
		{"plain text", "plain text"},
		{"<br/>", ""},
		{"<p>  spaces  </p>", "spaces"},
	}
	for _, c := range cases {
		got := stripHTML(c.in)
		if got != c.want {
			t.Errorf("stripHTML(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestStripHTML_Truncation(t *testing.T) {
	long := strings.Repeat("a", 400)
	ri := makeRawItem("T", "<p>"+long+"</p>", 1*time.Hour)
	result := toNewsItem(ri)
	runes := []rune(result.Summary)
	if len(runes) != 300 {
		t.Errorf("expected summary truncated to 300 runes, got %d", len(runes))
	}
}
