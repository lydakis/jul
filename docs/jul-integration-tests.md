# Jul Integration Test Specification

**Document Version:** 0.3  
**Last Updated:** 2026-02-01  
**Based On:** _줄 Jul: AI-First Git Workflow_ (Design Spec v0.3)

## 0. Purpose and Scope

This document defines a comprehensive integration test plan (“IntecTests”) for **Jul** as described in the
design specification v0.3. The goal is to validate correctness and production safety for day-to-day
operations and edge cases, with special emphasis on:

- No silent data loss (drafts, checkpoints, metadata).
- Git invariants remain intact (refs, notes, `HEAD` model).
- Safe multi-device behavior under remote capability constraints.
- Predictable behavior under failures (network, disk, corruption, concurrency).
- Agent-first workflows (headless JSON, trace/provenance, review layer).

This plan covers **Jul Core** and optionally the **Review Layer**.

---

## 1. Definition of Done (Acceptance Criteria)

Jul is “production-safe” for single-player workflows when all of the following hold across the scenario suite:

1. **No Silent Data Loss**
   - Local edits are always recoverable via drafts and/or checkpoints.
   - Remote sync never destroys existing remote state without explicit override.

2. **Git Invariants Remain Sane**
   - `HEAD` is never left detached; it points to `refs/heads/jul/<workspace>` as specified.
   - Jul writes only within the specified `refs/jul/*` and `refs/notes/jul/*` namespaces (except `jul promote` which updates `refs/heads/<branch>` by design).

3. **Base/Lease Rules Are Obeyed**
   - “Base advanced” is detected and reported.
   - Draft base is not silently rewritten after checkpoints exist.

4. **Policy Gating Works**
   - Promote blocks when required checks/coverage/policies fail.
   - Bypasses require explicit flags (`--no-policy`, `--force-target`, etc.).

5. **Sync Is Capability-Correct**
   - Works in local-only mode.
   - Degrades gracefully when the sync remote blocks custom refs, notes, or non-fast-forward draft updates.

6. **Security Defaults Are Honored**
   - `.jul/**` never leaks into drafts.
   - Secret scanning blocks draft pushes by default.
   - Privacy defaults for prompts/summaries are respected.

---

## 2. Test Harness and Fixtures

### 2.1 Repository Fixtures

Create a small fixture repository per test run (temp directory), with:

- Source code + tests (at least 2 source files, 2 test files).
- A deterministic local checks configuration (e.g., `.jul/ci.toml` or equivalent).
- Mechanisms to intentionally:
  - Fail lint
  - Fail tests
  - Drop coverage below a threshold
  - Produce very large stdout/stderr output

### 2.2 Remote Simulation (Critical)

Most correctness bugs live in “remote capability + multi-device” behavior. Use local **bare** remotes with
server-side hooks that simulate:

1. Full compatibility: accepts `refs/jul/*`, `refs/notes/jul/*`, and allows non-FF updates for `refs/jul/sync/*`.
2. FF-only draft namespace: accepts `refs/jul/*` and notes, rejects non-FF updates for `refs/jul/sync/*`.
3. No custom refs: rejects `refs/jul/*` and optionally `refs/notes/jul/*`.
4. Notes blocked: accepts custom refs, rejects `refs/notes/jul/*`.
5. Selective protections: reject updates in specific namespaces unless fast-forward.

Also include a “flaky remote” mode that fails intermittently to test retry/resume behavior.

### 2.3 Multi-Device Simulation

Simulate “two devices” as two clones of the same remote, each with isolated:

- `HOME` / `XDG_CONFIG_HOME` (so device IDs differ)
- Per-repo `.jul/` state

### 2.4 Inspection Helpers

Provide helpers to:

- Resolve refs: `git rev-parse <ref>`
- Enumerate namespaces: `git for-each-ref refs/jul/...`
- Read notes: `git notes --ref refs/notes/jul/... show <sha>`
- Verify ancestry: `git merge-base --is-ancestor A B`
- Verify commit trailers (Change-Id, Trace-Base/Head, Jul-Type)
- Verify tree equality: `git diff --name-status <a> <b>` is empty
- Verify working tree & index unchanged when required

---

## 3. Scenario Template

Each scenario is specified as:

- **ID:** Stable test identifier
- **Covers:** Features/invariants
- **Setup:** Preconditions
- **Steps:** Commands/edits
- **Assertions:** Required outcomes (refs, notes, output, exit codes)
- **Variants:** Important permutations

---

# A) Initialization, Identity, and Remote Capability Detection

### IT-INIT-001 — Init in a Clean Repo With No Remotes (Local-Only)

**Covers:** Local-first mode; device ID creation; `.jul/` ignore; default workspace `@` normalization.  
**Setup:** `git init`, no remotes.  
**Steps:** Run `jul init`, then `jul status`.  
**Assertions:**
- Jul reports “no remote configured / working locally”.
- A device ID exists and is stable across runs.
- `.jul/` is ignored (both `.gitignore` and/or `.git/info/exclude` per implementation).
- `HEAD` points to `refs/heads/jul/<workspace>` (not detached).

### IT-INIT-002 — Init With `origin` Present (Auto-Select Sync Remote = origin)

**Covers:** Remote selection logic; refspec config; early notes fetch.  
**Setup:** Repo has remote `origin`; remote is “full compatibility”.  
**Steps:** `jul init`.  
**Assertions:**
- Sync remote = `origin` unless configured otherwise.
- Publish remote = `origin` unless overridden.
- Jul refspecs are configured for workspace/sync/traces/notes (excluding `refs/heads/jul/*`).
- Capability detection reports checkpoint sync and draft sync available.

### IT-INIT-003 — Init With Multiple Remotes and No `jul`/`origin`

**Covers:** Explicit remote selection requirement.  
**Setup:** Remotes `upstream` and `personal` exist; neither named `jul` or `origin`.  
**Steps:** `jul init`.  
**Assertions:**
- Jul does not silently pick a remote; it prompts for `jul remote set <name>`.
- Local-only workspace still initializes.

### IT-DOCTOR-001 — Doctor Detects “No Custom Refs”

**Covers:** Capability probing; safe downgrade.  
**Setup:** Remote hook rejects updates under `refs/jul/*` and `refs/notes/jul/*`.  
**Steps:** `jul remote set <remote>` then `jul doctor`.  
**Assertions:**
- Detected: “Checkpoint sync unavailable”.
- `jul sync` does not attempt pushing jul refs/notes; still updates local drafts.
- `jul promote` still works against publish remote (if configured).

### IT-DOCTOR-002 — Doctor Detects “Draft Sync Blocked” (Non-FF Rejected)

**Covers:** Non-FF probing for `refs/jul/sync/*`.  
**Setup:** Remote accepts `refs/jul/*` and notes, rejects non-FF updates for `refs/jul/sync/*`.  
**Steps:** `jul doctor`.  
**Assertions:**
- Checkpoint sync OK; draft sync unavailable.
- `jul sync` updates local draft but does not push draft ref; reports warning.
- Checkpoints and notes still sync.

### IT-IDENTITY-001 — User Namespace Canonicalization via Repo-Meta Notes

**Covers:** Stable `<user>` across devices.  
**Setup:** Remote already contains `refs/notes/jul/repo-meta` with `user_namespace = X`.  
**Steps:** Device A clone: `jul init`. Device B fresh clone: `jul init`.  
**Assertions:**
- Both devices use the same `<user>` ref path segment.
- If local cached namespace differs, repo-meta note wins (with a clear repair message).

### IT-IDENTITY-002 — Repo-Meta Conflict Requires Explicit Repair

