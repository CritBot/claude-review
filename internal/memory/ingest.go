package memory

import (
	"context"
	"fmt"

	"github.com/critbot/claude-review/internal/agents"
)

// Ingest stores all findings from a completed review run into the memory database.
// It checks for known false positives and skips them.
// Call this after every successful pipeline run when --memory is enabled.
func Ingest(ctx context.Context, db *DB, result *agents.PipelineResult, prRef string) error {
	for _, f := range result.Findings {
		// Check if this is a known false positive
		hash := descriptionHash(f.File, string(f.Category), f.Description)
		isFP, err := db.IsFalsePositive(f.File, string(f.Category), hash)
		if err != nil {
			return fmt.Errorf("checking false positive: %w", err)
		}
		if isFP {
			continue // silently skip known noise
		}

		rec := FindingRecord{
			ID:          f.ID,
			File:        f.File,
			Line:        f.Line,
			Severity:    string(f.Severity),
			Category:    string(f.Category),
			Description: f.Description,
			Accepted:    true,
			PRRef:       prRef,
		}
		if err := db.InsertFinding(rec); err != nil {
			return fmt.Errorf("inserting finding %s: %w", f.ID, err)
		}
	}
	return nil
}

// MarkRejected records a user-rejected finding as a false positive so it won't
// appear again in future reviews of the same codebase.
func MarkRejected(ctx context.Context, db *DB, file, category, description string) error {
	hash := descriptionHash(file, category, description)
	return db.RecordFalsePositive(file, category, hash)
}
