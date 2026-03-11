package memory

import (
	"context"
	"fmt"
	"strings"

	"github.com/critbot/claude-review/internal/diff"
)

// ContextSummary is the memory context injected into finder agent prompts.
type ContextSummary struct {
	HotspotFiles   []HotspotFile
	RecentInsights []string
}

// HotspotFile is a file with a known history of bugs.
type HotspotFile struct {
	Path        string
	BugCount    int
	Categories  []string
	TopSeverity string
}

// Query builds a context summary from memory for the files in the given diff.
// This is called before finder agents run so they can prioritize historically buggy areas.
func Query(ctx context.Context, db *DB, payload *diff.Payload) (*ContextSummary, error) {
	files := make([]string, 0, len(payload.Files))
	for _, f := range payload.Files {
		files = append(files, f.FilePath)
	}

	findings, err := db.GetFindingsForFiles(files)
	if err != nil {
		return nil, fmt.Errorf("querying memory: %w", err)
	}

	// Group findings by file to find hotspots
	byFile := map[string][]FindingRecord{}
	for _, f := range findings {
		if f.Accepted {
			byFile[f.File] = append(byFile[f.File], f)
		}
	}

	hotspots := make([]HotspotFile, 0, len(byFile))
	for path, recs := range byFile {
		cats := map[string]bool{}
		topSev := "suggestion"
		for _, r := range recs {
			cats[r.Category] = true
			if severityScore(r.Severity) > severityScore(topSev) {
				topSev = r.Severity
			}
		}
		catList := make([]string, 0, len(cats))
		for c := range cats {
			catList = append(catList, c)
		}
		hotspots = append(hotspots, HotspotFile{
			Path:        path,
			BugCount:    len(recs),
			Categories:  catList,
			TopSeverity: topSev,
		})
	}

	// Fetch recent consolidated insights
	consolidations, err := db.GetRecentConsolidations(5)
	if err != nil {
		return nil, err
	}
	insights := make([]string, 0, len(consolidations))
	for _, c := range consolidations {
		insights = append(insights, c.InsightText)
	}

	return &ContextSummary{
		HotspotFiles:   hotspots,
		RecentInsights: insights,
	}, nil
}

// FormatContextBlock formats the memory context for injection into a finder prompt.
func (cs *ContextSummary) FormatContextBlock() string {
	if len(cs.HotspotFiles) == 0 && len(cs.RecentInsights) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n--- MEMORY CONTEXT (from past reviews of this codebase) ---\n")

	if len(cs.HotspotFiles) > 0 {
		b.WriteString("Files with a history of bugs (prioritize these):\n")
		for _, h := range cs.HotspotFiles {
			b.WriteString(fmt.Sprintf("  %s — %d past findings (%s) top severity: %s\n",
				h.Path, h.BugCount, strings.Join(h.Categories, ", "), h.TopSeverity))
		}
	}

	if len(cs.RecentInsights) > 0 {
		b.WriteString("Recent cross-PR patterns:\n")
		for _, ins := range cs.RecentInsights {
			b.WriteString("  - " + ins + "\n")
		}
	}

	b.WriteString("--- END MEMORY CONTEXT ---\n\n")
	return b.String()
}

func severityScore(s string) int {
	switch s {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	default:
		return 1
	}
}