**Covers:** Conflict rule: repo-meta must not auto-merge.  
**Setup:** Force a conflicting repo-meta note from two devices.  
**Steps:** Third device runs `jul sync`.  
**Assertions:**
- Jul refuses silent merge; enters “needs repair”.
- Jul does not silently fork a second `<user>` namespace.

---

# B) Workspace and `HEAD` Model (Git Compatibility)

### IT-HEAD-001 — `HEAD` Always Points to `refs/heads/jul/<workspace>`

**Covers:** `HEAD` model invariants.  
**Setup:** Initialized repo.  
**Steps:** Run `jul checkpoint`, `jul sync`, `jul status`, `jul review`, `jul trace`.  
**Assertions:**
- After each command, `git symbolic-ref HEAD` points to `refs/heads/jul/<ws>`.
- Commands that must not move `HEAD` do not.
- Commands that must move `HEAD` do so (checkpoint, ws checkout/switch/restack, promote).

### IT-WS-001 — Create Workspace; Verify Base/Track Defaults

**Covers:** `jul ws new`; base pinning; workspace-meta.  
**Setup:** `main` exists; publish remote configured.  
**Steps:** `jul ws new feature-auth`.  
**Assertions:**
- `refs/heads/jul/feature-auth` exists and is checked out.
- `refs/jul/workspaces/<user>/feature-auth` exists.
- Workspace-meta note is written with base_ref, track_ref, base_sha, workspace_id.

### IT-WS-002 — Workspace Name Normalization (`@` → `default`)

**Covers:** Ref naming normalization.  
**Steps:** Operate on `@`; inspect refs.  
**Assertions:**
- Refs use `default` internally; CLI may show `@`.

### IT-WS-003 — Switching Workspaces Preserves Dirty State and Index

**Covers:** `jul ws switch`; local save/restore of working tree + staging.  
**Setup:** Two workspaces exist. In workspace A create:
- Modified tracked file
- Untracked file
- Staged hunks via `git add -p`
**Steps:** `jul ws switch <wsB>`, then back to `<wsA>`.  
**Assertions:**
- Workspace A’s working tree and index are restored exactly.
- Workspace B is unaffected.

### IT-WS-004 — Manual `git switch` Away From Jul Branch Triggers Warning

**Covers:** Out-of-band git operation detection.  
**Steps:** `git switch main` manually, then `jul status`.  
**Assertions:**
- Jul warns that `HEAD` is not on `refs/heads/jul/<workspace>`.
- Jul suggests recovery via `jul ws checkout @` (or equivalent).

---

# C) Drafts and Shadow Index (Day-to-Day Editing Safety)

### IT-DRAFT-001 — `jul sync` Updates Draft Without Touching Git Index

**Covers:** Shadow index; no staging interference.  
**Setup:** Have both staged and unstaged changes.  
**Steps:** Capture `git diff --cached` and `git status --porcelain`, run `jul sync`, capture again.  
**Assertions:**
- Index is unchanged (cached diff identical).
- Working tree changes remain.
- Draft commit is created/updated; tree matches working directory minus ignored paths.

### IT-DRAFT-002 — Draft Is Idempotent When Tree Unchanged

**Covers:** “Only new draft if tree changed”.  
**Steps:** Run `jul sync` twice with no changes.  
**Assertions:** Draft SHA remains identical.

### IT-DRAFT-003 — Draft Commits Are Siblings (No Infinite Chain)

**Covers:** Parent model: all drafts share the same parent (base commit).  
**Steps:** Edit → `jul sync` (draft1), edit → `jul sync` (draft2).  
**Assertions:** `parent(draft1) == parent(draft2) == base_commit`; draft1 is not ancestor of draft2.

### IT-DRAFT-004 — `.jul/**` Is Always Excluded From Draft Snapshots

**Covers:** Critical ignore rule.  
**Steps:** Modify `.jul/*`, run `jul sync`.  
**Assertions:** Draft commit does not include `.jul/**`.

### IT-DRAFT-005 — `.jul/syncignore` Is Honored

**Covers:** syncignore behavior.  
**Steps:** Add `.env`, list it in `.jul/syncignore`, run `jul sync`.  
**Assertions:** Draft commit excludes it.

### IT-DRAFT-006 — Untracked Files Are Captured (Except Ignored)

**Covers:** Shadow add-all semantics.  
**Steps:** Create an untracked file not ignored; run `jul sync`.  
**Assertions:** Draft commit includes it.

### IT-DRAFT-007 — Executable Bit Preserved in Draft Snapshots

**Covers:** Mode bits.  
**Steps:** `chmod +x script.sh`, `jul sync`.  
**Assertions:** Draft tree records executable bit (verify via `git ls-tree`).

### IT-DRAFT-008 — Symlink Handling (If Supported)

**Covers:** Symlink correctness.  
**Steps:** Create symlink; run `jul sync`.  
**Assertions:** Draft stores a symlink (mode 120000) with correct target.

---

# D) Sync Behavior Across Capability Modes

### IT-SYNC-LOCAL-001 — Local-Only Sync Still Snapshots Drafts

**Covers:** Local-first promise.  
**Setup:** No remote configured.  
**Steps:** Edit files; run `jul sync`.  
**Assertions:** Draft updated; no remote actions attempted.

### IT-SYNC-CP-001 — Checkpoint Sync: Workspace Refs and Notes Sync Across Devices

**Covers:** Workspace ref + notes sync.  
**Setup:** Compatible sync remote; two clones A and B.  
**Steps:** On A: change + `jul checkpoint`. On B: `jul sync` then `jul ws checkout @`.  
**Assertions:** B sees updated workspace tip and workspace-meta notes.

### IT-SYNC-DRAFT-001 — Draft Sync Pushes Per-Device Draft Ref (Force-With-Lease)

**Covers:** Per-device draft refs; lease safety.  
**Setup:** Remote allows non-FF for `refs/jul/sync/*`.  
**Steps:** On A: run `jul sync` multiple times; inspect remote refs.  
**Assertions:**
- Remote contains `refs/jul/sync/<user>/<device>/<ws>`.
- Ref updates replace prior draft (non-FF) without losing local draft safety.
- Lease semantics prevent clobbering if remote changed unexpectedly.

### IT-SYNC-DRAFT-002 — Draft Sync Unavailable: Draft Stays Local, Checkpoints Still Sync

**Covers:** Graceful degradation.  
**Setup:** FF-only for draft refs.  
**Steps:** Edit, `jul sync`, `jul checkpoint`, `jul sync`.  
**Assertions:** Draft not pushed; checkpoints and notes sync.

### IT-SYNC-BASEADV-001 — Base Advanced Is Detected and Does Not Rewrite Local Base

**Covers:** Base advancement invariants.  
**Setup:** Two devices with checkpoint sync.  
**Steps:** Device A checkpoints; device B has draft on old base; device B runs `jul sync`.  
**Assertions:**
- B reports “Base advanced”.
- B’s draft base is not silently rewritten.
- Promote is blocked until explicit restack/checkout.

### IT-SYNC-AUTORESTACK-001 — Autorestack On: Clean Restack Happens Automatically

**Covers:** `sync.autorestack = true` when conflict-free.  
**Setup:** Autorestack enabled; A advances base; B is behind but changes are restack-clean.  
**Steps:** B runs `jul sync`.  
**Assertions:** B creates a restack checkpoint and advances workspace tip appropriately.

### IT-SYNC-AUTORESTACK-002 — Autorestack Stops When Conflicts Occur

**Covers:** “Never auto-merge”.  
**Setup:** Conflicting changes between A and B.  
**Steps:** A checkpoints; B syncs with autorestack on.  
**Assertions:** Restack stops with “needs merge”; no half-resolved state.

