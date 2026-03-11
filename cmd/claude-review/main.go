package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/critbot/claude-review/internal/agents"
	"github.com/critbot/claude-review/internal/config"
	"github.com/critbot/claude-review/internal/diff"
	"github.com/critbot/claude-review/internal/hooks"
	"github.com/critbot/claude-review/internal/output"
	"github.com/spf13/cobra"
)

var version = "0.1.0"

func main() {
	root := buildRoot()
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// shared CLI flags
type globalFlags struct {
	configPath string
	format     string
	outputFile string
	fix        bool
	estimate   bool
	agentCount int
	focus      string
	model      string
	verbose    bool
	noColor    bool
}

func buildRoot() *cobra.Command {
	var gf globalFlags

	root := &cobra.Command{
		Use:   "claude-review",
		Short: "Multi-agent AI code review powered by Claude",
		Long: `claude-review runs a parallel multi-agent code review on your git diff.

It uses your ANTHROPIC_API_KEY to call Claude, analyzes your changes across
multiple focus areas (logic, security, performance, types, tests), and produces
a ranked list of findings in Markdown or JSON.

Quick start:
  export ANTHROPIC_API_KEY=sk-ant-...
  git add .
  claude-review               # review staged changes
  claude-review diff HEAD~1   # review last commit
  claude-review pr <url>      # review a GitHub PR or GitLab MR`,
		Version: version,
		// Default action: review staged changes
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(gf)
			if err != nil {
				return err
			}
			payload, err := diff.GetStaged()
			if err != nil {
				return err
			}
			return runReview(cmd.Context(), cfg, payload, gf)
		},
	}

	addGlobalFlags(root, &gf)

	root.AddCommand(buildDiffCmd(&gf))
	root.AddCommand(buildPRCmd(&gf))
	root.AddCommand(buildInstallHookCmd())

	return root
}

func buildDiffCmd(gf *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff [range]",
		Short: "Review a git diff range",
		Long: `Review changes in a git diff range.

Examples:
  claude-review diff HEAD~1              # last commit
  claude-review diff main..feature       # branch diff
  claude-review diff HEAD~3..HEAD        # last 3 commits
  claude-review diff --files src/auth.go # specific files`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(*gf)
			if err != nil {
				return err
			}

			var payload *diff.Payload
			if len(args) == 0 {
				payload, err = diff.GetStaged()
			} else {
				payload, err = diff.GetRange(args[0])
			}
			if err != nil {
				return err
			}
			return runReview(cmd.Context(), cfg, payload, *gf)
		},
	}
	return cmd
}

