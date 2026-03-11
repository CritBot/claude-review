package output

// Package output/annotations produces GitHub Checks API annotation JSON,
// suitable for piping into a GitHub Actions workflow or GitLab code quality report.

import (
	"encoding/json"
	"os"
	"time"

	"github.com/critbot/claude-review/internal/agents"
	"github.com/critbot/claude-review/internal/diff"
)

// GitHubAnnotationLevel maps severity to GitHub Checks API annotation level.
func githubLevel(s agents.Severity) string {
	switch s {
	case agents.SeverityCritical, agents.SeverityHigh:
		return "failure"
	case agents.SeverityMedium:
		return "warning"
	default:
		return "notice"
	}
}

// GitHubAnnotation is one entry in the GitHub Checks API annotations array.
type GitHubAnnotation struct {
	Path            string `json:"path"`
	StartLine       int    `json:"start_line"`
	EndLine         int    `json:"end_line"`
	AnnotationLevel string `json:"annotation_level"` // failure | warning | notice
	Message         string `json:"message"`
	Title           string `json:"title,omitempty"`
	RawDetails      string `json:"raw_details,omitempty"`
}

// GitHubAnnotationsReport is the full output written to file for --format annotations.
type GitHubAnnotationsReport struct {
	GeneratedAt string               `json:"generated_at"`
	Source      diff.Source          `json:"source"`
	Annotations []GitHubAnnotation   `json:"annotations"`
	Summary     string               `json:"summary"`
}

// WriteAnnotations writes GitHub Checks-compatible annotation JSON to path.
// If path is "-", writes to stdout.
func WriteAnnotations(path string, result *agents.PipelineResult, payload *diff.Payload) error {
	annotations := make([]GitHubAnnotation, 0, len(result.Findings))

	for _, f := range result.Findings {
		endLine := f.EndLine
		if endLine == 0 || endLine < f.Line {
			endLine = f.Line
		}
		startLine := f.Line
		if startLine == 0 {
			startLine = 1
			endLine = 1
		}

		detail := f.Description
		if f.SuggestedFix != "" {
			detail += "\n\nSuggested fix:\n" + f.SuggestedFix
		}

		annotations = append(annotations, GitHubAnnotation{
			Path:            f.File,
			StartLine:       startLine,
			EndLine:         endLine,
			AnnotationLevel: githubLevel(f.Severity),
			Title:           "[" + string(f.Severity) + "] " + string(f.Category),
			Message:         f.Description,
			RawDetails:      detail,
		})
	}

	counts := result.SeverityCounts()
	summary := "claude-review found: " +
		itoa(counts[agents.SeverityCritical]) + " critical, " +
		itoa(counts[agents.SeverityHigh]) + " high, " +
		itoa(counts[agents.SeverityMedium]) + " medium, " +
		itoa(counts[agents.SeveritySuggestion]) + " suggestions"

	report := GitHubAnnotationsReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Source:      payload.Source,
		Annotations: annotations,
		Summary:     summary,
	}

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}

	if path == "-" {
		_, err = os.Stdout.Write(data)
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 10)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	return string(buf)
}
