package syncer

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/syncignore"
)

var draftSecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`ghp_[A-Za-z0-9]{30,}`),
	regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{10,}`),
	regexp.MustCompile(`sk-[A-Za-z0-9]{16,}`),
	regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._-]+`),
	regexp.MustCompile(`(?i)(api[_-]?key|secret|token|password|pwd)\s*[:=]\s*\S+`),
}

func DraftPushAllowed(repoRoot, baseSHA, draftSHA string, allowSecrets bool) (bool, string, error) {
	if allowSecrets {
		return true, "", nil
	}
	base := strings.TrimSpace(baseSHA)
	draft := strings.TrimSpace(draftSHA)
	if draft == "" || repoRoot == "" {
		return true, "", nil
	}
	files, err := draftChangedFiles(repoRoot, base, draft)
	if err != nil {
		return false, "", err
	}
	if len(files) == 0 {
		return true, "", nil
	}
	ignore := syncignore.Load(repoRoot)
	for _, path := range files {
		if syncignore.Match(path, ignore) {
			continue
		}
		content, err := gitutil.Git("-C", repoRoot, "show", fmt.Sprintf("%s:%s", draft, path))
		if err != nil {
			continue
		}
		if containsSecret(content) {
			return false, fmt.Sprintf("draft sync blocked: potential secret in %s (use --allow-secrets to override)", path), nil
		}
	}
	return true, "", nil
}

func draftChangedFiles(repoRoot, baseSHA, draftSHA string) ([]string, error) {
	args := []string{"-C", repoRoot, "diff", "--name-only"}
	if strings.TrimSpace(baseSHA) != "" {
		args = append(args, strings.TrimSpace(baseSHA), strings.TrimSpace(draftSHA))
	} else {
		args = append(args, "--root", strings.TrimSpace(draftSHA))
	}
	out, err := gitutil.Git(args...)
	if err != nil {
		return nil, err
	}
	lines := strings.Fields(strings.TrimSpace(out))
	return lines, nil
}

func containsSecret(content string) bool {
	for _, re := range draftSecretPatterns {
		if re.MatchString(content) {
			return true
		}
	}
	return false
}
