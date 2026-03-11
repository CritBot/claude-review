package agents

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/critbot/claude-review/internal/config"
	"github.com/critbot/claude-review/internal/diff"
)

// RunPipeline executes the full multi-agent review pipeline against the given diff payload.
//
// Pipeline phases:
//   Phase 1–3 (parallel): Finder agents, one per focus area, split across available agents
//   Phase 4a (sequential): Verifier agent deduplicates and filters findings
//   Phase 4b (sequential): Ranker agent sorts by severity and confidence
func RunPipeline(ctx context.Context, payload *diff.Payload, cfg *config.Config, logf func(string, ...any)) (*PipelineResult, error) {
	start := time.Now()

	focusAreas := cfg.Focus
	if len(focusAreas) == 0 {
		focusAreas = cfg.AllFocusAreas()
	}

	// Split files evenly across agents to avoid context overflow
	fileSplits := diff.SplitForAgents(payload.Files, cfg.Agents)

	// Phase 1–3: Run finder agents in parallel (with concurrency limit)
	logf("Running %d finder agents in parallel (focus: %v)...", len(focusAreas), focusAreas)

	type finderResult struct {
		findings []Finding
		usage    TokenUsage
		focus    config.FocusArea
		err      error
	}

	results := make([]finderResult, len(focusAreas))
	sem := make(chan struct{}, cfg.ConcurrentAgents) // semaphore for concurrency limit
	var wg sync.WaitGroup

	for i, focus := range focusAreas {
		wg.Add(1)
		go func(idx int, fa config.FocusArea) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			// Each agent gets a subset of files (or all files if fewer agents than files)
			agentFiles := payload.Files
			if idx < len(fileSplits) && len(fileSplits[idx]) > 0 {
				agentFiles = fileSplits[idx]
			}
			diffText := payload.SerializeFileSubset(agentFiles)

			findings, usage, err := RunFinder(ctx, fa, diffText, idx, cfg)
			results[idx] = finderResult{findings: findings, usage: usage, focus: fa, err: err}

			if err != nil {
				logf("  ⚠  Finder agent %d (%s) failed: %v", idx+1, fa, err)
			} else {
				logf("  ✓  Finder agent %d (%s): %d findings", idx+1, fa, len(findings))
			}
		}(i, focus)
	}
	wg.Wait()

	// Collect all findings and usage
	var allFindings []Finding
	usages := make([]TokenUsage, 0, len(focusAreas))
	for _, r := range results {
		if r.err == nil {
			allFindings = append(allFindings, r.findings...)
			usages = append(usages, r.usage)
		}
	}

	if len(allFindings) == 0 {
		logf("No findings from any finder agent — this diff looks clean!")
		return &PipelineResult{
			Findings:     []Finding{},
			Usage:        aggregateUsage(usages, cfg.Model),
			DurationSecs: time.Since(start).Seconds(),
		}, nil
	}

	logf("Verifying %d raw findings...", len(allFindings))

	// Phase 4a: Verifier
	diffText := payload.SerializeDiff()
	accepted, rejected, verifierUsage, err := RunVerifier(ctx, allFindings, diffText, cfg)
	if err != nil {
		return nil, fmt.Errorf("verifier agent: %w", err)
	}
	usages = append(usages, verifierUsage)
	logf("Verifier: %d accepted, %d rejected", len(accepted), len(rejected))

	// Phase 4b: Ranker
	logf("Ranking %d findings...", len(accepted))
	ranked, rankerUsage, err := RunRanker(ctx, accepted, diffText, cfg)
	if err != nil {
		ranked = deterministicSort(accepted)
	}
	usages = append(usages, rankerUsage)

	aggregated := aggregateUsage(usages, cfg.Model)
	logf("Done. Cost: $%.4f | Time: %.1fs", aggregated.EstimatedCostUSD, time.Since(start).Seconds())

	return &PipelineResult{
		Findings:     ranked,
		Usage:        aggregated,
		SkippedCount: len(rejected),
		DurationSecs: time.Since(start).Seconds(),
	}, nil
}

func aggregateUsage(usages []TokenUsage, model string) AggregatedUsage {
	agg := AggregatedUsage{ByAgent: usages}
	for _, u := range usages {
		agg.TotalInputTokens += u.InputTokens
		agg.TotalOutputTokens += u.OutputTokens
	}
	agg.EstimatedCostUSD = ComputeCost(agg.TotalInputTokens, agg.TotalOutputTokens, model)
	return agg
}

// ComputeCost returns the estimated USD cost for the given token counts and model.
func ComputeCost(inputTokens, outputTokens int, model string) float64 {
	type pricing struct{ input, output float64 }
	// Price per 1M tokens (USD)
	prices := map[string]pricing{
		"claude-haiku-4-5-20251001":  {0.80, 4.00},
		"claude-sonnet-4-6":          {3.00, 15.00},
		"claude-opus-4-6":            {15.00, 75.00},
	}
	p, ok := prices[model]
	if !ok {
		p = pricing{3.00, 15.00} // default to Sonnet pricing
	}
	return (float64(inputTokens)/1_000_000)*p.input + (float64(outputTokens)/1_000_000)*p.output
}

// EstimateCost returns a rough pre-run cost estimate.
func EstimateCost(diffLen int, numAgents int, model string) float64 {
	// Rough: 1 token ≈ 4 chars, 500 base tokens per agent for system prompt
	inputTokensPerAgent := diffLen/4 + 500
	outputTokensPerAgent := 800 // average output per finder agent
	// +1 for verifier, +1 for ranker
	total := numAgents + 2
	return ComputeCost(inputTokensPerAgent*total, outputTokensPerAgent*total, model)
}
