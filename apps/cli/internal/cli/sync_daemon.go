package cli

import (
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/syncer"
)

func runSyncDaemon(opts syncer.SyncOptions) int {
	repoRoot, err := gitutil.RepoTopLevel()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to locate repo root: %v\n", err)
		return 1
	}

	pidPath := filepath.Join(repoRoot, ".jul", "sync-daemon.pid")
	if err := os.MkdirAll(filepath.Dir(pidPath), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to prepare daemon state: %v\n", err)
		return 1
	}
	if pid, ok := readDaemonPID(pidPath); ok {
		if daemonRunning(pid) {
			fmt.Fprintln(os.Stdout, "Sync daemon already running.")
			return 0
		}
		_ = os.Remove(pidPath)
	}
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "failed to record daemon pid: %v\n", err)
		return 1
	}
	defer func() {
		_ = os.Remove(pidPath)
	}()

	debounce := time.Duration(config.SyncDebounceSeconds()) * time.Second
	minInterval := time.Duration(config.SyncMinIntervalSeconds()) * time.Second
	if debounce < 0 {
		debounce = 0
	}
	if minInterval < 0 {
		minInterval = 0
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start watcher: %v\n", err)
		return 1
	}
	defer watcher.Close()

	if err := watchRepo(watcher, repoRoot); err != nil {
		fmt.Fprintf(os.Stderr, "failed to watch repo: %v\n", err)
		return 1
	}

	fmt.Fprintf(os.Stdout, "Sync daemon running (debounce %ds, min interval %ds)\n", int(debounce.Seconds()), int(minInterval.Seconds()))

	syncCh := make(chan struct{}, 1)
	var syncMu sync.Mutex
	var lastSync time.Time

	runSync := func() {
		syncMu.Lock()
		defer syncMu.Unlock()
		if minInterval > 0 && !lastSync.IsZero() {
			if since := time.Since(lastSync); since < minInterval {
				time.Sleep(minInterval - since)
			}
		}
		if _, err := syncer.SyncWithOptions(opts); err != nil {
			fmt.Fprintf(os.Stderr, "sync failed: %v\n", err)
		}
		lastSync = time.Now()
	}

	go func() {
		for range syncCh {
			runSync()
		}
	}()

	var timerMu sync.Mutex
	var timer *time.Timer
	schedule := func() {
		timerMu.Lock()
		defer timerMu.Unlock()
		if timer != nil {
			_ = timer.Stop()
		}
		timer = time.AfterFunc(debounce, func() {
			select {
			case syncCh <- struct{}{}:
			default:
			}
		})
	}

	// Run an initial sync.
	schedule()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	for {
		select {
		case <-sigCh:
			fmt.Fprintln(os.Stdout, "Sync daemon stopped.")
			return 0
		case event, ok := <-watcher.Events:
			if !ok {
				return 0
			}
			if shouldIgnorePath(event.Name) {
				if isJulPath(event.Name) {
					_ = ensureJulDir(repoRoot)
				}
				continue
			}
			if event.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					if shouldIgnorePath(event.Name) {
						continue
					}
					_ = watchRepo(watcher, event.Name)
				}
			}
			schedule()
		case err, ok := <-watcher.Errors:
			if !ok {
				return 0
			}
			fmt.Fprintf(os.Stderr, "watcher error: %v\n", err)
		}
	}
}

func readDaemonPID(path string) (int, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}

func daemonRunning(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

func watchRepo(watcher *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			return nil
		}
		if shouldIgnorePath(path) {
			return filepath.SkipDir
		}
		return watcher.Add(path)
	})
}

func shouldIgnorePath(path string) bool {
	cleaned := filepath.Clean(path)
	parts := strings.Split(cleaned, string(os.PathSeparator))
	for _, part := range parts {
		if part == ".git" || part == ".jul" {
			return true
		}
	}
	return false
}

func isJulPath(path string) bool {
	cleaned := filepath.Clean(path)
	parts := strings.Split(cleaned, string(os.PathSeparator))
	for _, part := range parts {
		if part == ".jul" {
			return true
		}
	}
	return false
}
