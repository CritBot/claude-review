package output

import (
	"fmt"
	"io"

	"github.com/critbot/claude-review/internal/agents"
)

// PrintCostSummary writes a cost summary line to the given writer (typically os.Stderr).
func PrintCostSummary(w io.Writer, usage agents.AggregatedUsage, durationSecs float64) {
	fmt.Fprintf(w, "\n─────────────────────────────────────────\n")
	fmt.Fprintf(w, "  Input tokens:  %d\n", usage.TotalInputTokens)
	fmt.Fprintf(w, "  Output tokens: %d\n", usage.TotalOutputTokens)
	fmt.Fprintf(w, "  Estimated cost: $%.4f\n", usage.EstimatedCostUSD)
	fmt.Fprintf(w, "  Time: %.1fs\n", durationSecs)
	fmt.Fprintf(w, "─────────────────────────────────────────\n")
}

// PrintEstimate prints a pre-run cost estimate.
func PrintEstimate(w io.Writer, diffLen, numAgents int, model string) {
	est := agents.EstimateCost(diffLen, numAgents, model)
	inputTokenEst := (diffLen/4 + 500) * (numAgents + 2)
	fmt.Fprintf(w, "─────────────────────────────────────────\n")
	fmt.Fprintf(w, "  Cost estimate (before running)\n")
	fmt.Fprintf(w, "  Model:         %s\n", model)
	fmt.Fprintf(w, "  Agents:        %d finder + 1 verifier + 1 ranker\n", numAgents)
	fmt.Fprintf(w, "  ~Input tokens: %d\n", inputTokenEst)
	fmt.Fprintf(w, "  ~Est. cost:    $%.4f\n", est)
	fmt.Fprintf(w, "─────────────────────────────────────────\n")
}
