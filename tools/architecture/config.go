// Package architecture inspects the project's package roles and enforcement inputs.
// The graph selectors follow itos-cc architecture-check (ADR-0051) and itos
// ADR-0053, with no copied legacy exceptions or generated configuration.
package architecture

import (
	"fmt"
	"os"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Rule is the depguard policy needed to validate that inspection is meaningful.
type Rule struct {
	Files []string `yaml:"files"`
	Allow []string `yaml:"allow"`
	Deny  []struct {
		Pkg string `yaml:"pkg"`
	} `yaml:"deny"`
}

// Rules reads real YAML, rejecting an absent, malformed or empty rule set.
func Rules(path string) (map[string]Rule, error) {
	data, err := os.ReadFile(path) //nolint:gosec // callers name the policy file in the inspected checkout
	if err != nil {
		return nil, err
	}
	var config struct {
		Version string `yaml:"version"`
		Linters struct {
			Settings struct {
				Depguard struct {
					Rules map[string]Rule `yaml:"rules"`
				} `yaml:"depguard"`
			} `yaml:"settings"`
		} `yaml:"linters"`
	}
	if err = yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	rules := config.Linters.Settings.Depguard.Rules
	if config.Version != "2" || len(rules) == 0 {
		return nil, fmt.Errorf("config: version 2 and nonempty depguard rules required")
	}
	for name, rule := range rules {
		if len(rule.Files) == 0 || len(rule.Allow)+len(rule.Deny) == 0 {
			return nil, fmt.Errorf("config: empty depguard rule %q", name)
		}
		for _, deny := range rule.Deny {
			if strings.TrimSpace(deny.Pkg) == "" {
				return nil, fmt.Errorf("config: empty deny in rule %q", name)
			}
		}
	}
	return rules, nil
}
