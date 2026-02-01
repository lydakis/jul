package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	AttestationID          string    `json:"attestation_id"`
	CommitSHA              string    `json:"commit_sha"`
	AttestationInheritFrom string    `json:"attestation_inherit_from,omitempty"`
	ChangeID               string    `json:"change_id"`
	Type                   string    `json:"type"`
	Status                 string    `json:"status"`
	CompileStatus          string    `json:"compile_status,omitempty"`
	TestStatus             string    `json:"test_status,omitempty"`
	CoverageLinePct        *float64  `json:"coverage_line_pct,omitempty"`
	CoverageBranchPct      *float64  `json:"coverage_branch_pct,omitempty"`
	StartedAt              time.Time `json:"started_at"`
	FinishedAt             time.Time `json:"finished_at"`
	SignalsJSON            string    `json:"signals_json"`
	CreatedAt              time.Time `json:"created_at,omitempty"`
}

type ReflogEntry struct {
	WorkspaceID string    `json:"workspace_id"`
	CommitSHA   string    `json:"commit_sha"`
	ChangeID    string    `json:"change_id"`
	CreatedAt   time.Time `json:"created_at"`
	Source      string    `json:"source"`
}

type QueryResult struct {
	CommitSHA         string    `json:"commit_sha"`
	ChangeID          string    `json:"change_id"`
	Author            string    `json:"author"`
	Message           string    `json:"message"`
	CreatedAt         time.Time `json:"created_at"`
	AttestationStatus string    `json:"attestation_status,omitempty"`
	TestStatus        string    `json:"test_status,omitempty"`
	CompileStatus     string    `json:"compile_status,omitempty"`
	CoverageLinePct   *float64  `json:"coverage_line_pct,omitempty"`
	CoverageBranchPct *float64  `json:"coverage_branch_pct,omitempty"`
}

type QueryFilters struct {
	Tests       string
	Compiles    *bool
	CoverageMin *float64
	CoverageMax *float64
	ChangeID    string
	Author      string
	Since       *time.Time
	Until       *time.Time
	Limit       int
}

type RepoInfo struct {
	Name     string `json:"name"`
	CloneURL string `json:"clone_url"`
}

