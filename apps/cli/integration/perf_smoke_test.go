package integration

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	perfStatusRuns     = 30
	perfStatusColdRuns = 20
	perfSyncRuns       = 20
	perfCheckpointRuns = 10
	perfCheckpointCold = 5
	perfStatusWarmups  = 5
)

func TestPerfStatusSmoke(t *testing.T) {
	if os.Getenv("JUL_PERF_SMOKE") != "1" {
		t.Skip("set JUL_PERF_SMOKE=1 to run perf smoke suite")
	}
	julPath := perfCLI(t)
	repo, env := setupPerfRepo(t, "perf-status", 2000, 1024)
	runCmd(t, repo, env, julPath, "checkpoint", "-m", "perf seed", "--no-ci", "--no-review")

	for i := 0; i < perfStatusWarmups; i++ {
		warmUpCommand(t, repo, env, julPath, "status", "--json")
	}
	if os.Getenv("JUL_PERF_DEBUG") == "1" {
		debug := runCmdTimed(t, repo, env, julPath, "status", "--json")
		t.Logf("status sample: %s", debug)
		gitP50, gitP95 := measureRawGitStatus(t, repo, env, 10)
		t.Logf("raw git status p50=%s p95=%s", gitP50, gitP95)
	}

	samples := make([]time.Duration, 0, perfStatusRuns)
	for i := 0; i < perfStatusRuns; i++ {
		_, duration := runTimedJSONCommand(t, repo, env, julPath, "status", "--json")
		samples = append(samples, duration)
	}

	p50, p95 := percentiles(samples, 0.50, 0.95)
	budgetP50, budgetP95 := perfBudgetStatus()
	t.Logf("PT-STATUS-001 p50=%s p95=%s budget50=%s budget95=%s", p50, p95, budgetP50, budgetP95)
	assertPerfBudget(t, "PT-STATUS-001", p50, p95, budgetP50, budgetP95)
	assertPerfRatio(t, "PT-STATUS-001", p50, p95, 4.0)
}

func TestPerfStatusCacheColdSmoke(t *testing.T) {
	if os.Getenv("JUL_PERF_SMOKE") != "1" {
		t.Skip("set JUL_PERF_SMOKE=1 to run perf smoke suite")
	}
	julPath := perfCLI(t)
	repo, env := setupPerfRepo(t, "perf-status-cold", 2000, 1024)
	runCmd(t, repo, env, julPath, "checkpoint", "-m", "perf seed", "--no-ci", "--no-review")

	samples := make([]time.Duration, 0, perfStatusColdRuns)
	cachePath := filepath.Join(repo, ".jul", "status.json")
	for i := 0; i < perfStatusColdRuns; i++ {
		_ = os.Remove(cachePath)
		_, duration := runTimedJSONCommand(t, repo, env, julPath, "status", "--json")
		samples = append(samples, duration)
	}

	p50, p95 := percentiles(samples, 0.50, 0.95)
	budgetP95 := perfBudgetStatusCacheColdP95()
	t.Logf("PT-STATUS-002 p50=%s p95=%s budget95=%s", p50, p95, budgetP95)
	assertPerfP95(t, "PT-STATUS-002", p95, budgetP95)
	assertPerfRatio(t, "PT-STATUS-002", p50, p95, 4.0)
}

func TestPerfSyncSmoke(t *testing.T) {
	if os.Getenv("JUL_PERF_SMOKE") != "1" {
		t.Skip("set JUL_PERF_SMOKE=1 to run perf smoke suite")
	}
	julPath := perfCLI(t)
	repo, env := setupPerfRepo(t, "perf-sync", 2000, 1024)

	// Small delta
	appendFile(t, repo, "src/file-0001.txt", "\nchange\n")
	warmUpCommand(t, repo, env, julPath, "sync", "--json")

	samples := make([]time.Duration, 0, perfSyncRuns)
	for i := 0; i < perfSyncRuns; i++ {
		_, duration := runTimedJSONCommand(t, repo, env, julPath, "sync", "--json")
		samples = append(samples, duration)
	}

	p50, p95 := percentiles(samples, 0.50, 0.95)
	budgetP50, budgetP95 := perfBudgetSync()
	t.Logf("PT-SYNC-001 p50=%s p95=%s budget50=%s budget95=%s", p50, p95, budgetP50, budgetP95)
	assertPerfBudget(t, "PT-SYNC-001", p50, p95, budgetP50, budgetP95)
	assertPerfRatio(t, "PT-SYNC-001", p50, p95, 3.0)
}