### IT-SYNC-DAEMON-001 — Daemon Debounces Bursts and Avoids Half-Written Snapshots

**Covers:** Continuous sync mode behavior.  
**Setup:** `jul sync --daemon` running; large write burst.  
**Steps:** Perform burst; observe sync behavior.  
**Assertions:** Final draft corresponds to stable end state; sync frequency respects min interval.

### IT-SYNC-FAIL-001 — Remote Push Failure Does Not Lose Local Draft

**Covers:** Network/remote errors.  
**Setup:** Remote fails pushes intermittently.  
**Steps:** Edit; run `jul sync`.  
**Assertions:** Local draft updates even if push fails; retry succeeds later.

---

# E) Checkpoint: Locking Work Safely

### IT-CP-001 — Checkpoint Creates Durable Commit + Keep-Ref + Updates Workspace + Change Refs

**Covers:** Checkpoint semantics and ref updates.  
**Setup:** Repo with changes and passing checks.  
**Steps:** `jul checkpoint -m "feat: x"` (or accept generated message).  
**Assertions:**
- New checkpoint commit exists with Change-Id trailer or mapping.
- Workspace ref advances to checkpoint.
- Change ref advances to checkpoint.
- Keep-ref is created for the checkpoint.
- Anchor ref is created on first checkpoint for a Change-Id (and never moves).
- A new draft starts with parent = checkpoint.

### IT-CP-002 — Checkpoint Behavior When Checks Fail (Policy Choice Must Be Stable)

**Covers:** Checkpoint + checks behavior.  
**Setup:** Failing tests.  
**Steps:** `jul checkpoint`.  
**Assertions:** Behavior matches the chosen policy (either refuse checkpoint or create checkpoint with failing attestation); must be consistent and explicit.

### IT-CP-003 — Checkpoint When Workspace Push Is Rejected (Non-FF)

**Covers:** Diverged workspace state handling.  
**Setup:** Device A checkpoints and pushes first; device B checkpoints without syncing; push rejected.  
**Steps:** Device B runs `jul checkpoint`.  
**Assertions:** Checkpoint is kept locally; workspace becomes “diverged”; promote blocked until restack/checkout.

### IT-CP-004 — Checkpoint Flushes Final Trace (Trace Tree == Checkpoint Tree)

**Covers:** Flush rule correctness.  
**Setup:** Tracing enabled; create trace; edit more.  
**Steps:** `jul checkpoint`.  
**Assertions:** Checkpoint has Trace-Head trailer; trace_head tree equals checkpoint tree exactly.

### IT-CP-005 — Change-Id Lifecycle: Promote Starts a New Change, Not Checkpoint

**Covers:** Change-Id rollover rules.  
**Steps:** Create two checkpoints; verify they share Change-Id; promote; create another checkpoint.  
**Assertions:** Pre-promote checkpoints share Change-Id; post-promote work starts a new Change-Id.

### IT-CP-006 — `jul checkpoint --adopt` Adopts `HEAD` as Checkpoint Without Rewriting

**Covers:** Adoption semantics.  
**Setup:** User creates a git commit manually on the workspace branch.  
**Steps:** `jul checkpoint --adopt`.  
**Assertions:** Adopted commit becomes checkpoint; keep-ref and metadata are written; no history rewrite occurs.

---

# F) Traces (Provenance Side History)

### IT-TRACE-001 — Explicit Trace Creation Updates Canonical Trace Tip Ref

**Covers:** Trace creation; single tip ref with parent chain.  
**Steps:** `jul trace --prompt "x" --agent "y"`.  
**Assertions:** Trace ref advances; trace parent is previous trace tip; metadata note written.

### IT-TRACE-002 — Prompt Privacy Defaults: Only Hash Syncs by Default

**Covers:** Privacy defaults for prompt data.  
**Setup:** Sync remote configured; default trace settings.  
**Steps:** Create trace with prompt; sync; inspect remote notes.  
**Assertions:** Remote notes include prompt hash, not full prompt or summary by default.

### IT-TRACE-003 — `jul sync` Does Not Create a New Trace When Tree Unchanged

**Covers:** Anti-spam idempotency for implicit tracing.  
**Steps:** Run `jul sync` repeatedly without file changes.  
**Assertions:** Trace tip does not advance.

### IT-TRACE-MERGE-001 — Multi-Device Trace Merge When Checkpoint Tips Match

**Covers:** Trace merge algorithm; merge trace commit.  
**Setup:** Two devices produce divergent trace chains; checkpoint tips match.  
**Steps:** Sync both devices until merge happens.  
**Assertions:** Merge trace commit created; two parents; tree matches canonical checkpoint tip; blame skips merge traces.

### IT-TRACE-BASEADV-001 — Base Advanced: Canonical Trace Tip Does Not Advance

**Covers:** Rule: do not advance canonical trace tip when base advanced.  
**Setup:** Device A advances workspace tip; device B is behind.  
**Steps:** Device B runs `jul sync`.  
**Assertions:** Device trace-sync ref updates; canonical trace ref does not.

---

# G) CI and Attestations (Trustworthy Gating)

### IT-CI-001 — Draft Checks Coalesce and Do Not Run Unbounded

**Covers:** Coalescing/cancellation rules.  
**Setup:** Enable `run_on_draft = true`; checks slow enough to overlap.  
**Steps:** Edit + `jul sync`; quickly edit + `jul sync`.  
**Assertions:** Only latest draft is “current”; stale runs are cancelled/ignored; status is accurate.

### IT-CI-002 — Checkpoint Attestation Stored in Notes and Synced

**Covers:** Checkpoint attestation notes and size caps.  
**Steps:** `jul checkpoint`.  
**Assertions:** Checkpoint attestation exists in `refs/notes/jul/attestations/checkpoint`; payload is structured and within size limit.

### IT-CI-003 — Promote Gating Does Not Trust Remote-Only Attestations

**Covers:** Trust model.  
**Setup:** Device A has passing attestation synced; device B has not run checks locally.  
**Steps:** On B attempt `jul promote` with required checks.  
**Assertions:** Promote re-runs checks locally or refuses; remote note is informational only.

### IT-CI-004 — Inherited Attestations After Restack Are Marked Stale and Not Used for Gating

**Covers:** Restack inheritance rules.  
**Steps:** Create passing checkpoint; restack; check status.  
**Assertions:** Inherited results (if shown) are stale; gating requires fresh checks for new SHA.

### IT-CI-005 — Coverage Threshold Enforcement

**Covers:** `min_coverage_pct` gating and reporting.  
**Setup:** Coverage below threshold.  
**Steps:** `jul promote --to main`.  
**Assertions:** Promote blocked; output pinpoints coverage shortfall; `--no-policy` bypass is explicit.

---

# H) Review Layer: Suggestions Lifecycle (Optional)

### IT-SUGG-001 — Review Produces Suggestion Refs and Metadata Notes

**Covers:** `jul review`/checkpoint review; suggestion commit storage.  
**Setup:** Stub/mock agent provider to deterministically generate a suggestion.  
**Steps:** `jul checkpoint` (review enabled).  
**Assertions:** Suggestion ref exists; suggestion metadata note exists with base SHA and fields.

### IT-SUGG-002 — `jul apply` Applies Suggestion Into Current Draft Without Committing

**Covers:** Apply semantics.  
**Steps:** `jul apply <id>`.  
**Assertions:** Working tree updated; no new checkpoint unless requested; status updates.

### IT-SUGG-003 — Staleness: New Checkpoint Makes Old Suggestions Stale

