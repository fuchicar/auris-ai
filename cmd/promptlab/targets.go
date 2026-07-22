package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// Target identifies one LLM to run the suite/prompt against: the same shape
// as the -provider/-base-url/-api-key/-model flags, plus a display Name for
// the matrix table and JSON output.
type Target struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
	BaseURL  string `json:"base_url,omitempty"`
	APIKey   string `json:"api_key,omitempty"`
	Model    string `json:"model"`
}

// TargetList is the top-level shape of a -targets JSON file.
type TargetList struct {
	Targets []Target `json:"targets"`
}

// LoadTargets reads and validates a -targets suite file.
func LoadTargets(path string) ([]Target, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var list TargetList
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if len(list.Targets) == 0 {
		return nil, fmt.Errorf("%s: no targets defined", path)
	}
	seen := make(map[string]bool, len(list.Targets))
	for i, t := range list.Targets {
		if t.Name == "" {
			return nil, fmt.Errorf("%s: targets[%d]: name is required", path, i)
		}
		if seen[t.Name] {
			return nil, fmt.Errorf("%s: duplicate target name %q", path, t.Name)
		}
		seen[t.Name] = true
		if t.Provider == "" {
			return nil, fmt.Errorf("%s: targets[%d] (%s): provider is required", path, i, t.Name)
		}
		if t.Model == "" {
			return nil, fmt.Errorf("%s: targets[%d] (%s): model is required", path, i, t.Name)
		}
	}
	return list.Targets, nil
}
