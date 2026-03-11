package agents

import "testing"

func TestComputeCost_Haiku(t *testing.T) {
	// 1M input + 1M output with haiku pricing ($0.80 + $4.00 = $4.80)
	got := ComputeCost(1_000_000, 1_000_000, "claude-haiku-4-5-20251001")
	want := 4.80
	if abs(got-want) > 0.001 {
		t.Errorf("ComputeCost haiku = %.4f, want %.4f", got, want)
	}
}

func TestComputeCost_Sonnet(t *testing.T) {
	// 1M input + 1M output with sonnet pricing ($3.00 + $15.00 = $18.00)
	got := ComputeCost(1_000_000, 1_000_000, "claude-sonnet-4-6")
	want := 18.00
	if abs(got-want) > 0.001 {
		t.Errorf("ComputeCost sonnet = %.4f, want %.4f", got, want)
	}
}

func TestComputeCost_UnknownModel(t *testing.T) {
	// Unknown model should fall back to Sonnet pricing
	got := ComputeCost(1_000_000, 1_000_000, "claude-unknown-model")
	want := 18.00 // Sonnet default
	if abs(got-want) > 0.001 {
		t.Errorf("ComputeCost unknown = %.4f, want %.4f", got, want)
	}
}

func TestComputeCost_ZeroTokens(t *testing.T) {
	got := ComputeCost(0, 0, "claude-haiku-4-5-20251001")
	if got != 0 {
		t.Errorf("expected $0 for zero tokens, got %.4f", got)
	}
}

func TestComputeCost_SmallAmount(t *testing.T) {
	// 10k input tokens + 2k output tokens with haiku
	// (10000/1M)*0.80 + (2000/1M)*4.00 = 0.008 + 0.008 = 0.016
	got := ComputeCost(10_000, 2_000, "claude-haiku-4-5-20251001")
	want := 0.016
	if abs(got-want) > 0.0001 {
		t.Errorf("ComputeCost small = %.6f, want %.6f", got, want)
	}
}

func TestEstimateCost_Positive(t *testing.T) {
	est := EstimateCost(10000, 4, "claude-haiku-4-5-20251001")
	if est <= 0 {
		t.Error("expected positive estimate")
	}
}

func TestEstimateCost_MoreAgentsCostsMore(t *testing.T) {
	est4 := EstimateCost(10000, 4, "claude-haiku-4-5-20251001")
	est8 := EstimateCost(10000, 8, "claude-haiku-4-5-20251001")
	if est8 <= est4 {
		t.Error("expected 8 agents to cost more than 4 agents")
	}
}

func TestAggregateUsage(t *testing.T) {
	usages := []TokenUsage{
		{InputTokens: 1000, OutputTokens: 200, Model: "claude-haiku-4-5-20251001"},
		{InputTokens: 2000, OutputTokens: 400, Model: "claude-haiku-4-5-20251001"},
	}
	agg := aggregateUsage(usages, "claude-haiku-4-5-20251001")
	if agg.TotalInputTokens != 3000 {
		t.Errorf("expected 3000 input tokens, got %d", agg.TotalInputTokens)
	}
	if agg.TotalOutputTokens != 600 {
		t.Errorf("expected 600 output tokens, got %d", agg.TotalOutputTokens)
	}
	if agg.EstimatedCostUSD <= 0 {
		t.Error("expected positive cost")
	}
	if len(agg.ByAgent) != 2 {
		t.Errorf("expected 2 ByAgent entries, got %d", len(agg.ByAgent))
	}
}

func TestSeverityScore(t *testing.T) {
	if SeverityCritical.Score() <= SeverityHigh.Score() {
		t.Error("critical should score higher than high")
	}
	if SeverityHigh.Score() <= SeverityMedium.Score() {
		t.Error("high should score higher than medium")
	}
	if SeverityMedium.Score() <= SeveritySuggestion.Score() {
		t.Error("medium should score higher than suggestion")
	}
}

func TestSeverityCounts(t *testing.T) {
	result := &PipelineResult{
		Findings: []Finding{
			{Severity: SeverityCritical},
			{Severity: SeverityCritical},
			{Severity: SeverityHigh},
			{Severity: SeverityMedium},
			{Severity: SeveritySuggestion},
		},
	}
	counts := result.SeverityCounts()
	if counts[SeverityCritical] != 2 {
		t.Errorf("expected 2 critical, got %d", counts[SeverityCritical])
	}
	if counts[SeverityHigh] != 1 {
		t.Errorf("expected 1 high, got %d", counts[SeverityHigh])
	}
	if counts[SeverityMedium] != 1 {
		t.Errorf("expected 1 medium, got %d", counts[SeverityMedium])
	}
	if counts[SeveritySuggestion] != 1 {
		t.Errorf("expected 1 suggestion, got %d", counts[SeveritySuggestion])
	}
}

func TestTruncateDiff_ShortDiff(t *testing.T) {
	diff := "line1\nline2\nline3"
	got := truncateDiff(diff, 4000)
	if got != diff {
		t.Error("short diff should not be truncated")
	}
}

func TestTruncateDiff_LongDiff(t *testing.T) {
	// Create a diff longer than maxTokens*4*60/100 chars
	line := "+" + string(make([]byte, 100))
	var lines []string
	for i := 0; i < 500; i++ {
		lines = append(lines, line)
	}
	bigDiff := ""
	for _, l := range lines {
		bigDiff += l + "\n"
	}
	got := truncateDiff(bigDiff, 100) // very small limit
	if len(got) >= len(bigDiff) {
		t.Error("long diff should be truncated")
	}
	if got == "" {
		t.Error("truncated diff should not be empty")
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
