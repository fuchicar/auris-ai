// Package market — helper for stripping sensitive credentials from
// transport-layer errors before they bubble up to tool results, the LLM,
// and on-disk session history.
package market

import (
	"errors"
	"net/url"
	"strings"
)

// redactedURL is the literal placeholder used in place of a URL that
// could not be parsed back without leaking its content.
const redactedURL = "(redacted)"

// transportErr is used as the leaf cause when sanitizing collapses a
// chain of *url.Error wrappers whose original leaf has already been
// dropped. Keeping it stable lets callers/tests assert on its presence
// without depending on the underlying transport.
var transportErr = errors.New("transport error")

// RedactURLError returns err with the values of any sensitive query
// parameters in a *url.Error's URL replaced by "REDACTED". Comparison is
// case-insensitive against each key. URLs that fail to parse are replaced
// entirely with "(redacted)" rather than leaked verbatim. Non-*url.Error
// errors (and nil) pass through unchanged so unrelated context is
// preserved.
//
// http.Client.Do typically wraps a transport's *url.Error with its own
// *url.Error, so the original chain can contain several URL-bearing
// layers. RedactURLError collapses the whole chain into a single
// sanitized *url.Error — otherwise the inner *url.Error.Err would
// re-print the unsanitized URL when the error is formatted.
func RedactURLError(err error, keys ...string) error {
	if err == nil {
		return nil
	}
	var urlErr *url.Error
	if !errors.As(err, &urlErr) {
		return err
	}

	leaf := collapseURLErrorChain(urlErr)

	sanitized, ok := sanitizeURLString(urlErr.URL, keys)
	if !ok {
		return &url.Error{Op: urlErr.Op, URL: redactedURL, Err: leaf}
	}
	return &url.Error{Op: urlErr.Op, URL: sanitized, Err: leaf}
}

// collapseURLErrorChain walks down a chain of *url.Error siblings and
// returns the deepest non-*url.Error cause. If no such cause exists it
// returns transportErr so the caller never holds onto an unsanitized
// URL in the leaf position.
func collapseURLErrorChain(urlErr *url.Error) error {
	inner := urlErr.Err
	for inner != nil {
		nested, ok := inner.(*url.Error)
		if !ok {
			return inner
		}
		inner = nested.Err
	}
	return transportErr
}

// sanitizeURLString parses rawURL and rewrites the values of any query
// parameters whose name matches one of keys (case-insensitive). Returns
// (sanitizedURL, true) on success, or ("", false) when the URL itself
// does not parse.
func sanitizeURLString(rawURL string, keys []string) (string, bool) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", false
	}
	if len(keys) == 0 || u.RawQuery == "" {
		return u.String(), true
	}

	keySet := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		keySet[strings.ToLower(k)] = struct{}{}
	}

	q := u.Query()
	changed := false
	for param := range q {
		if _, ok := keySet[strings.ToLower(param)]; !ok {
			continue
		}
		q.Set(param, "REDACTED")
		changed = true
	}
	if !changed {
		return u.String(), true
	}
	u.RawQuery = q.Encode()
	return u.String(), true
}