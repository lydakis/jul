package gitutil

import (
	"fmt"
	"strings"
)

func EnsureHeadRef(repoRoot, ref, sha string) error {
	if strings.TrimSpace(ref) == "" || strings.TrimSpace(sha) == "" {
		return fmt.Errorf("head ref and sha required")
	}
	if err := UpdateRef(ref, strings.TrimSpace(sha)); err != nil {
		return err
	}
	if current, err := Git("-C", repoRoot, "symbolic-ref", "-q", "HEAD"); err == nil {
		if strings.TrimSpace(current) == strings.TrimSpace(ref) {
			return nil
		}
	}
	_, err := Git("-C", repoRoot, "symbolic-ref", "HEAD", ref)
	return err
}
