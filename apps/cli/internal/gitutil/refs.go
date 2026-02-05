package gitutil

import (
	"fmt"
	"strings"
)

func RefExists(ref string) bool {
	if strings.TrimSpace(ref) == "" {
		return false
	}
	if exists, ok := refExistsFast(ref); ok {
		return exists
	}
	_, err := git("show-ref", "--verify", "--quiet", ref)
	return err == nil
}

func ResolveRef(ref string) (string, error) {
	if strings.TrimSpace(ref) == "" {
		return "", fmt.Errorf("ref required")
	}
	if sha, ok := resolveRefFast(ref); ok {
		return sha, nil
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

func ExtractTraceHead(message string) string {
	return extractTrailer("Trace-Head", message)
}

func ExtractTraceBase(message string) string {
	return extractTrailer("Trace-Base", message)
}

func extractTrailer(key, message string) string {
	if strings.TrimSpace(message) == "" || strings.TrimSpace(key) == "" {
		return ""
	}
	lines := strings.Split(message, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		prefix := key + ":"
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

func TreeOf(ref string) (string, error) {
	if strings.TrimSpace(ref) == "" {
		return "", fmt.Errorf("ref required")
	}
	return git("rev-parse", ref+"^{tree}")
}

func CommitTree(treeSHA, parentSHA, message string) (string, error) {
	if strings.TrimSpace(treeSHA) == "" {
		return "", fmt.Errorf("tree sha required")
	}
	args := []string{"commit-tree", treeSHA}
	if strings.TrimSpace(parentSHA) != "" {
		args = append(args, "-p", parentSHA)
	}
	if strings.TrimSpace(message) == "" {
		return "", fmt.Errorf("commit message required")
	}
	args = append(args, "-m", message)
	return git(args...)
}

func CommitTreeWithParents(treeSHA string, parents []string, message string) (string, error) {
	if strings.TrimSpace(treeSHA) == "" {
		return "", fmt.Errorf("tree sha required")
	}
	args := []string{"commit-tree", treeSHA}
	for _, parent := range parents {
		if strings.TrimSpace(parent) == "" {
			continue
		}
		args = append(args, "-p", parent)
	}
	if strings.TrimSpace(message) == "" {
		return "", fmt.Errorf("commit message required")
	}
	args = append(args, "-m", message)
	return git(args...)
}

func MergeBase(a, b string) (string, error) {
	if strings.TrimSpace(a) == "" || strings.TrimSpace(b) == "" {
		return "", fmt.Errorf("merge base requires two refs")
	}
	return git("merge-base", a, b)
}

func IsAncestor(ancestor, descendant string) bool {
	if strings.TrimSpace(ancestor) == "" || strings.TrimSpace(descendant) == "" {
		return false
	}
	_, err := git("merge-base", "--is-ancestor", ancestor, descendant)
	return err == nil
}
