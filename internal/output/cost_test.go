package output

import (
	"bytes"
	"strings"
	"testing"

	"github.com/critbot/claude-review/internal/agents"
)

func TestPrintCostSummary(t *testing.T) {
	var buf bytes.Buffer
	usage := agents.AggregatedUsage{
		TotalInputTokens:  12000,
		TotalOutputTokens: 3000,
		EstimatedCostUSD:  0.0132,
	}
	PrintCostSummary(&buf, usage, 42.5)

	out := buf.String()
	if !strings.Contains(out, "12000") {
		t.Error("expected input token count in summary")
	}
	if !strings.Contains(out, "3000") {
		t.Error("expected output token count in summary")
	}
	if !strings.Contains(out, "0.0132") {
		t.Error("expected cost in summary")
	}
	if !strings.Contains(out, "42.5") {
		t.Error("expected duration in summary")
	}
}

func TestPrintEstimate(t *testing.T) {
	var buf bytes.Buffer
	PrintEstimate(&buf, 50000, 4, "claude-haiku-4-5-20251001")

	out := buf.String()
	if !strings.Contains(out, "claude-haiku-4-5-20251001") {
		t.Error("expected model name in estimate")
	}
	if !strings.Contains(out, "4") {
		t.Error("expected agent count in estimate")
	}
	if !strings.Contains(out, "$") {
		t.Error("expected cost symbol in estimate")
	}
}
