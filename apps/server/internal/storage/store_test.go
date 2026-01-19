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
