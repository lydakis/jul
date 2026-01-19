package storage

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("not found")

const timeFormat = time.RFC3339

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	if err := runMigrations(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) RecordSync(ctx context.Context, payload SyncPayload) (SyncResult, error) {
	if payload.WorkspaceID == "" || payload.CommitSHA == "" {
		return SyncResult{}, fmt.Errorf("workspace_id and commit_sha are required")
	}

	user, name := splitWorkspace(payload.WorkspaceID)
	if user == "" || name == "" {
		return SyncResult{}, fmt.Errorf("workspace_id must be in the form user/name")
	}

	commitMessage := strings.TrimSpace(payload.Message)
	if commitMessage == "" {
		commitMessage = "(no message)"
	}

	title := firstLine(commitMessage)
	if title == "" {
		title = "(no title)"
	}

	author := strings.TrimSpace(payload.Author)
	if author == "" {
		author = "unknown"
	}

	committedAt := payload.CommittedAt.UTC()
	if committedAt.IsZero() {
		committedAt = time.Now().UTC()
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SyncResult{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var (
		existingChangeID string
		existingRevIndex int
		commitExists     bool
	)
	if err := tx.QueryRowContext(ctx, `SELECT change_id, rev_index FROM revisions WHERE commit_sha = ?`, payload.CommitSHA).
		Scan(&existingChangeID, &existingRevIndex); err == nil {
		commitExists = true
	} else if !errors.Is(err, sql.ErrNoRows) {
		return SyncResult{}, err
	}

	if payload.ChangeID == "" {
		if commitExists {
			payload.ChangeID = existingChangeID
		} else {
			payload.ChangeID = generateChangeID(payload.CommitSHA)
		}
	} else if commitExists && payload.ChangeID != existingChangeID {
		payload.ChangeID = existingChangeID
	}

	var changeExists bool
	row := tx.QueryRowContext(ctx, `SELECT change_id FROM changes WHERE change_id = ?`, payload.ChangeID)
	var existingID string
	if err := row.Scan(&existingID); err == nil {
		changeExists = true
	} else if !errors.Is(err, sql.ErrNoRows) {
		return SyncResult{}, err
	}

	if !changeExists {
		_, err = tx.ExecContext(ctx, `INSERT INTO changes (change_id, title, author, status, created_at, latest_rev_index, latest_commit_sha)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			payload.ChangeID, title, author, "draft", committedAt.Format(timeFormat), 0, "")
		if err != nil {
			return SyncResult{}, err
		}
	}

	var (
		revIndex    int
		newRevision bool
	)
	if commitExists {
		revIndex = existingRevIndex
	} else {
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(rev_index), 0) FROM revisions WHERE change_id = ?`, payload.ChangeID).Scan(&revIndex); err != nil {
			return SyncResult{}, err
		}
		revIndex++

		_, err = tx.ExecContext(ctx, `INSERT INTO revisions (commit_sha, change_id, rev_index, author, message, created_at, repo)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			payload.CommitSHA, payload.ChangeID, revIndex, author, commitMessage, committedAt.Format(timeFormat), payload.Repo)
		if err != nil {
			return SyncResult{}, err
		}
		newRevision = true
	}

	var (
		currentTitle     string
		currentAuthor    string
		currentStatus    string
		currentCreatedAt string
		currentLatestRev int
		currentLatestSHA string
	)
	if err := tx.QueryRowContext(ctx, `SELECT title, author, status, created_at, latest_rev_index, latest_commit_sha FROM changes WHERE change_id = ?`, payload.ChangeID).
		Scan(&currentTitle, &currentAuthor, &currentStatus, &currentCreatedAt, &currentLatestRev, &currentLatestSHA); err != nil {
		return SyncResult{}, err
	}

	shouldUpdateLatest := revIndex >= currentLatestRev
	if newRevision && !shouldUpdateLatest {
		shouldUpdateLatest = true
	}

	if shouldUpdateLatest {
		_, err = tx.ExecContext(ctx, `UPDATE changes SET title = ?, author = ?, latest_rev_index = ?, latest_commit_sha = ? WHERE change_id = ?`,
			title, author, revIndex, payload.CommitSHA, payload.ChangeID)
		if err != nil {
			return SyncResult{}, err
		}
		currentTitle = title
		currentAuthor = author
		currentLatestRev = revIndex
		currentLatestSHA = payload.CommitSHA
	}

	var prevCommit string
	var prevChange string
	if err := tx.QueryRowContext(ctx, `SELECT last_commit_sha, last_change_id FROM workspaces WHERE workspace_id = ?`, payload.WorkspaceID).
		Scan(&prevCommit, &prevChange); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return SyncResult{}, err
	}

	if prevCommit != "" && prevCommit != payload.CommitSHA {
		if prevChange == "" {
			if err := tx.QueryRowContext(ctx, `SELECT change_id FROM revisions WHERE commit_sha = ?`, prevCommit).Scan(&prevChange); err != nil && !errors.Is(err, sql.ErrNoRows) {
				return SyncResult{}, err
			}
		}
		if prevChange == "" {
			prevChange = payload.ChangeID
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO keep_refs (keep_id, workspace_id, commit_sha, change_id, created_at)
			VALUES (?, ?, ?, ?, ?)`,
			ulid.Make().String(), payload.WorkspaceID, prevCommit, prevChange, time.Now().UTC().Format(timeFormat))
		if err != nil {
			return SyncResult{}, err
		}
	}

	updatedAt := time.Now().UTC().Format(timeFormat)
	_, err = tx.ExecContext(ctx, `INSERT INTO workspaces (workspace_id, user, name, repo, branch, last_commit_sha, last_change_id, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(workspace_id) DO UPDATE SET
		user = excluded.user,
		name = excluded.name,
		repo = excluded.repo,
		branch = excluded.branch,
		last_commit_sha = excluded.last_commit_sha,
		last_change_id = excluded.last_change_id,
		updated_at = excluded.updated_at`,
		payload.WorkspaceID, user, name, payload.Repo, payload.Branch, payload.CommitSHA, payload.ChangeID, updatedAt)
	if err != nil {
		return SyncResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return SyncResult{}, err
	}

	workspace := Workspace{
		WorkspaceID:   payload.WorkspaceID,
		User:          user,
		Name:          name,
		Repo:          payload.Repo,
		Branch:        payload.Branch,
		LastCommitSHA: payload.CommitSHA,
		LastChangeID:  payload.ChangeID,
		UpdatedAt:     time.Now().UTC(),
	}

	change := Change{
		ChangeID:        payload.ChangeID,
		Title:           currentTitle,
		Author:          currentAuthor,
		CreatedAt:       parseTime(currentCreatedAt),
		Status:          currentStatus,
		LatestRevIndex:  currentLatestRev,
		LatestCommitSHA: currentLatestSHA,
		LatestRevision: Revision{
			RevIndex:  currentLatestRev,
			CommitSHA: currentLatestSHA,
		},
	}

	revision := Revision{
		ChangeID:  payload.ChangeID,
		RevIndex:  revIndex,
		CommitSHA: payload.CommitSHA,
		Author:    author,
		Message:   commitMessage,
		CreatedAt: committedAt,
	}

	return SyncResult{Workspace: workspace, Change: change, Revision: revision}, nil
}

func (s *Store) ListWorkspaces(ctx context.Context) ([]Workspace, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT workspace_id, user, name, repo, branch, last_commit_sha, last_change_id, updated_at FROM workspaces ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Workspace
	for rows.Next() {
		var ws Workspace
		var updatedAt string
		if err := rows.Scan(&ws.WorkspaceID, &ws.User, &ws.Name, &ws.Repo, &ws.Branch, &ws.LastCommitSHA, &ws.LastChangeID, &updatedAt); err != nil {
			return nil, err
		}
		ws.UpdatedAt = parseTime(updatedAt)
		out = append(out, ws)
	}
	return out, rows.Err()
}

func (s *Store) GetWorkspace(ctx context.Context, id string) (Workspace, error) {
	row := s.db.QueryRowContext(ctx, `SELECT workspace_id, user, name, repo, branch, last_commit_sha, last_change_id, updated_at FROM workspaces WHERE workspace_id = ?`, id)
	var ws Workspace
	var updatedAt string
	if err := row.Scan(&ws.WorkspaceID, &ws.User, &ws.Name, &ws.Repo, &ws.Branch, &ws.LastCommitSHA, &ws.LastChangeID, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Workspace{}, ErrNotFound
		}
		return Workspace{}, err
	}
	ws.UpdatedAt = parseTime(updatedAt)
	return ws, nil
}

func (s *Store) ListChanges(ctx context.Context) ([]Change, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT change_id, title, author, status, created_at, latest_rev_index, latest_commit_sha,
		(SELECT COUNT(1) FROM revisions r WHERE r.change_id = changes.change_id) as revision_count
		FROM changes ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Change
	for rows.Next() {
		var ch Change
		var createdAt string
		if err := rows.Scan(&ch.ChangeID, &ch.Title, &ch.Author, &ch.Status, &createdAt, &ch.LatestRevIndex, &ch.LatestCommitSHA, &ch.RevisionCount); err != nil {
			return nil, err
		}
		ch.CreatedAt = parseTime(createdAt)
		ch.LatestRevision = Revision{RevIndex: ch.LatestRevIndex, CommitSHA: ch.LatestCommitSHA}
		out = append(out, ch)
	}
	return out, rows.Err()
}

func (s *Store) GetChange(ctx context.Context, changeID string) (Change, error) {
	row := s.db.QueryRowContext(ctx, `SELECT change_id, title, author, status, created_at, latest_rev_index, latest_commit_sha FROM changes WHERE change_id = ?`, changeID)
	var ch Change
	var createdAt string
	if err := row.Scan(&ch.ChangeID, &ch.Title, &ch.Author, &ch.Status, &createdAt, &ch.LatestRevIndex, &ch.LatestCommitSHA); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Change{}, ErrNotFound
		}
		return Change{}, err
	}
	ch.CreatedAt = parseTime(createdAt)
	ch.LatestRevision = Revision{RevIndex: ch.LatestRevIndex, CommitSHA: ch.LatestCommitSHA}
	return ch, nil
}

