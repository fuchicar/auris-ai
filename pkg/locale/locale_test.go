package locale_test

import (
	"testing"

	"auris/pkg/locale"
)

func TestDetect_Spanish(t *testing.T) {
	t.Setenv("LC_ALL", "es_ES.UTF-8")
	t.Setenv("LANG", "")
	tag, certain := locale.Detect()
	if tag != "es" {
		t.Fatalf("Detect() tag = %q, want %q", tag, "es")
	}
	if !certain {
		t.Fatal("Detect() certain = false, want true for known locale")
	}
}

func TestDetect_FallbackUnknownLocale(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LANG", "fr_FR.UTF-8")
	tag, certain := locale.Detect()
	if tag != "en" {
		t.Fatalf("Detect() tag = %q, want %q", tag, "en")
	}
	if certain {
		t.Fatal("Detect() certain = true, want false for unsupported locale")
	}
}

func TestDetect_EmptyEnv(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LANG", "")
	tag, certain := locale.Detect()
	if tag != "en" {
		t.Fatalf("Detect() tag = %q, want %q", tag, "en")
	}
	if certain {
		t.Fatal("Detect() certain = true, want false when env is empty")
	}
}

func TestDetect_English(t *testing.T) {
	t.Setenv("LC_ALL", "en_US.UTF-8")
	t.Setenv("LANG", "")
	tag, certain := locale.Detect()
	if tag != "en" {
		t.Fatalf("Detect() tag = %q, want %q", tag, "en")
	}
	if !certain {
		t.Fatal("Detect() certain = false, want true for en_US")
	}
}

func TestT_English(t *testing.T) {
	if err := locale.Init("en"); err != nil {
		t.Fatalf("Init: %v", err)
	}
	got := locale.T("welcome.title")
	want := "Welcome to Auris, your personal finance assistant"
	if got != want {
		t.Fatalf("T(welcome.title) = %q, want %q", got, want)
	}
}

func TestT_Spanish(t *testing.T) {
	if err := locale.Init("es"); err != nil {
		t.Fatalf("Init: %v", err)
	}
	got := locale.T("welcome.title")
	want := "Bienvenido a Auris, su asistente personal de finanzas"
	if got != want {
		t.Fatalf("T(welcome.title) = %q, want %q", got, want)
	}
}

func TestT_MissingKey(t *testing.T) {
	if err := locale.Init("en"); err != nil {
		t.Fatalf("Init: %v", err)
	}
	// Unknown keys should be returned as-is without panicking.
	key := "does.not.exist"
	if got := locale.T(key); got != key {
		t.Fatalf("T(%q) = %q, want the key itself", key, got)
	}
}
