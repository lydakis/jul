package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
)

func (s *Store) CreateSuggestion(ctx context.Context, sug Suggestion) (Suggestion, error) {
	if sug.ChangeID == "" || sug.BaseCommitSHA == "" || sug.SuggestedCommitSHA == "" {
		return Suggestion{}, fmt.Errorf("change_id, base_commit_sha, and suggested_commit_sha are required")
	}
	if sug.SuggestionID == "" {
		sug.SuggestionID = ulid.Make().String()
	}
	if sug.CreatedAt.IsZero() {
		sug.CreatedAt = time.Now().UTC()
	}
	if sug.CreatedBy == "" {
		sug.CreatedBy = "client"
	}
	if sug.Status == "" {
		sug.Status = "pending"
	}
	sug.Status = normalizeSuggestionStatus(sug.Status)
	if sug.Reason == "" {
		sug.Reason = "unspecified"
	}
	if sug.Description == "" {
		sug.Description = ""
	}
	if sug.DiffstatJSON == "" {
		sug.DiffstatJSON = "{}"
	}

	var resolvedAt any
	if !sug.ResolvedAt.IsZero() {
		resolvedAt = sug.ResolvedAt.Format(timeFormat)
	}

	_, err := s.db.ExecContext(ctx, `INSERT INTO suggestions (suggestion_id, change_id, base_commit_sha, suggested_commit_sha, created_by, reason, description, confidence, status, diffstat_json, created_at, resolved_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sug.SuggestionID, sug.ChangeID, sug.BaseCommitSHA, sug.SuggestedCommitSHA, sug.CreatedBy, sug.Reason, sug.Description, sug.Confidence, sug.Status, sug.DiffstatJSON, sug.CreatedAt.Format(timeFormat), resolvedAt)
	if err != nil {
		return Suggestion{}, err
	}
	return sug, nil
}

func (s *Store) GetSuggestion(ctx context.Context, suggestionID string) (Suggestion, error) {
	row := s.db.QueryRowContext(ctx, `SELECT suggestion_id, change_id, base_commit_sha, suggested_commit_sha, created_by, reason, description, confidence, status, diffstat_json, created_at, resolved_at
		FROM suggestions WHERE suggestion_id = ?`, suggestionID)
	return scanSuggestion(row)
}

func (s *Store) ListSuggestions(ctx context.Context, changeID, status string, limit int) ([]Suggestion, error) {
	query := `SELECT suggestion_id, change_id, base_commit_sha, suggested_commit_sha, created_by, reason, description, confidence, status, diffstat_json, created_at, resolved_at
		FROM suggestions WHERE 1=1`
	args := []any{}
	if changeID != "" {
		query += " AND change_id = ?"
		args = append(args, changeID)
	}
	statuses := expandSuggestionStatusFilter(status)
	if len(statuses) > 0 {
		if len(statuses) == 1 {
			query += " AND status = ?"
			args = append(args, statuses[0])
		} else {
			query += " AND status IN (?, ?)"
			args = append(args, statuses[0], statuses[1])
		}
	}
	query += " ORDER BY created_at DESC"
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Suggestion
	for rows.Next() {
		sug, err := scanSuggestion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sug)
	}
	return out, rows.Err()
}

func (s *Store) UpdateSuggestionStatus(ctx context.Context, suggestionID, status string, resolvedAt time.Time) (Suggestion, error) {
	status = normalizeSuggestionStatus(status)
	var resolved any
	if !resolvedAt.IsZero() {
		resolved = resolvedAt.Format(timeFormat)
	}
	_, err := s.db.ExecContext(ctx, `UPDATE suggestions SET status = ?, resolved_at = ? WHERE suggestion_id = ?`, status, resolved, suggestionID)
	if err != nil {
		return Suggestion{}, err
	}
	return s.GetSuggestion(ctx, suggestionID)
}

type suggestionScanner interface {
	Scan(dest ...any) error
}

func scanSuggestion(row suggestionScanner) (Suggestion, error) {
	var sug Suggestion
	var createdAt string
	var resolvedAt sql.NullString
	if err := row.Scan(
		&sug.SuggestionID,
		&sug.ChangeID,
		&sug.BaseCommitSHA,
		&sug.SuggestedCommitSHA,
		&sug.CreatedBy,
		&sug.Reason,
		&sug.Description,
		&sug.Confidence,
		&sug.Status,
		&sug.DiffstatJSON,
		&createdAt,
		&resolvedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return Suggestion{}, ErrNotFound
		}
		return Suggestion{}, err
	}
	sug.CreatedAt = parseTime(createdAt)
	if resolvedAt.Valid {
		sug.ResolvedAt = parseTime(resolvedAt.String)
	}
	sug.Status = normalizeSuggestionStatus(sug.Status)
	return sug, nil
}

func normalizeSuggestionStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "open":
		return "pending"
	case "accepted":
		return "applied"
	default:
		return strings.TrimSpace(status)
	}
}

func expandSuggestionStatusFilter(status string) []string {
	normalized := normalizeSuggestionStatus(status)
	switch normalized {
	case "pending":
		return []string{"pending", "open"}
	case "applied":
		return []string{"applied", "accepted"}
	case "":
		return nil
	default:
		return []string{normalized}
	}
}
