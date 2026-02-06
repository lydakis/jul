package gitutil

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/lydakis/jul/cli/internal/syncignore"
)

func CreateDraftCommit(parentSHA, changeID string) (string, error) {
	repoRoot, err := RepoTopLevel()
	if err != nil {
		return "", err
	}
	treeSHA, err := DraftTree()
	if err != nil {
		return "", err
	}
	message := DraftMessage(changeID)
	return commitTree(repoRoot, treeSHA, parentSHA, message)
}

func DraftMessage(changeID string) string {
	if strings.TrimSpace(changeID) == "" {
		return "[draft] WIP"
	}
	return fmt.Sprintf("[draft] WIP\n\nChange-Id: %s\n", changeID)
}

func WorkspaceBaseMarkerMessage() string {
	return "jul: workspace base\n\nJul-Type: workspace-base\n"
}

func CreateWorkspaceBaseMarker(treeSHA, parentSHA string) (string, error) {
	repoRoot, err := RepoTopLevel()
	if err != nil {
		return "", err
	}
	message := WorkspaceBaseMarkerMessage()
	return commitTree(repoRoot, treeSHA, parentSHA, message)
}

func CreateDraftCommitFromTree(treeSHA, parentSHA, changeID string) (string, error) {
	repoRoot, err := RepoTopLevel()
	if err != nil {
		return "", err
	}
	message := DraftMessage(changeID)
	return commitTree(repoRoot, treeSHA, parentSHA, message)
}

func DraftTree() (string, error) {
	repoRoot, err := RepoTopLevel()
	if err != nil {
		return "", err
	}
	julDir := filepath.Join(repoRoot, ".jul")
	if err := os.MkdirAll(julDir, 0o755); err != nil {
		return "", err
	}
	indexPath := filepath.Join(julDir, "draft-index")
	return writeTree(repoRoot, indexPath)
}

func writeTree(repoRoot, indexPath string) (string, error) {
	excludePath, err := writeTempExcludes(repoRoot)
	if err != nil {
		return "", err
	}
	defer os.Remove(excludePath)

	if _, err := os.Stat(indexPath); err == nil {
		if err := updateIndexIncremental(repoRoot, indexPath, excludePath); err == nil {
			return gitWithEnv(repoRoot, map[string]string{
				"GIT_INDEX_FILE": indexPath,
			}, "write-tree")
		}
	}

	if err := runGitWithEnv(repoRoot, map[string]string{
		"GIT_INDEX_FILE": indexPath,
	}, "-c", "core.excludesfile="+excludePath, "add", "-A", "--", "."); err != nil {
		return "", err
	}
	return gitWithEnv(repoRoot, map[string]string{
		"GIT_INDEX_FILE": indexPath,
	}, "write-tree")
}

func updateIndexIncremental(repoRoot, indexPath, excludePath string) error {
	env := map[string]string{
		"GIT_INDEX_FILE": indexPath,
	}
	statusOut, err := gitWithEnvRaw(repoRoot, env, "-c", "core.excludesfile="+excludePath, "status", "--porcelain", "-z", "-unormal")
	if err != nil {
		return err
	}
	paths := collectChangedPathsFromStatus(statusOut)
	if len(paths) == 0 {
		return nil
	}
	args := append([]string{"-c", "core.excludesfile=" + excludePath, "add", "-A", "--"}, paths...)
	return runGitWithEnv(repoRoot, env, args...)
}

func collectChangedPathsFromStatus(statusOutput string) []string {
	seen := map[string]struct{}{}
	expectRenameTarget := false
	for _, entry := range strings.Split(statusOutput, "\x00") {
		if entry == "" {
			continue
		}
		path := ""
		if expectRenameTarget {
			path = strings.TrimSpace(entry)
			expectRenameTarget = false
		} else if len(entry) > 3 && entry[2] == ' ' {
			statusCode := entry[:2]
			path = strings.TrimSpace(entry[3:])
			if strings.Contains(statusCode, "R") || strings.Contains(statusCode, "C") {
				expectRenameTarget = true
			}
		} else {
			path = strings.TrimSpace(entry)
		}
		if path == "" {
			continue
		}
		seen[path] = struct{}{}
	}
	if len(seen) == 0 {
		return nil
	}
	paths := make([]string, 0, len(seen))
	for path := range seen {
		paths = append(paths, path)
	}
	return paths
}

func writeTempExcludes(repoRoot string) (string, error) {
	file, err := os.CreateTemp("", "jul-exclude-")
	if err != nil {
		return "", err
	}
	patterns := syncignore.Load(repoRoot)
	for _, pattern := range patterns {
		if _, err := file.WriteString(pattern + "\n"); err != nil {
			_ = file.Close()
			_ = os.Remove(file.Name())
			return "", err
		}
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(file.Name())
		return "", err
	}
	return file.Name(), nil
}

func commitTree(repoRoot, treeSHA, parentSHA, message string) (string, error) {
	args := []string{"commit-tree", treeSHA}
	if strings.TrimSpace(parentSHA) != "" {
		args = append(args, "-p", parentSHA)
	}
	args = append(args, "-m", message)
	return gitWithEnv(repoRoot, nil, args...)
}

func gitWithEnv(dir string, env map[string]string, args ...string) (string, error) {
	output, err := gitWithEnvRaw(dir, env, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

func gitWithEnvRaw(dir string, env map[string]string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), flattenEnv(env)...)
	}
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git -C %s %s failed: %s", dir, strings.Join(args, " "), strings.TrimSpace(stderr.String()))
	}
	return out.String(), nil
}

func runGitWithEnv(dir string, env map[string]string, args ...string) error {
	_, err := gitWithEnv(dir, env, args...)
	return err
}

func flattenEnv(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for key, value := range env {
		out = append(out, fmt.Sprintf("%s=%s", key, value))
	}
	return out
}
