# claude-review

**Open-source multi-agent code review CLI powered by Claude.**

> Self-hosted · Bring-your-own-key · Works on GitHub, GitLab, Bitbucket, and local git repos

[![GitHub release](https://img.shields.io/github/v/release/critbot/claude-review)](https://github.com/critbot/claude-review/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

---

## Why claude-review?

Anthropic's built-in Code Review is $15–25/review, GitHub-only, and requires an Enterprise plan. `claude-review` fills every gap:

| Feature | Anthropic Code Review | claude-review |
|---|---|---|
| Cost | $15–25/review | ~$0.50–2.00 (your API key) |
| Platforms | GitHub only | GitHub, GitLab, Bitbucket, local |
| Plan required | Team/Enterprise | Any Anthropic API access |
| Self-hosted | No | Yes |
| Open source | No | MIT |

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

All agents run via the **Anthropic API** using your own key. No data is stored anywhere.

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

## Roadmap

- **v1.0** — Core multi-agent pipeline, GitHub/GitLab support, Markdown/JSON output
- **v1.1** — Memory layer: SQLite-backed persistent findings, cross-PR pattern detection
- **v1.2** — `--fix` flag: auto-apply suggested fixes with confirmation
- **v1.3** — Bitbucket support, Azure DevOps support

---

## Contributing

PRs welcome. See [CONTRIBUTING.md](CONTRIBUTING.md).

```bash
git clone https://github.com/critbot/claude-review
cd claude-review
go test ./...
make build
```

---

## License

MIT — see [LICENSE](LICENSE).
