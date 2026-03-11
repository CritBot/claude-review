package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/critbot/claude-review/internal/agents"
)

// ShouldConsolidate returns true if the consolidation agent should run.
// Fires when: 30+ minutes since last run AND new findings exist, OR 10+ new findings.
func ShouldConsolidate(ctx context.Context, db *DB) (bool, error) {
	lastRun, err := db.LastConsolidationTime()
	if err != nil {
		return false, err
	}

	checkSince := lastRun
	if lastRun.IsZero() {
		checkSince = time.Now().Add(-24 * time.Hour) // first run: look at all recent
	}

	newCount, err := db.CountNewFindingsSince(checkSince)
	if err != nil {
		return false, err
	}

	// Volume trigger: 10+ new findings
	if newCount >= 10 {
		return true, nil
	}

	// Time trigger: 30+ minutes AND at least 1 new finding
	if !lastRun.IsZero() && time.Since(lastRun) >= 30*time.Minute && newCount > 0 {
		return true, nil
	}

	// First run with any findings
	if lastRun.IsZero() && newCount > 0 {
		return true, nil
	}

	return false, nil
}

// RunConsolidation calls the Anthropic API with finding metadata (no source code)
// to identify cross-PR patterns and stores the insights.
func RunConsolidation(ctx context.Context, db *DB, model string) error {
	// Fetch all recent accepted findings (last 200)
	allFindings, err := db.GetFindingsForFiles(nil) // nil = all files
	if err != nil {
		return err
	}
	if len(allFindings) == 0 {
		return nil
	}

	// Build compact metadata (no source code ever sent during consolidation)
	type findingMeta struct {
		File     string `json:"file"`
		Severity string `json:"severity"`
		Category string `json:"category"`
		Desc     string `json:"description"`
		PRRef    string `json:"pr_ref,omitempty"`
	}
	meta := make([]findingMeta, 0, len(allFindings))
	for _, f := range allFindings {
		meta = append(meta, findingMeta{
			File:     f.File,
			Severity: f.Severity,
			Category: f.Category,
			Desc:     f.Description,
			PRRef:    f.PRRef,
		})
	}
	metaJSON, _ := json.Marshal(meta)

	systemPrompt := `You are a code quality analyst. Analyze finding metadata across multiple PRs.
Return ONLY a JSON array of insight strings. No markdown, no explanation.
Each insight should describe a non-obvious cross-PR pattern.
Return [] if there are no meaningful patterns.`

	userPrompt := fmt.Sprintf(`Analyze these code review findings from multiple PRs and identify patterns.

FINDINGS METADATA (no source code):
%s

Find:
1. Files that repeatedly have the same category of bug
2. Recurring issue types across different files
3. Improvement trends (fewer findings in recent PRs)
4. High-risk areas that need more attention

Return a JSON array of plain-English insight strings, max 5 insights.
Example: ["auth.go has had 3 security findings across 4 PRs", "N+1 query patterns appear repeatedly in service layer"]`, string(metaJSON))

	raw, _, err := agents.CallAPIForConsolidation(ctx, model, 1000, systemPrompt, userPrompt)
	if err != nil {
		return fmt.Errorf("consolidation API call: %w", err)
	}

	var insights []string
	if err := parseInsightsJSON(raw, &insights); err != nil || len(insights) == 0 {
		return nil // no actionable insights
	}

	// Collect IDs of source findings
	ids := make([]string, 0, len(allFindings))
	for _, f := range allFindings {
		ids = append(ids, f.ID)
	}
	idsJSON, _ := json.Marshal(ids)

	for _, insight := range insights {
		if strings.TrimSpace(insight) == "" {
			continue
		}
		if err := db.InsertConsolidation(insight, string(idsJSON)); err != nil {
			return err
		}
	}
	return nil
}

func parseInsightsJSON(raw string, target *[]string) error {
	raw = strings.TrimSpace(raw)
	if idx := strings.Index(raw, "["); idx >= 0 {
		raw = raw[idx:]
		if end := strings.LastIndex(raw, "]"); end >= 0 {
			raw = raw[:end+1]
		}
	}
	return json.Unmarshal([]byte(raw), target)
}
