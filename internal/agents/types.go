package agents

import "github.com/critbot/claude-review/internal/config"

type Severity string

const (
	SeverityCritical   Severity = "critical"
	SeverityHigh       Severity = "high"
	SeverityMedium     Severity = "medium"
	SeveritySuggestion Severity = "suggestion"
)

func (s Severity) Score() int {
	switch s {
	case SeverityCritical:
		return 4
	case SeverityHigh:
		return 3
	case SeverityMedium:
		return 2
	default:
		return 1
	}
}

// Finding is a single code review issue identified by an agent.
type Finding struct {
	ID           string          `json:"id"`
	File         string          `json:"file"`
	Line         int             `json:"line"`       // 0 if not applicable
	EndLine      int             `json:"end_line"`   // 0 if single line
	Severity     Severity        `json:"severity"`
	Category     config.FocusArea `json:"category"`
	Description  string          `json:"description"`
	SuggestedFix string          `json:"suggested_fix,omitempty"`
	Confidence   float64         `json:"confidence"` // 0.0–1.0
	AgentIndex   int             `json:"agent_index"`
	CodeSnippet  string          `json:"code_snippet,omitempty"`
	NeedsContext bool            `json:"needs_context,omitempty"`
	RankNote     string          `json:"rank_note,omitempty"`
}

// rawFinding is what the LLM returns before validation.
type rawFinding struct {
	File         string  `json:"file"`
	Line         int     `json:"line"`
	EndLine      int     `json:"end_line"`
	Severity     string  `json:"severity"`
	Category     string  `json:"category"`
	Description  string  `json:"description"`
	SuggestedFix string  `json:"suggested_fix"`
	Confidence   float64 `json:"confidence"`
	CodeSnippet  string  `json:"code_snippet"`
}

// TokenUsage records API token consumption for a single agent call.
type TokenUsage struct {
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	Model        string `json:"model"`
}

// AggregatedUsage combines usage across all agents.
type AggregatedUsage struct {
	ByAgent          []TokenUsage `json:"by_agent"`
	TotalInputTokens  int         `json:"total_input_tokens"`
	TotalOutputTokens int         `json:"total_output_tokens"`
	EstimatedCostUSD  float64     `json:"estimated_cost_usd"`
}

// PipelineResult is the final output of the multi-agent pipeline.
type PipelineResult struct {
	Findings     []Finding
	Usage        AggregatedUsage
	SkippedCount int // findings rejected by verifier
	DurationSecs float64
}

// SeverityCounts returns a breakdown of findings by severity.
func (r *PipelineResult) SeverityCounts() map[Severity]int {
	counts := map[Severity]int{}
	for _, f := range r.Findings {
		counts[f.Severity]++
	}
	return counts
}
