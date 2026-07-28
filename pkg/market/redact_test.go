package market_test

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/fuchicar/auris-ai/pkg/market"
)

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

func mustURL(t *testing.T, raw string) *url.Error {
	t.Helper()
	return &url.Error{Op: "Get", URL: raw, Err: errors.New("dial tcp: connection refused")}
}

// ─────────────────────────────────────────────────────────────────────────────
// RedactURLError
// ─────────────────────────────────────────────────────────────────────────────

func TestRedactURLError_Nil(t *testing.T) {
	if got := market.RedactURLError(nil, "apikey"); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestRedactURLError_NonURLError_PassesThrough(t *testing.T) {
	plain := errors.New("boom")
	got := market.RedactURLError(plain, "apikey")
	if got != plain {
		t.Errorf("expected pass-through, got %v", got)
	}
}

func TestRedactURLError_URLWithNoQuery_Unchanged(t *testing.T) {
	in := mustURL(t, "https://api.example.com/v1/quote")
	got := market.RedactURLError(in, "apikey").Error()
	if !strings.Contains(got, "https://api.example.com/v1/quote") {
		t.Errorf("URL stripped unexpectedly: %s", got)
	}
	if strings.Contains(got, "REDACTED") {
		t.Errorf("REDACTED should not appear when no key was present: %s", got)
	}
}

func TestRedactURLError_StripsApikey(t *testing.T) {
	const canary = "SECRET_CANARY"
	in := mustURL(t, "https://api.example.com/quote?apikey="+canary+"&symbol=AAPL")
	got := market.RedactURLError(in, "apikey").Error()
	if strings.Contains(got, canary) {
		t.Errorf("API key leaked into error string: %s", got)
	}
	if !strings.Contains(got, "apikey=REDACTED") {
		t.Errorf("expected apikey=REDACTED in error string: %s", got)
	}
	if !strings.Contains(got, "symbol=AAPL") {
		t.Errorf("non-sensitive query params must be preserved: %s", got)
	}
}

func TestRedactURLError_StripsApiToken(t *testing.T) {
	const canary = "SECRET_EODHD_TOKEN"
	in := mustURL(t, "https://api.example.com/eod/AAPL.US?api_token="+canary+"&fmt=json")
	got := market.RedactURLError(in, "api_token").Error()
	if strings.Contains(got, canary) {
		t.Errorf("api_token leaked into error string: %s", got)
	}
	if !strings.Contains(got, "fmt=json") {
		t.Errorf("non-sensitive query params must be preserved: %s", got)
	}
}

func TestRedactURLError_MultipleKeys_AllStripped(t *testing.T) {
	const canary1 = "CANARY_KEY_1"
	const canary2 = "CANARY_KEY_2"
	in := mustURL(t, "https://api.example.com/x?apikey="+canary1+"&api_token="+canary2+"&symbol=AAPL")
	got := market.RedactURLError(in, "apikey", "api_token").Error()
	if strings.Contains(got, canary1) || strings.Contains(got, canary2) {
		t.Errorf("sensitive keys leaked: %s", got)
	}
	if !strings.Contains(got, "apikey=REDACTED") || !strings.Contains(got, "api_token=REDACTED") {
		t.Errorf("expected both keys redacted: %s", got)
	}
}

func TestRedactURLError_LeavesOtherParamsAlone(t *testing.T) {
	const canary = "CANARY_KEY"
	in := mustURL(t, "https://api.example.com/x?apikey="+canary+"&other=keepme")
	got := market.RedactURLError(in, "apikey").Error()
	if !strings.Contains(got, "other=keepme") {
		t.Errorf("non-sensitive params must be preserved: %s", got)
	}
}

func TestRedactURLError_CaseInsensitiveKeyMatch(t *testing.T) {
	const canary = "CANARY_KEY"
	in := mustURL(t, "https://api.example.com/x?APIKEY="+canary)
	got := market.RedactURLError(in, "apikey").Error()
	if strings.Contains(got, canary) {
		t.Errorf("uppercase APIKEY should also be redacted: %s", got)
	}
}

func TestRedactURLError_UnparseableURL_ReplacedWithLiteralRedacted(t *testing.T) {
	// %zz is not a valid percent-encoded sequence; url.Parse rejects it.
	in := &url.Error{Op: "Get", URL: "https://api.example.com/%zz?apikey=LEAK", Err: errors.New("dial: refused")}
	got := market.RedactURLError(in, "apikey").Error()
	if strings.Contains(got, "LEAK") {
		t.Errorf("URL leaked when parse failed: %s", got)
	}
	if !strings.Contains(got, "(redacted)") {
		t.Errorf("expected literal (redacted) placeholder: %s", got)
	}
}

func TestRedactURLError_WrappedChain_StillRedacts(t *testing.T) {
	const canary = "WRAPPED_CANARY"
	inner := mustURL(t, "https://api.example.com/x?apikey="+canary)
	wrapped := fmt.Errorf("outer layer: %w", inner)
	deeper := fmt.Errorf("even deeper: %w", wrapped)

	got := market.RedactURLError(deeper, "apikey").Error()
	if strings.Contains(got, canary) {
		t.Errorf("API key leaked through wrapped error chain: %s", got)
	}
	if !strings.Contains(got, "apikey=REDACTED") {
		t.Errorf("expected redaction through wrapped chain: %s", got)
	}
}

func TestRedactURLError_NoKeysRequested_LeavesURLAlone(t *testing.T) {
	const canary = "CANARY_KEY"
	in := mustURL(t, "https://api.example.com/x?apikey="+canary)
	got := market.RedactURLError(in).Error()
	// With no keys passed, no sanitization happens — caller is asking for
	// no-op behavior. (Not the recommended usage, but documented.)
	if !strings.Contains(got, canary) {
		t.Errorf("with zero keys, URL should be unchanged but was redacted: %s", got)
	}
}