func (s *Store) ListRevisions(ctx context.Context, changeID string) ([]Revision, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT change_id, rev_index, commit_sha, author, message, created_at FROM revisions WHERE change_id = ? ORDER BY rev_index ASC`, changeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Revision
	for rows.Next() {
		var rev Revision
		var createdAt string
		if err := rows.Scan(&rev.ChangeID, &rev.RevIndex, &rev.CommitSHA, &rev.Author, &rev.Message, &createdAt); err != nil {
			return nil, err
		}
		rev.CreatedAt = parseTime(createdAt)
		out = append(out, rev)
	}
	return out, rows.Err()
}

func (s *Store) GetRevisionByCommit(ctx context.Context, commitSHA string) (Revision, error) {
	row := s.db.QueryRowContext(ctx, `SELECT change_id, rev_index, commit_sha, author, message, created_at FROM revisions WHERE commit_sha = ?`, commitSHA)
	var rev Revision
	var createdAt string
	if err := row.Scan(&rev.ChangeID, &rev.RevIndex, &rev.CommitSHA, &rev.Author, &rev.Message, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Revision{}, ErrNotFound
		}
		return Revision{}, err
	}
	rev.CreatedAt = parseTime(createdAt)
	return rev, nil
}

func (s *Store) ListAttestations(ctx context.Context, commitSHA, changeID, status string) ([]Attestation, error) {
	query := `SELECT attestation_id, commit_sha, change_id, type, status, compile_status, test_status, coverage_line_pct, coverage_branch_pct, started_at, finished_at, signals_json, created_at FROM attestations WHERE 1=1`
	args := []any{}
	if commitSHA != "" {
		query += " AND commit_sha = ?"
		args = append(args, commitSHA)
	}
	if changeID != "" {
		query += " AND change_id = ?"
		args = append(args, changeID)
	}
	if status != "" {
		query += " AND status = ?"
		args = append(args, status)
	}
	query += " ORDER BY created_at DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Attestation
	for rows.Next() {
		var att Attestation
		var startedAt, finishedAt, createdAt string
		var coverageLine sql.NullFloat64
		var coverageBranch sql.NullFloat64
		if err := rows.Scan(
			&att.AttestationID,
			&att.CommitSHA,
			&att.ChangeID,
			&att.Type,
			&att.Status,
			&att.CompileStatus,
			&att.TestStatus,
			&coverageLine,
			&coverageBranch,
			&startedAt,
			&finishedAt,
			&att.SignalsJSON,
			&createdAt,
		); err != nil {
			return nil, err
		}
		if coverageLine.Valid {
			att.CoverageLinePct = &coverageLine.Float64
		}
		if coverageBranch.Valid {
			att.CoverageBranchPct = &coverageBranch.Float64
		}
		att.StartedAt = parseTime(startedAt)
		att.FinishedAt = parseTime(finishedAt)
		att.CreatedAt = parseTime(createdAt)
		out = append(out, att)
	}
	return out, rows.Err()
}

func (s *Store) CreateAttestation(ctx context.Context, att Attestation) (Attestation, error) {
	if att.AttestationID == "" {
		att.AttestationID = ulid.Make().String()
	}
	if att.CreatedAt.IsZero() {
		att.CreatedAt = time.Now().UTC()
	}
	if att.StartedAt.IsZero() {
		att.StartedAt = att.CreatedAt
	}
	if att.FinishedAt.IsZero() {
		att.FinishedAt = att.CreatedAt
	}
	if att.TestStatus == "" {
		att.TestStatus = att.Status
	}
	if att.CompileStatus == "" {
		att.CompileStatus = att.Status
	}

	var coverageLine any
	var coverageBranch any
	if att.CoverageLinePct != nil {
		coverageLine = *att.CoverageLinePct
	}
	if att.CoverageBranchPct != nil {
		coverageBranch = *att.CoverageBranchPct
	}

	_, err := s.db.ExecContext(ctx, `INSERT INTO attestations (attestation_id, commit_sha, change_id, type, status, compile_status, test_status, coverage_line_pct, coverage_branch_pct, started_at, finished_at, signals_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		att.AttestationID, att.CommitSHA, att.ChangeID, att.Type, att.Status,
		att.CompileStatus, att.TestStatus, coverageLine, coverageBranch,
		att.StartedAt.Format(timeFormat), att.FinishedAt.Format(timeFormat), att.SignalsJSON, att.CreatedAt.Format(timeFormat))
	if err != nil {
		return Attestation{}, err
	}
	return att, nil
}

