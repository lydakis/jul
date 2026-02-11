package integration

import (
	"bytes"
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
	"sync"
	"testing"
	"time"
)

const (
	perfStatusRuns      = 50
	perfStatusColdRuns  = 20
	perfSyncRuns        = 20
	perfSyncFullRuns    = 5
	perfSyncCloneRuns   = 5
	perfCheckpointRuns  = 10
	perfCheckpointCold  = 5
	perfStatusWarmups   = 5
	perfNotesRuns       = 5
	perfPromoteRuns     = 5
	perfSuggestionsRuns = 10
	perfDaemonEvents    = 1000

	perfProgressVisibleDeadline = 250 * time.Millisecond
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

func TestPerfStatusCloneColdSmoke(t *testing.T) {
	if os.Getenv("JUL_PERF_SMOKE") != "1" {
		t.Skip("set JUL_PERF_SMOKE=1 to run perf smoke suite")
	}
	julPath := perfCLI(t)
	root := t.TempDir()
	seed := setupPerfSeedRepo(t, root, "perf-status-clone-cold-seed", 2000, 1024)

	samples := make([]time.Duration, 0, perfStatusColdRuns)
	for i := 0; i < perfStatusColdRuns; i++ {
		cloneDir := filepath.Join(root, fmt.Sprintf("clone-%02d", i))
		runCmd(t, root, nil, "git", "clone", "--quiet", seed, cloneDir)

		home := filepath.Join(root, fmt.Sprintf("home-%02d", i))
		env := perfEnv(home)
		runCmd(t, cloneDir, env, julPath, "init", fmt.Sprintf("perf-status-clone-cold-%d", i))

		_, duration := runTimedJSONCommand(t, cloneDir, env, julPath, "status", "--json")
		samples = append(samples, duration)
	}

	p50, p95 := percentiles(samples, 0.50, 0.95)
	budgetP95 := perfBudgetStatusCloneColdP95()
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

func TestPerfSyncFullRebuildSmoke(t *testing.T) {
	if os.Getenv("JUL_PERF_SMOKE") != "1" {
		t.Skip("set JUL_PERF_SMOKE=1 to run perf smoke suite")
	}
	julPath := perfCLI(t)
	repo, env := setupPerfRepo(t, "perf-sync-full", 2000, 1024)

	assertSyncProgressVisible(t, repo, env, julPath)
	assertSyncCancellationSafe(t, julPath)

	samples := make([]time.Duration, 0, perfSyncFullRuns)
	for i := 0; i < perfSyncFullRuns; i++ {
		appendFile(t, repo, "src/file-0001.txt", fmt.Sprintf("\nfull-rebuild-%d\n", i))
		invalidateDraftIndex(t, repo)

		output := runCmdTimed(t, repo, env, julPath, "sync", "--json")
		if snapshot, ok := parsePhaseTiming(t, output, "snapshot"); !ok || snapshot <= 0 {
			t.Fatalf("expected snapshot phase timing in sync output, got %s", output)
		}
		if _, ok := parsePhaseTiming(t, output, "finalize"); !ok {
			t.Fatalf("expected finalize phase timing in sync output, got %s", output)
		}

		totalMs, ok := parseTimings(t, output)
		if !ok {
			t.Fatalf("expected sync timings in json output, got %s", output)
		}
		samples = append(samples, time.Duration(totalMs)*time.Millisecond)
	}

	p50, p95 := percentiles(samples, 0.50, 0.95)
	budgetP50, budgetP95 := perfBudgetSyncFullRebuild()
	t.Logf("PT-SYNC-002 p50=%s p95=%s budget50=%s budget95=%s", p50, p95, budgetP50, budgetP95)
	assertPerfBudget(t, "PT-SYNC-002", p50, p95, budgetP50, budgetP95)
	assertPerfRatio(t, "PT-SYNC-002", p50, p95, 4.0)
}

func TestPerfSyncLocalTransportSmoke(t *testing.T) {
	if os.Getenv("JUL_PERF_SMOKE") != "1" {
		t.Skip("set JUL_PERF_SMOKE=1 to run perf smoke suite")
	}
	julPath := perfCLI(t)
	repo, env := setupPerfRepo(t, "perf-sync-local", 2000, 1024)
	_ = setupPerfRemote(t, repo, env, julPath)

	appendFile(t, repo, "src/file-0001.txt", "\nlocal-transport-warmup\n")
	warmUpCommand(t, repo, env, julPath, "sync", "--json")

	samples := make([]time.Duration, 0, perfSyncRuns)
	snapshotSamples := make([]time.Duration, 0, perfSyncRuns)
	pushSamples := make([]time.Duration, 0, perfSyncRuns)
	for i := 0; i < perfSyncRuns; i++ {
		appendFile(t, repo, "src/file-0001.txt", fmt.Sprintf("\nlocal-transport-%d\n", i))
		output := runCmdTimed(t, repo, env, julPath, "sync", "--json")
		snapshot, ok := parsePhaseTiming(t, output, "snapshot")
		if !ok || snapshot <= 0 {
			t.Fatalf("expected snapshot phase timing in sync output, got %s", output)
		}
		push, ok := parsePhaseTiming(t, output, "push")
		if !ok || push <= 0 {
			t.Fatalf("expected push phase timing in sync output, got %s", output)
		}
		totalMs, ok := parseTimings(t, output)
		if !ok {
			t.Fatalf("expected sync timings in json output, got %s", output)
		}
		samples = append(samples, time.Duration(totalMs)*time.Millisecond)
		snapshotSamples = append(snapshotSamples, snapshot)
		pushSamples = append(pushSamples, push)
	}

	p50, p95 := percentiles(samples, 0.50, 0.95)
	budgetP50, budgetP95 := perfBudgetSyncLocalTransport()
	t.Logf("PT-SYNC-003 p50=%s p95=%s budget50=%s budget95=%s", p50, p95, budgetP50, budgetP95)
	assertPerfBudget(t, "PT-SYNC-003", p50, p95, budgetP50, budgetP95)
	assertPerfRatio(t, "PT-SYNC-003", p50, p95, 3.0)

	// Transport smoke should preserve local snapshot cost and keep local push overhead
	// in the <=20MB pack addendum bucket from the performance spec.
	_, snapshotP95 := percentiles(snapshotSamples, 0.50, 0.95)
	_, snapshotBudgetP95 := perfBudgetSync()
	assertPerfP95(t, "PT-SYNC-003 snapshot phase", snapshotP95, snapshotBudgetP95)
	_, pushP95 := percentiles(pushSamples, 0.50, 0.95)
	addendumBudgetP95 := perfBudgetSyncTransportAddendumP95()
	assertPerfP95(t, "PT-SYNC-003 transport addendum", pushP95, addendumBudgetP95)
}

func TestPerfSyncCloneColdLocalTransportSmoke(t *testing.T) {
	if os.Getenv("JUL_PERF_SMOKE") != "1" {
		t.Skip("set JUL_PERF_SMOKE=1 to run perf smoke suite")
	}
	julPath := perfCLI(t)
	root := t.TempDir()
	seed := setupPerfSeedRepo(t, root, "perf-sync-clone-cold-seed", 2000, 1024)
	remoteDir := filepath.Join(root, "origin.git")
	runCmd(t, root, nil, "git", "init", "--bare", remoteDir)
	runCmd(t, seed, nil, "git", "remote", "add", "origin", remoteDir)
	runCmd(t, seed, nil, "git", "push", "-u", "origin", "HEAD:refs/heads/main")
	runCmd(t, remoteDir, nil, "git", "--git-dir", remoteDir, "symbolic-ref", "HEAD", "refs/heads/main")

	firstSamples := make([]time.Duration, 0, perfSyncCloneRuns)
	for i := 0; i < perfSyncCloneRuns; i++ {
		cloneDir := filepath.Join(root, fmt.Sprintf("sync-clone-%02d", i))
		runCmd(t, root, nil, "git", "clone", "--quiet", remoteDir, cloneDir)

		home := filepath.Join(root, fmt.Sprintf("home-sync-clone-%02d", i))
		env := perfEnv(home)
		runCmd(t, cloneDir, env, julPath, "init", fmt.Sprintf("perf-sync-clone-cold-%d", i))

		firstOut := runCmdTimed(t, cloneDir, env, julPath, "sync", "--json")
		if snapshot, ok := parsePhaseTiming(t, firstOut, "snapshot"); !ok || snapshot <= 0 {
			t.Fatalf("expected snapshot phase timing in first clone-cold sync output, got %s", firstOut)
		}
		if push, ok := parsePhaseTiming(t, firstOut, "push"); !ok || push <= 0 {
			t.Fatalf("expected push phase timing in first clone-cold sync output, got %s", firstOut)
		}
		firstTotalMs, ok := parseTimings(t, firstOut)
		if !ok {
			t.Fatalf("expected sync timings in first clone-cold sync output, got %s", firstOut)
		}
		firstDuration := time.Duration(firstTotalMs) * time.Millisecond
		firstSamples = append(firstSamples, firstDuration)

		secondOut := runCmdTimed(t, cloneDir, env, julPath, "sync", "--json")
		secondTotalMs, ok := parseTimings(t, secondOut)
		if !ok {
			t.Fatalf("expected sync timings in second clone-cold sync output, got %s", secondOut)
		}
		secondDuration := time.Duration(secondTotalMs) * time.Millisecond
		if firstDuration > 0 && secondDuration > firstDuration*2 {
			t.Fatalf("PT-SYNC-004 failed: second sync %s was unexpectedly slower than first clone-cold sync %s", secondDuration, firstDuration)
		}
	}

	p50, p95 := percentiles(firstSamples, 0.50, 0.95)
	budgetP95 := perfBudgetSyncCloneColdLocalP95()
	t.Logf("PT-SYNC-004 p50=%s p95=%s budget95=%s", p50, p95, budgetP95)
	assertPerfP95(t, "PT-SYNC-004", p95, budgetP95)
	assertPerfRatio(t, "PT-SYNC-004", p50, p95, 4.0)
}

func TestPerfNotesMergeSmoke(t *testing.T) {
	if os.Getenv("JUL_PERF_SMOKE") != "1" {
		t.Skip("set JUL_PERF_SMOKE=1 to run perf smoke suite")
	}
	julPath := perfCLI(t)

	samples := make([]time.Duration, 0, perfNotesRuns)
	notesRef := "refs/notes/jul/suggestions"
	for i := 0; i < perfNotesRuns; i++ {
		repo, env := setupPerfRepo(t, fmt.Sprintf("perf-notes-%d", i), 500, 512)
		remoteDir := setupPerfRemote(t, repo, env, julPath)

		_ = addNotesEntries(t, filepath.Dir(remoteDir), remoteDir, notesRef, "remote-seed", 0, 100, 100) // 10k events preload
		_ = addNotesEntries(t, repo, "", notesRef, "local-delta", 0, 10, 100)                            // +1k events before sync

		output := runCmdTimed(t, repo, env, julPath, "sync", "--json")
		notesMerge, ok := parsePhaseTiming(t, output, "notes_merge")
		if !ok {
			t.Fatalf("expected notes_merge phase timing in sync output, got %s", output)
		}
		samples = append(samples, notesMerge)
	}

	p50, p95 := percentiles(samples, 0.50, 0.95)
	budgetP50, budgetP95 := perfBudgetNotesMerge()
	t.Logf("PT-NOTES-001 p50=%s p95=%s budget50=%s budget95=%s", p50, p95, budgetP50, budgetP95)
	assertPerfBudget(t, "PT-NOTES-001", p50, p95, budgetP50, budgetP95)
	assertPerfRatio(t, "PT-NOTES-001", p50, p95, 3.0)
}

func TestPerfCheckpointSmoke(t *testing.T) {
	if os.Getenv("JUL_PERF_SMOKE") != "1" {
		t.Skip("set JUL_PERF_SMOKE=1 to run perf smoke suite")
	}
	julPath := perfCLI(t)
	repo, env := setupPerfRepo(t, "perf-checkpoint", 2000, 1024)
	bgBefore := backgroundArtifactCounts(repo)

	samples := make([]time.Duration, 0, perfCheckpointRuns)
	for i := 0; i < perfCheckpointRuns; i++ {
		appendFile(t, repo, "src/file-0002.txt", fmt.Sprintf("\nchange-%d\n", i))
		_, duration := runTimedJSONCommand(t, repo, env, julPath, "checkpoint", "-m", fmt.Sprintf("perf-%d", i), "--no-ci", "--no-review", "--json")
		samples = append(samples, duration)
		bgAfter := backgroundArtifactCounts(repo)
		if bgAfter != bgBefore {
			t.Fatalf("PT-CHECKPOINT-001 failed: --no-ci/--no-review spawned background artifacts (before=%+v after=%+v)", bgBefore, bgAfter)
		}
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

func TestPerfPromoteWarmSmoke(t *testing.T) {
	if os.Getenv("JUL_PERF_SMOKE") != "1" {
		t.Skip("set JUL_PERF_SMOKE=1 to run perf smoke suite")
	}
	julPath := perfCLI(t)
	samples := make([]time.Duration, 0, perfPromoteRuns)

	for i := 0; i < perfPromoteRuns; i++ {
		repo, env := setupPerfRepo(t, fmt.Sprintf("perf-promote-%d", i), 1200, 768)
		_ = setupPerfRemote(t, repo, env, julPath)

		for cp := 0; cp < 3; cp++ {
			appendFile(t, repo, "src/file-0003.txt", fmt.Sprintf("\npromote-change-%d-%d\n", i, cp))
			runCmd(t, repo, env, julPath, "checkpoint", "-m", fmt.Sprintf("promote-%d-%d", i, cp), "--no-ci", "--no-review", "--json")
		}

		output := runCmdTimed(t, repo, env, julPath, "promote", "--to", "main", "--rebase", "--no-policy", "--json")
		totalMs, ok := parseTimings(t, output)
		if !ok {
			t.Fatalf("expected promote timings in json output, got %s", output)
		}
		for _, phase := range []string{"fetch", "rewrite", "push"} {
			if _, ok := parsePhaseTiming(t, output, phase); !ok {
				t.Fatalf("expected promote phase %q in timings: %s", phase, output)
			}
		}
		samples = append(samples, time.Duration(totalMs)*time.Millisecond)
	}

	p50, p95 := percentiles(samples, 0.50, 0.95)
	budgetP50, budgetP95 := perfBudgetPromoteWarm()
	t.Logf("PT-PROMOTE-001 p50=%s p95=%s budget50=%s budget95=%s", p50, p95, budgetP50, budgetP95)
	assertPerfBudget(t, "PT-PROMOTE-001", p50, p95, budgetP50, budgetP95)
	assertPerfRatio(t, "PT-PROMOTE-001", p50, p95, 3.0)
}

func TestPerfPromoteCloneColdSmoke(t *testing.T) {
	if os.Getenv("JUL_PERF_SMOKE") != "1" {
		t.Skip("set JUL_PERF_SMOKE=1 to run perf smoke suite")
	}
	julPath := perfCLI(t)
	root := t.TempDir()
	seed := setupPerfSeedRepo(t, root, "perf-promote-clone-cold-seed", 1200, 768)
	remoteDir := filepath.Join(root, "origin.git")
	runCmd(t, root, nil, "git", "init", "--bare", remoteDir)
	runCmd(t, seed, nil, "git", "remote", "add", "origin", remoteDir)
	runCmd(t, seed, nil, "git", "push", "-u", "origin", "HEAD:refs/heads/main")
	runCmd(t, remoteDir, nil, "git", "--git-dir", remoteDir, "symbolic-ref", "HEAD", "refs/heads/main")

	samples := make([]time.Duration, 0, perfPromoteRuns)
	for i := 0; i < perfPromoteRuns; i++ {
		cloneDir := filepath.Join(root, fmt.Sprintf("promote-clone-%02d", i))
		runCmd(t, root, nil, "git", "clone", "--quiet", remoteDir, cloneDir)

		home := filepath.Join(root, fmt.Sprintf("home-promote-clone-%02d", i))
		env := perfEnv(home)
		runCmd(t, cloneDir, env, julPath, "init", fmt.Sprintf("perf-promote-clone-cold-%d", i))

		for cp := 0; cp < 3; cp++ {
			appendFile(t, cloneDir, "src/file-0003.txt", fmt.Sprintf("\npromote-clone-cold-%d-%d\n", i, cp))
			runCmd(t, cloneDir, env, julPath, "checkpoint", "-m", fmt.Sprintf("promote-clone-cold-%d-%d", i, cp), "--no-ci", "--no-review", "--json")
		}

		output := runCmdTimed(t, cloneDir, env, julPath, "promote", "--to", "main", "--rebase", "--no-policy", "--json")
		totalMs, ok := parseTimings(t, output)
		if !ok {
			t.Fatalf("expected promote timings in json output, got %s", output)
		}
		for _, phase := range []string{"fetch", "rewrite", "push"} {
			if _, ok := parsePhaseTiming(t, output, phase); !ok {
				t.Fatalf("expected promote phase %q in timings: %s", phase, output)
			}
		}
		samples = append(samples, time.Duration(totalMs)*time.Millisecond)
	}

	p50, p95 := percentiles(samples, 0.50, 0.95)
	budgetP95 := perfBudgetPromoteCloneColdP95()
	t.Logf("PT-PROMOTE-002 p50=%s p95=%s budget95=%s", p50, p95, budgetP95)
	assertPerfP95(t, "PT-PROMOTE-002", p95, budgetP95)
	assertPerfRatio(t, "PT-PROMOTE-002", p50, p95, 4.0)
}

func TestPerfSuggestionsSmoke(t *testing.T) {
	if os.Getenv("JUL_PERF_SMOKE") != "1" {
		t.Skip("set JUL_PERF_SMOKE=1 to run perf smoke suite")
	}
	julPath := perfCLI(t)
	repo, env := setupPerfRepo(t, "perf-suggestions", 800, 512)

	appendFile(t, repo, "src/file-0004.txt", "\nseed-suggestions\n")
	checkpointOut := runCmdTimed(t, repo, env, julPath, "checkpoint", "-m", "perf suggestions seed", "--no-ci", "--no-review", "--json")
	var checkpoint struct {
		CheckpointSHA string `json:"CheckpointSHA"`
		ChangeID      string `json:"ChangeID"`
	}
	if err := json.NewDecoder(strings.NewReader(checkpointOut)).Decode(&checkpoint); err != nil {
		t.Fatalf("failed to decode checkpoint output: %v", err)
	}
	if strings.TrimSpace(checkpoint.CheckpointSHA) == "" || strings.TrimSpace(checkpoint.ChangeID) == "" {
		t.Fatalf("expected checkpoint sha and change id, got %s", checkpointOut)
	}

	addSuggestionNotesEntries(t, repo, checkpoint.ChangeID, checkpoint.CheckpointSHA, 1000)

	for i := 0; i < 3; i++ {
		_ = runCmdTimed(t, repo, env, julPath, "suggestions", "--json")
	}
	if os.Getenv("JUL_PERF_DEBUG") == "1" {
		sample := runCmdTimed(t, repo, env, julPath, "suggestions", "--json")
		t.Logf("suggestions sample: %s", sample)
	}

	samples := make([]time.Duration, 0, perfSuggestionsRuns)
	for i := 0; i < perfSuggestionsRuns; i++ {
		output := runCmdTimed(t, repo, env, julPath, "suggestions", "--json")
		totalMs, ok := parseTimings(t, output)
		if !ok {
			t.Fatalf("expected suggestions timings in json output, got %s", output)
		}
		samples = append(samples, time.Duration(totalMs)*time.Millisecond)

		var payload struct {
			Suggestions []json.RawMessage `json:"suggestions"`
		}
		if err := json.NewDecoder(strings.NewReader(output)).Decode(&payload); err != nil {
			t.Fatalf("failed to decode suggestions output: %v", err)
		}
		if len(payload.Suggestions) == 0 || len(payload.Suggestions) >= 1000 {
			t.Fatalf("expected default suggestions pagination (<1000 results), got %d", len(payload.Suggestions))
		}
	}

	p50, p95 := percentiles(samples, 0.50, 0.95)
	budgetP50, budgetP95 := perfBudgetSuggestions()
	t.Logf("PT-SUGGESTIONS-001 p50=%s p95=%s budget50=%s budget95=%s", p50, p95, budgetP50, budgetP95)
	assertPerfBudget(t, "PT-SUGGESTIONS-001", p50, p95, budgetP50, budgetP95)
	assertPerfRatio(t, "PT-SUGGESTIONS-001", p50, p95, 3.0)
}

func TestPerfDaemonStormSmoke(t *testing.T) {
	if os.Getenv("JUL_PERF_SMOKE") != "1" {
		t.Skip("set JUL_PERF_SMOKE=1 to run perf smoke suite")
	}
	julPath := perfCLI(t)
	repo, env := setupPerfRepo(t, "perf-daemon", 400, 256)

	daemonCfg := "[sync]\ndebounce_seconds = 1\nmin_interval_seconds = 1\n"
	if err := os.WriteFile(filepath.Join(repo, ".jul", "config.toml"), []byte(daemonCfg), 0o644); err != nil {
		t.Fatalf("failed to write daemon config: %v", err)
	}

	cmd := exec.Command(julPath, "sync", "--daemon")
	cmd.Dir = repo
	cmd.Env = mergeEnv(env)
	var stdout syncBuffer
	var stderr syncBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start sync daemon: %v", err)
	}
	defer func() {
		_ = cmd.Process.Signal(os.Interrupt)
		_ = cmd.Wait()
	}()

	waitForDaemonOutput(t, &stdout, &stderr, "Sync daemon running", 4*time.Second)
	deviceID := readDeviceIDFromHome(t, env["HOME"])
	syncRef := fmt.Sprintf("refs/jul/sync/perf/%s/@", strings.TrimSpace(deviceID))

	baselineSHA, ok := waitForRefSHA(repo, env, syncRef, 6*time.Second)
	if !ok {
		t.Fatalf("timed out waiting for daemon to create sync ref %s", syncRef)
	}

	stormStart := time.Now()
	for i := 0; i < perfDaemonEvents; i++ {
		file := filepath.Join("storm", fmt.Sprintf("event-%04d.txt", i%200))
		appendFile(t, repo, file, fmt.Sprintf("event-%d\n", i))
	}
	finalMarker := fmt.Sprintf("final-event-%d", perfDaemonEvents-1)
	writeFile(t, repo, filepath.Join("storm", "sentinel.txt"), finalMarker+"\n")
	stormEnd := time.Now()
	settleDeadline := stormEnd.Add(10 * time.Second)

	type cpuSample struct {
		value float64
		err   error
	}
	cpuResult := make(chan cpuSample, 1)
	go func(pid int, sampleStart time.Time) {
		if wait := time.Until(sampleStart); wait > 0 {
			time.Sleep(wait)
		}
		value, err := averageProcessCPU(pid, 2*time.Second)
		cpuResult <- cpuSample{value: value, err: err}
	}(cmd.Process.Pid, stormEnd.Add(8*time.Second))

	markerObserved := false
	for time.Now().Before(settleDeadline) {
		current := strings.TrimSpace(resolveRefQuiet(repo, env, syncRef))
		if current != "" && draftContainsMarker(repo, env, current, finalMarker) {
			markerObserved = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !markerObserved {
		t.Fatalf("PT-DAEMON-002 failed: daemon did not include sentinel marker within 10s after storm")
	}
	if wait := time.Until(settleDeadline); wait > 0 {
		time.Sleep(wait)
	}

	cpu := <-cpuResult
	if cpu.err != nil {
		t.Fatalf("PT-DAEMON-002 failed: cpu sample error: %v", cpu.err)
	}
	if cpu.value >= 1.0 {
		t.Fatalf("PT-DAEMON-002 failed: idle cpu %.2f%% exceeded 1%% near settle deadline", cpu.value)
	}

	time.Sleep(2 * time.Second)

	rawLogs := stdout.String() + "\n" + stderr.String()
	attemptStarts, maxInFlight, err := daemonSyncAttemptStats(rawLogs)
	if err != nil {
		t.Fatalf("PT-DAEMON-002 failed: %v", err)
	}
	if maxInFlight > 1 {
		t.Fatalf("PT-DAEMON-002 failed: expected at most one sync in flight, observed %d", maxInFlight)
	}
	stormAttempts := make([]time.Time, 0, len(attemptStarts))
	for _, started := range attemptStarts {
		if !started.Before(stormStart) {
			stormAttempts = append(stormAttempts, started)
		}
	}
	if len(stormAttempts) == 0 {
		t.Fatalf("PT-DAEMON-002 failed: no daemon sync attempts recorded during file storm")
	}
	sort.Slice(stormAttempts, func(i, j int) bool { return stormAttempts[i].Before(stormAttempts[j]) })
	debounce := time.Second
	for i := 1; i < len(stormAttempts); i++ {
		delta := stormAttempts[i].Sub(stormAttempts[i-1])
		if delta < debounce {
			t.Fatalf("PT-DAEMON-002 failed: sync transitions too close (%s < %s)", delta, debounce)
		}
	}
	for _, started := range stormAttempts {
		if started.After(settleDeadline) {
			t.Fatalf("PT-DAEMON-002 failed: observed sync attempt at %s after settle deadline %s without new events", started.Format(time.RFC3339Nano), settleDeadline.Format(time.RFC3339Nano))
		}
	}

	startsBeforeJul := len(attemptStarts)
	syncBeforeJul := strings.TrimSpace(resolveRefQuiet(repo, env, syncRef))
	for i := 0; i < 200; i++ {
		file := filepath.Join(".jul", "noise", fmt.Sprintf("event-%04d.txt", i%20))
		appendFile(t, repo, file, fmt.Sprintf("ignored-%d\n", i))
	}
	time.Sleep(2500 * time.Millisecond)

	rawLogsAfterJul := stdout.String() + "\n" + stderr.String()
	attemptStartsAfterJul, _, err := daemonSyncAttemptStats(rawLogsAfterJul)
	if err != nil {
		t.Fatalf("PT-DAEMON-002 failed: %v", err)
	}
	if len(attemptStartsAfterJul) != startsBeforeJul {
		t.Fatalf("PT-DAEMON-002 failed: .jul writes triggered sync attempts (%d -> %d)", startsBeforeJul, len(attemptStartsAfterJul))
	}
	if syncBeforeJul != "" {
		syncAfterJul := strings.TrimSpace(resolveRefQuiet(repo, env, syncRef))
		if syncAfterJul != syncBeforeJul {
			t.Fatalf("PT-DAEMON-002 failed: sync ref advanced after .jul-only writes (%s -> %s)", syncBeforeJul, syncAfterJul)
		}
	}
	if latest := strings.TrimSpace(resolveRefQuiet(repo, env, syncRef)); latest == baselineSHA {
		t.Fatalf("PT-DAEMON-002 failed: sync ref did not advance during file storm")
	}

	t.Logf("PT-DAEMON-002 attempts=%d cpu=%.2f%% settle=10s", len(stormAttempts), cpu.value)
}

func perfCLI(t *testing.T) string {
	t.Helper()
	return buildCLI(t)
}

func setupPerfRepo(t *testing.T, name string, files int, bytesPerFile int) (string, map[string]string) {
	t.Helper()
	root := t.TempDir()
	repo := setupPerfSeedRepo(t, root, name, files, bytesPerFile)

	home := filepath.Join(root, "home")
	env := perfEnv(home)
	julPath := perfCLI(t)
	runCmd(t, repo, env, julPath, "init", name)
	return repo, env
}

func setupPerfSeedRepo(t *testing.T, root, name string, files int, bytesPerFile int) string {
	t.Helper()
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
	return repo
}

func perfEnv(home string) map[string]string {
	return map[string]string{
		"HOME":                home,
		"JUL_WORKSPACE":       "perf/@",
		"JUL_NO_SYNC":         "1",
		"GIT_AUTHOR_NAME":     "Perf User",
		"GIT_AUTHOR_EMAIL":    "perf@example.com",
		"GIT_COMMITTER_NAME":  "Perf User",
		"GIT_COMMITTER_EMAIL": "perf@example.com",
		"PATH":                perfPath(),
	}
}

func perfWatchEnv(base map[string]string) map[string]string {
	out := make(map[string]string, len(base)+2)
	for key, value := range base {
		out[key] = value
	}
	out["JUL_WATCH"] = "1"
	out["JUL_WATCH_STREAM"] = "stdout"
	return out
}

func invalidateDraftIndex(t *testing.T, repo string) {
	t.Helper()
	draftIndexPath := filepath.Join(repo, ".jul", "draft-index")
	if err := os.Remove(draftIndexPath); err != nil && !os.IsNotExist(err) {
		t.Fatalf("failed to remove draft index: %v", err)
	}
}

func assertSyncProgressVisible(t *testing.T, repo string, env map[string]string, julPath string) {
	t.Helper()
	appendFile(t, repo, "src/file-0001.txt", fmt.Sprintf("\nfull-rebuild-progress-%d\n", time.Now().UnixNano()))
	invalidateDraftIndex(t, repo)

	cmd := exec.Command(julPath, "sync")
	cmd.Dir = repo
	cmd.Env = mergeEnv(perfWatchEnv(env))
	var stdout syncBuffer
	var stderr syncBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start sync progress check: %v", err)
	}
	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	needle := "sync: running"
	deadline := time.Now().Add(perfProgressVisibleDeadline)
	for time.Now().Before(deadline) {
		combined := stdout.String() + "\n" + stderr.String()
		if strings.Contains(combined, needle) {
			if err := <-waitCh; err != nil {
				t.Fatalf("sync failed after progress output: %v\n%s", err, combined)
			}
			return
		}
		select {
		case err := <-waitCh:
			t.Fatalf("sync exited before progress output (%v): %s", err, combined)
		default:
		}
		time.Sleep(10 * time.Millisecond)
	}

	combined := stdout.String() + "\n" + stderr.String()
	_ = cmd.Process.Signal(os.Interrupt)
	select {
	case <-waitCh:
	case <-time.After(2 * time.Second):
		_ = cmd.Process.Kill()
		<-waitCh
	}
	t.Fatalf("expected progress output %q within %s, got %s", needle, perfProgressVisibleDeadline, combined)
}

func assertSyncCancellationSafe(t *testing.T, julPath string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Log("skipping interrupt-based cancellation check on windows")
		return
	}

	repo, env := setupPerfRepo(t, "perf-sync-cancel", 10000, 1024)
	appendFile(t, repo, "src/file-0001.txt", fmt.Sprintf("\nfull-rebuild-cancel-%d\n", time.Now().UnixNano()))
	invalidateDraftIndex(t, repo)

	cmd := exec.Command(julPath, "sync")
	cmd.Dir = repo
	cmd.Env = mergeEnv(perfWatchEnv(env))
	var stdout syncBuffer
	var stderr syncBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start sync cancellation check: %v", err)
	}
	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	needle := "sync: running"
	progressDeadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(progressDeadline) {
		combined := stdout.String() + "\n" + stderr.String()
		if strings.Contains(combined, needle) {
			break
		}
		select {
		case err := <-waitCh:
			t.Fatalf("sync exited before cancellation signal (%v): %s", err, combined)
		default:
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		combined := stdout.String() + "\n" + stderr.String()
		t.Fatalf("failed to interrupt full rebuild sync: %v\n%s", err, combined)
	}

	select {
	case <-waitCh:
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		<-waitCh
		t.Fatalf("sync did not exit promptly after interrupt")
	}

	lockPath := filepath.Join(repo, ".jul", "draft-index.lock")
	if _, err := os.Stat(lockPath); err == nil {
		t.Fatalf("expected draft index lock to be removed after interrupt")
	} else if !os.IsNotExist(err) {
		t.Fatalf("failed to stat draft index lock: %v", err)
	}

	output := runCmdTimed(t, repo, env, julPath, "sync", "--json")
	if _, ok := parseTimings(t, output); !ok {
		t.Fatalf("expected sync timings after interrupted run, got %s", output)
	}
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

func perfBudgetStatusCloneColdP95() time.Duration {
	_, p95 := applyPerfMultiplier(0, 250*time.Millisecond)
	return p95
}

func perfBudgetSync() (time.Duration, time.Duration) {
	return applyPerfMultiplier(300*time.Millisecond, 1*time.Second)
}

func perfBudgetSyncFullRebuild() (time.Duration, time.Duration) {
	return applyPerfMultiplier(1500*time.Millisecond, 5*time.Second)
}

func perfBudgetSyncLocalTransport() (time.Duration, time.Duration) {
	return applyPerfMultiplier(1100*time.Millisecond, 1150*time.Millisecond)
}

func perfBudgetSyncTransportAddendumP95() time.Duration {
	// Local transport addendum budget for pack <=20MB.
	_, p95 := applyPerfMultiplier(0, 500*time.Millisecond)
	return p95
}

func perfBudgetSyncCloneColdLocalP95() time.Duration {
	_, p95 := applyPerfMultiplier(0, 3500*time.Millisecond)
	return p95
}

func perfBudgetCheckpoint() (time.Duration, time.Duration) {
	return applyPerfMultiplier(250*time.Millisecond, 800*time.Millisecond)
}

func perfBudgetCheckpointCloneColdP95() time.Duration {
	_, p95 := applyPerfMultiplier(0, 2*time.Second)
	return p95
}

func perfBudgetNotesMerge() (time.Duration, time.Duration) {
	return applyPerfMultiplier(500*time.Millisecond, 3*time.Second)
}

func perfBudgetPromoteWarm() (time.Duration, time.Duration) {
	return applyPerfMultiplier(1500*time.Millisecond, 5*time.Second)
}

func perfBudgetPromoteCloneColdP95() time.Duration {
	_, p95 := applyPerfMultiplier(0, 8*time.Second)
	return p95
}

func perfBudgetSuggestions() (time.Duration, time.Duration) {
	return applyPerfMultiplier(50*time.Millisecond, 200*time.Millisecond)
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

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func waitForDaemonOutput(t *testing.T, stdout, stderr *syncBuffer, needle string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		combined := stdout.String() + stderr.String()
		if strings.Contains(combined, needle) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for daemon output %q, got %s", needle, stdout.String()+stderr.String())
}

func readDeviceIDFromHome(t *testing.T, home string) string {
	t.Helper()
	path := filepath.Join(home, ".config", "jul", "device")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read device id: %v", err)
	}
	return strings.TrimSpace(string(data))
}

func waitForRefSHA(repo string, env map[string]string, ref string, timeout time.Duration) (string, bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		sha := strings.TrimSpace(resolveRefQuiet(repo, env, ref))
		if sha != "" {
			return sha, true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return "", false
}

func resolveRefQuiet(repo string, env map[string]string, ref string) string {
	cmd := exec.Command("git", "rev-parse", ref)
	cmd.Dir = repo
	cmd.Env = mergeEnv(env)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func draftContainsMarker(repo string, env map[string]string, draftSHA string, marker string) bool {
	if strings.TrimSpace(draftSHA) == "" || strings.TrimSpace(marker) == "" {
		return false
	}
	pathSpec := fmt.Sprintf("%s:%s", strings.TrimSpace(draftSHA), "storm/sentinel.txt")
	cmd := exec.Command("git", "show", pathSpec)
	cmd.Dir = repo
	cmd.Env = mergeEnv(env)
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), marker)
}

type daemonSyncLog struct {
	Event     string `json:"event"`
	AttemptID int64  `json:"attempt_id"`
	AtUnixMs  int64  `json:"at_unix_ms"`
	InFlight  int64  `json:"in_flight"`
}

func daemonSyncAttemptStats(raw string) ([]time.Time, int64, error) {
	lines := strings.Split(raw, "\n")
	starts := make([]time.Time, 0, 16)
	maxInFlight := int64(0)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, "\"event\":\"daemon_sync_") {
			continue
		}
		var entry daemonSyncLog
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.InFlight > maxInFlight {
			maxInFlight = entry.InFlight
		}
		if entry.Event == "daemon_sync_start" && entry.AtUnixMs > 0 {
			starts = append(starts, time.UnixMilli(entry.AtUnixMs))
		}
	}
	if len(starts) == 0 {
		return nil, 0, fmt.Errorf("no daemon sync attempts found in output")
	}
	return starts, maxInFlight, nil
}

func averageProcessCPU(pid int, window time.Duration) (float64, error) {
	if pid <= 0 {
		return 0, fmt.Errorf("invalid pid %d", pid)
	}
	if window <= 0 {
		return 0, fmt.Errorf("invalid cpu sample window %s", window)
	}
	startCPU, err := processCPUTime(pid)
	if err != nil {
		return 0, err
	}
	started := time.Now()
	time.Sleep(window)
	endCPU, err := processCPUTime(pid)
	if err != nil {
		return 0, err
	}
	elapsed := time.Since(started)
	if elapsed <= 0 {
		return 0, fmt.Errorf("invalid elapsed duration %s", elapsed)
	}
	used := endCPU - startCPU
	if used < 0 {
		used = 0
	}
	return (used.Seconds() / elapsed.Seconds()) * 100, nil
}

func processCPUTime(pid int) (time.Duration, error) {
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "cputime=").Output()
	if err != nil {
		return 0, err
	}
	return parseProcessCPUTime(string(out))
}

func parseProcessCPUTime(raw string) (time.Duration, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, fmt.Errorf("empty cputime output")
	}
	days := 0
	if dash := strings.Index(value, "-"); dash >= 0 {
		parsedDays, err := strconv.Atoi(value[:dash])
		if err != nil {
			return 0, fmt.Errorf("invalid cputime days %q: %w", value[:dash], err)
		}
		days = parsedDays
		value = value[dash+1:]
	}
	parts := strings.Split(value, ":")
	parseSeconds := func(input string) (float64, error) {
		return strconv.ParseFloat(strings.TrimSpace(input), 64)
	}
	hours := 0
	minutes := 0
	seconds := 0.0
	switch len(parts) {
	case 3:
		parsedHours, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			return 0, fmt.Errorf("invalid cputime hours %q: %w", parts[0], err)
		}
		hours = parsedHours
		parsedMinutes, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return 0, fmt.Errorf("invalid cputime minutes %q: %w", parts[1], err)
		}
		minutes = parsedMinutes
		parsedSeconds, err := parseSeconds(parts[2])
		if err != nil {
			return 0, fmt.Errorf("invalid cputime seconds %q: %w", parts[2], err)
		}
		seconds = parsedSeconds
	case 2:
		parsedMinutes, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			return 0, fmt.Errorf("invalid cputime minutes %q: %w", parts[0], err)
		}
		minutes = parsedMinutes
		parsedSeconds, err := parseSeconds(parts[1])
		if err != nil {
			return 0, fmt.Errorf("invalid cputime seconds %q: %w", parts[1], err)
		}
		seconds = parsedSeconds
	case 1:
		parsedSeconds, err := parseSeconds(parts[0])
		if err != nil {
			return 0, fmt.Errorf("invalid cputime seconds %q: %w", parts[0], err)
		}
		seconds = parsedSeconds
	default:
		return 0, fmt.Errorf("unsupported cputime format %q", raw)
	}
	totalSeconds := float64(days*24*60*60 + hours*60*60 + minutes*60)
	totalSeconds += seconds
	if totalSeconds < 0 {
		totalSeconds = 0
	}
	return time.Duration(totalSeconds * float64(time.Second)), nil
}

