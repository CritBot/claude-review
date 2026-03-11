# claude-review

**Open-source multi-agent code review CLI powered by Claude — with a memory that learns your codebase.**

> Self-hosted · Bring-your-own-key · Works on GitHub, GitLab, Bitbucket, and local git repos

[![GitHub release](https://img.shields.io/github/v/release/critbot/claude-review)](https://github.com/critbot/claude-review/releases)
[![CI](https://github.com/critbot/claude-review/actions/workflows/ci.yml/badge.svg)](https://github.com/critbot/claude-review/actions/workflows/ci.yml)
[![Coverage](https://img.shields.io/badge/coverage-40%25%2B-brightgreen)](https://github.com/critbot/claude-review/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

---

## What makes this different

Most AI code review tools — including Anthropic's own $25/review managed service — treat every PR in isolation. They look at the diff, produce findings, and forget everything. The next PR starts from zero.

`claude-review` doesn't.

After every review, findings are stored locally in a SQLite database. A background daemon wakes every 30 minutes and uses Claude to find patterns across your entire review history — patterns a single-PR review can never surface. Before the next review starts, those patterns are injected into every finder agent's prompt. The tool knows which files in your repo are hotspots and which categories of bugs keep slipping through.

After a month of use, your instance of `claude-review` knows things about your codebase that Anthropic's managed service never will — because theirs resets to zero on every PR. The memory is entirely local, entirely yours.

**The three jobs the always-on memory agent runs as a persistent local process:**

- **Ingest** — after every review run, all findings (file, line, severity, category, whether you later rejected them as noise) are stored in `.claude-review/memory.db`. Automatic, no configuration.
- **Consolidate** — the daemon wakes every 30 minutes (or after 10 new findings), reads across all stored findings, and calls Claude with *metadata only — no source code* — to find non-obvious cross-PR patterns. "Your payments module has had null pointer bugs in 4 of the last 6 PRs." "Every time someone touches `auth.ts`, a critical slips through." Costs fractions of a cent per cycle.
- **Query** — before the next review starts, finder agents query memory for the files being changed. They already know which areas are hotspots and which findings you've previously rejected as noise.

This is built on SQLite. No vector database, no cloud sync, no embeddings.

---

## Why claude-review vs the alternatives

| Feature | Anthropic Code Review | claude-review |
|---|---|---|
| Cost | $15–25/review | ~$0.50–2.00 (your API key) |
| Platforms | GitHub only | GitHub, GitLab, Bitbucket, local |
| Plan required | Team/Enterprise | Any Anthropic API access |
| Self-hosted | No | Yes |
| Open source | No | MIT |
| **Memory across PRs** | **No — resets to zero every PR** | **Yes — learns your codebase over time** |
| Cross-PR pattern detection | No | Yes — background consolidation daemon |
| False positive suppression | No | Yes — rejected findings are remembered |

---

## Installation

### macOS (Homebrew)
```bash
brew install critbot/tap/claude-review
```

### Linux / macOS (direct download)
```bash
curl -sSL https://github.com/critbot/claude-review/releases/latest/download/claude-review-$(uname -s | tr '[:upper:]' '[:lower:]')-$(uname -m | sed 's/x86_64/amd64/').tar.gz | tar xz
sudo mv claude-review /usr/local/bin/
```

### Build from source
```bash
git clone https://github.com/critbot/claude-review
cd claude-review
make install
```

---

## Quick Start

```bash
export ANTHROPIC_API_KEY=sk-ant-...

# Review staged changes
git add .
claude-review

# Review last commit
claude-review diff HEAD~1

# Review a branch diff
claude-review diff main..my-feature

# Review a GitHub PR
claude-review pr https://github.com/org/repo/pull/123

# Review a GitLab MR
claude-review pr https://gitlab.com/org/project/-/merge_requests/45

# PR number shorthand (auto-detects remote)
claude-review pr 123
```

---

## How It Works

`claude-review` runs a **4-phase multi-agent pipeline**:

```
Phase 1–3 (parallel):   Finder agents
  ├── Logic bug finder
  ├── Security vulnerability finder
  ├── Performance issue finder
  ├── Type safety finder
  └── Test coverage finder

Phase 4a (sequential):  Verifier agent
  └── Deduplicates, filters low-confidence findings

Phase 4b (sequential):  Ranker agent
  └── Sorts by severity, elevates critical-file issues
```

All agents run via the **Anthropic API** using your own key. No data is stored anywhere except your local `memory.db`.

---

## Always-On Memory

The memory layer is enabled with `--memory`. Enable it once in your project config and forget about it.

```bash
# Start the background daemon (runs consolidation every 30 min or after 10 new findings)
claude-review memory start

# Run a review with memory context
claude-review diff --memory
claude-review pr 123 --memory

# See what the daemon has learned about your codebase
claude-review insights
```

The database lives at `.claude-review/memory.db` in your repo root (already in `.gitignore`). The daemon writes a PID file and log to `~/.claude-review/`.

**What gets stored:** file path, line, severity, category, description, PR reference, whether you accepted or rejected the finding. No source code is ever written to disk or sent to the API during consolidation — only this metadata.

**False positive suppression:** if you mark a finding as noise twice, it's suppressed for the rest of the project's lifetime. The threshold is 2 rejections before suppression kicks in, to avoid accidentally silencing real bugs.

---

## Commands

```bash
claude-review                          # Review staged changes (default)
claude-review diff HEAD~1              # Review last commit
claude-review diff main..feature       # Review branch diff
claude-review pr <url>                 # Review GitHub PR or GitLab MR

claude-review diff --format json       # Output as JSON (for CI/bots)
claude-review diff --estimate          # Show cost estimate, don't run
claude-review diff --agents 6          # Use 6 finder agents
claude-review diff --focus security    # Security-only review
claude-review diff --model claude-sonnet-4-6  # Use Sonnet for deeper analysis
claude-review diff --memory            # Memory-augmented review

claude-review memory start             # Start consolidation daemon
claude-review memory stop              # Stop daemon
claude-review memory status            # Show daemon status and DB stats
claude-review memory clear             # Wipe all stored findings
claude-review memory install           # Install daemon as launchd/systemd service
claude-review insights                 # Plain-English summary of cross-PR patterns

claude-review install-hook             # Add as pre-commit hook
claude-review install-hook --remove    # Remove the hook
```

---

## Output

### Markdown (default — `REVIEW.md`)

```markdown
# Code Review Report
Branch: feature/auth-refactor → main
Changes: +142 -38 across 8 files

## Summary
| Severity | Count |
|----------|-------|
| 🔴 Critical | 1 |
| 🟠 High | 2 |
| 🟡 Medium | 4 |
| 💡 Suggestion | 1 |

Cost: $0.43 · Agents: 5 finder + verifier + ranker · Time: 38s

---

## 🔴 Critical

### JWT expiry is never validated before granting access

**File**: `src/auth/token.go` · **Line**: 87 · **Confidence**: 95%

The decoded token's expiry (`exp` claim) is extracted but never compared against
the current time. Any expired token will be accepted as valid.

**Suggested Fix**:
if claims.ExpiresAt.Unix() < time.Now().Unix() {
    return nil, ErrTokenExpired
}
```

### JSON (`--format json`)

Structured output for CI pipelines, GitHub comment bots, Slack bots, dashboards:

```json
{
  "generated_at": "2026-03-11T14:30:00Z",
  "source": "github-pr",
  "summary": { "critical": 1, "high": 2, "medium": 4, "suggestion": 1 },
  "findings": [...],
  "cost": { "input_tokens": 12400, "output_tokens": 3200, "estimated_usd": 0.43 }
}
```

---

## Configuration

Create `claude-review.config.json` in your project root:

```json
{
  "agents": 4,
  "focus": ["logic", "security", "performance", "types", "tests"],
  "model": "claude-haiku-4-5-20251001",
  "confidence_threshold": 0.80,
  "output": "REVIEW.md",
  "max_tokens_per_agent": 4000,
  "max_cost_usd": 5.00
}
```

| Field | Default | Description |
|-------|---------|-------------|
| `agents` | 4 | Number of parallel finder agents |
| `focus` | all 5 | Which concern areas to review |
| `model` | haiku | Claude model to use |
| `confidence_threshold` | 0.80 | Minimum confidence to include a finding |
| `output` | REVIEW.md | Output filename |
| `max_tokens_per_agent` | 4000 | Max output tokens per agent call |
| `max_cost_usd` | 0 (disabled) | Abort if estimated cost exceeds this |

### Models

| Model | Speed | Cost/review | Best for |
|-------|-------|-------------|---------|
| `claude-haiku-4-5-20251001` | Fast | ~$0.50–2.00 | Most PRs (default) |
| `claude-sonnet-4-6` | Medium | ~$2–8 | Complex/large PRs |
| `claude-opus-4-6` | Slow | ~$10–30 | Critical security audits |

---

## CI Integration

```yaml
# .github/workflows/review.yml
name: Code Review
on: [pull_request]

jobs:
  review:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Install claude-review
        run: |
          curl -sSL https://github.com/critbot/claude-review/releases/latest/download/claude-review-linux-amd64.tar.gz | tar xz
          sudo mv claude-review /usr/local/bin/

      - name: Review PR
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: |
          claude-review pr ${{ github.event.pull_request.html_url }} --format json --output review.json

      - name: Upload review
        uses: actions/upload-artifact@v4
        if: always()
        with:
          name: code-review
          path: review.json
```

---

## CI/CD

Two GitHub Actions workflows are included:

### `ci.yml` — runs on every push and PR
| Step | Details |
|------|---------|
| `go vet` | Static analysis |
| `go test -race ./...` | Tests with race detector |
| Coverage report | Generates `coverage/coverage.out` |
| **Coverage gate** | Fails build if total coverage drops below **40%** |
| Cross-platform build | Verifies binary compiles for linux/darwin/windows × amd64/arm64 |

The coverage threshold is enforced in both CI and the release pipeline — a tag push cannot produce a release if coverage falls below the minimum.

### `release.yml` — runs on `v*` tag push
| Step | Details |
|------|---------|
| Tests + coverage gate | Same checks as CI — must pass before release |
| Cross-compile | Builds binaries for all 5 platforms |
| `sha256sum` | Generates `checksums.txt` and attaches it to the release |
| GitHub Release | Creates release with all binaries and auto-generated notes |
| Homebrew tap | Auto-updates `critbot/homebrew-tap` with new version and SHA256s |

### Local coverage commands
```bash
make coverage          # run tests and print per-function coverage
make coverage-check    # fail if total < 40%
make coverage-html     # open HTML report in browser
```

---

## Roadmap

- **v0.1.0** ✅ — Core multi-agent pipeline (finder/verifier/ranker), GitHub/GitLab/Bitbucket support, Markdown/JSON/annotations output, cost transparency, pre-commit hook, `--files` flag, PR number shorthand
- **v1.1** ✅ — Always-on memory layer: SQLite-backed persistent findings, 30-minute consolidation daemon, cross-PR pattern detection, `memory` and `insights` subcommands
- **v0.2.0** — `--fix` flag: auto-apply suggested fixes with diff preview and confirmation
- **v0.3.0** — Azure DevOps support
- **v1.0.0** — Stable API, plugin system for custom focus areas

---

## Contributing

PRs welcome. See [CONTRIBUTING.md](CONTRIBUTING.md).

```bash
git clone https://github.com/critbot/claude-review
cd claude-review
go test -race ./...        # run tests
make coverage              # generate coverage report
make coverage-check        # verify ≥40% threshold
make build                 # build binary to dist/
```

---

## License

MIT — see [LICENSE](LICENSE).
