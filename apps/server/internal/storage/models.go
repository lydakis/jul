package storage

import "time"

type Workspace struct {
	WorkspaceID   string    `json:"workspace_id"`
	User          string    `json:"user"`
	Name          string    `json:"name"`
	Repo          string    `json:"repo"`
	Branch        string    `json:"branch"`
	LastCommitSHA string    `json:"last_commit_sha"`
	LastChangeID  string    `json:"last_change_id"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type Change struct {
	ChangeID        string    `json:"change_id"`
	Title           string    `json:"title"`
	Author          string    `json:"author"`
	CreatedAt       time.Time `json:"created_at"`
	Status          string    `json:"status"`
	LatestRevision  Revision  `json:"latest_revision"`
	LatestRevIndex  int       `json:"-"`
	LatestCommitSHA string    `json:"-"`
	RevisionCount   int       `json:"revision_count"`
}

type Revision struct {
	ChangeID  string    `json:"change_id"`
	RevIndex  int       `json:"rev_index"`
	CommitSHA string    `json:"commit_sha"`
	Author    string    `json:"author"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
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
	CreatedAt     time.Time `json:"created_at"`
}

type KeepRef struct {
	KeepID      string    `json:"keep_id"`
	WorkspaceID string    `json:"workspace_id"`
	CommitSHA   string    `json:"commit_sha"`
	ChangeID    string    `json:"change_id"`
	CreatedAt   time.Time `json:"created_at"`
}

type QueryFilters struct {
	Tests    string
	ChangeID string
	Author   string
	Limit    int
}

type QueryResult struct {
	CommitSHA         string    `json:"commit_sha"`
	ChangeID          string    `json:"change_id"`
	Author            string    `json:"author"`
	Message           string    `json:"message"`
	CreatedAt         time.Time `json:"created_at"`
	AttestationStatus string    `json:"attestation_status,omitempty"`
}

type Event struct {
	EventID   string    `json:"event_id"`
	Type      string    `json:"type"`
	DataJSON  string    `json:"data_json"`
	CreatedAt time.Time `json:"created_at"`
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
	Revision  Revision  `json:"revision"`
}