func parsePhaseTiming(t *testing.T, output string, phase string) (time.Duration, bool) {
	t.Helper()
	var payload timingsPayload
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to decode timings payload: %v\n%s", err, output)
	}
	if payload.Timings.Phase == nil {
		return 0, false
	}
	value, ok := payload.Timings.Phase[phase]
	if !ok {
		return 0, false
	}
	if value < 0 {
		value = 0
	}
	return time.Duration(value) * time.Millisecond, true
}

type backgroundCounts struct {
	CIRuns        int
	CILogs        int
	ReviewResults int
	ReviewLogs    int
}

func backgroundArtifactCounts(repo string) backgroundCounts {
	return backgroundCounts{
		CIRuns:        countFiles(filepath.Join(repo, ".jul", "ci", "runs")),
		CILogs:        countFiles(filepath.Join(repo, ".jul", "ci", "logs")),
		ReviewResults: countFiles(filepath.Join(repo, ".jul", "review", "results")),
		ReviewLogs:    countFiles(filepath.Join(repo, ".jul", "review", "logs")),
	}
}

func countFiles(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			count++
		}
	}
	return count
}

func setupPerfRemote(t *testing.T, repo string, env map[string]string, julPath string) string {
	t.Helper()
	remoteRoot := t.TempDir()
	remoteDir := filepath.Join(remoteRoot, "remote.git")
	runCmd(t, remoteRoot, nil, "git", "init", "--bare", remoteDir)
	runCmd(t, repo, nil, "git", "remote", "add", "origin", remoteDir)
	runCmd(t, repo, nil, "git", "push", "-u", "origin", "HEAD:refs/heads/main")
	runCmd(t, remoteDir, nil, "git", "--git-dir", remoteDir, "symbolic-ref", "HEAD", "refs/heads/main")
	runCmd(t, repo, env, julPath, "remote", "set", "origin")
	return remoteDir
}

