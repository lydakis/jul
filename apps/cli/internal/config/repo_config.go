package config

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func SetRepoConfigValue(section, key, value string) error {
	path, err := repoConfigPath()
	if err != nil {
		return err
	}
	sections := map[string]map[string]string{}
	if data, err := os.ReadFile(path); err == nil {
		sections = parseConfigSections(string(data))
	}
	sec := strings.TrimSpace(section)
	if sec == "" {
		sec = "default"
	}
	if sections[sec] == nil {
		sections[sec] = map[string]string{}
	}
	sections[sec][strings.TrimSpace(key)] = value

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	content := renderConfigSections(sections)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return err
	}
	updateConfigCache(path, []byte(content))
	return nil
}

func parseConfigSections(raw string) map[string]map[string]string {
	out := map[string]map[string]string{}
	flat := parseUserConfig(raw)
	for fullKey, val := range flat {
		section := ""
		key := fullKey
		if parts := strings.SplitN(fullKey, ".", 2); len(parts) == 2 {
			section = parts[0]
			key = parts[1]
		}
		if section == "" {
			section = "default"
		}
		if out[section] == nil {
			out[section] = map[string]string{}
		}
		out[section][key] = val
	}
	return out
}

func renderConfigSections(sections map[string]map[string]string) string {
	sectionNames := make([]string, 0, len(sections))
	for name := range sections {
		sectionNames = append(sectionNames, name)
	}
	sort.Strings(sectionNames)

	var b strings.Builder
	for i, name := range sectionNames {
		if i > 0 {
			b.WriteString("\n")
		}
		if name != "default" {
			b.WriteString("[")
			b.WriteString(name)
			b.WriteString("]\n")
		}
		keys := make([]string, 0, len(sections[name]))
		for key := range sections[name] {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			b.WriteString(key)
			b.WriteString(" = ")
			b.WriteString("\"")
			b.WriteString(sections[name][key])
			b.WriteString("\"\n")
		}
	}
	return b.String()
}
