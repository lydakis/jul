package storage

import "database/sql"

func runMigrations(db *sql.DB) error {
	stmts := []string{
		`PRAGMA foreign_keys = ON;`,
		`CREATE TABLE IF NOT EXISTS workspaces (
			workspace_id TEXT PRIMARY KEY,
			user TEXT NOT NULL,
			name TEXT NOT NULL,
			repo TEXT NOT NULL,
			branch TEXT NOT NULL,
			last_commit_sha TEXT NOT NULL,
			last_change_id TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS changes (
			change_id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			author TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at TEXT NOT NULL,
			latest_rev_index INTEGER NOT NULL,
			latest_commit_sha TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS revisions (
			commit_sha TEXT PRIMARY KEY,
			change_id TEXT NOT NULL,
			rev_index INTEGER NOT NULL,
			author TEXT NOT NULL,
			message TEXT NOT NULL,
			created_at TEXT NOT NULL,
			FOREIGN KEY(change_id) REFERENCES changes(change_id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_revisions_change_id ON revisions(change_id);`,
		`CREATE TABLE IF NOT EXISTS attestations (
			attestation_id TEXT PRIMARY KEY,
			commit_sha TEXT NOT NULL,
			change_id TEXT NOT NULL,
			type TEXT NOT NULL,
			status TEXT NOT NULL,
			started_at TEXT NOT NULL,
			finished_at TEXT NOT NULL,
			signals_json TEXT NOT NULL,
			created_at TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_attestations_commit ON attestations(commit_sha);`,
		`CREATE INDEX IF NOT EXISTS idx_attestations_change ON attestations(change_id);`,
		`CREATE TABLE IF NOT EXISTS events (
			event_id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			data_json TEXT NOT NULL,
			created_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS keep_refs (
			keep_id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL,
			commit_sha TEXT NOT NULL,
			change_id TEXT NOT NULL,
			created_at TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_keep_refs_workspace ON keep_refs(workspace_id);`,
		`CREATE INDEX IF NOT EXISTS idx_keep_refs_created ON keep_refs(created_at);`,
		`CREATE INDEX IF NOT EXISTS idx_events_created_at ON events(created_at);`,
	}

	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}

	if err := ensureColumn(db, "revisions", "repo", "TEXT"); err != nil {
		return err
	}
	return nil
}

func ensureColumn(db *sql.DB, table, column, columnType string) error {
	rows, err := db.Query(`PRAGMA table_info(` + table + `);`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name string
		var ctype string
		var notnull int
		var dfltValue any
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	_, err = db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column + ` ` + columnType + `;`)
	return err
}