type Suggestion struct {
	SuggestionID       string    `json:"suggestion_id"`
	ChangeID           string    `json:"change_id"`
	BaseCommitSHA      string    `json:"base_commit_sha"`
	SuggestedCommitSHA string    `json:"suggested_commit_sha"`
	CreatedBy          string    `json:"created_by"`
	Reason             string    `json:"reason"`
	Description        string    `json:"description"`
	Confidence         float64   `json:"confidence"`
	Status             string    `json:"status"`
	ResolutionMessage  string    `json:"resolution_message,omitempty"`
	DiffstatJSON       string    `json:"diffstat_json"`
	CreatedAt          time.Time `json:"created_at"`
	ResolvedAt         time.Time `json:"resolved_at,omitempty"`
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

func (c *Client) Checkpoint(payload SyncPayload) (SyncResult, error) {
	var result SyncResult
	path := "/api/v1/workspaces/" + payload.WorkspaceID + "/checkpoint"
	if err := c.doJSON(http.MethodPost, path, payload, &result); err != nil {
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

func (c *Client) ListWorkspaces() ([]Workspace, error) {
	var workspaces []Workspace
	if err := c.doJSON(http.MethodGet, "/api/v1/workspaces", nil, &workspaces); err != nil {
		return nil, err
	}
	return workspaces, nil
}

func (c *Client) DeleteWorkspace(id string) error {
	return c.doJSON(http.MethodDelete, "/api/v1/workspaces/"+id, nil, nil)
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

func (c *Client) CreateAttestation(att Attestation) (Attestation, error) {
	var created Attestation
	if err := c.doJSON(http.MethodPost, "/api/v1/attestations", att, &created); err != nil {
		return Attestation{}, err
	}
	return created, nil
}

func (c *Client) Promote(workspaceID, targetBranch, commitSHA string, force bool) error {
	payload := map[string]any{
		"target_branch": targetBranch,
		"commit_sha":    commitSHA,
		"force":         force,
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

func (c *Client) Query(filters QueryFilters) ([]QueryResult, error) {
	var results []QueryResult
	path := "/api/v1/query"
	query := make([]string, 0, 4)
	if filters.Tests != "" {
		query = append(query, "tests="+urlQueryEscape(filters.Tests))
	}
	if filters.Compiles != nil {
		value := "false"
		if *filters.Compiles {
			value = "true"
		}
		query = append(query, "compiles="+value)
	}
	if filters.CoverageMin != nil {
		query = append(query, fmt.Sprintf("coverage_min=%g", *filters.CoverageMin))
	}
	if filters.CoverageMax != nil {
		query = append(query, fmt.Sprintf("coverage_max=%g", *filters.CoverageMax))
	}
	if filters.ChangeID != "" {
		query = append(query, "change_id="+urlQueryEscape(filters.ChangeID))
	}
	if filters.Author != "" {
		query = append(query, "author="+urlQueryEscape(filters.Author))
	}
	if filters.Since != nil {
		query = append(query, "since="+urlQueryEscape(filters.Since.Format(time.RFC3339)))
	}
	if filters.Until != nil {
		query = append(query, "until="+urlQueryEscape(filters.Until.Format(time.RFC3339)))
	}
	if filters.Limit > 0 {
		query = append(query, fmt.Sprintf("limit=%d", filters.Limit))
	}
	if len(query) > 0 {
		path = path + "?" + strings.Join(query, "&")
	}

	if err := c.doJSON(http.MethodGet, path, nil, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (c *Client) CreateRepo(name string) (RepoInfo, error) {
	var repo RepoInfo
	payload := map[string]string{"name": name}
	if err := c.doJSON(http.MethodPost, "/api/v1/repos", payload, &repo); err != nil {
		return RepoInfo{}, err
	}
	return repo, nil
}

func (c *Client) ListSuggestions(changeID, status string, limit int) ([]Suggestion, error) {
	var out []Suggestion
	path := "/api/v1/suggestions"
	query := make([]string, 0, 3)
	if changeID != "" {
		query = append(query, "change_id="+urlQueryEscape(changeID))
	}
	if status != "" {
		query = append(query, "status="+urlQueryEscape(status))
	}
	if limit > 0 {
		query = append(query, fmt.Sprintf("limit=%d", limit))
	}
	if len(query) > 0 {
		path = path + "?" + strings.Join(query, "&")
	}
	if err := c.doJSON(http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) GetSuggestion(id string) (Suggestion, error) {
	var out Suggestion
	if err := c.doJSON(http.MethodGet, "/api/v1/suggestions/"+id, nil, &out); err != nil {
		return Suggestion{}, err
	}
	return out, nil
}

func (c *Client) CreateSuggestion(req SuggestionCreateRequest) (Suggestion, error) {
	var out Suggestion
	if err := c.doJSON(http.MethodPost, "/api/v1/suggestions", req, &out); err != nil {
		return Suggestion{}, err
	}
	return out, nil
}

func (c *Client) UpdateSuggestionStatus(id, action string) (Suggestion, error) {
	var out Suggestion
	path := fmt.Sprintf("/api/v1/suggestions/%s/%s", id, action)
	if err := c.doJSON(http.MethodPost, path, map[string]string{}, &out); err != nil {
		return Suggestion{}, err
	}
	return out, nil
}

type SuggestionCreateRequest struct {
	ChangeID           string          `json:"change_id"`
	BaseCommitSHA      string          `json:"base_commit_sha"`
	SuggestedCommitSHA string          `json:"suggested_commit_sha"`
	CreatedBy          string          `json:"created_by,omitempty"`
	Reason             string          `json:"reason,omitempty"`
	Description        string          `json:"description,omitempty"`
	Confidence         float64         `json:"confidence,omitempty"`
	Diffstat           json.RawMessage `json:"diffstat,omitempty"`
	Repo               string          `json:"repo,omitempty"`
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

func urlQueryEscape(value string) string {
	return url.QueryEscape(value)
}
