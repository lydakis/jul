package gitutil

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type CommitInfo struct {
	SHA       string
	Author    string
	Message   string
	Committed time.Time
	Branch    string
	RepoName  string
	ChangeID  string
	TopLevel  string
}

func CurrentCommit() (CommitInfo, error) {
	sha, err := git("rev-parse", "HEAD")
	if err != nil {
		return CommitInfo{}, err
	}
	branch, _ := git("rev-parse", "--abbrev-ref", "HEAD")
	message, _ := git("log", "-1", "--format=%B")
	author, _ := git("log", "-1", "--format=%an")
	committedISO, err := git("log", "-1", "--format=%cI")
	if err != nil {
		return CommitInfo{}, err
	}
	top, _ := git("rev-parse", "--show-toplevel")

	committed := time.Now().UTC()
	if committedISO != "" {
		if parsed, err := time.Parse(time.RFC3339, committedISO); err == nil {
			committed = parsed
		}
	}

	changeID := ExtractChangeID(message)
	repoName := repoNameFromTopLevel(top)

	return CommitInfo{
		SHA:       sha,
		Author:    author,
		Message:   message,
		Committed: committed,
		Branch:    branch,
		RepoName:  repoName,
		ChangeID:  changeID,
		TopLevel:  top,
	}, nil
}

func RepoTopLevel() (string, error) {
	return git("rev-parse", "--show-toplevel")
}

func GitPath(repoRoot, path string) (string, error) {
	if strings.TrimSpace(repoRoot) == "" {
		return "", fmt.Errorf("repo root required")
	}
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("git path required")
	}
	resolved, err := gitWithDir(repoRoot, "rev-parse", "--git-path", path)
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(resolved) {
		return resolved, nil
	}
	return filepath.Join(repoRoot, resolved), nil
}

func ExtractChangeID(message string) string {
	lines := strings.Split(message, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "change-id:") {
			value := strings.TrimSpace(trimmed[len("change-id:"):])
			if strings.HasPrefix(value, "I") && len(value) == 41 {
				return value
			}
		}
		if strings.HasPrefix(lower, "change-id ") {
			value := strings.TrimSpace(trimmed[len("change-id "):])
			if strings.HasPrefix(value, "I") && len(value) == 41 {
				return value
			}
		}
	}
	return ""
}

func repoNameFromTopLevel(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	parts := strings.Split(strings.TrimRight(path, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func FallbackChangeID(commitSHA string) string {
	hash := sha1.Sum([]byte(commitSHA))
	return "I" + hex.EncodeToString(hash[:])
}

func git(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s failed: %s", strings.Join(args, " "), strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

// Git exposes git execution for CLI commands that need to resolve refs.
func Git(args ...string) (string, error) {
	return git(args...)
}

func gitWithDir(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git -C %s %s failed: %s", dir, strings.Join(args, " "), strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(out.String()), nil
}
