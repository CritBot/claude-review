package output

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/critbot/claude-review/internal/agents"
	"github.com/critbot/claude-review/internal/diff"
)

func TestGithubLevel(t *testing.T) {
	cases := map[agents.Severity]string{
		agents.SeverityCritical:   "failure",
		agents.SeverityHigh:       "failure",
		agents.SeverityMedium:     "warning",
		agents.SeveritySuggestion: "notice",
	}
	for sev, want := range cases {
		got := githubLevel(sev)
		if got != want {
			t.Errorf("githubLevel(%q) = %q, want %q", sev, got, want)
		}
	}
}

func TestItoa(t *testing.T) {
	cases := map[int]string{0: "0", 1: "1", 42: "42", 100: "100", 999: "999"}
	for n, want := range cases {
		got := itoa(n)
		if got != want {
			t.Errorf("itoa(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestWriteAnnotations(t *testing.T) {
	result := &agents.PipelineResult{
		Findings: []agents.Finding{
			{
				File:        "src/auth.go",
				Line:        42,
				EndLine:     45,
				Severity:    agents.SeverityCritical,
				Category:    "security",
				Description: "SQL injection vulnerability",
				Confidence:  0.95,
			},
			{
				File:        "src/db.go",
				Line:        10,
				Severity:    agents.SeverityMedium,
				Category:    "performance",
				Description: "N+1 query",
				SuggestedFix: "Use JOIN instead",
				Confidence:  0.80,
			},
		},
	}
	payload := &diff.Payload{Source: diff.SourceGitHubPR}

	tmpFile := filepath.Join(t.TempDir(), "annotations.json")
	if err := WriteAnnotations(tmpFile, result, payload); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(tmpFile)
	var report GitHubAnnotationsReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}

	if len(report.Annotations) != 2 {
		t.Fatalf("expected 2 annotations, got %d", len(report.Annotations))
	}
	if report.Annotations[0].AnnotationLevel != "failure" {
		t.Errorf("expected failure for critical, got %q", report.Annotations[0].AnnotationLevel)
	}
	if report.Annotations[1].AnnotationLevel != "warning" {
		t.Errorf("expected warning for medium, got %q", report.Annotations[1].AnnotationLevel)
	}
	if report.Annotations[0].Path != "src/auth.go" {
		t.Errorf("expected path=src/auth.go, got %q", report.Annotations[0].Path)
	}
	if report.Annotations[0].StartLine != 42 || report.Annotations[0].EndLine != 45 {
		t.Errorf("unexpected line range: %d-%d", report.Annotations[0].StartLine, report.Annotations[0].EndLine)
	}
	// Finding with suggested fix should have it in raw_details
	if report.Annotations[1].RawDetails == "" {
		t.Error("expected raw_details for finding with suggested fix")
	}
}

func TestWriteAnnotations_ZeroLine(t *testing.T) {
	// Findings with line=0 should default to line 1
	result := &agents.PipelineResult{
		Findings: []agents.Finding{
			{File: "main.go", Line: 0, Severity: agents.SeverityMedium, Description: "general issue"},
		},
	}
	payload := &diff.Payload{}
	tmpFile := filepath.Join(t.TempDir(), "annotations.json")
	if err := WriteAnnotations(tmpFile, result, payload); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(tmpFile)
	var report GitHubAnnotationsReport
	json.Unmarshal(data, &report)
	if report.Annotations[0].StartLine != 1 {
		t.Errorf("expected line=1 for zero-line finding, got %d", report.Annotations[0].StartLine)
	}
}

func TestWriteAnnotations_Summary(t *testing.T) {
	result := &agents.PipelineResult{
		Findings: []agents.Finding{
			{Severity: agents.SeverityCritical, File: "a.go", Line: 1, Description: "x"},
			{Severity: agents.SeverityHigh, File: "b.go", Line: 1, Description: "y"},
		},
	}
	payload := &diff.Payload{}
	tmpFile := filepath.Join(t.TempDir(), "annotations.json")
	WriteAnnotations(tmpFile, result, payload)
	data, _ := os.ReadFile(tmpFile)
	var report GitHubAnnotationsReport
	json.Unmarshal(data, &report)
	if report.Summary == "" {
		t.Error("expected non-empty summary")
	}
}
