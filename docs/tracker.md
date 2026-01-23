# Jul Build Tracker

## Current Focus (v0.4 Trace/Provenance)
- [ ] Trace system (side history) + refs (`refs/jul/traces`, `refs/jul/trace-sync`)
- [ ] `jul trace` command + trace metadata (prompt hash/summary/agent/session)
- [ ] Trace privacy + local storage (`.jul/traces/`, scrubber)
- [ ] Trace CI (lightweight attestations) + config (`run_on_trace`, `trace_checks`)
- [ ] `jul blame` (checkpoint + trace provenance)
- [ ] Replace prompt notes with trace metadata (remove `refs/notes/jul/prompts`)

## Current Focus (v0.3 Pivot: Local-First)
- [x] Local sync engine: shadow-index draft commits + `refs/jul/sync/<user>/<device>/<ws>`
- [x] Device ID + `workspace_base` tracking per workspace
- [x] Remote selection rules (origin fallback) + `jul remote set/show`
- [x] Update `jul init` and config to local-first defaults (`[remote]`, `[user]`)
- [x] Suggestion statuses (`pending/applied/rejected`) + `jul apply`
- [x] CI state files in `.jul/ci` + `jul ci status/cancel`

## Next Up
- [x] Checkpoint semantics: new checkpoint commit + new draft + keep-ref
- [x] Workspace ref lease + auto-merge flow (`jul sync`, `jul merge`)
- [x] Local metadata in notes (attestations, suggestions)
- [x] Notes for review (local-only by default)
- [x] Notes for prompts (optional, local-only by default) [superseded by trace metadata]
- [x] Agent sandbox + review pipeline (internal agent)
- [x] Agent headless prompt mode (external providers)
- [x] `jul ws checkout` + local workspace save/restore integration
- [x] Query/log/diff/show over local metadata
- [x] Local reflog + status summaries from refs/notes
- [x] CI config wiring for draft/background runs (run_on_draft, draft_ci_blocking)
- [x] Align `jul status --json` and human output with new spec (draft/checkpoint summaries)
- [x] Status shows working tree like git + porcelain option
- [x] Skip auto draft CI when no `.jul/ci.toml` configured
- [x] Clarify draft file counts relative to checkpoint base
- [x] Show module path for CI checks when using go.work
- [x] Add `jul ci config --show` for resolved commands
- [x] Hide stale draft CI when no config present
- [x] Keep stale draft CI visible when commands are inferred
- [x] Reuse draft commit when working tree unchanged
- [x] Adopt git commits as implicit checkpoints (opt-in)
- [x] Document checkpoint vs branch commits + adopt config
- [x] Sync output mentions background CI status
- [x] Document CI run types + visibility
- [x] Normalize CI commands (watch flag + list)
- [x] Add `jul review` (internal agent, worktree isolation)
- [x] Update smoke tests: local-only, Git remote, Jul-remote (optional)
- [x] GoReleaser + Homebrew packaging (bundle OpenCode)

## Completed (pre-pivot groundwork)
- [x] Monorepo scaffold (CLI/server/web/infra)
- [x] Go server skeleton + storage + SSE
- [x] Go CLI skeleton + server calls
- [x] Basic tests (storage + server + CLI parsing)
- [x] Git hooks for auto-sync (post-commit)
- [x] Keep-refs handling on server side (workspace reflog)
- [x] Smoke tests (server + CLI + git repo)
- [x] Promotion policy checks + ref updates
- [x] Attestation ingestion from CI runner
- [x] JSON output modes to CLI commands (`changes`, `sync`)
- [x] CI trigger endpoint + query API
- [x] Mirror attestations into git notes
- [x] Suggestions API + refs
- [x] Query endpoints (advanced filters)
- [x] CLI config wizard (agent/provider selection, default server, workspace)
- [x] Align CLI config format with v0.2 spec (server/workspace/init sections)
- [x] Workspace commands (`jul ws` list/set/switch/rename/delete)
