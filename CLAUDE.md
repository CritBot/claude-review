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
- [x] Push `critbot/claude-review` to GitHub and tag `v0.1.0`
- [ ] Set GitHub repo description and topics (`claude`, `code-review`, `ai`, `multi-agent`, `cli`)
- [ ] Set `TAP_GITHUB_TOKEN` secret in repo settings (needed for auto-update-homebrew-tap job in release workflow)

## v1.2.0 — `--fix` Auto-Apply

- [ ] Implement `--fix` flag: parse `suggested_fix` field from findings and apply to source files
- [ ] Show a unified diff of each proposed change before applying
- [ ] Prompt for confirmation per-finding (skip in CI when `--yes` flag is passed)
- [ ] Guard: verify the target line content still matches what the LLM saw before patching (abort if file changed)
- [ ] Support `--yes` flag to apply all fixes non-interactively (for scripted use)
- [ ] Add `--fix` tests covering apply, skip, and mismatch scenarios

## v1.2.0 — `.reviewrc.yml` Custom Rules

Custom YAML rules file injected into finder agent prompts — enterprise adoption driver.

- [ ] Define schema: array of named rules with `id`, `focus` area, `severity`, `description`, and optional `pattern` (regex) + `example` fields
- [ ] Load `.reviewrc.yml` from repo root (walk up to git root if not found)
- [ ] Merge with global `~/.claude-review/rules.yml` (local takes precedence on conflicts)
- [ ] Serialize active rules into a `## Custom Rules` block prepended to each finder agent prompt alongside memory context
- [ ] Add `--rules` CLI flag to specify an alternate rules file path
- [ ] Add `rules` subcommand: `validate` (schema-check), `list` (show active), `init` (scaffold a starter file)
- [ ] Support `ignore` rules: patterns that suppress a finding (complement to false-positive suppression in memory)
- [ ] Tests: rule loading, merge precedence, prompt injection, `validate` schema errors

## v1.2.0 — Context Scraper

Seeds memory on day one by scraping existing PR history — eliminates the cold-start problem.

- [ ] `internal/context/scraper.go` — fetches closed PRs from GitHub/GitLab/Bitbucket APIs (paginated)
- [ ] Per-PR: title, description, inline review comments, approval/rejection outcome, merged-by info
- [ ] Feed scraped data to a new Context Ingest Agent that converts PR history into memory-format findings (file hotspots, recurring comment themes, team preferences)
- [ ] `claude-review context scrape` command: `--limit N` (default 100), `--since <date>`, `--dry-run`
- [ ] Progress output: "Scraped 47/100 PRs…" with spinner
- [ ] Store scraped signals in a new `context_events` table in `memory.db` (separate from agent findings to preserve provenance)
- [ ] Consolidation agent updated to read `context_events` alongside `findings` — "your team always rejects performance suggestions on protobuf files"
- [ ] Tests: GitHub/GitLab/Bitbucket API mocks, context ingest agent JSON parsing, consolidation integration

## v1.3.0 — CVE Database Integration

Elevates security findings from "suspicious pattern" to "known vulnerability class".

- [ ] After verifier phase, for each `security` finding, run a new CVE Lookup Agent
- [ ] Agent queries OSV API (`api.osv.dev`) first (free, no key, JSON), falls back to NVD (`services.nvd.nist.gov`) if no results
- [ ] Framing: "This pattern is **similar to** CVE-2021-44228 (Log4Shell)" — never "this IS CVE-X"; include confidence note
- [ ] Attach `cve_references: []string` to the `Finding` struct (omitempty in JSON output)
- [ ] In Markdown output: render CVE references as a collapsible "Related CVEs" block below the finding
- [ ] Cache CVE lookups in `memory.db` (`cve_cache` table, TTL 7 days) to avoid redundant API calls
- [ ] `--no-cve` flag to disable lookups (for air-gapped environments)
- [ ] Tests: OSV API mock, NVD fallback mock, cache hit/miss, Markdown rendering with CVE block

## v1.3.0 — Plugin / Recipe System

Named bundles of prompts + rules + CVE categories — community moat.

- [ ] Define recipe format: YAML file with `name`, `version`, `description`, `focus` overrides, `rules` (array of rule objects), `cve_categories` (filter CVE lookups to relevant CWE classes), `prompt_extras` (strings appended to each finder system prompt)
- [ ] Built-in recipes: `default` (current behavior), `fintech-pci` (PCI-DSS focused), `hipaa`, `react-performance`, `go-concurrency`
- [ ] Load recipes from `~/.claude-review/recipes/` directory and from `--recipe <name-or-path>` flag
- [ ] Recipe resolution: name → look in built-ins → look in `~/.claude-review/recipes/` → treat as file path
- [ ] `claude-review recipe list` — list available recipes with descriptions
- [ ] `claude-review recipe show <name>` — dump full YAML of a recipe
- [ ] `claude-review recipe init <name>` — scaffold a new recipe file
- [ ] Community marketplace: document `critbot/recipes` repo as central index; each recipe is a standalone `.yml` file installable via `curl` or a future `recipe install` command
- [ ] Tests: recipe loading, built-in fallback, prompt injection, `--recipe` flag resolution

## v1.3.0 — Azure DevOps Support

- [ ] Implement `internal/diff/azuredevops.go` — fetch PR diff via Azure DevOps REST API
- [ ] URL detection: `dev.azure.com/org/project/_git/repo/pullrequest/ID`
- [ ] Wire into `diff/router.go` and `cmd/claude-review/main.go` `buildPRCmd`
- [ ] Add `azure_devops_token` to config + `AZURE_DEVOPS_TOKEN` env var
- [ ] Update README install/CI example with Azure DevOps URL format

## v2.0.0 — Self-Hosting / On-Prem

Enterprise play: on-prem deployment, alternative AI backends, multi-user.

- [ ] Abstract the Anthropic HTTP client behind an `LLMClient` interface in `internal/llm/` — swap implementations without touching agent code
- [ ] Implement `BedrockClient` (AWS Bedrock, Claude model ARNs) and `VertexClient` (GCP Vertex AI) behind the interface
- [ ] Config: `llm_backend: bedrock | vertex | anthropic` (default), with backend-specific credential fields
- [ ] Abstract `memory.DB` behind a `Store` interface; implement `PostgresStore` alongside the existing `SQLiteStore`
- [ ] `claude-review server` command: HTTP API wrapper that accepts diff payloads and returns JSON findings — enables web UIs and IDE plugins
- [ ] Multi-user: per-user API key isolation, shared org-level memory (opt-in), RBAC for `insights` and `memory clear`
- [ ] Web UI: minimal React dashboard showing `insights` output, finding history, false-positive management
- [ ] Design constraint: all v1.x code must compile and run without the server component; server is additive
- [ ] CHANGELOG.md, migration guide from v0.x, semantic versioning commitment

## v2.0.0 — Stable Public Go API

- [ ] Define stable public Go API (exported types in `pkg/` for embedding in other tools)
- [ ] Plugin interface: allow custom focus-area agents via external Go plugins or config-defined prompts
- [ ] Deprecation policy and CHANGELOG.md
- [ ] Semantic versioning commitment + migration guide from v0.x
