# Contributing to claude-review

Thanks for your interest in contributing!

## Setup

```bash
git clone https://github.com/critbot/claude-review
cd claude-review
go test -race ./...    # run tests
make coverage-check    # verify coverage ≥ 40%
make build             # build binary to dist/
```

Requires Go 1.22+.

## Coverage

The project enforces a **40% minimum coverage** threshold on tested packages
(`internal/agents`, `internal/diff`, `internal/output`). The CI pipeline and
release pipeline both fail if coverage drops below this.

Network-bound code (GitHub/GitLab/Bitbucket API clients, Anthropic API runner,
git shell-out functions) is intentionally excluded from coverage requirements —
these are integration-tested separately.

```bash
make coverage       # generate coverage/coverage.out and print per-function table
make coverage-check # fail if total < 40%
make coverage-html  # open HTML report in browser
```

When adding new pure-logic code, add tests. When adding code that calls external
APIs, tests are not required but integration tests in a `_integration_test.go`
file (skipped unless `RUN_INTEGRATION=1`) are welcome.

## Running locally

```bash
export ANTHROPIC_API_KEY=sk-ant-...
go run ./cmd/claude-review diff HEAD~1
```

## Project structure

| Path | Purpose |
|------|---------|
| `cmd/claude-review/main.go` | CLI entry point (Cobra commands) |
| `internal/agents/` | Finder, verifier, ranker agents + pipeline orchestrator |
| `internal/diff/` | Diff parsers for local git, GitHub, GitLab, Bitbucket |
| `internal/output/` | Markdown, JSON, annotations formatters |
| `internal/config/` | Config loading and validation |
| `internal/hooks/` | Pre-commit hook install/remove |
| `internal/memory/` | v1.1 SQLite memory layer |
| `templates/` | Agent prompt templates (embedded into binary) |

## Adding a new focus area

1. Add a constant to `internal/config/config.go`
2. Create `templates/finder-<name>.md` with a focused prompt
3. Add it to `Defaults.Focus` in config

## Adding a new diff source

1. Create `internal/diff/<platform>.go` with a `FetchXxxPR()` function
2. Add an `IsXxxURL()` helper
3. Wire it into `cmd/claude-review/main.go` `buildPRCmd`
4. Add URL detection to `internal/diff/router.go`

## Pull requests

- Keep PRs focused — one feature or fix per PR
- Add tests for new logic
- Run `go vet ./...` before submitting
- Agent prompt changes should include example before/after outputs in the PR description

## Reporting issues

Please include: OS, Go version, the command you ran, and the error output.
