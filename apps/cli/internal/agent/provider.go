package agent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/lydakis/jul/cli/internal/config"
)

var (
	ErrAgentNotConfigured = errors.New("agent not configured")
	ErrBundledMissing     = errors.New("bundled agent not found")
)

type Provider struct {
	Name     string
	Command  string
	Protocol string
	Mode     string
	Timeout  time.Duration
	Bundled  bool
	Headless string
	Actions  map[string]string
}

func ResolveProvider() (Provider, error) {
	if cmd := strings.TrimSpace(os.Getenv("JUL_AGENT_CMD")); cmd != "" {
		mode := strings.TrimSpace(os.Getenv("JUL_AGENT_MODE"))
		if mode == "" {
			mode = "stdin"
		}
		return Provider{
			Name:     "custom",
			Command:  cmd,
			Protocol: "jul-agent-v1",
			Mode:     mode,
			Timeout:  5 * time.Minute,
		}, nil
	}

	cfg := config.DefaultAgentProvider()
	if cfg.Name == "" {
		return Provider{}, ErrAgentNotConfigured
	}
	provider := Provider{
		Name:     cfg.Name,
		Command:  cfg.Command,
		Protocol: cfg.Protocol,
		Mode:     cfg.Mode,
		Timeout:  cfg.Timeout,
		Bundled:  cfg.Bundled,
		Headless: cfg.Headless,
		Actions:  cfg.Actions,
	}
	if provider.Mode == "" {
		provider.Mode = "stdin"
	}
	if provider.Timeout <= 0 {
		provider.Timeout = 5 * time.Minute
	}
	if provider.Bundled {
		path, err := bundledOpenCodePath()
		if err != nil {
			return Provider{}, ErrBundledMissing
		}
		provider.Command = path
	}
	if strings.TrimSpace(provider.Command) == "" {
		return Provider{}, ErrAgentNotConfigured
	}
	return provider, nil
}

func (p Provider) HeadlessFor(action string) string {
	if p.Actions != nil {
		if cmd := strings.TrimSpace(p.Actions[action]); cmd != "" {
			return cmd
		}
	}
	return p.Headless
}

func bundledOpenCodePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return bundledOpenCodePathFor(exe)
}

func bundledOpenCodePathFor(exe string) (string, error) {
	name := "opencode"
	if runtime.GOOS == "windows" {
		name = "opencode.exe"
	}

	dirs := []string{filepath.Dir(exe)}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil && resolved != exe {
		dirs = append(dirs, filepath.Dir(resolved))
	}

	candidates := make([]string, 0, len(dirs)*4)
	seen := make(map[string]struct{}, len(dirs)*4)
	for _, dir := range dirs {
		paths := []string{
			filepath.Join(dir, "..", "libexec", "jul", name),
			filepath.Join(dir, "libexec", "jul", name),
			filepath.Join(dir, name),
			filepath.Join(dir, "..", name),
		}
		for _, path := range paths {
			path = filepath.Clean(path)
			if _, ok := seen[path]; ok {
				continue
			}
			seen[path] = struct{}{}
			candidates = append(candidates, path)
		}
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("bundled opencode not found near %s", filepath.Dir(exe))
}
