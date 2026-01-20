package integration

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestSmokeLegacyAttestationQuery(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "legacy.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE revisions (
		commit_sha TEXT PRIMARY KEY,
		change_id TEXT NOT NULL,
		rev_index INTEGER NOT NULL,
		author TEXT NOT NULL,
		message TEXT NOT NULL,
		created_at TEXT NOT NULL
	);`)
	if err != nil {
		t.Fatalf("failed to create revisions table: %v", err)
	}

	_, err = db.Exec(`CREATE TABLE attestations (
		attestation_id TEXT PRIMARY KEY,
		commit_sha TEXT NOT NULL,
		change_id TEXT NOT NULL,
		type TEXT NOT NULL,
		status TEXT NOT NULL,
		started_at TEXT NOT NULL,
		finished_at TEXT NOT NULL,
		signals_json TEXT NOT NULL,
		created_at TEXT NOT NULL
	);`)
	if err != nil {
		t.Fatalf("failed to create attestations table: %v", err)
	}

	commitSHA := "legacy-commit"
	changeID := "I9999999999999999999999999999999999999999"
	now := time.Now().UTC().Format(time.RFC3339)

	_, err = db.Exec(`INSERT INTO revisions (commit_sha, change_id, rev_index, author, message, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`, commitSHA, changeID, 1, "alice", "legacy", now)
	if err != nil {
		t.Fatalf("failed to insert revision: %v", err)
	}

	_, err = db.Exec(`INSERT INTO attestations (attestation_id, commit_sha, change_id, type, status, started_at, finished_at, signals_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, "01HLEGACYATTESTATION", commitSHA, changeID, "ci", "pass", now, now, "{}", now)
	if err != nil {
		t.Fatalf("failed to insert attestation: %v", err)
	}

	baseURL, cleanup := startServerWithDB(t, dbPath, "")
	defer cleanup()

	queryReq, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/v1/query?tests=pass&compiles=true&limit=5", baseURL), nil)
	if err != nil {
		t.Fatalf("failed to build query request: %v", err)
	}
	queryResp, err := http.DefaultClient.Do(queryReq)
	if err != nil {
		t.Fatalf("query request failed: %v", err)
	}
	defer queryResp.Body.Close()
	if queryResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", queryResp.StatusCode)
	}
	var queryResults []struct {
		CommitSHA string `json:"commit_sha"`
	}
	if err := json.NewDecoder(queryResp.Body).Decode(&queryResults); err != nil {
		t.Fatalf("failed to decode query response: %v", err)
	}
	found := false
	for _, res := range queryResults {
		if res.CommitSHA == commitSHA {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected query to include %s", commitSHA)
	}

	attReq, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/v1/commits/%s/attestation", baseURL, commitSHA), nil)
	if err != nil {
		t.Fatalf("failed to build attestation request: %v", err)
	}
	attResp, err := http.DefaultClient.Do(attReq)
	if err != nil {
		t.Fatalf("attestation request failed: %v", err)
	}
	defer attResp.Body.Close()
	if attResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", attResp.StatusCode)
	}
	var att struct {
		Status        string `json:"status"`
		CompileStatus string `json:"compile_status"`
		TestStatus    string `json:"test_status"`
	}
	if err := json.NewDecoder(attResp.Body).Decode(&att); err != nil {
		t.Fatalf("failed to decode attestation response: %v", err)
	}
	if att.Status != "pass" {
		t.Fatalf("expected status pass, got %s", att.Status)
	}
	if att.CompileStatus != "pass" || att.TestStatus != "pass" {
		t.Fatalf("expected compile/test status pass, got compile=%s test=%s", att.CompileStatus, att.TestStatus)
	}
}