**Covers:** Staleness detection.  
**Steps:** Create suggestion for checkpoint A; create checkpoint B; list and apply old suggestion.  
**Assertions:** Suggestion marked stale; apply warns/refuses unless forced.

### IT-SUGG-004 — Reject Suggestion Records Durable Reason

**Covers:** Audit trail of suggestion lifecycle.  
**Steps:** `jul reject <id> -m "reason"`.  
**Assertions:** Status becomes rejected; reason stored; promote warning logic respects policy.

### IT-SUGG-005 — Cleanup Cascade When Keep-Ref Expires (If Prune Implemented)

**Covers:** Retention-driven cleanup.  
**Steps:** Expire a checkpoint keep-ref (time travel or prune).  
**Assertions:** Suggestion refs and associated notes are removed per retention rules.

---

# I) Promote (Publishing to Real Branches Safely)

### IT-PROMOTE-REBASE-001 — Promote (Rebase) Lands Checkpoints as Linear Commits on Target

**Covers:** Rebase promote strategy; mapping records.  
**Setup:** Target branch exists; 2 checkpoints in current Change-Id.  
**Steps:** `jul promote --to main --rebase`.  
**Assertions:** `main` advances via fast-forward; workspace base marker created; mappings and reverse index notes written; new draft started.

### IT-PROMOTE-SQUASH-001 — Promote (Squash) Creates One Published Commit With Full-Range Trace Anchors

**Covers:** Squash mapping rules; trace range spanning.  
**Steps:** `jul promote --to main --squash`.  
**Assertions:** One commit on main; reverse index includes Change-Id and correct anchors spanning entire change.

### IT-PROMOTE-MERGE-001 — Promote (Merge) Records Mainline for Deterministic Revert

**Covers:** Merge strategy; revert determinism.  
**Steps:** `jul promote --to main --merge`.  
**Assertions:** Promote metadata includes merge commit SHA and mainline.

### IT-PROMOTE-REWRITE-001 — Target Rewritten on Publish Remote Requires Explicit Confirmation

**Covers:** Rewrite detection; safety prompt.  
**Setup:** Rewrite remote target behind Jul’s last seen track tip.  
**Steps:** `jul promote --to main`.  
**Assertions:** Jul warns loudly; refuses unless user confirms; if confirmed, restacks and proceeds; otherwise aborts without mutating target.

### IT-PROMOTE-FORCE-001 — `--force-target` Is Gated and Loud

**Covers:** Dangerous override behavior.  
**Steps:** `jul promote --to main --force-target`.  
**Assertions:** Requires explicit confirmation or guarded flag; produces audit-friendly metadata.

### IT-PROMOTE-STACK-001 — Stacked Promote Lands Stack Bottom-Up

**Covers:** Graphite-style stack landing.  
**Setup:** Parent workspace change A; child workspace stacked with checkpoints.  
**Steps:** Run `jul promote` on top workspace.  
**Assertions:** Parent lands first; child rebases and lands; partial failures do not undo published layers.

---

# J) Restack and Stacking (Upstream Drift Without Chaos)

### IT-STACK-001 — `jul ws stack` Requires Parent Checkpoint and Pins `base_change_id`

**Covers:** Stacking boundary and metadata correctness.  
**Steps:** Attempt stack without checkpoint (refuse); checkpoint; stack.  
**Assertions:** Child workspace meta includes base ref to parent change ref and `base_change_id`; track_ref inherited correctly.

### IT-RESTACK-001 — Restack Onto Updated Base Creates New Checkpoint, Preserves Change-Id

**Covers:** Restack semantics; synthetic restack trace.  
**Setup:** Base branch advances; workspace has checkpoints.  
**Steps:** `jul ws restack`.  
**Assertions:** New restack checkpoint created; same Change-Id; base_sha updated; trace_type=restack recorded; suggestions stale.

### IT-RESTACK-002 — Restack With Conflicts Routes to `jul merge`

**Covers:** Conflict detection and routing.  
**Steps:** Create conflict; run restack.  
**Assertions:** Restack stops; merge required; no silent auto-merge.

---

# K) Revert by Change-Id (Post-Promote Safety Net)

### IT-REVERT-001 — Revert Latest Promote Event by Change-Id Creates Revert Checkpoint

**Covers:** Mapping lookup; revert staging.  
**Setup:** Change promoted to main.  
**Steps:** `jul revert <change-id> --to main`.  
**Assertions:** Revert checkpoint created; diff corresponds to recorded published commits; promote would land revert.

### IT-REVERT-002 — Revert No-Op Produces Clear Message and No Checkpoint (Unless Forced)

**Covers:** Empty revert handling.  
**Steps:** Revert already reverted change.  
**Assertions:** No new checkpoint by default; with `--force`, allow-empty checkpoint.

### IT-REVERT-003 — Merge Promote Revert Uses Recorded Mainline

**Covers:** Deterministic merge reverts.  
**Steps:** Revert a merge-promoted change.  
**Assertions:** Uses stored mainline; results match expected.

---

# L) Read Commands (`log`, `diff`, `show`, `blame`, `query`, `reflog`, `git`)

### IT-READ-001 — `jul log` Groups by Change-Id and Hides Workspace Base Markers by Default

**Covers:** History display semantics.  
**Steps:** After promote, run `jul log` and `jul log --verbose`.  
**Assertions:** Base markers hidden by default, visible in verbose; grouping is consistent.

### IT-READ-002 — `jul diff` Works for Draft vs Base, Checkpoint vs Parent, Change Net Diff

**Covers:** Change-aware diff semantics.  
**Assertions:** Output matches the correct underlying git diff ranges.

### IT-READ-003 — `jul show <checkpoint>` Displays Attestations and Suggestions

**Covers:** Metadata lookup correctness.

### IT-BLAME-001 — `jul blame` Falls Back Safely When Attribution Is Ambiguous

**Covers:** Best-effort provenance with safe fallback.  
**Setup:** Renames, large conflict resolution, partial-line changes.  
**Assertions:** When trace attribution is uncertain, output falls back to checkpoint/commit blame; never fabricates provenance.

### IT-QUERY-001 — `jul query` Filters by Checks/Coverage Reliably

**Covers:** Attestation querying correctness.

### IT-GIT-001 — `jul git <args>` Forwards Arguments Unchanged

**Covers:** Git passthrough reliability.

---

# M) Local Workspaces (Client-Side Time Travel)

### IT-LOCAL-001 — Save/Restore Preserves Staged Changes and Untracked Files

**Covers:** `jul local save/restore`.  
**Steps:** Create dirty state; save; clean; restore.  
**Assertions:** Exact restoration; no interference with drafts.

### IT-LOCAL-002 — List/Delete Semantics Are Correct

**Covers:** Local workspace management operations.

---

# N) Git + Jul Interop and Hooks (Baseline)

### IT-HOOKS-001 — Post-Commit Hook Adoption Flow (If Implemented)

**Covers:** Git-first mode hook integration.  
**Setup:** Hooks installed; git commits should be adopted.  
**Steps:** Make git commit; inspect Jul state.  
**Assertions:** Commit is adopted as checkpoint (or queued for adoption) per design; no double-counting.

### IT-OOB-RESET-001 — `git reset --hard` Recovery via `jul ws checkout`

**Covers:** Recovery from working tree destruction.  
**Steps:** Create checkpoint; edit; `git reset --hard`; run `jul ws checkout @`.  
**Assertions:** Working tree restored to canonical workspace tip; lease repaired.

### IT-OOB-COMMIT-001 — Unadopted Commits on Workspace Branch Are Detected

**Covers:** Warning logic for “commits exist but not adopted as checkpoints.”  
**Steps:** Create git commit on `refs/heads/jul/<ws>` without adoption; run `jul status`.  
**Assertions:** Jul warns and offers `jul checkpoint --adopt` or `jul ws checkout @`.

