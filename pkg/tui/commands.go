package tui

import (
	"slices"
	"strings"
)

// knownCommands lists all slash commands recognized by the main menu.
// This slice is used for hint display and autocompletion in the future.
var knownCommands = []string{"theme", "language", "profile", "model", "marketproviders", "aiproviders", "changepassphrase", "encryption", "agent", "menu", "exit"}

// parseCommand parses a slash-prefixed user input string into a [CommandResult].
// Input examples: "/theme", "/theme dark", "/language es".
//
// Returns (result, true) when the input starts with "/" and contains a
// recognised command name. Returns (zero, false) for any other input.
func parseCommand(input string) (CommandResult, bool) {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/") {
		return CommandResult{}, false
	}
	parts := strings.Fields(input[1:])
	if len(parts) == 0 {
		return CommandResult{}, false
	}
	cmd := strings.ToLower(parts[0])
	if slices.Contains(knownCommands, cmd) {
		return CommandResult{Cmd: cmd, Args: parts[1:]}, true
	}
	return CommandResult{}, false
}
