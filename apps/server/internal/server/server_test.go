package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	return New(Config{Address: ":0", ReposDir: t.TempDir()}, store, broker), store
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

func TestQueryRejectsUnsupportedFilters(t *testing.T) {
	srv, store := newTestServer(t)
	defer store.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/query?unsupported=1", nil)
	w := httptest.NewRecorder()
	srv.handleQuery(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCITriggerInvalidRepo(t *testing.T) {
	srv, store := newTestServer(t)
	defer store.Close()

	payload := storage.SyncPayload{
		WorkspaceID: "bob/laptop",
		Repo:        "demo",
		Branch:      "main",
		CommitSHA:   "abc123",
		ChangeID:    "I7777777777777777777777777777777777777777",
		Message:     "feat: add",
		Author:      "bob",
		CommittedAt: time.Now().UTC(),
	}
	if _, err := store.RecordSync(httptest.NewRequest(http.MethodGet, "/", nil).Context(), payload); err != nil {
		t.Fatalf("RecordSync failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ci/trigger", bytes.NewReader([]byte(`{"commit_sha":"abc123","profile":"unit","repo":"../bad"}`)))
	w := httptest.NewRecorder()
	srv.handleCITrigger(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCITriggerMissingRepo(t *testing.T) {
	tempRepos := t.TempDir()
	storePath := t.TempDir() + "/jul.db"
	store, err := storage.Open(storePath)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	broker := events.NewBroker()
	srv := New(Config{ReposDir: tempRepos}, store, broker)

	payload := storage.SyncPayload{
		WorkspaceID: "bob/laptop",
		Repo:        "demo",
		Branch:      "main",
		CommitSHA:   "abc999",
		ChangeID:    "I8888888888888888888888888888888888888888",
		Message:     "feat: add",
		Author:      "bob",
		CommittedAt: time.Now().UTC(),
	}
	if _, err := store.RecordSync(httptest.NewRequest(http.MethodGet, "/", nil).Context(), payload); err != nil {
		t.Fatalf("RecordSync failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ci/trigger", bytes.NewReader([]byte(`{"commit_sha":"abc999","profile":"unit"}`)))
	w := httptest.NewRecorder()
	srv.handleCITrigger(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestCheckpointEndpoint(t *testing.T) {
	srv, store := newTestServer(t)
	defer store.Close()

	payload := storage.SyncPayload{
		WorkspaceID: "bob/laptop",
		Repo:        "demo",
		Branch:      "main",
		CommitSHA:   "abc123",
		ChangeID:    "I9999999999999999999999999999999999999999",
		Message:     "feat: checkpoint",
		Author:      "bob",
		CommittedAt: time.Now().UTC(),
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/bob/laptop/checkpoint", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleWorkspaceRoutes(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var res storage.SyncResult
	if err := json.NewDecoder(w.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if res.Revision.CommitSHA != payload.CommitSHA {
		t.Fatalf("expected commit %s, got %s", payload.CommitSHA, res.Revision.CommitSHA)
	}
}

func TestDeleteWorkspaceEndpoint(t *testing.T) {
	srv, store := newTestServer(t)
	defer store.Close()

	payload := storage.SyncPayload{
		WorkspaceID: "bob/laptop",
		Repo:        "demo",
		Branch:      "main",
		CommitSHA:   "abc123",
		ChangeID:    "I9999999999999999999999999999999999999999",
		Message:     "feat: sync",
		Author:      "bob",
		CommittedAt: time.Now().UTC(),
	}
	if _, err := store.RecordSync(context.Background(), payload); err != nil {
		t.Fatalf("RecordSync failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/workspaces/bob/laptop", nil)
	w := httptest.NewRecorder()
	srv.handleWorkspaceRoutes(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
}

func TestReposEndpointCreatesRepo(t *testing.T) {
	srv, store := newTestServer(t)
	defer store.Close()

	body := []byte(`{"name":"demo"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/repos", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleRepos(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}

	var info RepoInfo
	if err := json.NewDecoder(w.Body).Decode(&info); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if info.Name != "demo" {
		t.Fatalf("expected repo name demo, got %s", info.Name)
	}

	path := filepath.Join(srv.cfg.ReposDir, "demo.git")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected repo at %s: %v", path, err)
	}
}
