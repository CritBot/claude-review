package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/critbot/claude-review/internal/agents"
	"github.com/critbot/claude-review/internal/config"
	"github.com/critbot/claude-review/internal/diff"
	"github.com/critbot/claude-review/internal/hooks"
	"github.com/critbot/claude-review/internal/memory"
	"github.com/critbot/claude-review/internal/output"
	"github.com/spf13/cobra"
)

// consolidationWG lets the on-wake background consolidation finish even when
// the primary command is fast (e.g. "insights"). For long-running reviews the
// goroutine will almost always complete before the review does.
var consolidationWG sync.WaitGroup

var version = "0.1.0"

func main() {
	root := buildRoot()
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// Wait for any background consolidation triggered by the on-wake check.
	consolidationWG.Wait()
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
	useMemory  bool
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

	// On-wake consolidation: piggybacks on every command invocation.
	// If the memory DB exists and 30+ minutes have passed (or 10+ new findings),
	// consolidation runs in the background — no daemon required.
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		maybeConsolidateBackground(cmd.Context())
		return nil
	}

	root.AddCommand(buildDiffCmd(&gf))
	root.AddCommand(buildPRCmd(&gf))
	root.AddCommand(buildInstallHookCmd())
	root.AddCommand(buildMemoryCmd())
	root.AddCommand(buildInsightsCmd())

	return root
}

func buildDiffCmd(gf *globalFlags) *cobra.Command {
	var files []string

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
			switch {
			case len(files) > 0:
				payload, err = diff.GetFiles(files)
			case len(args) == 0:
				payload, err = diff.GetStaged()
			default:
				payload, err = diff.GetRange(args[0])
			}
			if err != nil {
				return err
			}
			return runReview(cmd.Context(), cfg, payload, *gf)
		},
	}
	cmd.Flags().StringSliceVar(&files, "files", nil, "Review specific files only (comma-separated or repeated flag)")
	return cmd
}

