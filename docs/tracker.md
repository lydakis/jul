# Jul Build Tracker

## Priority Roadmap (Spec → Safe Daily Use)

**Note:** No backward compatibility guarantees; prioritize spec correctness.

### P0 — Repo Safety & Core Invariants (must‑fix before daily use)
- [x] **Workspace base tracking**: persist `base_ref` + pinned `base_sha` per workspace (e.g., `.jul/workspaces/<ws>/config`) and use it for diffs, suggestions, CRs, status, and divergence checks.
- [x] **`jul ws restack`**: rebase checkpoint chain onto `base_ref` tip; support `--onto`; update `base_sha`, move `HEAD`, emit **restack trace per rewritten checkpoint**, mark suggestions stale, trigger CI for new SHAs.
- [x] **Sync algorithm alignment**: `jul sync` must **not** rewrite workspace refs by default; only checkpoint/restack/checkout update base. Detect base advancement via draft parent vs workspace tip; update `workspace_lease` only when incorporated; allow safe clean FF of worktree only when draft tree == base tree; honor lease corruption rule.
- [x] **HEAD model**: keep `HEAD` on `refs/heads/jul/<workspace>` (base commit); update this ref on checkpoint/restack/checkout/switch/promote.
- [x] **Stacked promote (auto‑land stack)**: `jul promote` should land full stack bottom‑up, rebasing each layer onto the target branch; stop on conflict and require `jul merge`.
- [x] **Promote safety invariant**: fetch target tip; only fast‑forward update target by default; rename flags to `--no-policy` and `--force-target`; record per‑layer `promote_events` mapping.
- [x] **Stack base resolution**: when `base_ref` is a workspace, resolve base tip to **parent’s latest checkpoint** (not its draft).
- [x] **Trace correctness**: add `trace_type` metadata; update `jul blame` to skip merge+restack traces; ensure trace merge tree uses canonical workspace tip after sync.
- [x] **Incident 2026‑01‑24 regression**: add safeguards + tests to prevent target overwrite (see `docs/incidents/2026-01-24-main-overwrite.md`).

### P1 — Core Workflow Completeness
- [x] **Repo meta + user namespace**: implement `refs/notes/jul/repo-meta` + stable `user_namespace` resolution for ref paths.
- [x] **Change refs**: stable per‑change tips (`refs/jul/change/<change-id>`) + anchor refs.
- [x] **Draft handoff**: `jul draft list/show/adopt` (per‑device drafts) + explicit adopt merge flow.
- [x] **Sync safety features**: draft secret scan before push (`--allow-secrets` override), `.jul/syncignore` support.
- [x] **Tracked base drift**: persist `track-tip` for publish branch; surface base‑advanced in status; update only on restack/checkout/promote.
- [x] **`jul doctor`**: verify remote supports custom refs + non‑FF updates under `refs/jul/*` (and report fallbacks).
- [x] **`jul log --traces`**: show trace history in log output.
- [x] **Suggestions default filter**: exclude stale suggestions unless explicitly requested.
- [x] **CI + privacy alignment**: sync only structured attestation fields by default; gate agent‑review summaries and CI output snippets behind opt‑in + scrubber.
- [x] **Restack attestation inheritance**: store `attestation_inherit_from` on rebased checkpoints and display prior results as **stale** (display‑only, never gating).
- [x] **Sync modes**: implement `on-command|continuous|explicit` with config wiring.
- [x] **Sync auto‑restack (opt‑in)**: implement `sync.autorestack=true` to allow sync to restack checkpoints when safe (per spec note).
- [x] **Retention cleanup**: `jul prune`, keep‑ref cascading cleanup (suggestions/notes) + CR anchor pinning.

### P2 — UX + Tooling
- [ ] **`jul remote clear`** command.
- [ ] **`jul git` passthrough** (thin wrapper for git commands).
- [ ] **Promote policies**: `.jul/policy.toml` enforcement for CI/coverage/etc.
- [ ] **Output polish**: align human output with spec (icons/colors/layout).
- [ ] **Smoke‑test plan + coverage**: design doc + full local/remote/opencode scenarios.
- [ ] **Server scope cleanup**: remove legacy APIs; keep only git‑remote + frontend support.
- [ ] **Git‑remote service**: upload‑pack/receive‑pack, non‑FF custom refs for `refs/jul/*`.

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
- [x] Trace system (side history) + refs (`refs/jul/traces`, `refs/jul/trace-sync`)
- [x] `jul trace` command + trace metadata (prompt hash/summary/agent/session)
- [x] Trace privacy + local storage (`.jul/traces/`, scrubber)
- [x] Trace CI (lightweight attestations) + config (`run_on_trace`, `trace_checks`)
- [x] `jul blame` (checkpoint + trace provenance)
- [x] Replace prompt notes with trace metadata (remove `refs/notes/jul/prompts`)
- [x] Spec clarifications (trace canonical gating, notes merge, adopt Change-Id)
- [x] Local sync engine: shadow-index draft commits + `refs/jul/sync/<user>/<device>/<ws>`
- [x] Device ID + `workspace_lease` tracking per workspace
- [x] Remote selection rules (origin fallback) + `jul remote set/show`
- [x] Update `jul init` and config to local-first defaults (`[remote]`, `[user]`)
- [x] Suggestion statuses (`pending/applied/rejected`) + `jul apply`
- [x] CI state files in `.jul/ci` + `jul ci status/cancel`
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
- [x] Suggestions API + refs
- [x] Query endpoints (advanced filters)
- [x] CLI config wizard (agent/provider selection, default server, workspace)
- [x] Align CLI config format with v0.2 spec (server/workspace/init sections)
- [x] Workspace commands (`jul ws` list/set/switch/rename/delete)