---

# O) Security and Privacy

### IT-SEC-001 — Secret Scan Blocks Draft Push by Default

**Covers:** Draft secret scanning behavior.  
**Setup:** Add high-signal secret fixture.  
**Steps:** `jul sync` with draft sync enabled.  
**Assertions:** Remote draft push blocked; local draft still updated; explicit override required.

### IT-SEC-002 — Prompt Summary Scrubber Runs Before Sync (When Opt-In Enabled)

**Covers:** Privacy and scrubbing.  
**Setup:** Enable `traces.sync_prompt_summary = true`; prompt contains secret-like content.  
**Assertions:** Summary is scrubbed or sync blocked per policy; raw secret never synced silently.

### IT-SEC-003 — Notes Size Caps Enforced (Attestations/Suggestions)

**Covers:** Prevent repo bloat.  
**Setup:** Generate huge test output.  
**Assertions:** Notes remain within size limit; full logs remain local; truncation is explicit.

---

# P) Retention, Keep-Refs, and GC Safety

### IT-KEEP-001 — Keep-Refs Created on Checkpoint, Not on Draft

**Covers:** Fetchability rules.  
**Assertions:** Keep-ref exists only for checkpoint SHAs.

### IT-KEEP-002 — Checkpoints Remain Fetchable Across Clones (No allowAnySHA1InWant Needed)

**Covers:** Keep-refs ensure fetchability.

### IT-PRUNE-001 — Prune Removes Expired Keep-Refs and Cascades Cleanup (If Implemented)

**Covers:** Conservative cleanup safety.  
**Assertions:** Only expired refs removed; published branches unaffected.

---

# Q) Crash/Interrupt/Partial-Failure Robustness

### IT-ROBUST-001 — Power Loss Mid-Sync Leaves Repo Recoverable

**Covers:** Shadow index atomicity; derived-state repair.  
**Method:** Kill process during sync at multiple points.  
**Assertions:** Next `jul sync` repairs; `.git/index` uncorrupted; no silent draft loss.

### IT-ROBUST-002 — Interrupted Checkpoint Does Not Create Half-Baked Metadata

**Covers:** Checkpoint atomicity.  
**Assertions:** Operation is all-or-nothing from user perspective; no misleading refs.

### IT-ROBUST-003 — Notes Merge Conflicts Use Union Strategy for Append-Only Notes

**Covers:** `cat_sort_uniq` merge approach for NDJSON notes.  
**Assertions:** Union without duplicates; deterministic ordering.

### IT-ROBUST-004 — Remote Fetch Fails During Promote; No Target Branch Mutated

**Covers:** Publish safety.

---

# R) Performance and Scale (Baseline)

### IT-SCALE-001 — Large Repo Snapshot Performance Is Reasonable

**Covers:** Draft snapshot cost; daemon stability.

### IT-SCALE-002 — Ref Namespace Size Stays Bounded (No Ref Explosion for Traces)

**Covers:** Single trace tip ref design.

---

# S) Agent Coordination and Multi-Agent Scenarios

These scenarios validate agent-first workflows and agent-specific failure modes.

### IT-AGENT-001 — Multiple Agents on the Same Workspace Create a Single, Ordered Trace Chain

**Covers:** Trace ordering with multiple agents; no lost provenance.  
**Setup:** Two agent processes (A and B) configured for the same workspace.  
**Steps:**
1. Agent A: `jul trace --prompt "Implement auth" --agent "claude-code"`
2. Agent A: make edits
3. Agent B: `jul trace --prompt "Add tests" --agent "codex"`
4. Agent B: make edits
5. `jul sync`
**Assertions:**
- Trace chain includes both trace commits in a single parent chain in invocation order.
- No trace commits are lost or orphaned.
- `jul blame` attributes lines to the correct agent when trace index/heuristics can support it.
**Variants:**
- Run the two `jul trace` commands concurrently; ensure ref update is transactional (no lost update). If lost, this is a correctness bug.

### IT-AGENT-002 — Agent Handoff via `jul draft adopt` Across Devices

**Covers:** “Pick up where another agent left off” workflow.  
**Setup:** Two clones (device A and device B); draft sync enabled.  
**Steps:**
1. Device A: substantial edits; `jul sync`
2. Device B: `jul draft adopt <deviceA>`
3. Device B: continue edits; `jul checkpoint`
**Assertions:**
- B’s checkpoint includes A’s changes.
- Trace history reflects the handoff (either via parent chain or new trace with correct base).
- No duplicate draft commits; remote draft ref updates cleanly.
**Variants:**
- Base mismatch: adoption should refuse by default and require explicit `--onto <checkpoint>`.

### IT-AGENT-003 — Agent Timeout Mid-Trace Does Not Corrupt Trace Chain

**Covers:** Partial trace creation; crash recovery.  
**Setup:** Simulate agent process killed after `jul trace` but before file edits.  
**Steps:**
1. Start `jul trace --prompt "X"`
2. Kill process before any file changes
3. Start new agent session; run `jul trace --prompt "Y"`
**Assertions:**
- Trace chain remains valid; canonical trace ref is not left dangling.
- If a trace commit was created without tree changes, it must be safe (tree identical to parent); blame must not attribute edits to it.
- Subsequent trace parents correctly from the latest trace tip.

### IT-AGENT-004 — Concurrent `jul sync` From Two Agents on the Same Device and Workspace

**Covers:** File locking; shadow index safety under concurrency.  
**Setup:** Two terminal sessions on same device/workspace.  
**Steps:** Run `jul sync` simultaneously.  
**Assertions:**
- No shadow index corruption.
- No duplicate draft commits (or, at minimum, final state is correct and earlier work is not lost).
- One sync wins; the other waits or fails gracefully with actionable retry guidance.
- `.jul/` state remains consistent.

### IT-AGENT-005 — One Agent Creates Checkpoint While Another Has Advanced Traces

**Covers:** Checkpoint flush rule under multi-agent use.  
**Setup:** Agent A creates traces and edits; agent B runs checkpoint.  
**Steps:**
1. Agent A: `jul trace`, edit files
2. Agent B: `jul checkpoint` on the same workspace/device
**Assertions:**
- Checkpoint trace_head reflects the latest trace chain and flush rule (trace_head tree equals checkpoint tree).
- No orphaned trace segments.

### IT-AGENT-006 — Headless Mode Produces Machine-Parseable JSON Errors

**Covers:** `--json` output; deterministic failures for automation.  
**Steps:**
- `jul checkpoint --json` with failing checks
- `jul promote --json` with policy violations
- `jul sync --json` with simulated network failure
**Assertions:**
- Output is valid JSON (no progress logs on stdout if strict mode is intended).
- Error payload includes stable `code`, human `message`, and `next_actions`.
- Exit codes are deterministic.

### IT-AGENT-007 — Agent Backoff When Checks Are Slow (No Unbounded Check Spawning)

**Covers:** Check runner coalescing and rate limiting under agent loops.  
**Setup:** Slow checks (≥5s).  
**Steps:** Invoke `jul checkpoint` repeatedly while checks still running.  
**Assertions:**
- Jul does not spawn unbounded parallel check processes.
- Status reports “checks in progress” accurately.
- Headless output provides an agent-readable “wait vs proceed” signal.

---

# T) Daemon Lifecycle (Long-Running Process Gauntlet)

### IT-DAEMON-001 — Daemon Startup Is Idempotent and Enforces Single Instance

