# claude-review — Implementation Checklist

## v1.0 Gaps (ship blockers)

- [x] Add `LICENSE` file (MIT)
- [x] Fix `strings.Title` deprecation in `internal/output/markdown.go:96` → use `golang.org/x/text/cases`
- [x] Wire `--files` flag to `diff` subcommand (calls `diff.GetFiles()` already in local.go)
- [x] Implement `output/annotations.go` for `--format annotations` (GitHub/GitLab inline comment JSON)
- [x] Implement Bitbucket PR support (`internal/diff/bitbucket.go` + URL detection)
- [x] PR shorthand: `claude-review pr 123` → detect `git remote get-url origin` and build full URL
- [x] Add `CONTRIBUTING.md`

## Tests

- [x] `internal/diff/parser_test.go` — unified diff parsing (added, deleted, renamed, binary, multi-hunk)
- [ ] `internal/diff/local_test.go` — staged/range diff (mock exec)
- [x] `internal/agents/runner_test.go` — JSON extraction from malformed LLM output
- [x] `internal/output/markdown_test.go` — markdown generation from mock findings
- [x] `internal/output/json_test.go` — JSON report structure
- [ ] `internal/agents/pipeline_test.go` — pipeline with mocked agent calls

## v1.1 — Memory Layer

- [x] `internal/memory/db.go` — SQLite schema + connection (3 tables: findings, consolidations, false_positives)
- [x] `internal/memory/ingest.go` — Ingest agent: stores findings post-review into memory.db
- [x] `internal/memory/query.go` — Query agent: retrieves relevant past findings before each review
- [x] `internal/memory/consolidation.go` — Consolidation agent: cross-PR pattern detection
- [x] `internal/memory/daemon.go` — Background daemon with PID file, launchd/systemd unit generation
- [x] Wire `--memory` flag to pipeline (call query agent before finders, ingest agent after)
- [x] `memory` subcommand: `start`, `stop`, `status`, `clear`, `install`
- [x] `insights` subcommand: plain-English summary of cross-PR patterns

## Coverage & CI

- [x] Add `make coverage` / `make coverage-html` / `make coverage-check` targets to Makefile
- [x] Set minimum coverage threshold at **40%** (enforced by `make coverage-check`)
- [x] Add `pipeline_test.go` — `ComputeCost`, `EstimateCost`, `aggregateUsage`, `SeverityCounts`, `truncateDiff`
- [x] Add `annotations_test.go` — `githubLevel`, `itoa`, `WriteAnnotations`
- [x] Add `cost_test.go` — `PrintCostSummary`, `PrintEstimate`
- [x] Add `types_test.go` — `SerializeDiff`, `SerializeFileSubset`
- [x] Add `.github/workflows/ci.yml` — tests + race detector + coverage gate + cross-platform build check on every push/PR
- [x] Update `.github/workflows/release.yml` — coverage gate as prerequisite job, checksums.txt in release assets, auto-update Homebrew tap
- [x] Update README.md — CI/CD section with badges and workflow tables
- [x] Update CONTRIBUTING.md — coverage policy and commands

## Infrastructure

- [x] Create `critbot/homebrew-tap` repo with `Formula/claude-review.rb`
- [ ] Push `critbot/claude-review` to GitHub and tag `v0.1.0`
- [ ] Set GitHub repo description and topics (`claude`, `code-review`, `ai`, `multi-agent`, `cli`)
