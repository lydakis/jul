package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lydakis/jul/server/internal/events"
	"github.com/lydakis/jul/server/internal/storage"
)

func newTestServer(t *testing.T) (*Server, *storage.Store) {
	storePath := t.TempDir() + "/jul.db"
	store, err := storage.Open(storePath)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	broker := events.NewBroker()
	return New(Config{Address: ":0"}, store, broker), store
}

func TestSyncEndpoint(t *testing.T) {
	srv, store := newTestServer(t)
	defer store.Close()

	payload := storage.SyncPayload{
		WorkspaceID: "bob/laptop",
		Repo:        "demo",
		Branch:      "main",
		CommitSHA:   "abc123",
		ChangeID:    "I0123456789abcdef0123456789abcdef01234567",
		Message:     "feat: add",
		Author:      "bob",
		CommittedAt: time.Now().UTC(),
	}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sync", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleSync(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var res storage.SyncResult
	if err := json.NewDecoder(w.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if res.Change.ChangeID != payload.ChangeID {
		t.Fatalf("expected change id %s, got %s", payload.ChangeID, res.Change.ChangeID)
	}
}

func TestWorkspacesEndpoint(t *testing.T) {
	srv, store := newTestServer(t)
	defer store.Close()

	_, err := store.RecordSync(httptest.NewRequest(http.MethodGet, "/", nil).Context(), storage.SyncPayload{
		WorkspaceID: "bob/desktop",
		Repo:        "demo",
		Branch:      "main",
		CommitSHA:   "def456",
		ChangeID:    "I1111111111111111111111111111111111111111",
		Message:     "fix: test",
		Author:      "bob",
		CommittedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("RecordSync failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces", nil)
	w := httptest.NewRecorder()
	srv.handleWorkspaces(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestWorkspacePromoteNameRouting(t *testing.T) {
	srv, store := newTestServer(t)
	defer store.Close()

	_, err := store.RecordSync(httptest.NewRequest(http.MethodGet, "/", nil).Context(), storage.SyncPayload{
		WorkspaceID: "alice/promote",
		Repo:        "demo",
		Branch:      "main",
		CommitSHA:   "abc999",
		ChangeID:    "I2222222222222222222222222222222222222222",
		Message:     "chore: test",
		Author:      "alice",
		CommittedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("RecordSync failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/alice/promote", nil)
	w := httptest.NewRecorder()
	srv.handleWorkspaceRoutes(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestWorkspaceReflogEndpoint(t *testing.T) {
	srv, store := newTestServer(t)
	defer store.Close()

	workspaceID := "alice/laptop"
	first := storage.SyncPayload{
		WorkspaceID: workspaceID,
		Repo:        "demo",
		Branch:      "main",
		CommitSHA:   "commit-a",
		ChangeID:    "I3333333333333333333333333333333333333333",
		Message:     "feat: first",
		Author:      "alice",
		CommittedAt: time.Now().UTC(),
	}
	second := storage.SyncPayload{
		WorkspaceID: workspaceID,
		Repo:        "demo",
		Branch:      "main",
		CommitSHA:   "commit-b",
		ChangeID:    "I3333333333333333333333333333333333333333",
		Message:     "feat: second",
		Author:      "alice",
		CommittedAt: time.Now().UTC().Add(1 * time.Minute),
	}

	if _, err := store.RecordSync(httptest.NewRequest(http.MethodGet, "/", nil).Context(), first); err != nil {
		t.Fatalf("RecordSync first failed: %v", err)
	}
	if _, err := store.RecordSync(httptest.NewRequest(http.MethodGet, "/", nil).Context(), second); err != nil {
		t.Fatalf("RecordSync second failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/alice/laptop/reflog", nil)
	w := httptest.NewRecorder()
	srv.handleWorkspaceRoutes(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var entries []ReflogEntry
	if err := json.NewDecoder(w.Body).Decode(&entries); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(entries) < 2 {
		t.Fatalf("expected at least 2 entries, got %d", len(entries))
	}
	if entries[0].CommitSHA != second.CommitSHA || entries[0].Source != "current" {
		t.Fatalf("expected current commit %s, got %s (%s)", second.CommitSHA, entries[0].CommitSHA, entries[0].Source)
	}
}