**Covers:** Single-instance enforcement.  
**Steps:** Start daemon twice (`jul sync --daemon` twice).  
**Assertions:**
- Second invocation reports daemon already running (or signals it) and exits cleanly.
- No duplicate daemons; no zombies.

### IT-DAEMON-002 — Daemon Shutdown on SIGTERM Cleans Up Gracefully

**Covers:** Clean shutdown; no partial state.  
**Steps:** Start daemon; send SIGTERM.  
**Assertions:**
- Daemon exits promptly.
- Any in-progress sync completes or aborts safely (no corrupt shadow index).
- No orphaned child processes (check runners).

### IT-DAEMON-003 — Daemon Crash Recovery (Stale Lock/PID Marker)

**Covers:** Recovery after unclean exit.  
**Setup:** Create a stale “daemon running” marker (pidfile or lock).  
**Steps:** Start daemon.  
**Assertions:**
- Jul detects stale marker and recovers cleanly.
- New daemon starts.

### IT-DAEMON-004 — Daemon Survives Temporary Remote Unavailability

**Covers:** Network resilience; backoff.  
**Setup:** Daemon running; make remote unreachable temporarily.  
**Assertions:**
- Daemon does not crash.
- Backoff prevents hammering remote.
- Daemon resumes normal sync after recovery.

### IT-DAEMON-005 — Daemon vs Manual Sync Race

**Covers:** Concurrency between background and foreground sync.  
**Steps:** Daemon running; run `jul sync` manually during a daemon tick.  
**Assertions:** No corruption; no duplicate drafts; one may no-op.

### IT-DAEMON-006 — Daemon Picks Up `.jul/syncignore` Changes Without Restart

**Covers:** Config hot reload.  
**Steps:** Daemon running; update `.jul/syncignore`; create matching file; wait.  
**Assertions:** New ignore rule is honored in subsequent drafts.

### IT-DAEMON-007 — Daemon Handles Workspace Switch Gracefully

**Covers:** Workspace switching behavior with daemon running.  
**Steps:** Daemon running on workspace A; run `jul ws switch B`.  
**Assertions:** Behavior matches design choice (switches, stays, or requires restart) and never corrupts state.

### IT-DAEMON-008 — Daemon CPU Usage Stays Bounded Under Rapid File Changes

**Covers:** Debouncing effectiveness and resource caps.  
**Steps:** Generate 1000 rapid file changes.  
**Assertions:** Debouncing prevents per-file sync; system remains responsive; final draft matches end state.

### IT-DAEMON-009 — Daemon Ignores Internal Agent Worktree Churn

**Covers:** Avoid infinite loops caused by `.jul/agent-workspace/worktree`.  
**Setup:** Daemon running; trigger a review that writes in agent workspace.  
**Assertions:** Daemon does not treat agent workspace changes as user edits; no sync storm.

---

# U) `jul merge` (Conflict Resolution Path)

Note: The v0.3 spec describes `jul merge` as an agent-assisted flow producing a resolution suggestion and asking the user/agent to accept or reject it. If your implementation adds `--continue/--abort`, test those as well.

### IT-MERGE-001 — Merge After Restack Conflict: Resolve and Complete

**Covers:** Restack conflict → merge flow; resolution correctness.  
**Setup:** Create a restack conflict.  
**Steps:**
1. `jul ws restack` → conflicts
2. Run `jul merge` and accept the resolution (or resolve manually if agent rejects)
**Assertions:**
- Conflicts are resolved in working tree.
- A new checkpoint is created (restack checkpoint) or the draft is updated per merge design.
- No conflict markers remain in committed tree.

### IT-MERGE-002 — Merge Abort (If Supported) Restores Pre-Merge State

**Covers:** Safe exit from merge.  
**Steps:** Trigger merge conflict; run abort mechanism.  
**Assertions:** Working tree and refs return to pre-merge state; no partial checkpoint.

### IT-MERGE-003 — Agent-Assisted Merge Resolution

**Covers:** Internal agent invocation for conflict resolution.  
**Setup:** Configure internal agent (mock or real); create conflict.  
**Steps:** `jul merge` (agent mode)  
**Assertions:** Agent invoked with conflict context; success completes merge; failure falls back to manual with clear message; trace records agent involvement.

### IT-MERGE-004 — Merge With Multiple Conflicting Files Tracks Remaining Conflicts

**Covers:** Partial resolution; state tracking.  
**Setup:** Conflicts in ≥3 files.  
**Steps:** Resolve one file; attempt merge completion.  
**Assertions:** Merge refuses completion until all conflicts resolved; output lists unresolved files.

### IT-MERGE-005 — Checkpoint Is Blocked While Merge Is In Progress (If Merge State Exists)

**Covers:** State machine enforcement.  
**Steps:** With merge in progress, attempt `jul checkpoint`.  
**Assertions:** Checkpoint refused with “merge in progress” guidance.

### IT-MERGE-006 — `jul merge` Outside Merge State Is Safe No-Op

**Covers:** Safe invocation.  
**Steps:** Run `jul merge` when no conflicts exist.  
**Assertions:** Clear message; no state changes; deterministic exit code.

### IT-MERGE-007 — Rejecting a Suggestion Preserves Manual Merge Path

**Covers:** Suggestion lifecycle; manual resolution path.  
**Setup:** Create a merge conflict with agent suggestions enabled.  
**Steps:** Run `jul merge` to create a suggestion, reject it, manually resolve conflicts in the agent worktree, then run `jul merge --apply`.  
**Assertions:** Manual resolution is applied; suggestion is recorded as rejected; no conflict markers remain.

### IT-MERGE-008 — Stale Agent Worktree Is Reset When Merge Refs Change

**Covers:** Correctness across ref changes; merge state hygiene.  
**Setup:** Create a merge conflict and run `jul merge` so MERGE_HEAD is set; modify files in the agent worktree.  
**Steps:** Advance sync/workspace refs to a new conflicting pair, then run `jul merge` again.  
**Assertions:** Agent worktree is reset to the new merge; stale edits are discarded; conflict markers reflect the new ours/theirs.

---

# V) Offline-to-Online Transitions

### IT-OFFLINE-001 — Work Offline, Then Add Sync Remote and Sync History

**Covers:** Retroactive sync enablement.  
**Setup:** No remote; create several checkpoints and traces.  
**Steps:** Add remote; `jul remote set`; `jul doctor`; `jul sync`.  
**Assertions:** Existing checkpoints, keep-refs, and notes sync; remote matches local; no duplicates.

### IT-OFFLINE-002 — Offline Work When Remote Already Has Jul State

**Covers:** Merge of offline local state with existing remote state.  
**Setup:** Device A creates checkpoints online; device B works offline.  
**Steps:** Device B reconnects; `jul sync`.  
**Assertions:** Divergence detected; no silent overwrite; explicit restack/checkout required when needed.

### IT-OFFLINE-003 — Offline Draft Accumulation, Online Sync Pushes Only Latest Draft

**Covers:** Draft ephemerality.  
**Steps:** Create D1, D2, D3 drafts offline; reconnect; `jul sync`.  
**Assertions:** Only latest draft is pushed to remote draft ref; older drafts remain local-only.

### IT-OFFLINE-004 — Offline Checkpoint Conflicts With Another Offline Device’s Checkpoint

**Covers:** Checkpoint conflict resolution safety.  
**Setup:** Two devices create offline checkpoints on same workspace.  
**Steps:** Reconnect; A syncs first; B syncs second.  
**Assertions:** B detects divergence; cannot push workspace ref; B must restack; both checkpoints remain preserved (via keep-refs).

### IT-OFFLINE-005 — Empty Repo Identity: Repo-Meta Published Only After First Commit Exists

