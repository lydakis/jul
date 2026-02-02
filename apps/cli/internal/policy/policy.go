package policy

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type PromotePolicy struct {
	MinCoveragePct              *float64
	Strategy                    string
	RequiredChecks              []string
	RequireSuggestionsAddressed *bool
}

func LoadPromotePolicy(repoRoot, target string) (PromotePolicy, bool, error) {
	if strings.TrimSpace(repoRoot) == "" {
		return PromotePolicy{}, false, fmt.Errorf("repo root required")
	}
	path := filepath.Join(repoRoot, ".jul", "policy.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return PromotePolicy{}, false, nil
		}
		return PromotePolicy{}, false, err
	}
	parsed := parsePolicyConfig(string(data))
	target = strings.TrimSpace(target)
	policy := PromotePolicy{}
	ok := false

	if target != "" {
		if applyPromoteSection(parsed, "promote."+target, &policy) {
			ok = true
		}
	}
	if !ok {
		if applyPromoteSection(parsed, "promote", &policy) {
			ok = true
		}
	}

	return policy, ok, nil
}

func applyPromoteSection(parsed map[string]string, section string, policy *PromotePolicy) bool {
	if policy == nil {
		return false
	}
	updated := false
	if val, ok := parsed[section+".min_coverage_pct"]; ok {
		if pct, err := strconv.ParseFloat(strings.TrimSpace(val), 64); err == nil {
			policy.MinCoveragePct = &pct
			updated = true
		}
	}
	if val, ok := parsed[section+".strategy"]; ok {
		if trimmed := strings.TrimSpace(val); trimmed != "" {
			policy.Strategy = trimmed
			updated = true
		}
	}
	if val, ok := parsed[section+".required_checks"]; ok {
		if list := parseStringList(val); len(list) > 0 {
			policy.RequiredChecks = list
			updated = true
		}
	}
	if val, ok := parsed[section+".require_suggestions_addressed"]; ok {
		if parsedBool, err := strconv.ParseBool(strings.TrimSpace(val)); err == nil {
			policy.RequireSuggestionsAddressed = &parsedBool
			updated = true
		}
	}
	return updated
}

func parseStringList(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
		trimmed = strings.TrimSpace(trimmed[1 : len(trimmed)-1])
	}
	if trimmed == "" {
		return nil
	}
	parts := strings.Split(trimmed, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		part = strings.Trim(part, "\"")
		part = strings.Trim(part, "'")
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func parsePolicyConfig(raw string) map[string]string {
	config := map[string]string{}
	section := ""
	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			section = strings.TrimSpace(trimmed[1 : len(trimmed)-1])
			continue
		}
		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, "\"")
		fullKey := key
		if section != "" {
			fullKey = section + "." + key
		}
		config[fullKey] = value
	}
	return config
}
