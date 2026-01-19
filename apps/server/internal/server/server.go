package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lydakis/jul/server/internal/events"
	"github.com/lydakis/jul/server/internal/storage"
)

type Config struct {
	Address  string
	BaseURL  string
	ReposDir string
}

type Server struct {
	cfg    Config
	mux    *http.ServeMux
	store  *storage.Store
	broker *events.Broker
}

type Capabilities struct {
	Version       string   `json:"version"`
	Features      []string `json:"features"`
	RefNamespaces []string `json:"ref_namespaces"`
}

type ReflogEntry struct {
	WorkspaceID string    `json:"workspace_id"`
	CommitSHA   string    `json:"commit_sha"`
	ChangeID    string    `json:"change_id"`
	CreatedAt   time.Time `json:"created_at"`
	Source      string    `json:"source"`
}

var ErrNonFastForward = errors.New("non-fast-forward update")

func New(cfg Config, store *storage.Store, broker *events.Broker) *Server {
	if cfg.Address == "" {
		cfg.Address = ":8000"
	}

	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://localhost" + cfg.Address
	}
	if cfg.ReposDir == "" {
		cfg.ReposDir = "./repos"
	}

	s := &Server{
		cfg:    cfg,
		mux:    http.NewServeMux(),
		store:  store,
		broker: broker,
	}

	s.routes()
	return s
}

func (s *Server) Start() error {
	return http.ListenAndServe(s.cfg.Address, s.mux)
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("/healthz", s.handleHealthz)
	s.mux.HandleFunc("/api/v1/capabilities", s.handleCapabilities)
	s.mux.HandleFunc("/api/v1/sync", s.handleSync)
	s.mux.HandleFunc("/api/v1/workspaces", s.handleWorkspaces)
	s.mux.HandleFunc("/api/v1/workspaces/", s.handleWorkspaceRoutes)
	s.mux.HandleFunc("/api/v1/changes", s.handleChanges)
	s.mux.HandleFunc("/api/v1/changes/", s.handleChangeRoutes)
	s.mux.HandleFunc("/api/v1/commits/", s.handleCommitRoutes)
	s.mux.HandleFunc("/api/v1/attestations", s.handleAttestations)
	s.mux.HandleFunc("/api/v1/ci/trigger", s.handleCITrigger)
	s.mux.HandleFunc("/api/v1/query", s.handleQuery)
	s.mux.HandleFunc("/events/stream", s.handleEvents)
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) handleCapabilities(w http.ResponseWriter, _ *http.Request) {
	payload := Capabilities{
		Version:  "v1",
		Features: []string{"workspaces", "changes", "attestations", "suggestions", "sync"},
		RefNamespaces: []string{
			"refs/jul/workspaces",
			"refs/jul/keep",
			"refs/jul/suggest",
			"refs/notes/jul",
		},
	}

	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var payload storage.SyncPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	result, err := s.store.RecordSync(r.Context(), payload)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	data := map[string]any{
		"workspace_id": result.Workspace.WorkspaceID,
		"commit_sha":   result.Revision.CommitSHA,
		"change_id":    result.Change.ChangeID,
	}
	s.emitEvent(r.Context(), "ref.updated", data)

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleWorkspaces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	workspaces, err := s.store.ListWorkspaces(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, workspaces)
}