**Covers:** Repo-meta lifecycle when repo has no commits at init time.  
**Setup:** New repo with no commits; remote configured later.  
**Steps:** `jul init` (local-only identity); create first checkpoint; then set remote and sync.  
**Assertions:** Repo-meta note appears after root commit exists; user namespace becomes canonical and stable across devices.

---

# W) Platform and Filesystem Edge Cases

### IT-PLATFORM-001 — Line Ending Handling (CRLF vs LF)

**Covers:** Cross-platform consistency; `.gitattributes` respect.  
**Setup:** `.gitattributes` uses `* text=auto`.  
**Steps:** Create file with CRLF on Windows-like config; sync; verify on Unix-like config.  
**Assertions:** Draft commit reflects git’s normalization; working trees have platform-appropriate endings; no spurious diffs.

### IT-PLATFORM-002 — Path Length Limits (Windows 260-Char)

**Covers:** Graceful handling of platform limits.  
**Setup:** Create deeply nested path beyond 260 chars (when possible).  
**Assertions:** Clear error on limited platforms; no partial state; works on non-limited platforms.

### IT-PLATFORM-003 — Case Sensitivity Handling

**Covers:** Case-insensitive filesystem collision warnings.  
**Setup:** Repo contains `Foo.txt` and `foo.txt`.  
**Assertions:** Jul warns; draft object store contains both; working tree behavior matches filesystem; no silent data loss.

### IT-PLATFORM-004 — Unicode Filenames (NFD vs NFC)

**Covers:** Unicode normalization issues.  
**Setup:** Filename with accent (e.g., `café.txt`).  
**Assertions:** No spurious delete/recreate; trace/blame operations work.

---

# X) Explicitly Unsupported Scenarios (Documented Behavior)

### IT-UNSUPPORTED-001 — Submodules: Warn and Proceed Safely

**Covers:** Submodule non-support acknowledgment.  
**Setup:** Repo with submodules.  
**Assertions:** Jul warns; gitlink recorded; submodule working tree changes not captured; no crashes.

### IT-UNSUPPORTED-002 — User-Managed Worktrees: Refuse or Warn (But Do Not Corrupt)

**Covers:** Limitations with multiple worktrees.  
**Setup:** Repo with additional user-created worktrees.  
**Assertions:** Jul refuses or warns with documented limitations; no cross-worktree corruption.

### IT-UNSUPPORTED-003 — Bare Repos: Init Fails Gracefully

**Covers:** Bare repo detection.  
**Setup:** Bare git repo.  
**Assertions:** Clear error; no partial state created.

### IT-UNSUPPORTED-004 — Shallow Clones: Warn About Limitations and Degrade Gracefully

**Covers:** Shallow clone detection.  
**Setup:** `git clone --depth 1`.  
**Assertions:** Warning; basic operations work; history-dependent operations fail with actionable guidance.

---

# Y) Additional Robustness Scenarios

### IT-ROBUST-005 — `.jul/` Deleted While Daemon Running

**Covers:** Runtime state loss recovery.  
**Steps:** Start daemon; delete `.jul/`; wait for tick.  
**Assertions:** Daemon exits cleanly or recreates minimal state; no infinite loop; no crash.

### IT-ROBUST-006 — `git gc` During Jul Operations

**Covers:** GC interaction.  
**Steps:** Create drafts and checkpoints; run `git gc --aggressive` during activity.  
**Assertions:** Keep-refs protect checkpoints; drafts may be collected; no corruption.

### IT-ROBUST-007 — Disk Full During Sync

**Covers:** Resource exhaustion handling.  
**Setup:** Almost-full filesystem.  
**Assertions:** Clear disk-full error; no partial writes; recovery works after freeing space.

### IT-ROBUST-008 — Permission Denied on `.jul/` Files

**Covers:** Permission error handling.  
**Setup:** Make `.jul/` files read-only or owned by another user.  
**Assertions:** Clear error identifying file; suggests remediation; no crash.

### IT-ROBUST-009 — Git Hooks Reject Commits

**Covers:** Hook interaction policy.  
**Setup:** Pre-commit hook rejects content.  
**Steps:** Trigger hook; run `jul checkpoint`.  
**Assertions:** Behavior matches chosen policy (run hooks or bypass); output is clear and consistent.

### IT-ROBUST-010 — Corrupted Notes Ref

**Covers:** Notes corruption recovery.  
**Setup:** Corrupt a notes ref to point to invalid object.  
**Assertions:** Jul detects corruption, does not propagate it to remote, and degrades gracefully.

---

# Z) Performance Edge Cases

### IT-PERF-001 — Binary File Handling

**Covers:** Large binaries; memory safety.  
**Setup:** Add large binary (e.g., 100MB).  
**Assertions:** Sync/checkpoint complete without memory blowup; draft includes binary.

### IT-PERF-002 — Many Small Files (node_modules-like)

**Covers:** Shadow index scale.  
**Setup:** Generate 50k files.  
**Assertions:** Sync completes; daemon debouncing prevents per-file sync storms.

### IT-PERF-003 — Deep Checkpoint History (1000+ Checkpoints)

**Covers:** Log/diff scalability; ref push efficiency.  
**Assertions:** Log pagination works; diff computes reasonably; sync does not re-push all keep-refs each time.

### IT-PERF-004 — Many Workspaces (100+)

**Covers:** Workspace listing and sync scaling.  
**Assertions:** `jul ws list` is responsive; sync does not fetch unnecessary workspaces.

---

# AA) Mixed Git Usage Scenarios (Git Commands While Using Jul)

These scenarios specifically validate “Git + Jul” mixed workflows and ensure Jul remains safe when users (or IDEs) issue Git commands out-of-band.

### IT-GITMIX-001 — `git add -p` / Staging Workflow Is Never Corrupted by Jul

**Covers:** Shadow index separation.  
**Setup:** Create partial staging with `git add -p`.  
**Steps:** Run `jul sync`, `jul status`, `jul trace`.  
**Assertions:** Staged hunks remain unchanged; Jul draft reflects working tree state, not staging state.

### IT-GITMIX-002 — `git stash` / `git stash -u` Round-Trip Does Not Break Jul State

**Covers:** Interaction with stash and untracked files.  
**Steps:** Create dirty state; stash; run `jul sync`; pop stash.  
**Assertions:** Draft and working tree behave predictably; `.jul/**` is never included (should be ignored).

### IT-GITMIX-003 — Manual `git commit` on Workspace Branch Is Detected as “Unadopted”

**Covers:** Unadopted commit detection.  
**Steps:** `git commit` on `refs/heads/jul/<ws>`; run `jul status`.  
**Assertions:** Jul warns; offers `jul checkpoint --adopt` or `jul ws checkout`.

### IT-GITMIX-004 — Adopt a Chain of Git Commits as Checkpoints (If Supported)

**Covers:** Adoption of multiple commits.  
**Setup:** Create 2–3 git commits on workspace branch.  
**Steps:** Run `jul checkpoint --adopt` (or adoption flow) repeatedly or once, per implementation.  
**Assertions:** Each adopted commit becomes a checkpoint with keep-ref and metadata; Change-Id mapping is correct.

### IT-GITMIX-005 — `git commit --amend` on an Adopted Checkpoint Is Rejected or Repaired

**Covers:** Checkpoint immutability defense.  
**Setup:** Create checkpoint via adopt; then amend it with git.  
**Assertions:** Jul detects that checkpoint SHA was rewritten; warns loudly; requires repair (e.g., re-adopt as a new checkpoint) and never silently continues with corrupted metadata.

### IT-GITMIX-006 — `git rebase -i` or History Rewrite on Workspace Branch Is Detected

