package ci

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func InferDefaultCommands(root string) []string {
	if root == "" {
		return []string{"true"}
	}
	if fileExists(filepath.Join(root, "go.mod")) {
		return []string{"go test ./..."}
	}
	workPath := filepath.Join(root, "go.work")
	if fileExists(workPath) {
		data, err := os.ReadFile(workPath)
		if err == nil {
			uses := parseGoWorkUses(string(data))
			if len(uses) > 0 {
				cmds := make([]string, 0, len(uses))
				for _, use := range uses {
					path := strings.TrimSpace(use)
					if path == "" {
						continue
					}
					cmds = append(cmds, "cd "+shellQuote(path)+" && go test ./...")
				}
				if len(cmds) > 0 {
					return cmds
				}
			}
		}
	}
	return []string{"true"}
}

func parseGoWorkUses(raw string) []string {
	var uses []string
	inBlock := false
	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}
		if strings.HasPrefix(trimmed, "use ") {
			rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "use"))
			if rest == "(" {
				inBlock = true
				continue
			}
			path := strings.Trim(strings.TrimSpace(rest), "\"")
			if path != "" {
				uses = append(uses, path)
			}
			continue
		}
		if trimmed == "use(" {
			inBlock = true
			continue
		}
		if inBlock {
			if trimmed == ")" {
				inBlock = false
				continue
			}
			trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "//"))
			if trimmed == "" {
				continue
			}
			path := strings.Trim(trimmed, "\"")
			if path != "" {
				uses = append(uses, path)
			}
		}
	}
	return uses
}

func shellQuote(value string) string {
	return strconv.Quote(value)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
