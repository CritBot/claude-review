package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/critbot/claude-review/internal/config"
	"github.com/critbot/claude-review/templates"
)

const verifierSystemPrompt = `You are a strict code review quality controller.
You MUST respond with ONLY valid JSON in the exact shape requested.
No markdown, no explanation, no preamble.`

type verifierOutput struct {
	Accepted []verifiedFinding `json:"accepted"`
	Rejected []verifiedFinding `json:"rejected"`
}

type verifiedFinding struct {
	Finding
	RejectionReason string `json:"rejection_reason,omitempty"`
	NeedsContext    bool   `json:"needs_context,omitempty"`
}

// RunVerifier filters and deduplicates findings from all finder agents.
func RunVerifier(ctx context.Context, findings []Finding, diffText string, cfg *config.Config) ([]Finding, []Finding, TokenUsage, error) {
	if len(findings) == 0 {
		return []Finding{}, []Finding{}, TokenUsage{}, nil
	}

	tmplBytes, err := templates.FS.ReadFile("verifier.md")
	if err != nil {
		return nil, nil, TokenUsage{}, fmt.Errorf("loading verifier template: %w", err)
	}

	findingsJSON, err := json.Marshal(findings)
	if err != nil {
		return nil, nil, TokenUsage{}, err
	}

	// Truncate diff for verifier context
	diffText = truncateDiff(diffText, cfg.MaxTokensPerAgent)

	prompt := string(tmplBytes)
	prompt = strings.ReplaceAll(prompt, "{{DIFF}}", diffText)
	prompt = strings.ReplaceAll(prompt, "{{FINDINGS}}", string(findingsJSON))
	prompt = strings.ReplaceAll(prompt, "{{CONFIDENCE_THRESHOLD}}",
		fmt.Sprintf("%.2f", cfg.ConfidenceThreshold))

	// Verifier needs more tokens since it handles all findings at once
	maxTokens := cfg.MaxTokensPerAgent * 2
	raw, usage, err := callAPI(ctx, cfg.Model, maxTokens, verifierSystemPrompt, prompt)
	if err != nil {
		// On verifier failure, fall back to simple confidence-based filtering
		return fallbackFilter(findings, cfg.ConfidenceThreshold), []Finding{}, TokenUsage{}, nil
	}

	var result verifierOutput
	if err := parseJSONResponse(raw, &result); err != nil {
		return fallbackFilter(findings, cfg.ConfidenceThreshold), []Finding{}, usage, nil
	}

	accepted := make([]Finding, 0, len(result.Accepted))
	for _, vf := range result.Accepted {
		f := vf.Finding
		f.NeedsContext = vf.NeedsContext
		accepted = append(accepted, f)
	}

	rejected := make([]Finding, 0, len(result.Rejected))
	for _, vf := range result.Rejected {
		rejected = append(rejected, vf.Finding)
	}

	return accepted, rejected, usage, nil
}

// fallbackFilter applies simple confidence threshold filtering when the verifier LLM call fails.
func fallbackFilter(findings []Finding, threshold float64) []Finding {
	result := make([]Finding, 0)
	seen := map[string]bool{}

	for _, f := range findings {
		if f.Confidence < threshold {
			continue
		}
		// Simple dedup: same file+line+severity
		key := fmt.Sprintf("%s:%d:%s", f.File, f.Line, f.Severity)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, f)
	}
	return result
}
