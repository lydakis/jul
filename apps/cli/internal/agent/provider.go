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

func bundledOpenCodePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(exe)
	name := "opencode"
	if runtime.GOOS == "windows" {
		name = "opencode.exe"
	}
	candidates := []string{
		filepath.Join(dir, "..", "libexec", "jul", name),
		filepath.Join(dir, "libexec", "jul", name),
		filepath.Join(dir, name),
		filepath.Join(dir, "..", name),
	}
	for _, candidate := range candidates {
		path := filepath.Clean(candidate)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("bundled opencode not found near %s", dir)
}
