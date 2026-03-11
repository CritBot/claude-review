package output

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/critbot/claude-review/internal/agents"
	"github.com/critbot/claude-review/internal/diff"
)

func TestWriteJSON(t *testing.T) {
	result := &agents.PipelineResult{
		Findings: []agents.Finding{
			{
				ID:          "x1",
				File:        "main.go",
				Line:        10,
				Severity:    agents.SeverityHigh,
				Category:    "logic",
				Description: "off by one",
				Confidence:  0.9,
			},
		},
		Usage: agents.AggregatedUsage{
			TotalInputTokens:  5000,
			TotalOutputTokens: 1000,
			EstimatedCostUSD:  0.008,
		},
		SkippedCount: 2,
		DurationSecs: 12.3,
	}
	payload := &diff.Payload{
		Source: diff.SourceGitHubPR,
		PRURL:  "https://github.com/org/repo/pull/1",
	}

	tmpFile := filepath.Join(t.TempDir(), "review.json")
	if err := WriteJSON(tmpFile, result, payload, "claude-haiku-4-5-20251001"); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatal(err)
	}

	var report JSONReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}

	if report.Summary.High != 1 {
		t.Errorf("expected 1 high finding, got %d", report.Summary.High)
	}
	if report.Summary.Filtered != 2 {
		t.Errorf("expected 2 filtered, got %d", report.Summary.Filtered)
	}
	if report.Cost.InputTokens != 5000 {
		t.Errorf("expected 5000 input tokens, got %d", report.Cost.InputTokens)
	}
	if report.PRURL != "https://github.com/org/repo/pull/1" {
		t.Errorf("unexpected PR URL: %s", report.PRURL)
	}
	if len(report.Findings) != 1 {
		t.Errorf("expected 1 finding in JSON, got %d", len(report.Findings))
	}
}
