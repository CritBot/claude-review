package output

import (
	"strings"
	"testing"

	"github.com/critbot/claude-review/internal/agents"
	"github.com/critbot/claude-review/internal/diff"
)

func TestBuildMarkdown_NoFindings(t *testing.T) {
	result := &agents.PipelineResult{
		Findings: []agents.Finding{},
		Usage: agents.AggregatedUsage{
			TotalInputTokens:  100,
			TotalOutputTokens: 50,
			EstimatedCostUSD:  0.001,
		},
		DurationSecs: 3.5,
	}
	payload := &diff.Payload{
		Source:         diff.SourceLocalStaged,
		TotalAdditions: 10,
		TotalDeletions: 2,
		Files:          []diff.FileDiff{{FilePath: "main.go"}},
	}

	md := buildMarkdown(result, payload)
	if !strings.Contains(md, "No issues found") {
		t.Error("expected 'No issues found' in empty results output")
	}
	if !strings.Contains(md, "Code Review Report") {
		t.Error("expected header in output")
	}
}

func TestBuildMarkdown_WithFindings(t *testing.T) {
	result := &agents.PipelineResult{
		Findings: []agents.Finding{
			{
				ID:          "abc123",
				File:        "src/auth.go",
				Line:        42,
				Severity:    agents.SeverityCritical,
				Category:    "security",
				Description: "JWT expiry not validated",
				Confidence:  0.95,
			},
			{
				ID:          "def456",
				File:        "src/db.go",
				Line:        10,
				Severity:    agents.SeverityMedium,
				Category:    "performance",
				Description: "N+1 query in loop",
				Confidence:  0.80,
			},
		},
		Usage:        agents.AggregatedUsage{EstimatedCostUSD: 0.43},
		DurationSecs: 38,
	}
	payload := &diff.Payload{
		Source:         diff.SourceGitHubPR,
		HeadRef:        "feature/auth",
		BaseRef:        "main",
		TotalAdditions: 142,
		TotalDeletions: 38,
		Files:          make([]diff.FileDiff, 8),
	}

	md := buildMarkdown(result, payload)

	if !strings.Contains(md, "Critical") {
		t.Error("expected Critical section")
	}
	if !strings.Contains(md, "JWT expiry not validated") {
		t.Error("expected finding description in output")
	}
	if !strings.Contains(md, "src/auth.go") {
		t.Error("expected file path in output")
	}
	if !strings.Contains(md, "42") {
		t.Error("expected line number in output")
	}
	if !strings.Contains(md, "$0.4300") {
		t.Error("expected cost in output")
	}
}

func TestBuildMarkdown_SuggestedFix(t *testing.T) {
	result := &agents.PipelineResult{
		Findings: []agents.Finding{
			{
				Severity:     agents.SeverityHigh,
				File:         "main.go",
				Line:         5,
				Description:  "Missing error check",
				SuggestedFix: "if err != nil { return err }",
				Confidence:   0.9,
			},
		},
		Usage: agents.AggregatedUsage{},
	}
	payload := &diff.Payload{Files: []diff.FileDiff{}}

	md := buildMarkdown(result, payload)
	if !strings.Contains(md, "Suggested Fix") {
		t.Error("expected Suggested Fix section when fix is present")
	}
	if !strings.Contains(md, "if err != nil") {
		t.Error("expected fix code in output")
	}
}

func TestBuildMarkdown_NeedsContext(t *testing.T) {
	result := &agents.PipelineResult{
		Findings: []agents.Finding{
			{
				Severity:     agents.SeverityMedium,
				File:         "main.go",
				Line:         1,
				Description:  "Possible issue",
				Confidence:   0.82,
				NeedsContext: true,
			},
		},
		Usage: agents.AggregatedUsage{},
	}
	md := buildMarkdown(result, &diff.Payload{Files: []diff.FileDiff{}})
	if !strings.Contains(md, "require broader codebase context") {
		t.Error("expected needs-context warning in output")
	}
}