**Covers:** Out-of-band history rewrite detection.  
**Steps:** Perform interactive rebase on `refs/heads/jul/<ws>`.  
**Assertions:** Jul detects divergence from canonical workspace ref and/or lease; prompts for `jul ws checkout` or a repair flow.

### IT-GITMIX-007 — Manual `git cherry-pick` Into Workspace Branch Is Safe (With Adoption Path)

**Covers:** Cherry-pick interop.  
**Steps:** Cherry-pick a commit onto `refs/heads/jul/<ws>`; run `jul status`.  
**Assertions:** Jul either treats it as unadopted (requires adopt) or supports adoption; no corruption.

### IT-GITMIX-008 — Manual Merge Into Workspace Branch Is Treated as Unadopted (Or Refused)

**Covers:** Merge commit handling in workspace branches.  
**Steps:** Create a merge commit on workspace branch using Git.  
**Assertions:** Jul warns; provides explicit path (adopt or discard); does not silently promote/stack incorrectly.

### IT-GITMIX-009 — `git reset`/`git restore`/`git clean` Interaction Safety

**Covers:** Recovery from destructive commands.  
**Steps:** Use `git restore --staged`, `git reset --mixed`, `git clean -fdx` while working; then run `jul status` and `jul ws checkout`.  
**Assertions:** Jul detects missing `.jul/` and repairs; draft/checkpoint objects remain intact.

### IT-GITMIX-010 — User Runs `git push`/`git fetch` That Changes Remote Tips Behind Jul

**Covers:** “Target rewritten” and “base advanced” detection under mixed usage.  
**Steps:** Force-push target branch; or update refs outside Jul; then run `jul promote` and `jul status`.  
**Assertions:** Jul warns loudly on rewrite; requires explicit confirmation; never silently force-updates targets.

### IT-GITMIX-011 — Manual Modification of Jul Notes Refs Does Not Crash Jul

**Covers:** Robustness against manual `git notes` edits.  
**Steps:** Manually add or delete notes under `refs/notes/jul/*`; run `jul sync`.  
**Assertions:** Jul handles merge conflicts safely; refuses to auto-merge single-truth notes (repo-meta) and requires repair when corrupted.

### IT-GITMIX-012 — Git Configuration Changes Mid-Session (autocrlf/ignorecase) Surface Clear Guidance

**Covers:** Config drift detection and clarity.  
**Steps:** Change git config values that impact working tree representation; run `jul sync` and `jul diff`.  
**Assertions:** Jul does not silently mis-snapshot; it warns when representation changes cause large diffs.

### IT-GITMIX-013 — Hooks Install/Uninstall Is Safe and Idempotent (If Implemented)

**Covers:** Hook management.  
**Steps:** Install hooks twice; uninstall; reinstall.  
**Assertions:** Hooks do not duplicate; adoption logic remains correct; no corruption.


### IT-GITMIX-014 — Manual `git switch` / Detached `HEAD` Mid-Session Is Detected and Recoverable

**Covers:** `HEAD` model enforcement; prevention of cross-branch corruption; recovery guidance.  
**Setup:** Jul initialized; active workspace exists; at least one checkpoint already created.  
**Steps:**
1. Verify `git symbolic-ref HEAD` points to `refs/heads/jul/<ws>`.
2. Run `jul sync` (ensure a current draft exists).
3. Simulate IDE/user branch switch:
   - `git switch -c tmp-branch` **or**
   - `git checkout --detach`
4. Make a small edit to a tracked file.
5. Run `jul status`.
6. Attempt `jul sync`.
7. Attempt `jul checkpoint`.
8. Recover with `jul ws checkout <ws>` (or `jul ws checkout @` for default).
**Assertions:**
- `jul status` warns that `HEAD` is not on the workspace head (`refs/heads/jul/<ws>`) and provides a clear remediation (e.g., `jul ws checkout <ws>`).
- `jul sync` and `jul checkpoint` MUST NOT silently corrupt Jul state. Acceptable behavior is either:
  - **Refuse** with a clear error and next actions, **or**
  - Proceed in a “safe mode” that does not update canonical Jul refs/notes and does not modify non-Jul branches.
- `refs/jul/workspaces/<user>/<ws>` is unchanged by the out-of-band branch switch unless the user explicitly returns to the workspace and performs a Jul operation that updates it.
- After recovery (`jul ws checkout`), `HEAD` is restored to `refs/heads/jul/<ws>`, and the workspace lease/base invariants hold.
**Variants:**
- Detached `HEAD` created by IDE “checkout commit” flows.
- Switch to `main` and create a Git commit; verify Jul offers adopt vs discard paths and does not treat the commit as a checkpoint automatically.

### IT-GITMIX-015 — `git pull` / `git merge` Advances the Tracked Target Branch While a Workspace Exists

**Covers:** Upstream drift observation; publish-remote truth; restack/promote safety under mixed Git usage.  
**Setup:** Publish remote configured; workspace tracks `main` (track ref = `refs/heads/main`); at least one checkpoint exists on the workspace.  
**Steps:**
1. Ensure the workspace has a checkpoint C1 and a current draft.
2. Advance the target branch outside Jul (simulate common muscle-memory operations):
   - `git switch main && git pull` (fast-forward) **or**
   - From a second clone, push a new commit to the publish remote `main`.
3. Return to the workspace via `jul ws checkout <ws>` (re-establish Jul’s `HEAD` model).
4. Run `jul sync`, then `jul status`.
5. Run `jul ws restack` (or `jul promote --to main`) to incorporate the new target tip.
**Assertions:**
- `jul sync` does not silently rewrite the workspace base/draft base due to target branch movement; it only snapshots and reports drift as appropriate.
- `jul status` surfaces that the tracked target advanced using the last known `track_tip` model; after a successful restack/promote, `track_tip` updates to the fetched remote tip used for the operation.
- `jul ws restack` / `jul promote` fetch the **publish remote** and use that target tip as the source of truth (not a potentially diverged local `main`).
- If the target branch was rewritten (force-push), Jul warns loudly and requires explicit confirmation before proceeding (no silent landing).
- If conflicts occur during restack/promote, Jul routes into the conflict flow (`jul merge` suggestion + manual fallback).
**Variants:**
- Target advanced by fast-forward (most common).
- Target rewritten (force-push) while the workspace exists; verify explicit confirmation gating.
- User ran `git pull` while still on the workspace branch (creates a merge commit on the workspace branch): this should be handled equivalently to IT-GITMIX-008 (unadopted merge commit) and must not silently publish.

---

## 4. Minimal “Day-to-Day Confidence Suite” (Recommended)

If you need a fast suite to run on every change to Jul itself, include:

- IT-INIT-001, IT-DOCTOR-001, IT-DOCTOR-002
- IT-HEAD-001
- IT-DRAFT-001, IT-DRAFT-002, IT-DRAFT-004
- IT-SYNC-BASEADV-001
- IT-CP-001, IT-CP-003
- IT-CI-002, IT-CI-005
- IT-PROMOTE-REBASE-001, IT-PROMOTE-REWRITE-001
- IT-SEC-001
- IT-AGENT-006
- IT-DAEMON-001, IT-DAEMON-002
- IT-MERGE-001
- IT-OFFLINE-001
- IT-UNSUPPORTED-001
- IT-ROBUST-005

---

## 5. Implementation Notes (To Keep the Suite Real)

- Avoid interactive prompts in integration tests; provide `--json` and/or deterministic noninteractive flags.
- The hardest bugs will be around multi-device divergence + notes merging + remote rejection behavior. Weight those heavily.
- For platform cases (case sensitivity, Unicode normalization, path length), run a CI matrix or containerized FS simulation if feasible.
