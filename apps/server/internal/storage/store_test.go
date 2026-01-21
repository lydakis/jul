package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "jul.db")
	store, err := Open(path)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
		_ = os.RemoveAll(tmp)
	})
	return store
}

func TestRecordSyncCreatesChangeRevision(t *testing.T) {
	store := newTestStore(t)
	payload := SyncPayload{
		WorkspaceID: "alice/laptop",
		Repo:        "demo",
		Branch:      "main",
		CommitSHA:   "abc123",
		ChangeID:    "I0123456789abcdef0123456789abcdef01234567",
		Message:     "feat: add thing",
		Author:      "alice",
		CommittedAt: time.Now().UTC(),
	}

	res, err := store.RecordSync(context.Background(), payload)
	if err != nil {
		t.Fatalf("RecordSync failed: %v", err)
	}
	if res.Change.ChangeID != payload.ChangeID {
		t.Fatalf("expected change id %s, got %s", payload.ChangeID, res.Change.ChangeID)
	}
	if res.Revision.RevIndex != 1 {
		t.Fatalf("expected rev 1, got %d", res.Revision.RevIndex)
	}

	rev, err := store.GetRevisionByCommit(context.Background(), payload.CommitSHA)
	if err != nil {
		t.Fatalf("GetRevisionByCommit failed: %v", err)
	}
	if rev.CommitSHA != payload.CommitSHA {
		t.Fatalf("expected commit %s, got %s", payload.CommitSHA, rev.CommitSHA)
	}
}

