package gitutil

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
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

var (
	repoTopLevelMu    sync.Mutex
	repoTopLevelCache = map[string]string{}
)

func CurrentCommit() (CommitInfo, error) {
	meta, err := git("log", "-1", "--format=%H%x00%an%x00%cI%x00%B")
	if err != nil {
		return CommitInfo{}, err
	}
	parts := strings.SplitN(meta, "\x00", 4)
	if len(parts) < 4 {
		return CommitInfo{}, fmt.Errorf("unexpected git log output")
	}
	sha := parts[0]
	author := parts[1]
	committedISO := parts[2]
	message := parts[3]
	refOut, _ := git("rev-parse", "--show-toplevel", "--abbrev-ref", "HEAD")
	top := ""
	branch := ""
	if strings.TrimSpace(refOut) != "" {
		lines := strings.Split(refOut, "\n")
		top = strings.TrimSpace(lines[0])
		if len(lines) > 1 {
			branch = strings.TrimSpace(lines[1])
		}
	}

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
	wd, err := os.Getwd()
	if err == nil {
		repoTopLevelMu.Lock()
		if cached, ok := repoTopLevelCache[wd]; ok && cached != "" {
			repoTopLevelMu.Unlock()
			return cached, nil
		}
		repoTopLevelMu.Unlock()
	}
	root, err := git("rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	if wd != "" && strings.TrimSpace(root) != "" {
		repoTopLevelMu.Lock()
		repoTopLevelCache[wd] = root
		repoTopLevelMu.Unlock()
	}
	return root, nil
}

func RootCommit() (string, error) {
	if _, err := git("rev-parse", "--verify", "HEAD"); err != nil {
		return "", nil
	}
	out, err := git("rev-list", "--max-parents=0", "HEAD")
	if err != nil {
		return "", err
	}
	lines := strings.Fields(out)
	if len(lines) == 0 {
		return "", nil
	}
	return strings.TrimSpace(lines[0]), nil
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