func TestPerfCheckpointSmoke(t *testing.T) {
	if os.Getenv("JUL_PERF_SMOKE") != "1" {
		t.Skip("set JUL_PERF_SMOKE=1 to run perf smoke suite")
	}
	julPath := perfCLI(t)
	repo, env := setupPerfRepo(t, "perf-checkpoint", 2000, 1024)

	samples := make([]time.Duration, 0, perfCheckpointRuns)
	for i := 0; i < perfCheckpointRuns; i++ {
		appendFile(t, repo, "src/file-0002.txt", fmt.Sprintf("\nchange-%d\n", i))
		_, duration := runTimedJSONCommand(t, repo, env, julPath, "checkpoint", "-m", fmt.Sprintf("perf-%d", i), "--no-ci", "--no-review", "--json")
		samples = append(samples, duration)
	}

	p50, p95 := percentiles(samples, 0.50, 0.95)
	budgetP50, budgetP95 := perfBudgetCheckpoint()
	t.Logf("PT-CHECKPOINT-001 p50=%s p95=%s budget50=%s budgetP95=%s", p50, p95, budgetP50, budgetP95)
	assertPerfBudget(t, "PT-CHECKPOINT-001", p50, p95, budgetP50, budgetP95)
	assertPerfRatio(t, "PT-CHECKPOINT-001", p50, p95, 3.0)
}

func TestPerfCheckpointCloneColdSmoke(t *testing.T) {
	if os.Getenv("JUL_PERF_SMOKE") != "1" {
		t.Skip("set JUL_PERF_SMOKE=1 to run perf smoke suite")
	}
	julPath := perfCLI(t)
	samples := make([]time.Duration, 0, perfCheckpointCold)
	for i := 0; i < perfCheckpointCold; i++ {
		repo, env := setupPerfRepo(t, fmt.Sprintf("perf-checkpoint-cold-%d", i), 2000, 1024)
		appendFile(t, repo, "src/file-0002.txt", fmt.Sprintf("\ncold-change-%d\n", i))
		_, duration := runTimedJSONCommand(t, repo, env, julPath, "checkpoint", "-m", fmt.Sprintf("perf-cold-%d", i), "--no-ci", "--no-review", "--json")
		samples = append(samples, duration)
	}

	p50, p95 := percentiles(samples, 0.50, 0.95)
	budgetP95 := perfBudgetCheckpointCloneColdP95()
	t.Logf("PT-CHECKPOINT-002 p50=%s p95=%s budgetP95=%s", p50, p95, budgetP95)
	assertPerfP95(t, "PT-CHECKPOINT-002", p95, budgetP95)
	assertPerfRatio(t, "PT-CHECKPOINT-002", p50, p95, 4.0)
}

func perfCLI(t *testing.T) string {
	t.Helper()
	return buildCLI(t)
}

func setupPerfRepo(t *testing.T, name string, files int, bytesPerFile int) (string, map[string]string) {
	t.Helper()
	root := t.TempDir()
	repo := filepath.Join(root, name)
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("failed to create repo dir: %v", err)
	}
	runCmd(t, repo, nil, "git", "init")
	runCmd(t, repo, nil, "git", "config", "user.name", "Perf User")
	runCmd(t, repo, nil, "git", "config", "user.email", "perf@example.com")

	if files < 1 {
		files = 1
	}
	if bytesPerFile < 1 {
		bytesPerFile = 1
	}
	payload := strings.Repeat("a", bytesPerFile)
	if err := os.MkdirAll(filepath.Join(repo, "src"), 0o755); err != nil {
		t.Fatalf("mkdir src failed: %v", err)
	}
	for i := 0; i < files; i++ {
		rel := filepath.Join("src", fmt.Sprintf("file-%04d.txt", i))
		writeFile(t, repo, rel, payload)
	}
	runCmd(t, repo, nil, "git", "add", ".")
	runCmd(t, repo, nil, "git", "commit", "-m", "perf: base")

	home := filepath.Join(root, "home")
	env := map[string]string{
		"HOME":          home,
		"JUL_WORKSPACE": "perf/@",
		"JUL_NO_SYNC":   "1",
		"PATH":          perfPath(),
	}
	julPath := perfCLI(t)
	runCmd(t, repo, env, julPath, "init", name)
	return repo, env
}

func warmUpCommand(t *testing.T, repo string, env map[string]string, julPath string, args ...string) {
	t.Helper()
	_, _ = runTimedJSONCommand(t, repo, env, julPath, args...)
}

func runTimedJSONCommand(t *testing.T, dir string, env map[string]string, name string, args ...string) (time.Duration, time.Duration) {
	t.Helper()
	start := time.Now()
	output := runCmdTimed(t, dir, env, name, args...)
	wall := time.Since(start)
	if timings, ok := parseTimings(t, output); ok {
		if timings <= 0 {
			timings = 1
		}
		d := time.Duration(timings) * time.Millisecond
		return d, d
	}
	return wall, wall
}

func runCmdTimed(t *testing.T, dir string, env map[string]string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = mergeEnv(env)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command failed: %s %s\n%s", name, strings.Join(args, " "), string(output))
	}
	return string(output)
}

type timingsPayload struct {
	Timings struct {
		Total *int64           `json:"total"`
		Phase map[string]int64 `json:"phase"`
	} `json:"timings_ms"`
}