func (s *Store) GetLatestAttestation(ctx context.Context, commitSHA string) (Attestation, error) {
	row := s.db.QueryRowContext(ctx, `SELECT attestation_id, commit_sha, change_id, type, status, compile_status, test_status, coverage_line_pct, coverage_branch_pct, started_at, finished_at, signals_json, created_at
		FROM attestations WHERE commit_sha = ? ORDER BY created_at DESC LIMIT 1`, commitSHA)
	var att Attestation
	var startedAt, finishedAt, createdAt string
	var coverageLine sql.NullFloat64
	var coverageBranch sql.NullFloat64
	if err := row.Scan(
		&att.AttestationID,
		&att.CommitSHA,
		&att.ChangeID,
		&att.Type,
		&att.Status,
		&att.CompileStatus,
		&att.TestStatus,
		&coverageLine,
		&coverageBranch,
		&startedAt,
		&finishedAt,
		&att.SignalsJSON,
		&createdAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Attestation{}, ErrNotFound
		}
		return Attestation{}, err
	}
	if coverageLine.Valid {
		att.CoverageLinePct = &coverageLine.Float64
	}
	if coverageBranch.Valid {
		att.CoverageBranchPct = &coverageBranch.Float64
	}
	att.StartedAt = parseTime(startedAt)
	att.FinishedAt = parseTime(finishedAt)
	att.CreatedAt = parseTime(createdAt)
	return att, nil
}

