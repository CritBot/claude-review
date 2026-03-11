// Package memory implements the v1.1 persistent codebase memory layer.
// It uses a local SQLite database (one per repo) to store findings across reviews,
// detect cross-PR patterns, and reduce false positives over time.
package memory

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS findings (
	id          TEXT PRIMARY KEY,
	repo        TEXT NOT NULL,
	file        TEXT NOT NULL,
	line        INTEGER,
	severity    TEXT NOT NULL,
	category    TEXT NOT NULL,
	description TEXT NOT NULL,
	accepted    INTEGER NOT NULL DEFAULT 1, -- 1=accepted, 0=rejected by user
	pr_ref      TEXT,
	created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS consolidations (
	id                  TEXT PRIMARY KEY,
	repo                TEXT NOT NULL,
	insight_text        TEXT NOT NULL,
	source_finding_ids  TEXT NOT NULL, -- JSON array of finding IDs
	created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS false_positives (
	id             TEXT PRIMARY KEY,
	repo           TEXT NOT NULL,
	file           TEXT NOT NULL,
	category       TEXT NOT NULL,
	pattern_hash   TEXT NOT NULL,
	times_rejected INTEGER NOT NULL DEFAULT 1,
	last_seen      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(repo, file, category, pattern_hash)
);

CREATE INDEX IF NOT EXISTS idx_findings_repo_file ON findings(repo, file);
CREATE INDEX IF NOT EXISTS idx_false_positives_hash ON false_positives(repo, pattern_hash);
`

// DB wraps the SQLite connection for the memory layer.
type DB struct {
	conn *sql.DB
	repo string // normalized repo identifier
	path string
}

// Open opens (or creates) the memory database for the given repo path.
// If dbPath is empty, defaults to <repo>/.claude-review/memory.db.
func Open(repoPath, dbPath string) (*DB, error) {
	if dbPath == "" {
		dbPath = filepath.Join(repoPath, ".claude-review", "memory.db")
	}

	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("creating memory directory: %w", err)
	}

	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening memory database: %w", err)
	}

	if _, err := conn.Exec(schema); err != nil {
		conn.Close()
		return nil, fmt.Errorf("initializing schema: %w", err)
	}

	// Enable WAL mode for better concurrent access
	conn.Exec("PRAGMA journal_mode=WAL;")
	conn.Exec("PRAGMA synchronous=NORMAL;")

	repo := repoID(repoPath)
	return &DB{conn: conn, repo: repo, path: dbPath}, nil
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.conn.Close()
}

// Path returns the filesystem path of the database file.
func (db *DB) Path() string {
	return db.path
}

// FindingRecord is one row in the findings table.
type FindingRecord struct {
	ID          string
	Repo        string
	File        string
	Line        int
	Severity    string
	Category    string
	Description string
	Accepted    bool
	PRRef       string
	CreatedAt   time.Time
}

// InsertFinding stores a finding in the database.
func (db *DB) InsertFinding(f FindingRecord) error {
	_, err := db.conn.Exec(`
		INSERT OR REPLACE INTO findings
			(id, repo, file, line, severity, category, description, accepted, pr_ref, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		f.ID, db.repo, f.File, f.Line, f.Severity, f.Category,
		f.Description, boolInt(f.Accepted), f.PRRef, time.Now().UTC(),
	)
	return err
}

// GetFindingsForFiles returns all accepted findings for the given files in this repo.
// If files is nil, returns all findings for the repo (used by consolidation).
func (db *DB) GetFindingsForFiles(files []string) ([]FindingRecord, error) {
	var rows *sql.Rows
	var err error

	if files == nil {
		rows, err = db.conn.Query(`
			SELECT id, file, line, severity, category, description, accepted, pr_ref, created_at
			FROM findings WHERE repo = ? ORDER BY created_at DESC LIMIT 200`, db.repo)
	} else {
		if len(files) == 0 {
			return nil, nil
		}
		placeholders := ""
		args := []any{db.repo}
		for i, f := range files {
			if i > 0 {
				placeholders += ","
			}
			placeholders += "?"
			args = append(args, f)
		}
		rows, err = db.conn.Query(fmt.Sprintf(`
			SELECT id, file, line, severity, category, description, accepted, pr_ref, created_at
			FROM findings
			WHERE repo = ? AND file IN (%s)
			ORDER BY created_at DESC
			LIMIT 100`, placeholders), args...)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []FindingRecord
	for rows.Next() {
		var r FindingRecord
		var accepted int
		err := rows.Scan(&r.ID, &r.File, &r.Line, &r.Severity, &r.Category,
			&r.Description, &accepted, &r.PRRef, &r.CreatedAt)
		if err != nil {
			continue
		}
		r.Accepted = accepted == 1
		r.Repo = db.repo
		results = append(results, r)
	}
	return results, rows.Err()
}

// CountNewFindingsSince counts findings added after the given time.
func (db *DB) CountNewFindingsSince(t time.Time) (int, error) {
	var count int
	err := db.conn.QueryRow(
		`SELECT COUNT(*) FROM findings WHERE repo = ? AND created_at > ?`,
		db.repo, t,
	).Scan(&count)
	return count, err
}

// ConsolidationRecord is one row in the consolidations table.
type ConsolidationRecord struct {
	ID               string
	InsightText      string
	SourceFindingIDs string // JSON array
	CreatedAt        time.Time
}

// InsertConsolidation stores a consolidation insight.
func (db *DB) InsertConsolidation(insight, sourceFindingIDs string) error {
	id := fmt.Sprintf("c%x", sha256.Sum256([]byte(insight+time.Now().String())))[:16]
	_, err := db.conn.Exec(`
		INSERT INTO consolidations (id, repo, insight_text, source_finding_ids, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		id, db.repo, insight, sourceFindingIDs, time.Now().UTC(),
	)
	return err
}

// GetRecentConsolidations returns the N most recent insights for this repo.
func (db *DB) GetRecentConsolidations(n int) ([]ConsolidationRecord, error) {
	rows, err := db.conn.Query(`
		SELECT id, insight_text, source_finding_ids, created_at
		FROM consolidations WHERE repo = ?
		ORDER BY created_at DESC LIMIT ?`, db.repo, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []ConsolidationRecord
	for rows.Next() {
		var r ConsolidationRecord
		if err := rows.Scan(&r.ID, &r.InsightText, &r.SourceFindingIDs, &r.CreatedAt); err != nil {
			continue
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// LastConsolidationTime returns when the last consolidation ran for this repo.
func (db *DB) LastConsolidationTime() (time.Time, error) {
	var t time.Time
	err := db.conn.QueryRow(
		`SELECT created_at FROM consolidations WHERE repo = ? ORDER BY created_at DESC LIMIT 1`,
		db.repo,
	).Scan(&t)
	if err == sql.ErrNoRows {
		return time.Time{}, nil
	}
	return t, err
}

// IsFalsePositive checks if a finding pattern has been rejected before.
func (db *DB) IsFalsePositive(file, category, descriptionHash string) (bool, error) {
	var count int
	err := db.conn.QueryRow(`
		SELECT times_rejected FROM false_positives
		WHERE repo = ? AND file = ? AND category = ? AND pattern_hash = ?`,
		db.repo, file, category, descriptionHash,
	).Scan(&count)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return count >= 2, err // require 2+ rejections before suppressing
}

// RecordFalsePositive increments the rejection counter for a finding pattern.
func (db *DB) RecordFalsePositive(file, category, descriptionHash string) error {
	_, err := db.conn.Exec(`
		INSERT INTO false_positives (id, repo, file, category, pattern_hash, times_rejected, last_seen)
		VALUES (?, ?, ?, ?, ?, 1, CURRENT_TIMESTAMP)
		ON CONFLICT(repo, file, category, pattern_hash) DO UPDATE SET
			times_rejected = times_rejected + 1,
			last_seen = CURRENT_TIMESTAMP`,
		newShortID(), db.repo, file, category, descriptionHash,
	)
	return err
}

// Stats returns summary statistics for this repo's memory.
type Stats struct {
	TotalFindings      int
	AcceptedFindings   int
	Consolidations     int
	FalsePositives     int
	LastConsolidation  time.Time
}

// GetStats returns summary statistics for this repo.
func (db *DB) GetStats() (Stats, error) {
	var s Stats
	db.conn.QueryRow(`SELECT COUNT(*), SUM(accepted) FROM findings WHERE repo = ?`, db.repo).
		Scan(&s.TotalFindings, &s.AcceptedFindings)
	db.conn.QueryRow(`SELECT COUNT(*) FROM consolidations WHERE repo = ?`, db.repo).
		Scan(&s.Consolidations)
	db.conn.QueryRow(`SELECT COUNT(*) FROM false_positives WHERE repo = ?`, db.repo).
		Scan(&s.FalsePositives)
	s.LastConsolidation, _ = db.LastConsolidationTime()
	return s, nil
}

// PruneOld removes findings older than maxAgeDays and, per file, keeps only
// the most recent maxPerFile findings. This keeps the DB small even on
// long-lived repos. Consolidation records and false_positives are not pruned.
//
// Defaults: maxAgeDays=90, maxPerFile=50. Pass 0 for either to skip that pass.
func (db *DB) PruneOld(maxAgeDays, maxPerFile int) error {
	if maxAgeDays > 0 {
		cutoff := time.Now().UTC().AddDate(0, 0, -maxAgeDays)
		if _, err := db.conn.Exec(
			`DELETE FROM findings WHERE repo = ? AND created_at < ?`,
			db.repo, cutoff,
		); err != nil {
			return fmt.Errorf("pruning old findings: %w", err)
		}
	}

	if maxPerFile > 0 {
		// For each file, delete findings beyond the most recent maxPerFile.
		// Done per-file in Go to avoid complex SQL that varies by SQLite version.
		rows, err := db.conn.Query(
			`SELECT DISTINCT file FROM findings WHERE repo = ?`, db.repo)
		if err != nil {
			return fmt.Errorf("listing files for per-file pruning: %w", err)
		}
		var files []string
		for rows.Next() {
			var f string
			if err := rows.Scan(&f); err == nil {
				files = append(files, f)
			}
		}
		rows.Close()

		for _, file := range files {
			if _, err := db.conn.Exec(`
				DELETE FROM findings
				WHERE repo = ? AND file = ? AND id NOT IN (
					SELECT id FROM findings
					WHERE repo = ? AND file = ?
					ORDER BY created_at DESC
					LIMIT ?
				)`, db.repo, file, db.repo, file, maxPerFile,
			); err != nil {
				return fmt.Errorf("pruning per-file findings for %s: %w", file, err)
			}
		}
	}

	// Reclaim disk space after deletions.
	_, err := db.conn.Exec("PRAGMA incremental_vacuum;")
	return err
}

// Clear deletes all memory for this repo.
func (db *DB) Clear() error {
	for _, table := range []string{"findings", "consolidations", "false_positives"} {
		if _, err := db.conn.Exec("DELETE FROM "+table+" WHERE repo = ?", db.repo); err != nil {
			return err
		}
	}
	return nil
}

// repoID produces a stable short identifier for a repo path.
func repoID(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	h := sha256.Sum256([]byte(abs))
	return fmt.Sprintf("%x", h[:8])
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func newShortID() string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
	return fmt.Sprintf("%x", h[:8])
}
