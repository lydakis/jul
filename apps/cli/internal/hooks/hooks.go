package hooks

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lydakis/jul/cli/internal/gitutil"
)

const (
	postCommitHookName = "post-commit"
	hookMarker         = "# jul post-commit hook"
)

func InstallPostCommit(repoRoot, cliCommand string) (string, error) {
	if repoRoot == "" {
		return "", errors.New("repo root required")
	}
	if cliCommand == "" {
		cliCommand = "jul"
	}

	hooksDir, err := gitutil.GitPath(repoRoot, "hooks")
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return "", err
	}

	hookPath := filepath.Join(hooksDir, postCommitHookName)
	if exists, err := fileExists(hookPath); err != nil {
		return "", err
	} else if exists {
		contents, err := os.ReadFile(hookPath)
		if err != nil {
			return "", err
		}
		if !strings.Contains(string(contents), hookMarker) {
			return "", fmt.Errorf("post-commit hook already exists and is not managed by jul: %s", hookPath)
		}
	}

	script := buildPostCommitHook(cliCommand)
	if err := os.WriteFile(hookPath, []byte(script), 0o755); err != nil {
		return "", err
	}

	return hookPath, nil
}

func UninstallPostCommit(repoRoot string) error {
	if repoRoot == "" {
		return errors.New("repo root required")
	}

	hooksDir, err := gitutil.GitPath(repoRoot, "hooks")
	if err != nil {
		return err
	}
	hookPath := filepath.Join(hooksDir, postCommitHookName)
	if exists, err := fileExists(hookPath); err != nil {
		return err
	} else if !exists {
		return nil
	}

	contents, err := os.ReadFile(hookPath)
	if err != nil {
		return err
	}
	if !strings.Contains(string(contents), hookMarker) {
		return fmt.Errorf("post-commit hook is not managed by jul: %s", hookPath)
	}

	return os.Remove(hookPath)
}

func StatusPostCommit(repoRoot string) (bool, string, error) {
	if repoRoot == "" {
		return false, "", errors.New("repo root required")
	}

	hooksDir, err := gitutil.GitPath(repoRoot, "hooks")
	if err != nil {
		return false, "", err
	}
	hookPath := filepath.Join(hooksDir, postCommitHookName)
	if exists, err := fileExists(hookPath); err != nil {
		return false, "", err
	} else if !exists {
		return false, hookPath, nil
	}

	contents, err := os.ReadFile(hookPath)
	if err != nil {
		return false, "", err
	}
	return strings.Contains(string(contents), hookMarker), hookPath, nil
}

func buildPostCommitHook(cliCommand string) string {
	return fmt.Sprintf(`#!/bin/sh
%s

set -e

if [ -n "$JUL_NO_SYNC" ]; then
  exit 0
fi

JUL_CMD="%s"
if [ -n "$JUL_HOOK_CMD" ]; then
  JUL_CMD="$JUL_HOOK_CMD"
fi

if command -v "$JUL_CMD" >/dev/null 2>&1; then
  "$JUL_CMD" sync >/dev/null 2>&1 || true
else
  if [ -n "$JUL_HOOK_VERBOSE" ]; then
    echo "jul hook: command not found: $JUL_CMD" >&2
  fi
fi
`, hookMarker, cliCommand)
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
