package gitutil

import (
	"fmt"
	"strings"
)

func RefExists(ref string) bool {
	if strings.TrimSpace(ref) == "" {
		return false
	}
	_, err := git("show-ref", "--verify", "--quiet", ref)
	return err == nil
}

func ResolveRef(ref string) (string, error) {
	if strings.TrimSpace(ref) == "" {
		return "", fmt.Errorf("ref required")
	}
	return git("rev-parse", ref)
}

func UpdateRef(ref, sha string) error {
	if strings.TrimSpace(ref) == "" || strings.TrimSpace(sha) == "" {
		return fmt.Errorf("ref and sha required")
	}
	_, err := git("update-ref", ref, sha)
	return err
}

func ParentOf(sha string) (string, error) {
	if strings.TrimSpace(sha) == "" {
		return "", fmt.Errorf("sha required")
	}
	return git("rev-parse", sha+"^")
}

func CommitMessage(ref string) (string, error) {
	if strings.TrimSpace(ref) == "" {
		return "", fmt.Errorf("ref required")
	}
	return git("log", "-1", "--format=%B", ref)
}
