# Jul Build Tracker

## Current Focus (v0.5 Spec Alignment & UX)
- [ ] No backward compatibility guarantees; prioritize spec correctness for all new changes
- [x] Remove server API dependency (`JUL_BASE_URL`/client calls); all commands operate locally + git remote only
- [x] Implement `jul merge` (conflict resolution flow) per spec
- [ ] Implement `jul submit` (one workspace = one review) + `review-state`/`review-comments` notes
- [ ] Implement `jul ws stack` (stacked workspaces, based on latest checkpoint; require checkpoint)
- [ ] Implement `jul local` save/restore/list/delete (client-side workspace states)
- [ ] Implement `jul ws new` to create workspace + start draft (not just set config)
- [ ] Implement `jul ws switch` to save/restore local state + sync + fetch + lease update
- [ ] Add `jul remote clear` to unset remote selection
- [ ] Align `jul init` with spec: start draft, ensure workspace ready, align output
- [ ] Align `jul suggestions` default to current base (exclude stale suggestions unless requested)
- [ ] Add `jul log --traces` (trace history in log output)
- [ ] Implement `jul doctor` to verify remote refspecs + non‑FF support for `refs/jul/*`
- [x] Sync idempotency: reuse draft commit when tree unchanged (avoid new commit per status)
- [x] Base divergence detection: compare draft parents before auto-merge
- [ ] Change-Id lifecycle: new Change-Id after promote
- [x] Change-Id lifecycle: keep same Change-Id across checkpoints
- [x] Promote writes `promote_events` + `anchor_sha` into `refs/notes/jul/meta` (for revert)
- [ ] Add sync modes (`on-command`/`continuous`/`explicit`) and config wiring
- [ ] Enforce promote policies + strategies (`.jul/policy.toml`, rebase/squash/merge)
- [ ] Add retention cleanup for keep-refs + cascading suggestion/notes cleanup
- [ ] Add explicit cleanup commands (`jul ws close`, `jul prune`) per retention policy
- [ ] Pin review anchor keep-refs while review is open (retention is last-touched)
- [ ] Align CLI human output with spec (icons/colors/layout for status/sync/ci/checkpoint/log)
- [ ] Align Change-Id lifecycle + base-commit terminology in outputs/errors (e.g., `base_diverged`)
- [x] Rename workspace lease file (`.jul/workspaces/<ws>/lease`) and update code/config from `workspace_base`
- [x] Suggestion staleness uses `suggestion.base_sha == parent(current_draft)`
- [ ] Add `trace_type` metadata and have `jul blame` skip merge traces
- [ ] Trace merge tree should use canonical workspace tip after sync (not local draft tree)
- [ ] Fix sync draft creation when `.jul` is gitignored (no hard failure)
- [ ] Expand smoke tests: full local-only flow, remote flow, opencode review, CI config/no-config
- [ ] Define server scope: git-remote compatibility only + frontend API/static hosting
- [ ] Remove legacy server API endpoints not needed for git-remote + frontend
- [ ] Implement git-remote HTTP service (upload-pack/receive-pack) with non‑FF custom refs for `refs/jul/*`
- [ ] Server repo management: bare repo create/list + auth/ACLs for refs (future)
- [ ] Draft smoke-test design doc (happy paths, sad paths, edge cases, complex interactions)
- [ ] Implement comprehensive smoke/integration tests from the design doc
- [ ] Replace review notes: move from `refs/notes/jul/review` to `review-state` + `review-comments`
- [ ] Remove legacy commands not in spec (e.g., `jul changes`, `jul clone`) or reintroduce in spec

## Current Focus (v0.4 Trace/Provenance)
- [x] Trace system (side history) + refs (`refs/jul/traces`, `refs/jul/trace-sync`)
- [x] `jul trace` command + trace metadata (prompt hash/summary/agent/session)
- [x] Trace privacy + local storage (`.jul/traces/`, scrubber)
- [x] Trace CI (lightweight attestations) + config (`run_on_trace`, `trace_checks`)
- [x] `jul blame` (checkpoint + trace provenance)
- [x] Replace prompt notes with trace metadata (remove `refs/notes/jul/prompts`)
- [x] Spec clarifications (trace canonical gating, notes merge, adopt Change-Id)

## Current Focus (v0.3 Pivot: Local-First)
- [x] Local sync engine: shadow-index draft commits + `refs/jul/sync/<user>/<device>/<ws>`
- [x] Device ID + `workspace_lease` tracking per workspace
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