func buildPRCmd(gf *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pr <url>",
		Short: "Review a GitHub PR or GitLab MR",
		Long: `Review a pull request or merge request by URL.

Supported platforms:
  GitHub:    https://github.com/owner/repo/pull/123
  GitLab:    https://gitlab.com/owner/repo/-/merge_requests/45

Set GITHUB_TOKEN or GITLAB_TOKEN for private repos or to avoid rate limits.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(*gf)
			if err != nil {
				return err
			}

			rawURL := args[0]
			var payload *diff.Payload

			switch {
			case diff.IsGitHubURL(rawURL):
				logf(cfg, "Fetching GitHub PR...")
				payload, err = diff.FetchGitHubPR(rawURL, cfg.GitHubToken)
			case diff.IsGitLabURL(rawURL):
				logf(cfg, "Fetching GitLab MR...")
				payload, err = diff.FetchGitLabMR(rawURL, cfg.GitLabToken, cfg.GitLabHost)
			default:
				return fmt.Errorf("unrecognized URL format: %s\n\nSupported: github.com/*/pull/*, gitlab.com/*/merge_requests/*", rawURL)
			}
			if err != nil {
				return err
			}

			return runReview(cmd.Context(), cfg, payload, *gf)
		},
	}
	return cmd
}

func buildInstallHookCmd() *cobra.Command {
	var remove bool
	cmd := &cobra.Command{
		Use:   "install-hook",
		Short: "Install claude-review as a git pre-commit hook",
		RunE: func(cmd *cobra.Command, args []string) error {
			if remove {
				return hooks.Remove()
			}
			return hooks.Install()
		},
	}
	cmd.Flags().BoolVar(&remove, "remove", false, "Remove the claude-review hook")
	return cmd
}

func addGlobalFlags(cmd *cobra.Command, gf *globalFlags) {
	cmd.PersistentFlags().StringVar(&gf.configPath, "config", "", "Path to claude-review.config.json")
	cmd.PersistentFlags().StringVar(&gf.format, "format", "markdown", "Output format: markdown|json")
	cmd.PersistentFlags().StringVar(&gf.outputFile, "output", "", "Output file path (default: REVIEW.md or review.json)")
	cmd.PersistentFlags().BoolVar(&gf.fix, "fix", false, "Auto-apply suggested fixes (v1.1)")
	cmd.PersistentFlags().BoolVar(&gf.estimate, "estimate", false, "Show cost estimate only, do not run review")
	cmd.PersistentFlags().IntVar(&gf.agentCount, "agents", 0, "Number of finder agents (overrides config)")
	cmd.PersistentFlags().StringVar(&gf.focus, "focus", "", "Focus areas: logic,security,performance,types,tests")
	cmd.PersistentFlags().StringVar(&gf.model, "model", "", "Claude model to use (overrides config)")
	cmd.PersistentFlags().BoolVar(&gf.verbose, "verbose", false, "Enable verbose logging")
	cmd.PersistentFlags().BoolVar(&gf.noColor, "no-color", false, "Disable colored output")
}

func loadConfig(gf globalFlags) (*config.Config, error) {
	cfg, err := config.Load(gf.configPath)
	if err != nil {
		return nil, err
	}

	// Apply CLI overrides
	if gf.agentCount > 0 {
		cfg.Agents = gf.agentCount
	}
	if gf.model != "" {
		cfg.Model = gf.model
	}
	if gf.focus != "" {
		parts := strings.Split(gf.focus, ",")
		cfg.Focus = make([]config.FocusArea, 0, len(parts))
		for _, p := range parts {
			cfg.Focus = append(cfg.Focus, config.FocusArea(strings.TrimSpace(p)))
		}
	}
	switch gf.format {
	case "json":
		cfg.Format = config.FormatJSON
	case "annotations":
		cfg.Format = config.FormatAnnotations
	default:
		cfg.Format = config.FormatMarkdown
	}
	cfg.Fix = gf.fix
	cfg.Verbose = gf.verbose

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func runReview(ctx context.Context, cfg *config.Config, payload *diff.Payload, gf globalFlags) error {
	fmt.Fprintf(os.Stderr, "claude-review v%s\n", version)
	fmt.Fprintf(os.Stderr, "Files: %d | +%d -%d lines | Model: %s\n\n",
		len(payload.Files), payload.TotalAdditions, payload.TotalDeletions, cfg.Model)

	if gf.fix {
		fmt.Fprintln(os.Stderr, "Note: --fix is coming in v1.1. Running review without auto-fix.")
	}

	// Estimate mode: show cost and exit
	if gf.estimate {
		diffLen := len(payload.SerializeDiff())
		output.PrintEstimate(os.Stderr, diffLen, cfg.Agents, cfg.Model)
		return nil
	}

	// Check max_cost_usd guard
	if cfg.MaxCostUSD > 0 {
		est := agents.EstimateCost(len(payload.SerializeDiff()), cfg.Agents, cfg.Model)
		if est > cfg.MaxCostUSD {
			return fmt.Errorf("estimated cost $%.4f exceeds max_cost_usd $%.4f — aborting\nUse --estimate to see the breakdown", est, cfg.MaxCostUSD)
		}
	}

	logger := func(format string, args ...any) {
		if cfg.Verbose || true { // always log progress to stderr
			fmt.Fprintf(os.Stderr, format+"\n", args...)
		}
	}

	result, err := agents.RunPipeline(ctx, payload, cfg, logger)
	if err != nil {
		return fmt.Errorf("review pipeline failed: %w", err)
	}

	output.PrintCostSummary(os.Stderr, result.Usage, result.DurationSecs)

	// Determine output path
	outPath := cfg.Output
	if gf.outputFile != "" {
		outPath = gf.outputFile
	}

	switch cfg.Format {
	case config.FormatJSON:
		if outPath == "REVIEW.md" {
			outPath = "review.json"
		}
		if err := output.WriteJSON(outPath, result, payload, cfg.Model); err != nil {
			return fmt.Errorf("writing JSON output: %w", err)
		}
		if outPath != "-" {
			fmt.Fprintf(os.Stderr, "\nWrote %s\n", outPath)
		}

	default: // markdown
		if err := output.WriteMarkdown(outPath, result, payload); err != nil {
			return fmt.Errorf("writing Markdown output: %w", err)
		}
		fmt.Fprintf(os.Stderr, "\nWrote %s\n", outPath)
		printSummaryToTerminal(result)
	}

	// Exit with non-zero if critical or high issues found (useful for CI)
	counts := result.SeverityCounts()
	if counts[agents.SeverityCritical] > 0 || counts[agents.SeverityHigh] > 0 {
		os.Exit(1)
	}
	return nil
}

func printSummaryToTerminal(result *agents.PipelineResult) {
	counts := result.SeverityCounts()
	total := len(result.Findings)
	if total == 0 {
		fmt.Fprintln(os.Stderr, "\n✅ No issues found.")
		return
	}

	fmt.Fprintln(os.Stderr, "\nFindings:")
	if n := counts[agents.SeverityCritical]; n > 0 {
		fmt.Fprintf(os.Stderr, "  🔴 Critical:   %d\n", n)
	}
	if n := counts[agents.SeverityHigh]; n > 0 {
		fmt.Fprintf(os.Stderr, "  🟠 High:       %d\n", n)
	}
	if n := counts[agents.SeverityMedium]; n > 0 {
		fmt.Fprintf(os.Stderr, "  🟡 Medium:     %d\n", n)
	}
	if n := counts[agents.SeveritySuggestion]; n > 0 {
		fmt.Fprintf(os.Stderr, "  💡 Suggestion: %d\n", n)
	}
}

func logf(cfg *config.Config, format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}
