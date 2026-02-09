package gitutil

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func findRepoRoot(start string) (string, bool) {
	dir := start
	for dir != "" {
		gitPath := filepath.Join(dir, ".git")
		if _, err := os.Stat(gitPath); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", false
}

type HeadInfo struct {
	SHA      string
	Branch   string
	RepoRoot string
}

func ReadHeadInfo() (HeadInfo, error) {
	repoRoot, err := RepoTopLevel()
	if err != nil || strings.TrimSpace(repoRoot) == "" {
		return HeadInfo{}, fmt.Errorf("repo root not found")
	}
	gitDir, err := gitDirForRoot(repoRoot)
	if err != nil {
		return HeadInfo{}, err
	}
	headPath := filepath.Join(gitDir, "HEAD")
	data, err := os.ReadFile(headPath)
	if err != nil {
		return HeadInfo{}, err
	}
	line := strings.TrimSpace(string(data))
	if line == "" {
		return HeadInfo{}, fmt.Errorf("empty HEAD")
	}
	if strings.HasPrefix(line, "ref:") {
		ref := strings.TrimSpace(strings.TrimPrefix(line, "ref:"))
		if ref == "" {
			return HeadInfo{}, fmt.Errorf("empty HEAD ref")
		}
		sha, ok := readRefFromGitDir(gitDir, ref)
		if !ok || strings.TrimSpace(sha) == "" {
			return HeadInfo{}, fmt.Errorf("failed to resolve %s", ref)
		}
		branch := ref
		if strings.HasPrefix(ref, "refs/heads/") {
			branch = strings.TrimPrefix(ref, "refs/heads/")
		}
		return HeadInfo{SHA: sha, Branch: branch, RepoRoot: repoRoot}, nil
	}
	return HeadInfo{SHA: line, Branch: "HEAD", RepoRoot: repoRoot}, nil
}

func gitDirForRoot(repoRoot string) (string, error) {
	gitPath := filepath.Join(repoRoot, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return gitPath, nil
	}
	data, err := os.ReadFile(gitPath)
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(data))
	const prefix = "gitdir:"
	if !strings.HasPrefix(line, prefix) {
		return "", fmt.Errorf("unexpected gitdir format")
	}
	path := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	if path == "" {
		return "", fmt.Errorf("empty gitdir path")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(repoRoot, path)
	}
	return path, nil
}

func resolveRefFast(ref string) (string, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", false
	}
	if ref != "HEAD" && !strings.HasPrefix(ref, "refs/") {
		return "", false
	}
	if strings.Contains(ref, "..") || strings.Contains(ref, "@{") || strings.ContainsAny(ref, "^~:") {
		return "", false
	}
	repoRoot, err := RepoTopLevel()
	if err != nil || strings.TrimSpace(repoRoot) == "" {
		return "", false
	}
	gitDir, err := gitDirForRoot(repoRoot)
	if err != nil {
		return "", false
	}
	return readRefFromGitDir(gitDir, ref)
}

func readRefFromGitDir(gitDir, ref string) (string, bool) {
	if strings.TrimSpace(ref) == "" {
		return "", false
	}
	if ref == "HEAD" {
		headPath := filepath.Join(gitDir, "HEAD")
		data, err := os.ReadFile(headPath)
		if err != nil {
			return "", false
		}
		line := strings.TrimSpace(string(data))
		if strings.HasPrefix(line, "ref:") {
			target := strings.TrimSpace(strings.TrimPrefix(line, "ref:"))
			if target == "" {
				return "", false
			}
			return readRefFromGitDir(gitDir, target)
		}
		if line == "" {
			return "", false
		}
		return line, true
	}
	if !strings.HasPrefix(ref, "refs/") {
		return "", false
	}
	refPath := filepath.Join(gitDir, filepath.FromSlash(ref))
	if data, err := os.ReadFile(refPath); err == nil {
		sha := strings.TrimSpace(string(data))
		if sha != "" {
			return sha, true
		}
	}
	packedPath := filepath.Join(gitDir, "packed-refs")
	file, err := os.Open(packedPath)
	if err != nil {
		return "", false
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "^") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if fields[1] == ref {
			return fields[0], true
		}
	}
	return "", false
}

func refExistsFast(ref string) (bool, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return false, true
	}
	if ref != "HEAD" && !strings.HasPrefix(ref, "refs/") {
		return false, false
	}
	if strings.Contains(ref, "..") || strings.Contains(ref, "@{") || strings.ContainsAny(ref, "^~:") {
		return false, false
	}
	repoRoot, err := RepoTopLevel()
	if err != nil || strings.TrimSpace(repoRoot) == "" {
		return false, false
	}
	gitDir, err := gitDirForRoot(repoRoot)
	if err != nil {
		return false, false
	}
	if _, ok := readRefFromGitDir(gitDir, ref); ok {
		return true, true
	}
	return false, true
}

func ListRefsFast(prefix string) ([]string, bool, error) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" || !strings.HasPrefix(prefix, "refs/") {
		return nil, false, nil
	}
	hadTrailingSlash := strings.HasSuffix(prefix, "/")
	normalizedPrefix := strings.TrimSuffix(prefix, "/")
	if normalizedPrefix == "" {
		return nil, false, nil
	}
	repoRoot, err := RepoTopLevel()
	if err != nil || strings.TrimSpace(repoRoot) == "" {
		return nil, false, nil
	}
	gitDir, err := gitDirForRoot(repoRoot)
	if err != nil {
		return nil, false, nil
	}

	seen := map[string]struct{}{}
	matchesPrefix := func(ref string) bool {
		if !strings.HasPrefix(ref, normalizedPrefix) {
			return false
		}
		if !hadTrailingSlash {
			return true
		}
		if len(ref) <= len(normalizedPrefix) {
			return false
		}
		return ref[len(normalizedPrefix)] == '/'
	}
	collect := func(ref string) {
		if ref == "" {
			return
		}
		if !matchesPrefix(ref) {
			return
		}
		seen[ref] = struct{}{}
	}

	dirPath := filepath.Join(gitDir, filepath.FromSlash(normalizedPrefix))
	if info, err := os.Stat(dirPath); err == nil {
		if info.IsDir() {
			_ = filepath.WalkDir(dirPath, func(path string, d os.DirEntry, err error) error {
				if err != nil {
					return nil
				}
				if d.IsDir() {
					return nil
				}
				rel, err := filepath.Rel(dirPath, path)
				if err != nil {
					return nil
				}
				ref := normalizedPrefix
				if rel != "." {
					ref = normalizedPrefix + "/" + filepath.ToSlash(rel)
				}
				collect(ref)
				return nil
			})
		} else if !hadTrailingSlash {
			collect(normalizedPrefix)
		}
	}

	packedPath := filepath.Join(gitDir, "packed-refs")
	if file, err := os.Open(packedPath); err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "^") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			collect(fields[1])
		}
	}

	if len(seen) == 0 {
		return []string{}, true, nil
	}
	refs := make([]string, 0, len(seen))
	for ref := range seen {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	return refs, true, nil
}