func addNotesEntries(t *testing.T, workDir string, gitDir string, ref string, prefix string, start int, entries int, eventsPerEntry int) int {
	t.Helper()
	if entries <= 0 {
		return start
	}
	if eventsPerEntry <= 0 {
		eventsPerEntry = 1
	}

	scratch := t.TempDir()
	keyPath := filepath.Join(scratch, "key.txt")
	notePath := filepath.Join(scratch, "note.txt")
	noteEnv := map[string]string{
		"GIT_AUTHOR_NAME":     "Perf Notes",
		"GIT_AUTHOR_EMAIL":    "perf-notes@example.com",
		"GIT_COMMITTER_NAME":  "Perf Notes",
		"GIT_COMMITTER_EMAIL": "perf-notes@example.com",
	}

	index := start
	for i := 0; i < entries; i++ {
		entryID := index + i
		keyData := []byte(fmt.Sprintf("%s-key-%06d\n", prefix, entryID))
		if err := os.WriteFile(keyPath, keyData, 0o644); err != nil {
			t.Fatalf("failed to write key payload: %v", err)
		}

		hashArgs := []string{"hash-object", "-w", keyPath}
		if strings.TrimSpace(gitDir) != "" {
			hashArgs = append([]string{"--git-dir", gitDir}, hashArgs...)
		}
		objectSHA := strings.TrimSpace(runCmdTimed(t, workDir, nil, "git", hashArgs...))
		if objectSHA == "" {
			t.Fatalf("failed to create object for notes entry %d", entryID)
		}

		var payload strings.Builder
		for line := 0; line < eventsPerEntry; line++ {
			payload.WriteString(fmt.Sprintf("{\"event\":\"%s-%06d-%04d\"}\n", prefix, entryID, line))
		}
		if err := os.WriteFile(notePath, []byte(payload.String()), 0o644); err != nil {
			t.Fatalf("failed to write note payload: %v", err)
		}

		noteArgs := []string{"notes", "--ref", ref, "add", "-f", "-F", notePath, objectSHA}
		if strings.TrimSpace(gitDir) != "" {
			noteArgs = append([]string{"--git-dir", gitDir}, noteArgs...)
		}
		_ = runCmdTimed(t, workDir, noteEnv, "git", noteArgs...)
	}

	return start + entries
}