func buildPRCmd(gf *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pr <url>",
		Short: "Review a GitHub PR or GitLab MR",
		Long: `Review a pull request or merge request by URL or number.

Supported platforms:
  GitHub:    https://github.com/owner/repo/pull/123
  GitLab:    https://gitlab.com/owner/repo/-/merge_requests/45
  Bitbucket: https://bitbucket.org/owner/repo/pull-requests/67

Shorthand (inside a git repo with a remote):
  claude-review pr 123   # detects remote from git remote get-url origin

Set GITHUB_TOKEN, GITLAB_TOKEN, or BITBUCKET_TOKEN for private repos.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(*gf)
			if err != nil {
				return err
			}

			rawURL, err := diff.ResolveURL(args[0])
			if err != nil {
				return err
			}
			var payload *diff.Payload

			switch {
			case diff.IsGitHubURL(rawURL):
				logf(cfg, "Fetching GitHub PR...")
				payload, err = diff.FetchGitHubPR(rawURL, cfg.GitHubToken)
			case diff.IsGitLabURL(rawURL):
				logf(cfg, "Fetching GitLab MR...")
				payload, err = diff.FetchGitLabMR(rawURL, cfg.GitLabToken, cfg.GitLabHost)
			case diff.IsBitbucketURL(rawURL):
				logf(cfg, "Fetching Bitbucket PR...")
				payload, err = diff.FetchBitbucketPR(rawURL, cfg.BitbucketToken)
			default:
				return fmt.Errorf("unrecognized URL format: %s\n\nSupported: github.com/*/pull/*, gitlab.com/*/merge_requests/*, bitbucket.org/*/pull-requests/*", rawURL)
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
	cmd.PersistentFlags().BoolVar(&gf.useMemory, "memory", false, "Enable persistent memory layer (v1.1)")
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

// maybeConsolidateBackground fires a background consolidation if the memory DB
// exists and ShouldConsolidate returns true. It never blocks the calling command.
// This is the on-wake trigger: consolidation piggybacks on normal usage rather
// than requiring a persistent daemon.
func maybeConsolidateBackground(ctx context.Context) {
	cwd, err := os.Getwd()
	if err != nil {
		return
	}

	// Only proceed if the user has ever used --memory (DB file exists).
	dbPath := filepath.Join(cwd, ".claude-review", "memory.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return
	}

	db, err := memory.Open(cwd, "")
	if err != nil {
		return
	}
	defer db.Close()

	should, err := memory.ShouldConsolidate(ctx, db)
	if err != nil || !should {
		return
	}

	cfg, _ := config.Load("")
	model := cfg.Model

	consolidationWG.Add(1)
	go func() {
		defer consolidationWG.Done()
		bgDB, err := memory.Open(cwd, "")
		if err != nil {
			return
		}
		defer bgDB.Close()
		fmt.Fprintln(os.Stderr, "[memory] consolidating patterns in background...")
		if err := memory.RunConsolidation(context.Background(), bgDB, model); err != nil {
			fmt.Fprintf(os.Stderr, "[memory] consolidation error: %v\n", err)
			return
		}
		fmt.Fprintln(os.Stderr, "[memory] ✓ consolidation complete")
	}()
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

	// v1.1: Query memory for context before running pipeline
	var memoryContext string
	if gf.useMemory {
		cwd, _ := os.Getwd()
		if db, err := memory.Open(cwd, ""); err == nil {
			if mctx, err := memory.Query(ctx, db, payload); err == nil && mctx != nil {
				memoryContext = mctx.FormatContextBlock()
				if memoryContext != "" {
					fmt.Fprintf(os.Stderr, "Memory: injecting context for %d hotspot file(s)\n", len(mctx.HotspotFiles))
				}
			}
			db.Close()
		}
	}

	result, err := agents.RunPipeline(ctx, payload, cfg, memoryContext, logger)
	if err != nil {
		return fmt.Errorf("review pipeline failed: %w", err)
	}

	// v1.1: Ingest findings into memory after pipeline
	if gf.useMemory {
		cwd, _ := os.Getwd()
		if db, err := memory.Open(cwd, ""); err == nil {
			_ = memory.Ingest(ctx, db, result, payload.PRURL)
			db.Close()
		}
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

	case config.FormatAnnotations:
		if outPath == "REVIEW.md" {
			outPath = "annotations.json"
		}
		if err := output.WriteAnnotations(outPath, result, payload); err != nil {
			return fmt.Errorf("writing annotations output: %w", err)
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

func buildMemoryCmd() *cobra.Command {
	var daemonLoop bool
	cmd := &cobra.Command{
		Use:   "memory",
		Short: "Manage the persistent memory layer (v1.1)",
		Long: `claude-review memory manages the SQLite-backed codebase memory layer.

Memory is opt-in. Without --memory on review commands, claude-review is stateless.
When enabled, findings are stored per-repo and used to prioritize future reviews.`,
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "start",
		Short: "Start the background consolidation daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, err := memory.DefaultDaemonPaths()
			if err != nil {
				return err
			}
			return memory.StartDaemon(paths)
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "stop",
		Short: "Stop the background consolidation daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, err := memory.DefaultDaemonPaths()
			if err != nil {
				return err
			}
			return memory.StopDaemon(paths)
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show daemon status and memory statistics",
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, err := memory.DefaultDaemonPaths()
			if err != nil {
				return err
			}
			status := memory.GetDaemonStatus(paths)
			if status.Running {
				fmt.Printf("Daemon: running (PID %d)\n", status.PID)
			} else {
				fmt.Println("Daemon: not running")
			}
			fmt.Printf("DB path: %s\n", paths.HomeDir)

			cwd, _ := os.Getwd()
			db, err := memory.Open(cwd, "")
			if err == nil {
				defer db.Close()
				stats, _ := db.GetStats()
				fmt.Printf("Findings stored: %d (accepted: %d)\n", stats.TotalFindings, stats.AcceptedFindings)
				fmt.Printf("Consolidations:  %d\n", stats.Consolidations)
				fmt.Printf("False positives: %d\n", stats.FalsePositives)
				if !stats.LastConsolidation.IsZero() {
					fmt.Printf("Last consolidation: %s\n", stats.LastConsolidation.Format("2006-01-02 15:04"))
				}
			}
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "clear",
		Short: "Delete all stored findings for this repo",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, _ := os.Getwd()
			db, err := memory.Open(cwd, "")
			if err != nil {
				return err
			}
			defer db.Close()
			if err := db.Clear(); err != nil {
				return err
			}
			fmt.Println("✓ Memory cleared for this repo")
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "install",
		Short: "Install the daemon as a login service (launchd/systemd)",
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, err := memory.DefaultDaemonPaths()
			if err != nil {
				return err
			}
			return memory.InstallAutostart(paths)
		},
	})

	// Hidden flag for the daemon loop itself
	cmd.PersistentFlags().BoolVar(&daemonLoop, "daemon-loop", false, "Run as background daemon loop")
	cmd.Flag("daemon-loop").Hidden = true
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if !daemonLoop {
			return cmd.Help()
		}
		return runDaemonLoop()
	}

	return cmd
}

// runDaemonLoop is the long-running consolidation daemon process.
func runDaemonLoop() error {
	fmt.Println("claude-review memory daemon started")
	cwd, _ := os.Getwd()
	cfg, _ := config.Load("")

	for {
		db, err := memory.Open(cwd, "")
		if err == nil {
			should, _ := memory.ShouldConsolidate(context.Background(), db)
			if should {
				fmt.Printf("[%s] Running consolidation...\n", time.Now().Format("15:04:05"))
				if err := memory.RunConsolidation(context.Background(), db, cfg.Model); err != nil {
					fmt.Printf("[%s] Consolidation error: %v\n", time.Now().Format("15:04:05"), err)
				}
			}
			db.Close()
		}
		time.Sleep(5 * time.Minute)
	}
}

func buildInsightsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "insights",
		Short: "Show cross-PR pattern insights from memory (v1.1)",
		Long:  "Displays consolidated insights about recurring patterns, hotspot files, and trends across all reviews stored in memory.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, _ := os.Getwd()
			db, err := memory.Open(cwd, "")
			if err != nil {
				return fmt.Errorf("memory not initialized — run a review with --memory first")
			}
			defer db.Close()

			consolidations, err := db.GetRecentConsolidations(20)
			if err != nil {
				return err
			}
			if len(consolidations) == 0 {
				fmt.Println("No insights yet. Run a few reviews with --memory to build up patterns.")
				return nil
			}

			fmt.Println("Cross-PR Insights")
			fmt.Println("─────────────────")
			for i, c := range consolidations {
				fmt.Printf("%d. %s\n", i+1, c.InsightText)
				fmt.Printf("   (recorded %s)\n\n", c.CreatedAt.Format("2006-01-02"))
			}
			return nil
		},
	}
}
