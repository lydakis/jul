package identity

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/metadata"
)

func ResolveUserNamespace(remoteName string) (string, error) {
	repoRoot, err := gitutil.RepoTopLevel()
	if err != nil {
		return fallbackNamespace(), nil
	}

	if strings.TrimSpace(remoteName) != "" {
		_, _ = gitutil.Git("-C", repoRoot, "fetch", remoteName, "+refs/notes/jul/repo-meta:refs/notes/jul/repo-meta")
	}

	rootSHA, _ := gitutil.RootCommit()
	repoMetaOK := false
	if rootSHA != "" {
		if meta, ok, err := metadata.ReadRepoMeta(rootSHA); err == nil && ok {
			if strings.TrimSpace(meta.UserNamespace) != "" {
				_ = config.SetRepoConfigValue("user", "user_namespace", strings.TrimSpace(meta.UserNamespace))
				return strings.TrimSpace(meta.UserNamespace), nil
			}
			repoMetaOK = ok
		}
	}

	if ns := strings.TrimSpace(config.UserNamespace()); ns != "" {
		if rootSHA != "" && !repoMetaOK {
			meta := metadata.RepoMeta{
				RepoID:        repoIDFromRoot(rootSHA),
				UserNamespace: ns,
				CreatedAt:     time.Now().UTC().Format(time.RFC3339),
				UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
			}
			_ = metadata.WriteRepoMeta(rootSHA, meta)
		}
		return ns, nil
	}

	ns := generateNamespace(config.UserName())
	_ = config.SetRepoConfigValue("user", "user_namespace", ns)

	if rootSHA != "" {
		meta := metadata.RepoMeta{
			RepoID:        repoIDFromRoot(rootSHA),
			UserNamespace: ns,
			CreatedAt:     time.Now().UTC().Format(time.RFC3339),
			UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
		}
		_ = metadata.WriteRepoMeta(rootSHA, meta)
	}

	return ns, nil
}

func repoIDFromRoot(rootSHA string) string {
	rootSHA = strings.TrimSpace(rootSHA)
	if len(rootSHA) >= 8 {
		return "jul:" + rootSHA[:8]
	}
	if rootSHA != "" {
		return "jul:" + rootSHA
	}
	return "jul:" + randomSuffix(4)
}

func generateNamespace(user string) string {
	base := slugify(user)
	if base == "" {
		base = "user"
	}
	return fmt.Sprintf("%s-%s", base, randomSuffix(2))
}

func fallbackNamespace() string {
	if ns := strings.TrimSpace(config.UserNamespace()); ns != "" {
		return ns
	}
	return generateNamespace(config.UserName())
}

func randomSuffix(bytes int) string {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return "0000"
	}
	return hex.EncodeToString(buf)
}

func slugify(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	var b strings.Builder
	prevDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			prevDash = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash {
				b.WriteRune('-')
				prevDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	return out
}