func (s *Server) handleWorkspaceRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/workspaces/")
	path = strings.Trim(path, "/")
	if path == "" {
		writeError(w, http.StatusBadRequest, "workspace id required")
		return
	}

	parts := strings.Split(path, "/")
	if len(parts) >= 3 && parts[len(parts)-1] == "promote" {
		id := strings.Join(parts[:len(parts)-1], "/")
		s.handlePromote(w, r, id)
		return
	}
	if len(parts) >= 3 && parts[len(parts)-1] == "reflog" {
		id := strings.Join(parts[:len(parts)-1], "/")
		s.handleReflog(w, r, id)
		return
	}

	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	workspace, err := s.store.GetWorkspace(r.Context(), path)
	if err != nil {
		if err == storage.ErrNotFound {
			writeError(w, http.StatusNotFound, "workspace not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, workspace)
}

func (s *Server) handlePromote(w http.ResponseWriter, r *http.Request, workspaceID string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		TargetBranch string `json:"target_branch"`
		CommitSHA    string `json:"commit_sha"`
		Force        bool   `json:"force"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.TargetBranch == "" {
		writeError(w, http.StatusBadRequest, "target_branch required")
		return
	}

	workspace, err := s.store.GetWorkspace(r.Context(), workspaceID)
	if err != nil {
		if err == storage.ErrNotFound {
			writeError(w, http.StatusNotFound, "workspace not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	commitSHA := body.CommitSHA
	if commitSHA == "" {
		commitSHA = workspace.LastCommitSHA
	}
	if commitSHA == "" {
		writeError(w, http.StatusBadRequest, "commit_sha required")
		return
	}

	if !body.Force {
		att, err := s.store.GetLatestAttestation(r.Context(), commitSHA)
		if err == nil && att.Status != "pass" {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":    "promotion blocked",
				"blockers": []string{"attestation.status != pass"},
			})
			return
		}
		if err != nil && err != storage.ErrNotFound {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	repoPath, err := s.repoPath(workspace.Repo)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := os.Stat(repoPath); err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "repo not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := updateRef(repoPath, body.TargetBranch, commitSHA, body.Force); err != nil {
		if errors.Is(err, ErrNonFastForward) {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":    "promotion blocked",
				"blockers": []string{"branch requires fast-forward"},
			})
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	data := map[string]any{
		"workspace_id":  workspaceID,
		"target_branch": body.TargetBranch,
		"commit_sha":    commitSHA,
	}
	s.emitEvent(r.Context(), "promote.applied", data)

	writeJSON(w, http.StatusOK, map[string]string{
		"status":       "promoted",
		"branch":       body.TargetBranch,
		"commit_sha":   commitSHA,
		"workspace_id": workspaceID,
	})
}

func (s *Server) handleReflog(w http.ResponseWriter, r *http.Request, workspaceID string) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	workspace, err := s.store.GetWorkspace(r.Context(), workspaceID)
	if err != nil {
		if err == storage.ErrNotFound {
			writeError(w, http.StatusNotFound, "workspace not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}

	keepRefs, err := s.store.ListKeepRefs(r.Context(), workspaceID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	entries := []ReflogEntry{
		{
			WorkspaceID: workspace.WorkspaceID,
			CommitSHA:   workspace.LastCommitSHA,
			ChangeID:    workspace.LastChangeID,
			CreatedAt:   workspace.UpdatedAt,
			Source:      "current",
		},
	}

	for _, ref := range keepRefs {
		entries = append(entries, ReflogEntry{
			WorkspaceID: ref.WorkspaceID,
			CommitSHA:   ref.CommitSHA,
			ChangeID:    ref.ChangeID,
			CreatedAt:   ref.CreatedAt,
			Source:      "keep",
		})
	}

	writeJSON(w, http.StatusOK, entries)
}

func (s *Server) handleChanges(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	changes, err := s.store.ListChanges(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, changes)
}

func (s *Server) repoPath(repo string) (string, error) {
	name := strings.TrimSpace(repo)
	if name == "" {
		return "", fmt.Errorf("workspace repo not set")
	}
	if strings.Contains(name, "..") || filepath.IsAbs(name) {
		return "", fmt.Errorf("invalid repo name")
	}
	if !strings.HasSuffix(name, ".git") {
		name += ".git"
	}
	return filepath.Join(s.cfg.ReposDir, name), nil
}

func updateRef(repoPath, branch, commitSHA string, force bool) error {
	if strings.TrimSpace(branch) == "" {
		return fmt.Errorf("branch required")
	}
	if strings.TrimSpace(commitSHA) == "" {
		return fmt.Errorf("commit sha required")
	}
	ref := fmt.Sprintf("refs/heads/%s", branch)

	current, exists, err := readRef(repoPath, ref)
	if err != nil {
		return err
	}

	if exists && !force {
		ok, err := isAncestor(repoPath, current, commitSHA)
		if err != nil {
			return err
		}
		if !ok {
			return ErrNonFastForward
		}
	}

	var cmd *exec.Cmd
	if exists && !force {
		cmd = exec.Command("git", "--git-dir", repoPath, "update-ref", ref, commitSHA, current)
	} else {
		cmd = exec.Command("git", "--git-dir", repoPath, "update-ref", ref, commitSHA)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git update-ref failed: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func ciProfileCommands(profile string) ([]string, bool) {
	switch strings.ToLower(profile) {
	case "unit":
		return []string{"go test ./..."}, true
	case "lint":
		return []string{"go vet ./..."}, true
	case "full":
		return []string{"go test ./...", "go vet ./..."}, true
	default:
		return nil, false
	}
}

func readRef(repoPath, ref string) (string, bool, error) {
	cmd := exec.Command("git", "--git-dir", repoPath, "rev-parse", "--verify", "--quiet", ref)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return "", false, nil
		}
		return "", false, fmt.Errorf("git rev-parse failed: %s", strings.TrimSpace(string(output)))
	}
	value := strings.TrimSpace(string(output))
	if value == "" {
		return "", false, nil
	}
	return value, true, nil
}

func isAncestor(repoPath, ancestor, descendant string) (bool, error) {
	cmd := exec.Command("git", "--git-dir", repoPath, "merge-base", "--is-ancestor", ancestor, descendant)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, fmt.Errorf("git merge-base failed: %s", strings.TrimSpace(string(output)))
	}
	return true, nil
}

func (s *Server) handleChangeRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/changes/")
	path = strings.Trim(path, "/")
	if path == "" {
		writeError(w, http.StatusBadRequest, "change id required")
		return
	}

	if strings.HasSuffix(path, "/revisions") {
		id := strings.TrimSuffix(path, "/revisions")
		s.handleRevisions(w, r, id)
		return
	}

	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	change, err := s.store.GetChange(r.Context(), path)
	if err != nil {
		if err == storage.ErrNotFound {
			writeError(w, http.StatusNotFound, "change not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, change)
}

func (s *Server) handleRevisions(w http.ResponseWriter, r *http.Request, changeID string) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	revisions, err := s.store.ListRevisions(r.Context(), changeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, revisions)
}

func (s *Server) handleCommitRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/commits/")
	path = strings.Trim(path, "/")
	if path == "" {
		writeError(w, http.StatusBadRequest, "commit sha required")
		return
	}

	if strings.HasSuffix(path, "/attestation") {
		sha := strings.TrimSuffix(path, "/attestation")
		s.handleCommitAttestation(w, r, sha)
		return
	}

	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	commit, err := s.lookupCommit(r.Context(), path)
	if err != nil {
		if err == storage.ErrNotFound {
			writeError(w, http.StatusNotFound, "commit not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, commit)
}

func (s *Server) lookupCommit(ctx context.Context, sha string) (map[string]any, error) {
	rev, err := s.store.GetRevisionByCommit(ctx, sha)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"commit_sha": rev.CommitSHA,
		"change_id":  rev.ChangeID,
		"author":     rev.Author,
		"message":    rev.Message,
		"created_at": rev.CreatedAt.Format(time.RFC3339),
	}, nil
}

func (s *Server) handleCommitAttestation(w http.ResponseWriter, r *http.Request, sha string) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	att, err := s.store.GetLatestAttestation(r.Context(), sha)
	if err != nil {
		if err == storage.ErrNotFound {
			writeError(w, http.StatusNotFound, "attestation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, att)
}

func (s *Server) handleAttestations(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		commitSHA := r.URL.Query().Get("commit_sha")
		changeID := r.URL.Query().Get("change_id")
		status := r.URL.Query().Get("status")
		atts, err := s.store.ListAttestations(r.Context(), commitSHA, changeID, status)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, atts)
	case http.MethodPost:
		var att storage.Attestation
		if err := json.NewDecoder(r.Body).Decode(&att); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		created, err := s.store.CreateAttestation(r.Context(), att)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.emitEvent(r.Context(), "ci.finished", map[string]any{"commit_sha": created.CommitSHA, "status": created.Status})
		writeJSON(w, http.StatusCreated, created)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleCITrigger(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		CommitSHA string `json:"commit_sha"`
		Profile   string `json:"profile"`
		Repo      string `json:"repo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.CommitSHA == "" {
		writeError(w, http.StatusBadRequest, "commit_sha required")
		return
	}

	profile := strings.TrimSpace(body.Profile)
	if profile == "" {
		profile = "unit"
	}
	commands, ok := ciProfileCommands(profile)
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown profile")
		return
	}

	repoName := strings.TrimSpace(body.Repo)
	if repoName == "" {
		repo, err := s.store.FindRepoForCommit(r.Context(), body.CommitSHA)
		if err != nil {
			if err == storage.ErrNotFound {
				writeError(w, http.StatusNotFound, "repo not found for commit")
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		repoName = repo
	}

	repoPath, err := s.repoPath(repoName)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := os.Stat(repoPath); err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "repo not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	rev, err := s.store.GetRevisionByCommit(r.Context(), body.CommitSHA)
	if err != nil {
		if err == storage.ErrNotFound {
			writeError(w, http.StatusNotFound, "commit not tracked")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.emitEvent(r.Context(), "ci.started", map[string]any{
		"commit_sha": body.CommitSHA,
		"profile":    profile,
	})

	result, err := runCI(repoPath, body.CommitSHA, commands)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	signals, err := json.Marshal(result)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to encode signals")
		return
	}

	att, err := s.store.CreateAttestation(r.Context(), storage.Attestation{
		CommitSHA:   body.CommitSHA,
		ChangeID:    rev.ChangeID,
		Type:        "ci",
		Status:      result.Status,
		StartedAt:   result.StartedAt,
		FinishedAt:  result.FinishedAt,
		SignalsJSON: string(signals),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.emitEvent(r.Context(), "ci.finished", map[string]any{
		"commit_sha": body.CommitSHA,
		"status":     att.Status,
	})

	writeJSON(w, http.StatusCreated, att)
}

func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	tests := strings.TrimSpace(r.URL.Query().Get("tests"))
	if tests != "" && tests != "pass" && tests != "fail" {
		writeError(w, http.StatusBadRequest, "tests must be pass or fail")
		return
	}

	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}

	filters := storage.QueryFilters{
		Tests:    tests,
		ChangeID: strings.TrimSpace(r.URL.Query().Get("change_id")),
		Author:   strings.TrimSpace(r.URL.Query().Get("author")),
		Limit:    limit,
	}

	results, err := s.store.QueryCommits(r.Context(), filters)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, results)
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("streaming unsupported"))
		return
	}

	ctx := r.Context()
	_, _ = fmt.Fprintf(w, "event: ready\ndata: %s\n\n", time.Now().UTC().Format(time.RFC3339))
	flusher.Flush()

	if sinceParam := r.URL.Query().Get("since"); sinceParam != "" {
		if since, err := time.Parse(time.RFC3339, sinceParam); err == nil {
			events, err := s.store.ListEventsSince(ctx, since, 1000)
			if err == nil {
				for _, evt := range events {
					writeSSE(w, evt)
					flusher.Flush()
				}
			}
		}
	}

	ch, cancel := s.broker.Subscribe()
	defer cancel()

	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case evt := <-ch:
			writeSSEFromBroker(w, evt)
			flusher.Flush()
		case <-keepalive.C:
			_, _ = fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

func (s *Server) emitEvent(ctx context.Context, eventType string, payload map[string]any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	stored, err := s.store.InsertEvent(ctx, storage.Event{Type: eventType, DataJSON: string(data)})
	if err != nil {
		return
	}
	s.broker.Publish(events.Event{
		ID:        stored.EventID,
		Type:      stored.Type,
		DataJSON:  []byte(stored.DataJSON),
		CreatedAt: stored.CreatedAt.Format(time.RFC3339),
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeSSE(w http.ResponseWriter, evt storage.Event) {
	_, _ = fmt.Fprintf(w, "id: %s\n", evt.EventID)
	_, _ = fmt.Fprintf(w, "event: %s\n", evt.Type)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", evt.DataJSON)
}

func writeSSEFromBroker(w http.ResponseWriter, evt events.Event) {
	_, _ = fmt.Fprintf(w, "id: %s\n", evt.ID)
	_, _ = fmt.Fprintf(w, "event: %s\n", evt.Type)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", evt.DataJSON)
}
