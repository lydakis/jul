package integration

import (
	"context"
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

func startServer(t *testing.T, reposDir string) (string, func()) {
	storePath := filepath.Join(t.TempDir(), "jul.db")
	store, err := storage.Open(storePath)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}

	if reposDir == "" {
		reposDir = filepath.Join(t.TempDir(), "repos")
	}
	if err := os.MkdirAll(reposDir, 0o755); err != nil {
		_ = store.Close()
		t.Fatalf("failed to create repos dir: %v", err)
	}

	broker := events.NewBroker()
	srv := server.New(server.Config{ReposDir: reposDir}, store, broker)

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
	fT := t
	fT.Fatalf("server health check failed")
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
