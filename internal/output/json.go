package output

import (
	"encoding/json"
	"os"
	"time"

	"github.com/critbot/claude-review/internal/agents"
	"github.com/critbot/claude-review/internal/diff"
)

type JSONReport struct {
	GeneratedAt string              `json:"generated_at"`
	Source      diff.Source         `json:"source"`
	PRURL       string              `json:"pr_url,omitempty"`
	Summary     JSONSummary         `json:"summary"`
	Findings    []agents.Finding    `json:"findings"`
	Cost        JSONCost            `json:"cost"`
}

type JSONSummary struct {
	Critical   int `json:"critical"`
	High       int `json:"high"`
	Medium     int `json:"medium"`
	Suggestion int `json:"suggestion"`
	Total      int `json:"total"`
	Filtered   int `json:"filtered"`
}

type JSONCost struct {
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	EstimatedUSD float64 `json:"estimated_usd"`
	Model        string  `json:"model"`
	DurationSecs float64 `json:"duration_secs"`
}

// WriteJSON writes the review report as JSON to path.
// If path is "-", writes to stdout.
func WriteJSON(path string, result *agents.PipelineResult, payload *diff.Payload, model string) error {
	counts := result.SeverityCounts()
	report := JSONReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Source:      payload.Source,
		PRURL:       payload.PRURL,
		Summary: JSONSummary{
			Critical:   counts[agents.SeverityCritical],
			High:       counts[agents.SeverityHigh],
			Medium:     counts[agents.SeverityMedium],
			Suggestion: counts[agents.SeveritySuggestion],
			Total:      len(result.Findings),
			Filtered:   result.SkippedCount,
		},
		Findings: result.Findings,
		Cost: JSONCost{
			InputTokens:  result.Usage.TotalInputTokens,
			OutputTokens: result.Usage.TotalOutputTokens,
			EstimatedUSD: result.Usage.EstimatedCostUSD,
			Model:        model,
			DurationSecs: result.DurationSecs,
		},
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
