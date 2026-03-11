package agents

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/critbot/claude-review/internal/config"
	"github.com/critbot/claude-review/templates"
)

const rankerSystemPrompt = `You are a senior engineering lead prioritizing code review findings.
You MUST respond with ONLY a valid JSON array of the complete findings list, re-ordered.
No markdown, no explanation.`

// RunRanker sorts findings by severity and confidence, optionally using LLM for context-aware re-scoring.
func RunRanker(ctx context.Context, findings []Finding, diffSummary string, cfg *config.Config) ([]Finding, TokenUsage, error) {
	if len(findings) == 0 {
		return findings, TokenUsage{}, nil
	}

	tmplBytes, err := templates.FS.ReadFile("ranker.md")
	if err != nil {
		// Fall back to deterministic sort
		return deterministicSort(findings), TokenUsage{}, nil
	}

	findingsJSON, err := json.Marshal(findings)
	if err != nil {
		return deterministicSort(findings), TokenUsage{}, nil
	}

	// Use a brief summary of the diff rather than the full diff to keep tokens low
	prompt := string(tmplBytes)
	prompt = strings.ReplaceAll(prompt, "{{FINDINGS}}", string(findingsJSON))
	prompt = strings.ReplaceAll(prompt, "{{DIFF_SUMMARY}}", truncateDiff(diffSummary, 1000))

	raw, usage, err := callAPI(ctx, cfg.Model, cfg.MaxTokensPerAgent, rankerSystemPrompt, prompt)
	if err != nil {
		return deterministicSort(findings), TokenUsage{}, nil
	}

	var ranked []Finding
	if err := parseJSONResponse(raw, &ranked); err != nil || len(ranked) == 0 {
		return deterministicSort(findings), usage, nil
	}

	// Validate we got all findings back (LLM sometimes drops items)
	if len(ranked) < len(findings) {
		return deterministicSort(findings), usage, nil
	}

	return ranked, usage, nil
}

// deterministicSort sorts findings by severity (desc) then confidence (desc) then file path.
func deterministicSort(findings []Finding) []Finding {
	sorted := make([]Finding, len(findings))
	copy(sorted, findings)
	sort.SliceStable(sorted, func(i, j int) bool {
		si, sj := sorted[i].Severity.Score(), sorted[j].Severity.Score()
		if si != sj {
			return si > sj
		}
		if sorted[i].Confidence != sorted[j].Confidence {
			return sorted[i].Confidence > sorted[j].Confidence
		}
		if sorted[i].File != sorted[j].File {
			return sorted[i].File < sorted[j].File
		}
		return sorted[i].Line < sorted[j].Line
	})
	return sorted
}