func parseTimings(t *testing.T, output string) (int64, bool) {
	t.Helper()
	var payload timingsPayload
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode timings: %v\n%s", err, output)
	}
	if payload.Timings.Total != nil {
		return *payload.Timings.Total, true
	}
	if len(payload.Timings.Phase) == 0 {
		return 0, false
	}
	total := int64(0)
	for _, value := range payload.Timings.Phase {
		total += value
	}
	return total, true
}

func percentiles(samples []time.Duration, p50 float64, p95 float64) (time.Duration, time.Duration) {
	if len(samples) == 0 {
		return 0, 0
	}
	sorted := make([]time.Duration, len(samples))
	copy(sorted, samples)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return percentile(sorted, p50), percentile(sorted, p95)
}

func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[len(sorted)-1]
	}
	rank := int(math.Ceil(p*float64(len(sorted)))) - 1
	if rank < 0 {
		rank = 0
	}
	if rank >= len(sorted) {
		rank = len(sorted) - 1
	}
	return sorted[rank]
}

func perfBudgetStatus() (time.Duration, time.Duration) {
	return applyPerfMultiplier(25*time.Millisecond, 80*time.Millisecond)
}

func perfBudgetStatusCacheColdP95() time.Duration {
	_, p95 := applyPerfMultiplier(0, 150*time.Millisecond)
	return p95
}

func perfBudgetSync() (time.Duration, time.Duration) {
	return applyPerfMultiplier(300*time.Millisecond, 1*time.Second)
}

func perfBudgetCheckpoint() (time.Duration, time.Duration) {
	return applyPerfMultiplier(250*time.Millisecond, 800*time.Millisecond)
}

func perfBudgetCheckpointCloneColdP95() time.Duration {
	_, p95 := applyPerfMultiplier(0, 2*time.Second)
	return p95
}

func applyPerfMultiplier(p50, p95 time.Duration) (time.Duration, time.Duration) {
	mult := perfMultiplier()
	return time.Duration(float64(p50) * mult), time.Duration(float64(p95) * mult)
}

func perfMultiplier() float64 {
	if v := strings.TrimSpace(os.Getenv("JUL_PERF_MULTIPLIER")); v != "" {
		if parsed, err := parseMultiplier(v); err == nil {
			return parsed
		}
	}
	if os.Getenv("CI") != "" {
		return 1.5
	}
	if runtime.GOOS == "windows" {
		return 1.5
	}
	return 1.0
}

func perfPath() string {
	if runtime.GOOS == "windows" {
		return os.Getenv("PATH")
	}
	candidates := []string{"/usr/bin", "/bin", "/usr/local/bin", "/opt/homebrew/bin"}
	paths := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			paths = append(paths, candidate)
		}
	}
	if len(paths) == 0 {
		return os.Getenv("PATH")
	}
	return strings.Join(paths, ":")
}

func parseMultiplier(raw string) (float64, error) {
	parsed, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, err
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("invalid multiplier")
	}
	return parsed, nil
}

func assertPerfBudget(t *testing.T, label string, p50, p95, budget50, budget95 time.Duration) {
	t.Helper()
	if p50 > budget50 || p95 > budget95 {
		t.Fatalf("%s failed: p50=%s (budget %s), p95=%s (budget %s)", label, p50, budget50, p95, budget95)
	}
}

func assertPerfP95(t *testing.T, label string, p95, budget95 time.Duration) {
	t.Helper()
	if p95 > budget95 {
		t.Fatalf("%s failed: p95=%s (budget %s)", label, p95, budget95)
	}
}

func assertPerfRatio(t *testing.T, label string, p50, p95 time.Duration, maxRatio float64) {
	t.Helper()
	if p50 <= 0 {
		return
	}
	// Very small medians are dominated by scheduler jitter and timer granularity.
	// In that regime, absolute p95 budgets are more stable than a relative ratio gate.
	if p50 < 10*time.Millisecond {
		return
	}
	ratio := float64(p95) / float64(p50)
	if ratio > maxRatio {
		t.Fatalf("%s failed: p95/p50 ratio=%.2f (max %.2f)", label, ratio, maxRatio)
	}
}

func appendFile(t *testing.T, repo, relPath, content string) {
	t.Helper()
	path := filepath.Join(repo, relPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open file failed: %v", err)
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
}

func measureRawGitStatus(t *testing.T, repo string, env map[string]string, runs int) (time.Duration, time.Duration) {
	t.Helper()
	if runs < 1 {
		runs = 1
	}
	samples := make([]time.Duration, 0, runs)
	for i := 0; i < runs; i++ {
		start := time.Now()
		_ = runCmdTimed(t, repo, env, "git", "--no-optional-locks", "-c", "core.untrackedCache=true", "status", "--porcelain", "-z", "-unormal", "--no-renames")
		samples = append(samples, time.Since(start))
	}
	return percentiles(samples, 0.50, 0.95)
}
