package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lydakis/jul/server/internal/events"
	"github.com/lydakis/jul/server/internal/server"
	"github.com/lydakis/jul/server/internal/storage"
)

type reflogEntry struct {
	CommitSHA string `json:"commit_sha"`
	ChangeID  string `json:"change_id"`
	Source    string `json:"source"`
}

func TestSmokeSyncAndReflog(t *testing.T) {
	baseURL, cleanup := startServer(t)
	defer cleanup()

	repo := t.TempDir()
	runCmd(t, repo, nil, "git", "init")
	runCmd(t, repo, nil, "git", "config", "user.name", "Test User")
	runCmd(t, repo, nil, "git", "config", "user.email", "test@example.com")

	julPath := buildCLI(t)
	workspaceID := "tester/workspace"
	env := map[string]string{
		"JUL_BASE_URL":  baseURL,
		"JUL_WORKSPACE": workspaceID,
	}

	// Install hook
	runCmd(t, repo, env, julPath, "hooks", "install")
	hooksDir := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", "--git-path", "hooks"))
	if !filepath.IsAbs(hooksDir) {
		hooksDir = filepath.Join(repo, hooksDir)
	}
	hookPath := filepath.Join(hooksDir, "post-commit")
	if _, err := os.Stat(hookPath); err != nil {
		t.Fatalf("expected hook at %s: %v", hookPath, err)
	}

	// Commit 1
	writeFile(t, repo, "README.md", "hello\n")
	runCmd(t, repo, nil, "git", "add", "README.md")
	runCmd(t, repo, nil, "git", "commit", "-m", "feat: first")
	sha1 := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", "HEAD"))
	runCmd(t, repo, env, julPath, "sync")

	// Commit 2
	writeFile(t, repo, "README.md", "hello\nworld\n")
	runCmd(t, repo, nil, "git", "add", "README.md")
	runCmd(t, repo, nil, "git", "commit", "-m", "feat: second")
	sha2 := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", "HEAD"))
	runCmd(t, repo, env, julPath, "sync")

	// CLI reflog should include latest commit
	reflogOut := runCmd(t, repo, env, julPath, "reflog", "--limit", "5")
	if !strings.Contains(reflogOut, sha2) {
		t.Fatalf("expected reflog output to contain latest commit %s", sha2)
	}

	// API reflog should return current + keep entries
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/v1/workspaces/%s/reflog?limit=10", baseURL, workspaceID), nil)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("reflog request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var entries []reflogEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		t.Fatalf("failed to decode reflog: %v", err)
	}
	if len(entries) < 2 {
		t.Fatalf("expected at least 2 entries, got %d", len(entries))
	}
	if entries[0].CommitSHA != sha2 || entries[0].Source != "current" {
		t.Fatalf("expected current entry %s, got %s (%s)", sha2, entries[0].CommitSHA, entries[0].Source)
	}
	foundKeep := false
	for _, entry := range entries {
		if entry.CommitSHA == sha1 && entry.Source == "keep" {
			foundKeep = true
			break
		}
	}
	if !foundKeep {
		t.Fatalf("expected keep entry for %s", sha1)
	}
}

func startServer(t *testing.T) (string, func()) {
	storePath := filepath.Join(t.TempDir(), "jul.db")
	store, err := storage.Open(storePath)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}

	broker := events.NewBroker()
	srv := server.New(server.Config{}, store, broker)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		_ = store.Close()
		t.Fatalf("failed to listen: %v", err)
	}

	httpServer := &http.Server{Handler: srv.Handler()}
	go func() {
		_ = httpServer.Serve(listener)
	}()

	baseURL := "http://" + listener.Addr().String()
	waitForHealth(t, baseURL)

	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(ctx)
		_ = listener.Close()
		_ = store.Close()
	}

	return baseURL, cleanup
}

func waitForHealth(t *testing.T, baseURL string) {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(baseURL + "/healthz")
		if err == nil && resp.StatusCode == http.StatusOK {
			_ = resp.Body.Close()
			return
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("server health check failed")
}

func buildCLI(t *testing.T) string {
	root := findRepoRoot(t)
	cliDir := filepath.Join(root, "apps", "cli")
	outPath := filepath.Join(t.TempDir(), "jul")

	cmd := exec.Command("go", "build", "-o", outPath, "./cmd/jul")
	cmd.Dir = cliDir
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build failed: %v\n%s", err, string(output))
	}
	return outPath
}

func findRepoRoot(t *testing.T) string {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}

	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	t.Fatalf("failed to locate repo root from %s", cwd)
	return ""
}

func runCmd(t *testing.T, dir string, env map[string]string, name string, args ...string) string {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = mergeEnv(env)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command failed: %s %s\n%s", name, strings.Join(args, " "), string(output))
	}
	return string(output)
}

func mergeEnv(extra map[string]string) []string {
	if len(extra) == 0 {
		return os.Environ()
	}

	env := os.Environ()
	for key, value := range extra {
		env = append(env, key+"="+value)
	}
	return env
}

func writeFile(t *testing.T, dir, name, content string) {
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
}