func addSuggestionNotesEntries(t *testing.T, repo string, changeID string, baseSHA string, total int) {
	t.Helper()
	if total <= 0 {
		return
	}

	scratch := t.TempDir()
	keyPath := filepath.Join(scratch, "key.txt")
	notePath := filepath.Join(scratch, "note.json")
	noteEnv := map[string]string{
		"GIT_AUTHOR_NAME":     "Perf Suggestions",
		"GIT_AUTHOR_EMAIL":    "perf-suggestions@example.com",
		"GIT_COMMITTER_NAME":  "Perf Suggestions",
		"GIT_COMMITTER_EMAIL": "perf-suggestions@example.com",
	}

	for i := 0; i < total; i++ {
		keyData := []byte(fmt.Sprintf("suggestion-key-%06d\n", i))
		if err := os.WriteFile(keyPath, keyData, 0o644); err != nil {
			t.Fatalf("failed to write suggestion key payload: %v", err)
		}
		objectSHA := strings.TrimSpace(runCmdTimed(t, repo, nil, "git", "hash-object", "-w", keyPath))
		if objectSHA == "" {
			t.Fatalf("failed to create suggestion object for index %d", i)
		}

		payload := map[string]any{
			"suggestion_id":        fmt.Sprintf("sug-%06d", i),
			"change_id":            changeID,
			"base_commit_sha":      baseSHA,
			"suggested_commit_sha": baseSHA,
			"created_by":           "perf",
			"reason":               "perf",
			"description":          "perf suggestion",
			"confidence":           0.5,
			"status":               "pending",
			"created_at":           time.Unix(1700000000+int64(i), 0).UTC().Format(time.RFC3339Nano),
		}
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("failed to marshal suggestion payload: %v", err)
		}
		if err := os.WriteFile(notePath, data, 0o644); err != nil {
			t.Fatalf("failed to write suggestion payload: %v", err)
		}
		_ = runCmdTimed(t, repo, noteEnv, "git", "notes", "--ref", "refs/notes/jul/suggestions", "add", "-f", "-F", notePath, objectSHA)
	}
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
	// Small medians are dominated by scheduler jitter and timer granularity.
	// In that regime, absolute p95 budgets are more stable than a relative ratio gate.
	if p50 < 50*time.Millisecond {
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