func TestListChanges(t *testing.T) {
	store := newTestStore(t)
	payload := SyncPayload{
		WorkspaceID: "alice/desktop",
		Repo:        "demo",
		Branch:      "main",
		CommitSHA:   "def456",
		ChangeID:    "I9999999999999999999999999999999999999999",
		Message:     "fix: bug",
		Author:      "alice",
		CommittedAt: time.Now().UTC(),
	}

	if _, err := store.RecordSync(context.Background(), payload); err != nil {
		t.Fatalf("RecordSync failed: %v", err)
	}

	changes, err := store.ListChanges(context.Background())
	if err != nil {
		t.Fatalf("ListChanges failed: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
}

func TestRecordSyncDeterministicChangeID(t *testing.T) {
	store := newTestStore(t)

	payload := SyncPayload{
		WorkspaceID: "alice/laptop",
		Repo:        "demo",
		Branch:      "main",
		CommitSHA:   "abc123",
		Message:     "feat: add thing",
		Author:      "alice",
		CommittedAt: time.Now().UTC(),
	}

	first, err := store.RecordSync(context.Background(), payload)
	if err != nil {
		t.Fatalf("RecordSync failed: %v", err)
	}

	second, err := store.RecordSync(context.Background(), payload)
	if err != nil {
		t.Fatalf("RecordSync failed: %v", err)
	}

	if first.Change.ChangeID != second.Change.ChangeID {
		t.Fatalf("expected stable change id, got %s and %s", first.Change.ChangeID, second.Change.ChangeID)
	}

	changes, err := store.ListChanges(context.Background())
	if err != nil {
		t.Fatalf("ListChanges failed: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
}

func TestRecordSyncDoesNotDowngradeLatest(t *testing.T) {
	store := newTestStore(t)
	changeID := "I0123456789abcdef0123456789abcdef01234567"

	first := SyncPayload{
		WorkspaceID: "alice/laptop",
		Repo:        "demo",
		Branch:      "main",
		CommitSHA:   "commit-a",
		ChangeID:    changeID,
		Message:     "feat: first",
		Author:      "alice",
		CommittedAt: time.Now().UTC(),
	}
	second := SyncPayload{
		WorkspaceID: "alice/laptop",
		Repo:        "demo",
		Branch:      "main",
		CommitSHA:   "commit-b",
		ChangeID:    changeID,
		Message:     "feat: second",
		Author:      "alice",
		CommittedAt: time.Now().UTC().Add(1 * time.Minute),
	}

	if _, err := store.RecordSync(context.Background(), first); err != nil {
		t.Fatalf("RecordSync first failed: %v", err)
	}
	if _, err := store.RecordSync(context.Background(), second); err != nil {
		t.Fatalf("RecordSync second failed: %v", err)
	}

	resync, err := store.RecordSync(context.Background(), first)
	if err != nil {
		t.Fatalf("RecordSync resync failed: %v", err)
	}

	if resync.Change.LatestCommitSHA != second.CommitSHA {
		t.Fatalf("expected latest commit %s, got %s", second.CommitSHA, resync.Change.LatestCommitSHA)
	}
	if resync.Change.LatestRevIndex != 2 {
		t.Fatalf("expected latest rev 2, got %d", resync.Change.LatestRevIndex)
	}
}

func TestKeepRefsInsertedOnWorkspaceMove(t *testing.T) {
	store := newTestStore(t)
	workspaceID := "alice/laptop"

	first := SyncPayload{
		WorkspaceID: workspaceID,
		Repo:        "demo",
		Branch:      "main",
		CommitSHA:   "commit-a",
		ChangeID:    "I4444444444444444444444444444444444444444",
		Message:     "feat: first",
		Author:      "alice",
		CommittedAt: time.Now().UTC(),
	}
	second := SyncPayload{
		WorkspaceID: workspaceID,
		Repo:        "demo",
		Branch:      "main",
		CommitSHA:   "commit-b",
		ChangeID:    "I4444444444444444444444444444444444444444",
		Message:     "feat: second",
		Author:      "alice",
		CommittedAt: time.Now().UTC().Add(1 * time.Minute),
	}

	if _, err := store.RecordSync(context.Background(), first); err != nil {
		t.Fatalf("RecordSync first failed: %v", err)
	}
	if _, err := store.RecordSync(context.Background(), second); err != nil {
		t.Fatalf("RecordSync second failed: %v", err)
	}

	refs, err := store.ListKeepRefs(context.Background(), workspaceID, 10)
	if err != nil {
		t.Fatalf("ListKeepRefs failed: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("expected 1 keep ref, got %d", len(refs))
	}
	if refs[0].CommitSHA != first.CommitSHA {
		t.Fatalf("expected keep ref for %s, got %s", first.CommitSHA, refs[0].CommitSHA)
	}
}

func TestQueryCommitsFilters(t *testing.T) {
	store := newTestStore(t)
	now := time.Now().UTC()

	first := SyncPayload{
		WorkspaceID: "alice/laptop",
		Repo:        "demo",
		Branch:      "main",
		CommitSHA:   "commit-a",
		ChangeID:    "I1111111111111111111111111111111111111111",
		Message:     "feat: first",
		Author:      "alice",
		CommittedAt: now.Add(-2 * time.Hour),
	}
	second := SyncPayload{
		WorkspaceID: "alice/laptop",
		Repo:        "demo",
		Branch:      "main",
		CommitSHA:   "commit-b",
		ChangeID:    "I2222222222222222222222222222222222222222",
		Message:     "feat: second",
		Author:      "alice",
		CommittedAt: now.Add(-1 * time.Hour),
	}

	if _, err := store.RecordSync(context.Background(), first); err != nil {
		t.Fatalf("RecordSync first failed: %v", err)
	}
	if _, err := store.RecordSync(context.Background(), second); err != nil {
		t.Fatalf("RecordSync second failed: %v", err)
	}

	lowCoverage := 70.0
	highCoverage := 85.0

	if _, err := store.CreateAttestation(context.Background(), Attestation{
		CommitSHA:       first.CommitSHA,
		ChangeID:        first.ChangeID,
		Type:            "ci",
		Status:          "fail",
		TestStatus:      "fail",
		CompileStatus:   "fail",
		CoverageLinePct: &lowCoverage,
		StartedAt:       now,
		FinishedAt:      now,
	}); err != nil {
		t.Fatalf("CreateAttestation first failed: %v", err)
	}

	if _, err := store.CreateAttestation(context.Background(), Attestation{
		CommitSHA:       second.CommitSHA,
		ChangeID:        second.ChangeID,
		Type:            "ci",
		Status:          "pass",
		TestStatus:      "pass",
		CompileStatus:   "pass",
		CoverageLinePct: &highCoverage,
		StartedAt:       now,
		FinishedAt:      now,
	}); err != nil {
		t.Fatalf("CreateAttestation second failed: %v", err)
	}

	compiles := true
	since := now.Add(-90 * time.Minute)
	until := now.Add(-30 * time.Minute)
	minCoverage := 80.0

	results, err := store.QueryCommits(context.Background(), QueryFilters{
		Tests:       "pass",
		Compiles:    &compiles,
		CoverageMin: &minCoverage,
		Since:       &since,
		Until:       &until,
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("QueryCommits failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].CommitSHA != second.CommitSHA {
		t.Fatalf("expected %s, got %s", second.CommitSHA, results[0].CommitSHA)
	}
	if results[0].CoverageLinePct == nil || *results[0].CoverageLinePct != highCoverage {
		t.Fatalf("expected coverage %.1f, got %v", highCoverage, results[0].CoverageLinePct)
	}
}

func TestSuggestionLifecycle(t *testing.T) {
	store := newTestStore(t)
	now := time.Now().UTC()

	created, err := store.CreateSuggestion(context.Background(), Suggestion{
		ChangeID:           "I3333333333333333333333333333333333333333",
		BaseCommitSHA:      "base-sha",
		SuggestedCommitSHA: "suggest-sha",
		CreatedBy:          "tester",
		Reason:             "fix_tests",
		Description:        "adjust failing test",
		Confidence:         0.82,
		Status:             "pending",
		DiffstatJSON:       `{"files_changed":1}`,
		CreatedAt:          now,
	})
	if err != nil {
		t.Fatalf("CreateSuggestion failed: %v", err)
	}

	fetched, err := store.GetSuggestion(context.Background(), created.SuggestionID)
	if err != nil {
		t.Fatalf("GetSuggestion failed: %v", err)
	}
	if fetched.ChangeID != created.ChangeID {
		t.Fatalf("expected change_id %s, got %s", created.ChangeID, fetched.ChangeID)
	}

	list, err := store.ListSuggestions(context.Background(), created.ChangeID, "pending", 10)
	if err != nil {
		t.Fatalf("ListSuggestions failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(list))
	}

	updated, err := store.UpdateSuggestionStatus(context.Background(), created.SuggestionID, "applied", time.Now().UTC())
	if err != nil {
		t.Fatalf("UpdateSuggestionStatus failed: %v", err)
	}
	if updated.Status != "applied" {
		t.Fatalf("expected status applied, got %s", updated.Status)
	}
	if updated.ResolvedAt.IsZero() {
		t.Fatalf("expected resolved_at to be set")
	}
}

func TestQueryCommitsByStatus(t *testing.T) {
	store := newTestStore(t)
	first := SyncPayload{
		WorkspaceID: "alice/laptop",
		Repo:        "demo",
		Branch:      "main",
		CommitSHA:   "commit-a",
		ChangeID:    "I5555555555555555555555555555555555555555",
		Message:     "feat: first",
		Author:      "alice",
		CommittedAt: time.Now().UTC(),
	}
	second := SyncPayload{
		WorkspaceID: "alice/laptop",
		Repo:        "demo",
		Branch:      "main",
		CommitSHA:   "commit-b",
		ChangeID:    "I5555555555555555555555555555555555555555",
		Message:     "feat: second",
		Author:      "alice",
		CommittedAt: time.Now().UTC().Add(1 * time.Minute),
	}

	if _, err := store.RecordSync(context.Background(), first); err != nil {
		t.Fatalf("RecordSync first failed: %v", err)
	}
	if _, err := store.RecordSync(context.Background(), second); err != nil {
		t.Fatalf("RecordSync second failed: %v", err)
	}

	_, err := store.CreateAttestation(context.Background(), Attestation{
		CommitSHA:   first.CommitSHA,
		ChangeID:    first.ChangeID,
		Type:        "ci",
		Status:      "pass",
		StartedAt:   time.Now().UTC(),
		FinishedAt:  time.Now().UTC(),
		SignalsJSON: "{}",
	})
	if err != nil {
		t.Fatalf("CreateAttestation failed: %v", err)
	}
	_, err = store.CreateAttestation(context.Background(), Attestation{
		CommitSHA:   second.CommitSHA,
		ChangeID:    second.ChangeID,
		Type:        "ci",
		Status:      "fail",
		StartedAt:   time.Now().UTC(),
		FinishedAt:  time.Now().UTC(),
		SignalsJSON: "{}",
	})
	if err != nil {
		t.Fatalf("CreateAttestation failed: %v", err)
	}

	results, err := store.QueryCommits(context.Background(), QueryFilters{Tests: "pass", Limit: 10})
	if err != nil {
		t.Fatalf("QueryCommits failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].CommitSHA != first.CommitSHA {
		t.Fatalf("expected commit %s, got %s", first.CommitSHA, results[0].CommitSHA)
	}
}

func TestAttestationNullFieldsFallback(t *testing.T) {
	store := newTestStore(t)
	now := time.Now().UTC()

	if _, err := store.RecordSync(context.Background(), SyncPayload{
		WorkspaceID: "alice/laptop",
		Repo:        "demo",
		Branch:      "main",
		CommitSHA:   "commit-null",
		ChangeID:    "I7777777777777777777777777777777777777777",
		Message:     "feat: null attestation",
		Author:      "alice",
		CommittedAt: now,
	}); err != nil {
		t.Fatalf("RecordSync failed: %v", err)
	}

	_, err := store.db.ExecContext(context.Background(), `INSERT INTO attestations
		(attestation_id, commit_sha, change_id, type, status, compile_status, test_status, started_at, finished_at, signals_json, created_at)
		VALUES (?, ?, ?, ?, ?, NULL, NULL, ?, ?, ?, ?)`,
		"01HNULLATTESTATION", "commit-null", "I7777777777777777777777777777777777777777", "ci", "pass",
		now.Format(timeFormat), now.Format(timeFormat), "{}", now.Format(timeFormat))
	if err != nil {
		t.Fatalf("insert attestation failed: %v", err)
	}

	list, err := store.ListAttestations(context.Background(), "commit-null", "", "")
	if err != nil {
		t.Fatalf("ListAttestations failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 attestation, got %d", len(list))
	}
	if list[0].CompileStatus != "pass" || list[0].TestStatus != "pass" {
		t.Fatalf("expected fallback statuses to be pass, got compile=%s test=%s", list[0].CompileStatus, list[0].TestStatus)
	}

	latest, err := store.GetLatestAttestation(context.Background(), "commit-null")
	if err != nil {
		t.Fatalf("GetLatestAttestation failed: %v", err)
	}
	if latest.CompileStatus != "pass" || latest.TestStatus != "pass" {
		t.Fatalf("expected fallback statuses to be pass, got compile=%s test=%s", latest.CompileStatus, latest.TestStatus)
	}
}

func TestFindRepoForHistoricalCommit(t *testing.T) {
	store := newTestStore(t)
	payload := SyncPayload{
		WorkspaceID: "alice/laptop",
		Repo:        "demo",
		Branch:      "main",
		CommitSHA:   "commit-a",
		ChangeID:    "I6666666666666666666666666666666666666666",
		Message:     "feat: first",
		Author:      "alice",
		CommittedAt: time.Now().UTC(),
	}
	if _, err := store.RecordSync(context.Background(), payload); err != nil {
		t.Fatalf("RecordSync failed: %v", err)
	}

	if _, err := store.RecordSync(context.Background(), SyncPayload{
		WorkspaceID: "alice/laptop",
		Repo:        "demo",
		Branch:      "main",
		CommitSHA:   "commit-b",
		ChangeID:    payload.ChangeID,
		Message:     "feat: second",
		Author:      "alice",
		CommittedAt: time.Now().UTC().Add(1 * time.Minute),
	}); err != nil {
		t.Fatalf("RecordSync second failed: %v", err)
	}

	repo, err := store.FindRepoForCommit(context.Background(), payload.CommitSHA)
	if err != nil {
		t.Fatalf("FindRepoForCommit failed: %v", err)
	}
	if repo != "demo" {
		t.Fatalf("expected repo demo, got %s", repo)
	}
}
