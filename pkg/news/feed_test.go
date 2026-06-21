package news

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/gofeed"
)

// TestDefaultFeeds_Metadata checks structural invariants of DefaultFeeds without
// touching the network. Every feed must have a non-empty name, a valid http(s)
// URL, and a recognised language tag.
func TestDefaultFeeds_Metadata(t *testing.T) {
	if len(DefaultFeeds) == 0 {
		t.Fatal("DefaultFeeds must not be empty")
	}

	seenNames := make(map[string]bool, len(DefaultFeeds))
	seenURLs := make(map[string]bool, len(DefaultFeeds))

	langCounts := map[string]int{}
	for i, f := range DefaultFeeds {
		if strings.TrimSpace(f.Name) == "" {
			t.Errorf("feed[%d]: Name is empty", i)
		}
		if u, err := url.Parse(f.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			t.Errorf("feed[%d] %q: URL %q is not a valid http(s) URL", i, f.Name, f.URL)
		}
		switch f.Language {
		case "es", "en":
			// ok
		default:
			t.Errorf("feed[%d] %q: Language %q is not 'es' or 'en'", i, f.Name, f.Language)
		}
		if seenNames[f.Name] {
			t.Errorf("duplicate feed name: %q", f.Name)
		}
		if seenURLs[f.URL] {
			t.Errorf("duplicate feed URL: %q", f.URL)
		}
		seenNames[f.Name] = true
		seenURLs[f.URL] = true
		langCounts[f.Language]++
	}

	// Sanity: project brief calls for a bilingual mix. Guard against regressions
	// where someone strips one language down to zero.
	if langCounts["es"] == 0 {
		t.Error("expected at least one Spanish feed in DefaultFeeds")
	}
	if langCounts["en"] == 0 {
		t.Error("expected at least one English feed in DefaultFeeds")
	}
}

// TestDefaultFeeds_Live is an integration test that hits every DefaultFeeds URL
// with gofeed and verifies we get a non-empty parse with items. It skips
// silently when the network is unavailable so it stays friendly to local
// development (mirroring the project's pattern in drivers/fmp and drivers/ollama).
func TestDefaultFeeds_Live(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live feed check in -short mode")
	}

	probeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := http.NewRequestWithContext(probeCtx, http.MethodHead, "https://feeds.bbci.co.uk/news/business/rss.xml", nil); err != nil {
		t.Skipf("network unavailable, skipping live feed test: %v", err)
	}
	// Cheap reachability probe via the real client used by gofeed.
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("https://feeds.bbci.co.uk/news/business/rss.xml")
	if err != nil || resp.StatusCode >= 500 {
		if resp != nil {
			resp.Body.Close()
		}
		t.Skipf("network probe failed, skipping live feed test: %v", err)
	}
	resp.Body.Close()

	fp := gofeed.NewParser()
	for _, f := range DefaultFeeds {
		f := f
		t.Run(f.Name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), feedTimeout)
			defer cancel()
			parsed, err := fp.ParseURLWithContext(f.URL, ctx)
			if err != nil {
				t.Fatalf("parse %q (%s): %v", f.Name, f.URL, err)
			}
			if parsed == nil {
				t.Fatalf("parse %q (%s): nil feed", f.Name, f.URL)
			}
			if len(parsed.Items) == 0 {
				t.Fatalf("parse %q (%s): feed returned 0 items", f.Name, f.URL)
			}
		})
	}
}