package tui

import (
	"reflect"
	"testing"
)

func TestParseCommand_ValidNoArgs(t *testing.T) {
	cmd, ok := parseCommand("/theme")
	if !ok {
		t.Fatal("parseCommand(/theme) ok = false, want true")
	}
	if cmd.Cmd != "theme" {
		t.Errorf("Cmd = %q, want %q", cmd.Cmd, "theme")
	}
	if len(cmd.Args) != 0 {
		t.Errorf("Args = %v, want empty", cmd.Args)
	}
}

func TestParseCommand_ValidWithArgs(t *testing.T) {
	cmd, ok := parseCommand("/theme dark")
	if !ok {
		t.Fatal("parseCommand(/theme dark) ok = false, want true")
	}
	if cmd.Cmd != "theme" {
		t.Errorf("Cmd = %q, want %q", cmd.Cmd, "theme")
	}
	if !reflect.DeepEqual(cmd.Args, []string{"dark"}) {
		t.Errorf("Args = %v, want [dark]", cmd.Args)
	}
}

func TestParseCommand_LanguageWithArg(t *testing.T) {
	cmd, ok := parseCommand("/language es")
	if !ok {
		t.Fatal("parseCommand(/language es) ok = false, want true")
	}
	if cmd.Cmd != "language" {
		t.Errorf("Cmd = %q, want %q", cmd.Cmd, "language")
	}
	if !reflect.DeepEqual(cmd.Args, []string{"es"}) {
		t.Errorf("Args = %v, want [es]", cmd.Args)
	}
}

func TestParseCommand_Exit(t *testing.T) {
	cmd, ok := parseCommand("/exit")
	if !ok {
		t.Fatal("parseCommand(/exit) ok = false, want true")
	}
	if cmd.Cmd != "exit" {
		t.Errorf("Cmd = %q, want %q", cmd.Cmd, "exit")
	}
}

func TestParseCommand_Unknown(t *testing.T) {
	_, ok := parseCommand("/foobar")
	if ok {
		t.Fatal("parseCommand(/foobar) ok = true, want false for unknown command")
	}
}

func TestParseCommand_NoSlash(t *testing.T) {
	_, ok := parseCommand("theme")
	if ok {
		t.Fatal("parseCommand(theme) ok = true, want false for missing slash")
	}
}

func TestParseCommand_EmptySlash(t *testing.T) {
	_, ok := parseCommand("/")
	if ok {
		t.Fatal("parseCommand(/) ok = true, want false for slash-only input")
	}
}

func TestParseCommand_CaseInsensitive(t *testing.T) {
	cmd, ok := parseCommand("/THEME")
	if !ok {
		t.Fatal("parseCommand(/THEME) ok = false, want true")
	}
	if cmd.Cmd != "theme" {
		t.Errorf("Cmd = %q, want %q", cmd.Cmd, "theme")
	}
}
