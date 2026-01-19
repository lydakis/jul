package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	http    *http.Client
}

type HTTPError struct {
	Status int
	Body   string
}

func (e *HTTPError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("request failed: status %d", e.Status)
	}
	return fmt.Sprintf("request failed: status %d: %s", e.Status, e.Body)
}

func New(baseURL string) *Client {
	trimmed := strings.TrimRight(baseURL, "/")
	return &Client{
		baseURL: trimmed,
		http: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

type Workspace struct {
	WorkspaceID   string    `json:"workspace_id"`
	Repo          string    `json:"repo"`
	Branch        string    `json:"branch"`
	LastCommitSHA string    `json:"last_commit_sha"`
	LastChangeID  string    `json:"last_change_id"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type Change struct {
	ChangeID       string    `json:"change_id"`
	Title          string    `json:"title"`
	Author         string    `json:"author"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	LatestRevision Revision  `json:"latest_revision"`
	RevisionCount  int       `json:"revision_count"`
}

type Revision struct {
	RevIndex  int    `json:"rev_index"`
	CommitSHA string `json:"commit_sha"`
}

type Attestation struct {
	AttestationID string    `json:"attestation_id"`
	CommitSHA     string    `json:"commit_sha"`
	ChangeID      string    `json:"change_id"`
	Type          string    `json:"type"`
	Status        string    `json:"status"`
	StartedAt     time.Time `json:"started_at"`
	FinishedAt    time.Time `json:"finished_at"`
	SignalsJSON   string    `json:"signals_json"`
}

type ReflogEntry struct {
	WorkspaceID string    `json:"workspace_id"`
	CommitSHA   string    `json:"commit_sha"`
	ChangeID    string    `json:"change_id"`
	CreatedAt   time.Time `json:"created_at"`
	Source      string    `json:"source"`
}

type SyncPayload struct {
	WorkspaceID string    `json:"workspace_id"`
	Repo        string    `json:"repo"`
	Branch      string    `json:"branch"`
	CommitSHA   string    `json:"commit_sha"`
	ChangeID    string    `json:"change_id"`
	Message     string    `json:"message"`
	Author      string    `json:"author"`
	CommittedAt time.Time `json:"committed_at"`
}

type SyncResult struct {
	Workspace Workspace `json:"workspace"`
	Change    Change    `json:"change"`
	Revision  struct {
		CommitSHA string `json:"commit_sha"`
		RevIndex  int    `json:"rev_index"`
	} `json:"revision"`
}

func (c *Client) Sync(payload SyncPayload) (SyncResult, error) {
	var result SyncResult
	if err := c.doJSON(http.MethodPost, "/api/v1/sync", payload, &result); err != nil {
		return SyncResult{}, err
	}
	return result, nil
}

func (c *Client) GetWorkspace(id string) (Workspace, error) {
	var ws Workspace
	if err := c.doJSON(http.MethodGet, "/api/v1/workspaces/"+id, nil, &ws); err != nil {
		return Workspace{}, err
	}
	return ws, nil
}

func (c *Client) ListChanges() ([]Change, error) {
	var changes []Change
	if err := c.doJSON(http.MethodGet, "/api/v1/changes", nil, &changes); err != nil {
		return nil, err
	}
	return changes, nil
}

func (c *Client) GetAttestation(commitSHA string) (*Attestation, error) {
	var att Attestation
	if err := c.doJSON(http.MethodGet, "/api/v1/commits/"+commitSHA+"/attestation", nil, &att); err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return &att, nil
}

func (c *Client) Promote(workspaceID, targetBranch, commitSHA string) error {
	payload := map[string]string{
		"target_branch": targetBranch,
		"commit_sha":    commitSHA,
	}
	return c.doJSON(http.MethodPost, "/api/v1/workspaces/"+workspaceID+"/promote", payload, nil)
}

func (c *Client) Reflog(workspaceID string, limit int) ([]ReflogEntry, error) {
	var entries []ReflogEntry
	path := "/api/v1/workspaces/" + workspaceID + "/reflog"
	if limit > 0 {
		path = fmt.Sprintf("%s?limit=%d", path, limit)
	}
	if err := c.doJSON(http.MethodGet, path, nil, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func (c *Client) doJSON(method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		buf := &bytes.Buffer{}
		enc := json.NewEncoder(buf)
		if err := enc.Encode(body); err != nil {
			return err
		}
		reader = buf
	}

	req, err := http.NewRequest(method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		payload, _ := io.ReadAll(resp.Body)
		return &HTTPError{Status: resp.StatusCode, Body: strings.TrimSpace(string(payload))}
	}

	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	if httpErr, ok := err.(*HTTPError); ok {
		return httpErr.Status == http.StatusNotFound
	}
	return strings.Contains(err.Error(), "not found")
}