func (s *Store) InsertEvent(ctx context.Context, evt Event) (Event, error) {
	if evt.EventID == "" {
		evt.EventID = ulid.Make().String()
	}
	if evt.CreatedAt.IsZero() {
		evt.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO events (event_id, type, data_json, created_at) VALUES (?, ?, ?, ?)`,
		evt.EventID, evt.Type, evt.DataJSON, evt.CreatedAt.Format(timeFormat))
	if err != nil {
		return Event{}, err
	}
	return evt, nil
}

func (s *Store) ListEventsSince(ctx context.Context, since time.Time, limit int) ([]Event, error) {
	query := `SELECT event_id, type, data_json, created_at FROM events WHERE created_at >= ? ORDER BY created_at ASC`
	args := []any{since.Format(timeFormat)}
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Event
	for rows.Next() {
		var evt Event
		var createdAt string
		if err := rows.Scan(&evt.EventID, &evt.Type, &evt.DataJSON, &createdAt); err != nil {
			return nil, err
		}
		evt.CreatedAt = parseTime(createdAt)
		out = append(out, evt)
	}
	return out, rows.Err()
}

func (s *Store) ListKeepRefs(ctx context.Context, workspaceID string, limit int) ([]KeepRef, error) {
	query := `SELECT keep_id, workspace_id, commit_sha, change_id, created_at FROM keep_refs WHERE workspace_id = ? ORDER BY created_at DESC`
	args := []any{workspaceID}
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []KeepRef
	for rows.Next() {
		var ref KeepRef
		var createdAt string
		if err := rows.Scan(&ref.KeepID, &ref.WorkspaceID, &ref.CommitSHA, &ref.ChangeID, &createdAt); err != nil {
			return nil, err
		}
		ref.CreatedAt = parseTime(createdAt)
		out = append(out, ref)
	}
	return out, rows.Err()
}

func (s *Store) QueryCommits(ctx context.Context, filters QueryFilters) ([]QueryResult, error) {
	query := `SELECT r.commit_sha, r.change_id, r.author, r.message, r.created_at,
		COALESCE((SELECT status FROM attestations a WHERE a.commit_sha = r.commit_sha ORDER BY a.created_at DESC LIMIT 1), '') AS att_status,
		(SELECT test_status FROM attestations a WHERE a.commit_sha = r.commit_sha ORDER BY a.created_at DESC LIMIT 1) AS test_status,
		(SELECT compile_status FROM attestations a WHERE a.commit_sha = r.commit_sha ORDER BY a.created_at DESC LIMIT 1) AS compile_status,
		(SELECT coverage_line_pct FROM attestations a WHERE a.commit_sha = r.commit_sha ORDER BY a.created_at DESC LIMIT 1) AS coverage_line_pct,
		(SELECT coverage_branch_pct FROM attestations a WHERE a.commit_sha = r.commit_sha ORDER BY a.created_at DESC LIMIT 1) AS coverage_branch_pct
		FROM revisions r WHERE 1=1`
	args := []any{}

	if filters.ChangeID != "" {
		query += " AND r.change_id = ?"
		args = append(args, filters.ChangeID)
	}
	if filters.Author != "" {
		query += " AND r.author LIKE ?"
		args = append(args, "%"+filters.Author+"%")
	}
	if filters.Tests != "" {
		query += " AND (SELECT test_status FROM attestations a WHERE a.commit_sha = r.commit_sha ORDER BY a.created_at DESC LIMIT 1) = ?"
		args = append(args, filters.Tests)
	}
	if filters.Compiles != nil {
		status := "fail"
		if *filters.Compiles {
			status = "pass"
		}
		query += " AND (SELECT compile_status FROM attestations a WHERE a.commit_sha = r.commit_sha ORDER BY a.created_at DESC LIMIT 1) = ?"
		args = append(args, status)
	}
	if filters.CoverageMin != nil {
		query += " AND (SELECT coverage_line_pct FROM attestations a WHERE a.commit_sha = r.commit_sha ORDER BY a.created_at DESC LIMIT 1) >= ?"
		args = append(args, *filters.CoverageMin)
	}
	if filters.CoverageMax != nil {
		query += " AND (SELECT coverage_line_pct FROM attestations a WHERE a.commit_sha = r.commit_sha ORDER BY a.created_at DESC LIMIT 1) <= ?"
		args = append(args, *filters.CoverageMax)
	}
	if filters.Since != nil {
		query += " AND r.created_at >= ?"
		args = append(args, filters.Since.Format(timeFormat))
	}
	if filters.Until != nil {
		query += " AND r.created_at <= ?"
		args = append(args, filters.Until.Format(timeFormat))
	}

	query += " ORDER BY r.created_at DESC"
	limit := filters.Limit
	if limit <= 0 {
		limit = 20
	}
	query += " LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []QueryResult
	for rows.Next() {
		var res QueryResult
		var createdAt string
		var attStatus sql.NullString
		var testStatus sql.NullString
		var compileStatus sql.NullString
		var coverageLine sql.NullFloat64
		var coverageBranch sql.NullFloat64
		if err := rows.Scan(
			&res.CommitSHA,
			&res.ChangeID,
			&res.Author,
			&res.Message,
			&createdAt,
			&attStatus,
			&testStatus,
			&compileStatus,
			&coverageLine,
			&coverageBranch,
		); err != nil {
			return nil, err
		}
		if attStatus.Valid {
			res.AttestationStatus = attStatus.String
		}
		if testStatus.Valid {
			res.TestStatus = testStatus.String
		}
		if compileStatus.Valid {
			res.CompileStatus = compileStatus.String
		}
		if coverageLine.Valid {
			res.CoverageLinePct = &coverageLine.Float64
		}
		if coverageBranch.Valid {
			res.CoverageBranchPct = &coverageBranch.Float64
		}
		res.CreatedAt = parseTime(createdAt)
		out = append(out, res)
	}
	return out, rows.Err()
}

func (s *Store) FindRepoForCommit(ctx context.Context, commitSHA string) (string, error) {
	row := s.db.QueryRowContext(ctx, `SELECT repo FROM revisions WHERE commit_sha = ? AND repo != '' LIMIT 1`, commitSHA)
	var repo string
	if err := row.Scan(&repo); err == nil {
		return repo, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}

	row = s.db.QueryRowContext(ctx, `SELECT repo FROM workspaces WHERE last_commit_sha = ? ORDER BY updated_at DESC LIMIT 1`, commitSHA)
	if err := row.Scan(&repo); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	return repo, nil
}

func parseTime(value string) time.Time {
	parsed, err := time.Parse(timeFormat, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func generateChangeID(seed string) string {
	h := sha1.Sum([]byte(seed))
	return "I" + hex.EncodeToString(h[:])
}

func splitWorkspace(id string) (string, string) {
	parts := strings.SplitN(id, "/", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
}

func firstLine(message string) string {
	lines := strings.Split(message, "\n")
	if len(lines) == 0 {
		return ""
	}
	return strings.TrimSpace(lines[0])
}
