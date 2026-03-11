package agents

import (
	"context"
	"fmt"
	"math/rand"
	"strings"

	"github.com/critbot/claude-review/internal/config"
	"github.com/critbot/claude-review/templates"
)

const finderSystemPrompt = `You are a code review expert. You MUST respond with ONLY a valid JSON array.
No markdown fences, no explanation text, no preamble. Just the raw JSON array.
If you find no issues, respond with an empty array: []`

// RunFinder executes a finder agent for a specific focus area.
// memoryContext is an optional block of text from past reviews (may be empty).
// When non-empty it is prepended to the prompt so the agent knows which files
// are hotspots and which patterns have been seen before.
func RunFinder(ctx context.Context, focus config.FocusArea, diffText, memoryContext string, agentIndex int, cfg *config.Config) ([]Finding, TokenUsage, error) {
	templateName := fmt.Sprintf("finder-%s.md", string(focus))
	tmplBytes, err := templates.FS.ReadFile(templateName)
	if err != nil {
		return nil, TokenUsage{}, fmt.Errorf("loading template %s: %w", templateName, err)
	}

	// Truncate diff if very large (guard against context overflow)
	diffText = truncateDiff(diffText, cfg.MaxTokensPerAgent)

	prompt := strings.ReplaceAll(string(tmplBytes), "{{DIFF}}", diffText)
	if memoryContext != "" {
		prompt = memoryContext + prompt
	}

	raw, usage, err := callAPI(ctx, cfg.Model, cfg.MaxTokensPerAgent, finderSystemPrompt, prompt)
	if err != nil {
		return nil, TokenUsage{}, fmt.Errorf("finder agent %d (%s): %w", agentIndex, focus, err)
	}

	var rawFindings []rawFinding
	if err := parseJSONResponse(raw, &rawFindings); err != nil {
		// LLM gave us unparseable output — return empty rather than failing the whole run
		return []Finding{}, usage, nil
	}

	findings := make([]Finding, 0, len(rawFindings))
	for _, rf := range rawFindings {
		if rf.File == "" || rf.Description == "" {
			continue
		}
		severity := normalizeSeverity(rf.Severity)
		if rf.Confidence < 0.4 {
			continue // too low confidence to even consider
		}

		findings = append(findings, Finding{
			ID:           newID(),
			File:         rf.File,
			Line:         rf.Line,
			EndLine:      rf.EndLine,
			Severity:     severity,
			Category:     config.FocusArea(rf.Category),
			Description:  rf.Description,
			SuggestedFix: rf.SuggestedFix,
			Confidence:   clamp(rf.Confidence, 0, 1),
			AgentIndex:   agentIndex,
			CodeSnippet:  rf.CodeSnippet,
		})
	}

	return findings, usage, nil
}

func normalizeSeverity(s string) Severity {
	switch strings.ToLower(s) {
	case "critical":
		return SeverityCritical
	case "high":
		return SeverityHigh
	case "medium":
		return SeverityMedium
	default:
		return SeveritySuggestion
	}
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func newID() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 8)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
}

// truncateDiff limits the diff text to avoid context window overflow.
// Rough estimate: 1 token ≈ 4 characters; we target 60% of the token budget for input.
func truncateDiff(diff string, maxTokens int) string {
	maxChars := maxTokens * 4 * 60 / 100
	if len(diff) <= maxChars {
		return diff
	}
	lines := strings.Split(diff, "\n")
	var kept []string
	total := 0
	for _, line := range lines {
		total += len(line) + 1
		if total > maxChars {
			kept = append(kept, "\n// [DIFF TRUNCATED — showing first portion only]")
			break
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}
