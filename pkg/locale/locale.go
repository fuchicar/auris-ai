// Package locale provides language detection and translation helpers for Auris.
// It wraps [go-i18n/v2] and embeds the bundled locale files so that the
// binary ships with all translations out of the box.
//
// Usage:
//
//	tag, certain := locale.Detect()
//	locale.Init(tag)
//	fmt.Println(locale.T("welcome.title"))
package locale

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

//go:embed locales/*.json
var localeFS embed.FS

var (
	bundle    *i18n.Bundle
	localizer *i18n.Localizer
)

// supportedLocales is the set of BCP-47 tags that Auris ships translations for.
var supportedLocales = map[string]bool{"en": true, "es": true}

// Detect reads LC_ALL and LANG environment variables and returns the best
// matching supported BCP-47 tag plus a certainty flag.
//
// certain is true when a supported language is unambiguously identified from
// the environment. It is false when both variables are empty or when the
// detected code is not in the supported set — in that case the caller should
// prompt the user to choose a language explicitly.
func Detect() (tag string, certain bool) {
	for _, env := range []string{"LC_ALL", "LANG"} {
		val := os.Getenv(env)
		if val == "" {
			continue
		}
		// "es_ES.UTF-8" → "es"
		lang := strings.ToLower(val)
		if idx := strings.IndexAny(lang, "_."); idx != -1 {
			lang = lang[:idx]
		}
		if supportedLocales[lang] {
			return lang, true
		}
		// A value was present but it's not a supported locale — uncertain.
		return "en", false
	}
	// Both variables were empty.
	return "en", false
}

// Init initializes the translation bundle for the given BCP-47 tag (e.g. "en"
// or "es"). Must be called once at startup, before any call to [T] or [Tp].
func Init(tag string) error {
	bundle = i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("json", json.Unmarshal)

	for _, f := range []string{"locales/en.json", "locales/es.json"} {
		data, err := localeFS.ReadFile(f)
		if err != nil {
			return fmt.Errorf("locale: read %s: %w", f, err)
		}
		if _, err := bundle.ParseMessageFileBytes(data, f); err != nil {
			return fmt.Errorf("locale: parse %s: %w", f, err)
		}
	}

	localizer = i18n.NewLocalizer(bundle, tag, "en")
	return nil
}

// T translates the message identified by id using the active locale.
// Returns id itself when the key is not found — it never panics.
func T(id string) string {
	if localizer == nil {
		return id
	}
	out, err := localizer.Localize(&i18n.LocalizeConfig{MessageID: id})
	if err != nil {
		return id
	}
	return out
}

// Tp translates the message identified by id, substituting data into Go
// template placeholders (e.g. {{.Name}}). Returns id on failure.
func Tp(id string, data any) string {
	if localizer == nil {
		return id
	}
	out, err := localizer.Localize(&i18n.LocalizeConfig{
		MessageID:    id,
		TemplateData: data,
	})
	if err != nil {
		return id
	}
	return out
}
