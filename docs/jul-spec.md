# 줄 Jul: AI-First Git Workflow

**Version**: 0.3 (Draft)
**Status**: Design Specification

---

## 0. What Jul Is

Jul is **Git with a built-in agent**. It's a local CLI tool that adds:

- **Rich metadata** on every checkpoint (check results, coverage, lint, traces)
- **Agent-native feedback loop**: checkpoint → get suggestions → act → repeat
- **Continuous sync**: Drafts sync automatically when available (on Jul commands or daemon ticks; see sync settings)
- **Local review agent**: Analyzes code, creates suggestions automatically

```
Agent (Codex / Claude Code / OpenCode)
              │
              ▼
        jul checkpoint
              │
              ▼
    ┌─────────────────────────┐
    │  Jul CLI (local)        │
    │  • Runs tests           │
    │  • Runs review agent    │
    │  • Creates attestation  │
    │  • Syncs to git remote  │
    └─────────────────────────┘
              │
              ▼
    Rich JSON feedback:
    {
      "ci": { "status": "pass", "coverage": 84 },
      "suggestions": [...],
      "next_actions": [...]
    }
              │
              ▼
    Agent acts on feedback, continues
```

**Jul is NOT:**
- A new VCS (it's built on Git)
- A special server (standard git remotes work; sync requires a compatible sync remote)
- A remote CI service (tests run locally)
- A remote execution platform (agents run locally)

**Terminology note:** In this spec, “checks” refers to local lint/test/typecheck runs performed by
Jul. The command remains `jul ci`, and JSON output uses the `ci` field. This does not replace your
project’s remote CI (GitHub Actions, etc.).

**Metadata travels with Git** via refs and notes when the sync remote allows custom refs + notes.
Draft sync additionally requires non‑fast‑forward updates to `refs/jul/sync/*`. If custom refs are
blocked, Jul runs local‑only (promote still works). See Section 0.3, Section 5.7, and Section 10.

### 0.1 Refined Pitch

Jul extends Git with Change-Ids that persist from first draft through production. This lets you operate on logical changes (not just commits): diff a change, revert a change, and trace a change back to the prompts that created it.

Three value props:
- Rich feedback loop: every checkpoint gets feedback (checks, review, suggestions); agents close the loop.
- Effortless sync: your work is always safe and always available.
- Change-Id as durable identity: operate on logical changes, not commits; provenance from prompt to production.

### 0.2 Two Layers: Core vs Review

Jul has a small **core** and an optional **review layer**:

- Jul Core (the default story):
  - Workspaces, drafts, checkpoints, Change-Ids, sync, restack, promote
  - Everything works offline and without any special server
- Review Layer (optional, still local-first):
  - Suggestions, `jul submit`, CR state/comments in notes, richer review flows
  - Useful even for "single player across devices," but not a PR replacement in v1

This spec describes both, but the core invariants are the load-bearing part.

### 0.3 Remote Compatibility and Sync Remotes (Read This First)

Jul's headline feature (continuous sync) depends on what your git remote allows. The safest
default is to use a **separate sync remote** (for example, a remote named `jul`) so you can keep
`origin` locked down while still getting sync.

**Minimum requirements for checkpoint sync:**
- Accept custom refs under `refs/jul/*`
- Accept notes under `refs/notes/jul/*` (read/write/delete)

**Additional requirement for draft sync (per-device drafts):**
- Allow non-fast-forward updates to `refs/jul/sync/*` (branches can remain protected)

Sync remote and publish remote can differ:
- Sync remote carries workspace refs + notes (and per‑device draft refs when draft sync is available)
- Publish remote (often `origin`) carries `refs/heads/*` and is what `jul promote` targets
`jul sync` talks only to the sync remote; publish remote fetches happen during `jul promote` and
`jul ws restack` when needed.

The compatibility table below refers to the sync remote.

Sync capabilities:

| Remote capability | What syncs |
|---|---|
| Custom refs + notes | **Checkpoints + metadata** (workspace ref + notes) |
| + Non-FF updates allowed on `refs/jul/sync/*` | **Drafts** (per-device draft refs) |
| No custom refs | No Jul ref/notes sync (local-only) |

**Important:** Workspace refs always track **checkpoints**. If non‑FF updates are blocked, Jul
still syncs checkpoints + notes; drafts stay local.

Jul should detect this with `jul doctor` and configure sync automatically. Jul will configure
refspecs once a sync remote is set (`jul init` or `jul remote set`), but it does **not** create
the remote server for you—you provide the URL. Section 5.7 and Section 10 define the requirements
and recommended setup.

---

## Quick Start (Happy Path)

```bash
$ jul init                    # one time
# ... write code ...
$ jul checkpoint              # save + run checks + suggestions
# ... iterate ...
$ jul log                     # change-aware history (by Change-Id)
$ jul promote --to main       # publish
```

That’s it. Sync, traces, and suggestions run automatically in the background.

## 1. Goals and Non-Goals

### 1.1 Goals

- **Local-first**: Everything runs on your machine
- **Continuous sync**: Drafts sync automatically when draft sync is available (on commands or daemon ticks)
- **Checkpoint model**: Lock work, agent generates message, run checks, get suggestions
- **Agent-native feedback**: Rich JSON responses for agents to act on
- **Workspaces over feature branches**: Named working streams; target branches remain publish destinations
- **Rich metadata**: checks (lint/test/coverage) and traces attached to checkpoints
- **Git compatibility**: Checkpoint sync requires custom refs + notes; draft sync additionally
  requires non‑FF updates to `refs/jul/sync/*`. Without a compatible sync remote, Jul runs
  local-only (promote still works)
- **JJ friendliness**: Works with JJ's git backend

### 1.2 Non-Goals (v1)

- Replacing Git (Jul extends Git; standard Git commands still work)
- Server-side execution (everything runs locally)
- Multi-user / teams (single-player for v1)
- Code review UI (use external tools)
- Issue tracking
- Replacing PRs/branches on the canonical publish path (branches remain the shared truth)

---

## 2. Core Concepts

### 2.1 Entities

| Entity | Description |
|--------|-------------|
| **Repo** | A normal Git repository |
| **Device** | A machine running Jul, identified by device ID (e.g., "swift-tiger") |
| **User Namespace** | Stable repo-scoped owner ID used in ref paths (`<user>`); stored canonically in repo meta notes |
| **Sync Remote** | Git remote used for Jul refs/notes sync (often a remote named `jul`) |
| **Publish Remote** | Git remote used for promote targets and upstream tracking (often `origin`) |
| **Workspace** | A named stream of work. Can host multiple Change-Ids over time. Replaces feature branches. Default: `@` (normalized to `default` in refs) |
| **Workspace Ref** | Canonical **workspace base tip** (`refs/jul/workspaces/...`) — shared across devices when checkpoint sync is available (usually a checkpoint; after promote, a workspace base marker) |
| **Workspace Lease** | Per-workspace file (`.jul/workspaces/<ws>/lease`) — the semantic lease |
| **Workspace Meta** | Canonical workspace intent stored in `refs/notes/jul/workspace-meta` (base/track/pinning/owner) |
| **Workspace ID** | Stable identifier for a workspace; helps detect name reuse/conflicts across devices |
| **Workspace Base Ref** | The branch or parent change ref this workspace is stacked on |
| **Workspace Base Change-Id** | When stacked, the parent Change-Id this workspace is pinned to |
| **Workspace Base SHA** | Pinned commit SHA of the base ref (diffs/reviews compute against this) |
| **Workspace Track Ref** | The target branch this workspace tracks for upstream drift (usually `refs/heads/main`) |
| **Workspace Track Tip** | Last observed tip of the tracked target branch (local tracking state) |
| **Base Commit** | The parent for the current draft (workspace base tip: latest checkpoint or workspace base marker after promote) |
| **Workspace Base Marker** | Synthetic commit created after promote; tree matches published tip and parent is the last checkpoint tip (marked `Jul-Type: workspace-base`) |
| **Sync Ref** | Per-device **draft** backup (`refs/jul/sync/<user>/<device>/...`) — pushed only when draft sync is available; local-only otherwise |
| **Trace Sync Ref** | Device trace backup (`refs/jul/trace-sync/...`) — pushed when checkpoint sync is available; local-only otherwise |
| **Draft** | Ephemeral commit capturing working tree (parent = base commit) |
| **Trace** | Fine-grained provenance unit (prompt, agent, session) — side history, keyed by SHA |
| **Checkpoint** | A locked unit of work with message, Change-Id, and trace_base/trace_head refs |
| **Change-Id** | Stable identifier for a logical change (`Iab4f3c2d...`), created at the first draft commit and persists after promote (new Change-Id starts for the next change) |
| **Change Ref** | Stable per-change tip ref (`refs/jul/changes/<change-id>`) used for stacking and lookup |
| **Attestation** | Check results (tests/coverage) attached to a trace, draft, checkpoint, or published commit |
| **Suggestion** | Agent-proposed fix targeting a checkpoint |
| **Local Workspace** | Client-side saved state for fast context switching |

**Change-Id scope:** A Change‑Id groups multiple checkpoints and the published commits produced from them. `jul promote` closes the change for new work, but the Change‑Id (and its mappings) remain queryable for diff/revert/blame.

**Change-Id lifecycle (v1 rule):** A new Change‑Id starts **only after `jul promote`**. Creating
checkpoints does **not** start a new Change‑Id; it advances the current one. (Future: explicit
`jul change new` or `jul checkpoint --new-change` could allow manual rollover.)

**Workspace vs Change-Id:** A workspace is a stream; a Change‑Id is the logical change. Workspaces can accumulate multiple Change‑Ids over time (especially the default `@`).

**User namespace (ref stability across devices):**
- `<user>` in ref paths is **not** derived from `user.name` / `user.email`.
- Jul resolves a stable `user_namespace` once per repo and stores it canonically in git notes
  (see `refs/notes/jul/repo-meta` and Section 3.8).
- Local config caches it, but the notes value wins when present.

**Workspace intent (base/track meaning):**
- Workspace properties like `base_ref`, `base_change_id`, `base_sha`, and `track_ref` are
  canonicalized in `refs/notes/jul/workspace-meta`.
- Files under `.jul/workspaces/<ws>/` are a local cache and should be repairable from git state.

### 2.1.1 Core Ontology (Mental Model)

This is the mental model to teach and promote. The entity table above remains the full reference.

| Concept | What it is |
|--------|-------------|
| **Workspace** | Your active stream of work. Can contain multiple changes over time. |
| **Change-Id** | The logical unit. Groups checkpoints into a coherent change and persists after promote. |
| **Checkpoint** | The intentional save point. Locked, durable, and policy-checked. |
| **Trace** | Provenance metadata linked to a checkpoint (prompt, agent, session). |

### 2.1.2 Why Change-Ids?

Change-Ids give a logical change a stable identity across history rewrites and publication steps.

- **Stable across rewrites:** restacks, publish-time rebases, and squashes change commit SHAs but not the Change-Id.
- **Durable after promote:** the Change-Id remains queryable on published branches via notes and mappings.
- **Enables change-level operations:** `jul diff <change-id>`, `jul show <change-id>`, and `jul revert <change-id>` work before and after publication.

**Checkpoint immutability (precise):**
- A checkpoint git object is never rewritten in place.
- There is no checkpoint amend in v1; new work always creates a **new checkpoint**.
- Restack produces new checkpoint commits (new SHAs) while keeping earlier checkpoints reachable via keep‑refs.

**Identifier formats (examples):**
- **Change‑Id**: `Iab4f3c2d...` (logical change group)
- **Checkpoint/Draft/Commit SHA**: `abc1234` (git object id)
- **Trace SHA**: `def4567` (trace commits are just git commits)
- **Suggestion ID**: `01HX7Y9A` (ULID‑ish)

### 2.2 The Trace → Draft → Checkpoint → Promote Model

Jul uses a four-stage model:

```
┌─────────────────────────────────────────────────────────────────────────┐
│  TRACE (side history)                                                   │
│    • Fine-grained provenance: prompt + agent + session                  │
│    • Created explicitly (jul trace) or implicitly (jul sync)            │
│    • Stored as side refs, not in main commit ancestry                   │
│    • Lightweight checks: lint, typecheck                                │
│    • Powers `jul blame` for "how did this line come to exist?"          │
├─────────────────────────────────────────────────────────────────────────┤
│  DRAFT (main ancestry)                                                  │
│    • Shadow capture of your working tree                                │
│    • Continuously updated (on Jul commands or daemon ticks)              │
│    • Synced automatically when draft sync is available                   │
│    • Change-Id assigned at first draft commit (carried forward; persists after promote) │
│    • No commit message yet                                              │
├─────────────────────────────────────────────────────────────────────────┤
│                           jul checkpoint                                │
├─────────────────────────────────────────────────────────────────────────┤
│  CHECKPOINT                                                             │
│    • Locked, immutable                                                  │
│    • Agent generates commit message (or user provides with -m)          │
│    • Records trace_base + trace_head (for blame)                        │
│    • Session summary: AI-generated summary of multi-turn work           │
│    • Checks run, attestation created                                    │
│    • Review runs, suggestions created                                   │
│    • New draft automatically started                                    │
├─────────────────────────────────────────────────────────────────────────┤
│                           jul promote --to main                         │
├─────────────────────────────────────────────────────────────────────────┤
│  PUBLISHED (refs/heads/main)                                            │
│    • Policy-checked (tests pass, coverage met, etc.)                    │
│    • Checkpoints rebased/squashed/merged onto target                    │
│    • Deployable                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

**Key insight**: Your working tree can still be "dirty" relative to HEAD (normal git). Jul continuously captures your dirty state as a draft commit. When the sync remote allows **draft sync**, Jul can push that draft for cross-device recovery; otherwise drafts stay local. The draft is your safety net, not your workspace. `jul checkpoint` is when you say "this is a logical unit." Traces track *how* you got there.

**Agent loop (default happy path):**
```
edit → (agent hook/manual) trace → trace → checkpoint → promote
```
For humans, the only Jul commands you should usually run are `jul checkpoint` and `jul promote`. Use Git (or `jul git`) for commit-level inspection if you prefer.

### 2.2.1 HEAD Model (Git Compatibility)

**Rule:** `HEAD` points at a per-workspace local branch (`refs/heads/jul/<workspace>`) which
advances to the **current base commit** (latest checkpoint, or workspace base marker after
promote). Commands that change the base commit update that ref (so Git does not see a detached
HEAD).

This keeps standard Git tooling happy (`git branch`, IDEs) because the head ref lives under
`refs/heads/*`.

Your working tree is dirty relative to that base; drafts are stored in side refs.

Why this matters:
- `git status`, `git diff`, and `git add -p` remain predictable.
- Drafts stay ephemeral and do not pollute `git log`.
- `jul sync`, `jul trace`, `jul review`, `jul status`, and `jul suggestions` **do not** move `HEAD`.
- `jul checkpoint`, `jul ws restack`, `jul ws checkout`, `jul ws switch`, and `jul promote` **do** move `HEAD`.

**User-facing mental model:** Drafts, sync refs, and shadow indexes are implementation details.
Users only need to think about **workspaces** and **checkpoints**.

### 2.2.2 Jul vs Git Commands (Change-Aware vs Commit-Aware)

Jul does not replace Git. It extends Git with Change-Ids and metadata that persist after promote, so Jul commands are change-aware and can operate on workspaces and published branches.

- **Jul (change-aware):** `jul log`, `jul diff <change-id>`, `jul show <change-id>`, `jul blame --prompts`, plus workflow commands (`checkpoint`, `sync`, `promote`, `ws`).
- **Git (commit-aware):** `git log/diff/show/blame/revert` remain valid on published commits.

For convenience, `jul git <args>` is a thin passthrough to `git`, so you can stay in the Jul CLI without losing any Git capability.

### 2.3 Workspaces Replace Feature Branches

Traditional Git:
```bash
git checkout -b feature-auth    # Create branch
git add . && git commit         # Work
git push origin feature-auth    # Push
git checkout main && git merge  # Merge
```

Jul:
```bash
jul ws new feature-auth         # Create workspace (or use default @)
# ... edit, sync optional ...
jul checkpoint                  # Lock with message
jul promote --to main           # Publish
```

**Key differences**:
- Target branches under `refs/heads/*` remain canonical publish destinations (`main`, `staging`, `release/*`)
- Workspaces replace feature branches (working branches), not canonical branches
- You typically do not **commit** directly to `refs/heads/main` or other targets (work lives in drafts/checkpoints until promote)
- Workspaces are where work happens; their **base tips** live at `refs/jul/workspaces/<user>/<name>`
- Your local checkout is a workspace branch under `refs/heads/jul/<workspace>` (shown by `git branch`)
- Each workspace has a **base ref**:
  - `jul ws new feature` → base is a branch (usually `main`)
  - `jul ws stack child` → base is the parent change ref (`refs/jul/changes/<change-id>`)
- **Sync does not restack base refs.** Upstream integration is explicit (`jul ws restack`) or happens
  at publish time (`jul promote`).
- Default workspace `@` means you don't need to name anything upfront

**Pinned bases (critical):**
- Each workspace stores:
  - `base_ref` — branch or parent change ref
  - `base_change_id` — parent Change-Id when stacked (pins meaning even if the parent workspace rolls forward)
  - `base_sha` — the exact commit it is currently pinned to
  - `track_ref` — the publish target branch to observe for upstream drift (resolved against the publish remote)
- These fields are canonicalized in git notes (`refs/notes/jul/workspace-meta`) so other devices
  can reconstruct the workspace's intent, not just its tree.
- Default `track_ref`:
  - If `base_ref` is a branch, `track_ref = base_ref`
  - If `base_ref` is a change ref, inherit the parent's `track_ref`
- `track_ref` is fetched from the publish remote (often `origin`) during `jul ws checkout`,
  `jul ws restack`, or `jul promote` (not during `jul sync`).
- Diffs/reviews/suggestions are computed against **`base_sha`**, not whatever `base_ref` points to right now.
- If `base_ref` advances, Jul should surface “base advanced, restack when ready” but **must not** silently change the diff.
- Keep these distinct:
  - **Draft base** = `parent(current_draft)` (latest checkpoint, or workspace base marker after promote)
  - **Tracked upstream tip** = tip of `track_ref` (e.g., `origin/main`)
  - Sync must not treat upstream tip changes as a reason to rewrite the draft base once checkpoints exist.

**Base invariants (naming + truth table):**
- `wc_base` = `parent(current_draft)` (the materialized base in your working tree)
- `workspace_tip` = `refs/jul/workspaces/<user>/<ws>` (canonical base across devices)
- `workspace_head` = `refs/heads/jul/<ws>` (local base branch; should match `workspace_tip` after
  checkout, and after sync **when no base_advanced**)
- `checkpoint_tip` = `workspace_tip` **unless** it is a workspace base marker, in which case
  `checkpoint_tip = parent(workspace_tip)`
- `base_sha` = pinned comparison base for diff/review (may differ from `wc_base`)
- `track_tip` = last‑seen upstream tip (observational; never changes `wc_base`)

**How they move:**
- `jul checkpoint` → advances `workspace_tip`/**workspace_head** to the new checkpoint; `wc_base` becomes that checkpoint.
- `jul ws restack` → appends a restack checkpoint; `workspace_tip`/**workspace_head** advance; `base_sha` updates.
- `jul promote` → publishes to target branch, then creates a **workspace base marker** commit
  (tree = published tip, parent = last checkpoint) and advances `workspace_tip`/**workspace_head** to it.
- `jul ws checkout` → materializes `workspace_tip` into the working tree and sets `wc_base`/`workspace_head`.
- `jul checkpoint --adopt` → sets `wc_base` to the adopted commit and advances `workspace_tip` to that commit.

### 2.4 Integration Options

Jul works at multiple levels. Choose your porcelain:
All setups work offline; add a remote only when you want sync/collaboration.

#### 2.4.1 Jul-First Mode

Jul is your primary interface.

```bash
$ jul configure                         # One-time setup
$ jul init my-project                   # Initialize Jul
$ git remote add origin git@github.com:you/myproject.git
$ jul remote publish set origin         # Publish remote (branches)
# Optional but recommended: separate sync remote
$ git remote add jul git@github.com:you/myproject-jul.git
$ jul remote set jul
$ jul doctor                            # Detect sync capabilities (checkpoint vs draft)
# ... edit ...
$ jul checkpoint                        # Lock + message + checks + review
$ jul promote --to main                 # Publish
```

#### 2.4.2 Git + Jul (Invisible Infrastructure)

Git is your porcelain. Jul can sync in background via hooks when a remote is configured.
**Note:** Without `jul hooks install`, raw `git commit` creates commits Jul doesn't track.
Use hooks or `jul checkpoint --adopt` to incorporate Git commits.

```bash
$ git init
$ git remote add origin git@github.com:you/myproject.git
$ jul init
# Optional: keep origin locked down, sync to a separate remote
$ git remote add jul git@github.com:you/myproject-jul.git
$ jul remote set jul
$ jul remote publish set origin
$ jul hooks install
# ... use normal git commands ...
# post-commit hook auto-syncs
$ jul status                            # Check attestations
$ jul promote --to main                 # When ready
```

#### 2.4.3 JJ + Jul

JJ handles local workflow. Jul handles optional remote sync/policy.

```bash
$ jj git init --colocate
$ jul init
$ jul remote set jul                    # or origin, if compatible
$ jul remote publish set origin
$ jul sync --daemon &                   # Background sync
# ... use jj commands ...
$ jul promote --to main
```

#### 2.4.4 Agent Mode

Agents use Jul programmatically with `--json` on all commands.

```bash
$ jul status --json
$ jul checkpoint --json
$ jul apply 01HX... --json
$ jul promote --to main --json
```

### 2.5 Suggestion Lifecycle

Suggestions are agent-proposed fixes tied to a specific checkpoint SHA:

```
checkpoint abc123 (change Iab4f...)
         │
         ▼
Checks + review run
         │
         ▼
suggestions created (base: checkpoint abc123)
         │
         ├─────────────────────────────────────────────────────────┐
         ▼                                ▼                        ▼
   jul apply <id>                  jul reject <id>          ignore (warn on promote)
         │                                │
         ▼                                ▼
   added to current draft          marked rejected
         │
         ▼
   jul checkpoint (locks fix)
```

**Suggestion metadata:**
```json
{
  "id": "01HX7Y9A",
  "change_id": "Iab4f...",
  "base": { "kind": "checkpoint", "sha": "abc123" }, // Base this was created against
  "commit": "def456",         // The suggestion's commit
  "status": "pending",        // pending | applied | rejected | stale
  "reason": "fix_failing_test",
  "confidence": 0.89
}
```

**Staleness:** A suggestion is **fresh** iff:
- `base.kind == "checkpoint"` and `base.sha == parent(current_draft)`
- `base.kind == "draft"` and `base.sha == current_draft`

If the base commit changes (new checkpoint or restack in the same change), existing suggestions become stale:

> Example: create a new checkpoint in the same Change‑Id; prior suggestions become stale because the draft’s base commit advanced.

```
checkpoint abc123 (change Iab4f...)
         │
         ├── suggestion created (base: checkpoint abc123)
         │
         ▼
new checkpoint def456 (same Iab4f...)
         │
         ├── suggestion marked "stale" (base mismatch)
         │
         ▼
$ jul apply 01HX7Y9A
⚠ Suggestion is stale (created for abc123, current is def456)
  Run 'jul review' to generate fresh suggestions.
```

**Why track base?** Change-Id survives restacks and publish-time rewrites, but the code changed. A suggestion that fixed line 45 in abc123 might not apply cleanly to def456 if you edited that area.

**Status transitions:**
- `pending` → `applied` (via `jul apply`)
- `pending` → `rejected` (via `jul reject`)
- `pending` → `stale` (base commit changed)
- `stale` → stays stale (must run fresh review)

**Result**: Clean history with your work and agent fixes as separate checkpoints.

```
main:
  abc123 "feat: add auth"              ← your work (change Iab4f...)
  def456 "fix: null check"             ← agent fix (change Iab4f...)
  ghi789 "feat: add refresh tokens"    ← your work (change Iab4f...)
```

#### 2.5.1 Optional Review Layer: Change Request (CR) Lifecycle (One Change-Id = One CR)

This is part of Jul's optional review layer. The core workflow does not require CRs.

Jul keeps change requests simple: **one Change-Id equals one CR**. A workspace can host multiple
Change-Ids over time; `jul submit` targets the current Change-Id.

- `jul submit` **creates or updates** the CR for the current Change-Id.
- There are **no CR IDs** and no `submit --new`.
- Each submit points the CR at the **latest checkpoint** (a new revision).
- If you created multiple checkpoints before the first submit, the CR reflects the latest one (cumulative diff from the base commit).
- After `jul promote`, the next draft starts a **new Change-Id**, and the next submit opens a **new CR** (prior Change-Ids remain queryable).
- Submit is **optional** — solo workflows can go straight from checkpoint → promote.
- In v1, CR state/comments are notes-based and local-first. They are not a PR replacement.

CR state lives in Git notes so it works offline and syncs with the repo:
- `refs/notes/jul/cr-state` — keyed by the **Change-Id anchor SHA** (the first checkpoint SHA for the change); stores Change-Id, status, and latest checkpoint
- `refs/notes/jul/cr-comments` — keyed by checkpoint SHA; stores CR comments/threads with `change_id` and optional file/line
  - The anchor SHA is also recorded in `refs/notes/jul/meta` for lookup by Change-Id

Comments can be **CR-level** (no file/line, applies to the whole Change-Id) or **checkpoint-level** (anchored to a specific checkpoint/file/line). Threads can span multiple checkpoints by reusing the same `thread_id`.

**CR anchor retention:** The Change‑Id anchor SHA never changes. While a CR is open, that anchor commit is pinned (its keep‑ref does not expire). Retention is based on **last‑touched** for open CRs.

Example:
```bash
$ jul ws new feature-auth
$ jul checkpoint
$ jul checkpoint
$ jul submit            # opens CR for current Change-Id in feature-auth
$ jul checkpoint
$ jul submit            # updates the same CR

$ jul ws stack feature-b  # create dependent workspace
$ jul checkpoint
$ jul submit            # opens CR for current Change-Id in feature-b (stacked)
```

### 2.6 Traces (Provenance Side History)

**The problem Mitchell Hashimoto identified:** "I need a `git blame` equivalent that maps each line back to the prompt that created it."

A checkpoint might come from 20 prompts and 50 file edits. Standard `git blame` shows the checkpoint SHA, but not *how* the code evolved within that checkpoint.

**Solution: Traces as side history.**

Traces capture fine-grained provenance (prompt, agent, session) without polluting the main commit ancestry:

```
Primary ancestry (clean, for promote):
  checkpoint0 ← checkpoint1 ← checkpoint2 → main

Side history (for blame/provenance):
  refs/jul/traces/george/default  →  t3 ← t2 ← t1   (CLI: @)
                               (single tip ref, parent chain provides history)
```

**Naming clarity:**
- **Change-Id** (`Iab4f...`): Stable identifier for a logical change (persists after promote)
- **Trace ID** (`t1`, `t2`): Identifier for a provenance unit within the trace chain

**Trace creation:**

```bash
# Explicit trace with prompt (harness calls this)
$ jul trace --prompt "add user authentication" --agent claude-code

# Or sync creates trace implicitly
$ jul sync   # creates trace from working tree, no prompt

# With full session context
$ jul trace \
  --prompt "fix the failing test" \
  --agent claude-code \
  --session-id abc123 \
  --turn 5
```

**How it flows:**

```
Agent prompt: "add auth"
  │
  ▼
jul trace --prompt "add auth" --agent claude-code
  → creates trace t1 (parent = previous trace or null)
  → updates refs/jul/traces/george/default to point to t1
  
Agent prompt: "use JWT instead"
  │
  ▼
jul trace --prompt "use JWT instead" --agent claude-code
  → creates trace t2 (parent = t1)
  → updates refs/jul/traces/george/default to point to t2

jul sync
  → pushes trace ref (single ref, not N refs)
  → creates draft (tree = t2's tree, parent = base commit)
  → draft is STILL a sibling, not end of trace chain

jul checkpoint "feat: add auth"
  → flushes final trace t3 if working tree changed since t2
  → creates checkpoint (parent = base commit)
  → writes trailers: trace_base = t0, trace_head = t3 (notes optional mirror)
  → trace chain stays for blame
```

**Ref structure (single tip, not N refs):**

Instead of N refs per trace:
```
refs/jul/traces/george/default/t1   # Bad: ref explosion (CLI: @)
refs/jul/traces/george/default/t2
refs/jul/traces/george/default/t3
```

Single tip ref with parent chain:
```
refs/jul/traces/george/default  # Points to t3 (CLI: @)
                                # t3.parent = t2, t2.parent = t1
```

This avoids ref explosion (fetch negotiation, packed-refs size, host limits).

**Why side history, not primary ancestry?**

1. **Merge stays simple** — Two devices with different trace chains? Merge the trees, merge the trace tips.
2. **`git log` stays clean** — No 9 million micro-commits.
3. **Checkpoint model unchanged** — All the sync/promote machinery works as designed.
4. **Provenance is durable** — `jul blame` can query: line → checkpoint → traces → prompt

**Multi-device trace merge:**

When two devices produce different trace chains:

```
Device A: t1 ← t2 ← t3 (tip)
Device B: t1 ← t4 ← t5 (tip)

When **checkpoint tips** match (i.e., `checkpoint_tip` matches across devices), trace history also merges:
  t1 ← t2 ← t3 ─┐
                ├─ t6 (merge trace, two parents)
  t1 ← t4 ← t5 ─┘

refs/jul/traces/george/default now points to t6 (CLI: @)
```

This keeps main ancestry clean while letting `jul blame` traverse the DAG and attribute lines to the real origin trace, not just "merge happened."

**Checkpoint metadata (base + head, not list):**

```json
{
  "checkpoint_sha": "def456",
  "change_id": "Iab4f...",
  "trace_base": "t0_sha",       // Previous checkpoint's trace tip (or null)
  "trace_head": "t3_sha",       // Current trace tip
  "trace_heads": ["t3", "t5"],  // If merge produced multiple heads
  "session_summary": "Added auth with JWT. First tried sessions, switched after test failures."
}
```

These are recorded as commit trailers on the checkpoint commit:
```
Trace-Base: <sha>
Trace-Head: <sha>
```

Blame walks from head(s) to base. Tiny metadata, same power. Avoids blowing Jul’s **self‑imposed**
16KB per‑note cap.

**Privacy defaults (secrets can leak in summaries too!):**

```toml
[traces]
prompt_hash_mode = "hmac"     # hmac (default) | sha256 | off
sync_prompt_summary = false   # Opt-in — summaries CAN leak paraphrased secrets!
sync_prompt_full = false      # Opt-in — definitely can leak
```

| Setting | Default | What syncs | Risk |
|---------|---------|-----------|------|
| `prompt_hash_mode` | `hmac` | HMAC‑SHA256 hash | Low (no cross‑device correlation) |
| `sync_prompt_summary` | false | AI summary | Medium (can paraphrase secrets) |
| `sync_prompt_full` | false | Full text | High |

HMAC prompt hashes reduce correlation risk by using a **local** secret key (stored in
`.jul/keys/prompt-hmac`, not synced). If you need cross-device equality, set
`prompt_hash_mode = "sha256"` and accept the increased risk.

If `sync_prompt_summary = true`, Jul runs a secret scrubber before syncing (detects API keys, passwords, tokens). But scrubbing isn't perfect — if you're paranoid, keep summaries local.

**Also covered by these privacy rules:** `session_summary`, agent review summaries, and checks output excerpts
are **local‑only by default** and must opt in to sync (with the same scrubber). Treat all AI‑generated
summaries and test output as potentially sensitive.

**Draft sync safety:** Before pushing draft refs, Jul scans **new/modified blobs** for high‑signal
secret patterns. If a secret is detected, Jul blocks the remote draft push by default (draft stays
local) and requires explicit override to sync it.

```bash
$ jul blame src/auth.py --prompts

44 │ change Iab4f... (checkpoint abc123) claude-code
   │ Prompt: [hash only, summary stored locally]

# If you have summary locally:
$ jul blame src/auth.py --prompts --local

44 │ change Iab4f... (checkpoint abc123) claude-code
   │ Summary: "Added null check for auth token"
   │ Prompt: "add null check for missing auth token"
```

This prevents accidental secret exfiltration to remotes.

**Checkpoint flush rule:**

`jul checkpoint` MUST flush a final trace before creating the checkpoint:

1. If working tree differs from last trace → create final trace (tree matches)
2. Then create checkpoint with trace_head = final trace

This ensures checkpoint tree and trace_head tree are identical. Otherwise blame becomes "almost right" (which is worse than wrong).

**Integration modes:**

| Mode | How traces are created | Prompt attached? |
|------|------------------------|------------------|
| **Harness integration** | Harness calls `jul trace --prompt "..."` | Yes |
| **Manual** | User calls `jul trace` | Optional |
| **Auto (no harness)** | `jul sync` creates trace implicitly | No |

Without harness integration, you still get file-level blame (which trace introduced which files), just no semantic context.

**Checks on traces:**

Traces get cheap, fast checks (lint, typecheck). Full checks run on checkpoint.

```bash
$ jul log --traces
abc123 (change Iab4f...) "feat: add auth"
  ├── (sha:abc1) claude-code "add auth" (auth.py, models.py)
  │       ✓ lint pass, ✓ typecheck pass
  ├── (sha:def2) claude-code "use JWT instead" (auth.py)
  │       ✓ lint pass, ✗ typecheck fail
  └── (sha:ghi3) claude-code "fix type error" (auth.py)
          ✓ lint pass, ✓ typecheck pass
```

---

## 3. Git Layer: Ref Namespaces

Jul uses Git refs with clear separation between sync and publish.

### 3.1 Published Refs (Promote Targets)

```
refs/heads/<branch>     # main, staging, etc.
refs/tags/<tag>         # Release tags
```

Standard Git refs. Any Git client can interact with them. Only `jul promote` updates them.

### 3.2 Change Refs (Stable Per Change-Id Tips)

```
refs/jul/changes/<change-id>
```

A stable ref per Change-Id that points to the latest checkpoint for that change. Change refs are
the stacking boundary: children stack onto a parent change ref, not the parent workspace stream.

Rules:
- Advances when a change gets a new checkpoint.
- Remains at the last checkpoint after promote (the workspace may move on, the change ref does not).
- Used for change-aware lookup on published branches.

Examples:
```
refs/jul/changes/Iab4f3c2d...
refs/jul/changes/Icd5e6f7a...
```

### 3.2.1 Anchor Refs (Change-Id → Anchor SHA)

```
refs/jul/anchors/<change-id>
```

A stable ref that points to the **first checkpoint** (anchor SHA) for a Change‑Id. This provides
O(1) lookup from Change‑Id → anchor without scanning notes. It never moves once created and is
safe for fast‑forward‑only remotes.

### 3.3 Workspace Refs

```
refs/jul/workspaces/<user>/<workspace>
```

The **canonical workspace base tip** — the shared truth for this workspace.

- With a compatible sync remote (custom refs + notes), this ref is shared across devices.
- Without one, this ref stays local (no remote sync).

Workspace names are normalized for ref safety. The CLI alias `@` resolves to the normalized
workspace name `default` in ref paths and local state directories.

**`<user>` namespace stability (cross-device):**
- `<user>` is a stable repo-scoped `user_namespace`, not `user.name`.
- Jul should resolve it in this order:
  1. `refs/notes/jul/repo-meta` (root-commit keyed) when present
  2. Local cached config
  3. Generate a new namespace and publish it to repo-meta when possible

**Workspace intent is git-native:**
- On every workspace tip update, Jul should write a workspace intent note in
  `refs/notes/jul/workspace-meta` keyed by that canonical tip commit.
- Other devices should treat that note as the source of truth for base/track/pinning and repair
  local `.jul/workspaces/<ws>/` state from it when needed.

Examples:
```
refs/jul/workspaces/george/default        # Default workspace (CLI: @)
refs/jul/workspaces/george/feature-auth   # Named workspace
```

Updated only when:
- `jul checkpoint` creates a new checkpoint
- `jul ws restack` / `jul ws checkout` moves the base to a different checkpoint
- `jul promote` (creates a workspace base marker after publish)

After promote, the workspace ref advances to a **workspace base marker** commit whose tree matches
the published tip and whose parent is the last checkpoint tip.

By construction, workspace ref updates are fast‑forward (checkpoint/base‑marker chain).

Local HEAD targets (local-only branches; never pushed or fetched):
```
refs/heads/jul/<workspace>    # Base ref for this workspace; HEAD points here
```
Refspecs must exclude `refs/heads/jul/*`.

### 3.4 Sync Refs

```
refs/jul/sync/<user>/<device>/<workspace>
```

Your **per-device draft ref** (the latest draft snapshot for this device).

- Pushes only when **draft sync** is available (non‑FF allowed for `refs/jul/sync/*`).
- Otherwise it stays local.

Examples:
```
refs/jul/sync/george/swift-tiger/default     # Laptop backup (CLI: @)
refs/jul/sync/george/quiet-mountain/default  # Desktop backup (CLI: @)
refs/jul/sync/george/swift-tiger/feature-auth
```

**Device ID:**
- Auto-generated on first `jul init` (e.g., "swift-tiger", "quiet-mountain")
- Stored in `~/.config/jul/device`
- Two random words, memorable and unique enough for personal use

**The relationship:**
- Workspace ref = latest **checkpoint** for the workspace (shared if checkpoint sync is available)
- Sync ref = latest **draft** for this device
- The draft's parent is the workspace base tip (unless your base is stale)

Draft refs are per-device; handoff is explicit via `jul draft adopt`.

### 3.5 Trace Refs (Provenance Side History)

Traces mirror the workspace/sync pattern with two ref levels:

```
refs/jul/traces/<user>/<workspace>              # Canonical tip (advances with checkpoint tip)
refs/jul/trace-sync/<user>/<device>/<workspace> # Device backup (pushed when sync is enabled)
```

**Why two levels?** Same reason as workspace/sync: canonical tip advances only when workspace
does, but the device backup never loses work during "conflicts pending" state. If no compatible
sync remote is configured, the backup ref stays local.

Examples:
```
refs/jul/traces/george/default                    # Canonical trace tip (CLI: @)
refs/jul/trace-sync/george/swift-tiger/default    # Laptop's trace backup (CLI: @)
refs/jul/trace-sync/george/quiet-mountain/default # Desktop's trace backup (CLI: @)
```

**Not N refs per trace:** To avoid ref explosion (fetch negotiation, packed-refs, host limits), we store one tip ref per workspace, not one ref per trace commit.

**Trace chain structure:**
```
refs/jul/traces/george/default  →  abc123  (tip SHA; CLI: @)
                              │
                              ▼
                            def456
                              │
                              ▼
                            ghi789
                              │
                              ▼
                            (null or previous checkpoint's trace_head)
```

**Trace ID is display-only:** The `t1, t2, t3` notation is for human readability (computed from position in chain or short SHA). Everything is keyed by the trace commit SHA, not a separate "trace ID" field.

**Multi-device trace merge:** When two devices produce different trace chains, the merge creates a trace merge commit with two parents:

```
refs/jul/traces/george/default  →  merge_sha  (merge trace; CLI: @)
                               /   \
                          abc123   xyz789
                             │       │
                          def456   uvw012
```

The merge trace commit uses strategy `ours` for its tree (tree = the **canonical checkpoint tip**;
if `workspace_tip` is a base marker, use `parent(workspace_tip)`). This keeps both device histories reachable
without requiring code conflict resolution just to unify traces.

This lets `jul blame` traverse the DAG to find the real origin trace.

**Trace metadata** (stored in notes keyed by trace commit SHA):
```json
{
  "prompt_hash": "hmac:abc123...",
  "agent": "claude-code",
  "trace_type": "prompt",       // prompt | sync | merge | restack
  "session_id": "abc123",
  "turn": 5,
  "device": "swift-tiger",
  "prompt_summary": "Added null check for auth token"
}
```

`prompt_summary` and `prompt_full` are **only present in synced notes** when `traces.sync_prompt_summary` / `traces.sync_prompt_full` are enabled. With defaults, only the hash is synced.

Note: No "trace_id" field — use short SHA for display. `trace_type=merge` and `trace_type=restack`
mark connective traces; `jul blame` skips attribution to those traces.

**Optional blame index (v1.1-friendly, can be sparse in v1):**
- Store a small per-trace index in `refs/notes/jul/trace-index` keyed by trace SHA.
- Suggested fields: `changed_paths[]`, `patch_id`, and optional `hunk_hashes[]`.
- `jul blame` can use this to avoid diffing full trees repeatedly.

**Privacy defaults (secrets can leak in summaries too):**

```toml
[traces]
prompt_hash_mode = "hmac"     # hmac (default) | sha256 | off
sync_prompt_summary = false   # Opt-in (summaries CAN leak secrets!)
sync_prompt_full = false      # Opt-in (definitely can leak)
```

By default, only the **HMAC** prompt hash syncs. Summaries are generated locally and stay local unless explicitly opted in. If `sync_prompt_summary = true`, Jul runs a secret scrubber before syncing (detects API keys, passwords, tokens).
If you opt into `prompt_hash_mode = "sha256"`, prompt hashes reveal equality and enable dictionary attacks on low-entropy prompts.

**Local storage:**
```
.jul/traces/
├── prompts/           # Full prompt text (keyed by trace SHA)
└── summaries/         # AI summaries (keyed by trace SHA)
```

**Lifecycle:**
- Created by `jul trace` (explicit) or **implicitly** on `jul sync` / `jul checkpoint` when
  implicit tracing is enabled (and throttles allow)
- Device trace-sync ref pushes when checkpoint sync is available; local-only keeps it local
- **Canonical trace tip advances with the checkpoint tip**  
  If the workspace base advanced but you haven't incorporated it, **do not update `refs/jul/traces/...`**
  (only update `trace-sync/...`). Base markers do not advance trace tips.
- Referenced by checkpoint metadata: `{trace_base, trace_head}`
- **Retention:** trace commits form a chain, so they are effectively retained while the trace tip is reachable.
  (Future: segment traces per checkpoint for bounded retention.)

**Idempotency:** `jul sync` does NOT create a new trace if working tree equals current trace tip tree. This prevents trace spam from repeated syncs with no actual changes.

**Not part of main ancestry:** Drafts and checkpoints do NOT have traces as parents. Traces are queryable side data.

### 3.6 Suggestion Refs

```
refs/jul/suggest/<Change-Id>/<suggestion_id>
```

- Points to suggested commit — the actual code changes
- Tied to a Change-Id **and** a specific base checkpoint SHA
- Immutable once created
- Can be fetched, inspected, cherry-picked
- Metadata (reasoning, confidence, base) stored in notes

**Staleness:** A suggestion is **fresh** iff:
- `base.kind == "checkpoint"` and `base.sha == parent(current_draft)`
- `base.kind == "draft"` and `base.sha == current_draft`

If the base commit changes (new checkpoint or restack in the same change), existing suggestions become stale.

**Cleanup:** Suggestion refs are deleted when their parent checkpoint's keep-ref expires. This prevents ref accumulation:

```
refs/jul/keep/george/default/Iab4f.../abc123  expires  # CLI: @
    → delete refs/jul/suggest/Iab4f.../*
    → delete associated notes
```

Without this, suggestion refs would accumulate forever even after their checkpoints are GC'd.

### 3.7 Keep Refs

```
refs/jul/keep/<user>/<workspace>/<change_id>/<sha>
```

Anchors checkpoints for retention/fetchability. Without a ref, git may GC unreachable commits.

### 3.8 Notes Namespaces

**Synced notes (pushed to remote, with privacy rules):**
```
refs/notes/jul/repo-meta                 # Repo-scoped metadata (user namespace, repo id)
refs/notes/jul/workspace-meta            # Workspace intent (base/track/pinning/owner)
refs/notes/jul/attestations/checkpoint   # Checkpoint check results (keyed by SHA)
refs/notes/jul/attestations/published    # Published check results (keyed by SHA)
refs/notes/jul/attestations/trace        # Trace check results (keyed by trace SHA)
refs/notes/jul/traces                    # Trace metadata (prompt hash, summary, agent)
refs/notes/jul/trace-index               # Optional blame index (changed paths, patch/hunk hashes)
refs/notes/jul/agent-review              # Agent review summaries/results (synced only when enabled)
refs/notes/jul/cr-comments               # Review layer: CR comments/threads (keyed by checkpoint SHA)
refs/notes/jul/cr-state                  # Review layer: CR state (keyed by Change-Id anchor)
refs/notes/jul/meta                      # Change-Id mappings
refs/notes/jul/change-id                 # Reverse index: commit SHA -> Change-Id (+ promote context)
refs/notes/jul/suggestions               # Suggestion metadata
```

**Repo meta (stable user namespace):**
- `refs/notes/jul/repo-meta` is keyed by the root commit when available.
- It stores repo-scoped identity like:
  - `repo_id` — stable repo identifier
  - `user_namespace` — stable owner segment used in ref paths (`<user>`)
- In an empty repo, `user_namespace` is local-only until the first commit exists; then publish it.
- On a new device, Jul should fetch this note early and adopt `user_namespace` when present.

Example repo meta note:
```json
{
  "repo_id": "jul:8b1f4c2d",
  "user_namespace": "george-7b3c",
  "created_at": "2026-01-25T09:00:00Z",
  "updated_at": "2026-01-25T09:00:00Z"
}
```

**Workspace meta (intent, not just trees):**
- `refs/notes/jul/workspace-meta` is keyed by the canonical workspace tip commit
  (`refs/jul/workspaces/<user>/<ws>`).
- It stores workspace intent and pinning so other devices can reconstruct meaning:
  - `workspace_id`, `workspace_name`, `owner_namespace`
  - `base_ref`, `base_change_id`, `base_sha`
  - `track_ref`, `track_tip`
  - timestamps and optional stacking metadata

Example workspace meta note:
```json
{
  "workspace_id": "ws:5c3a2e9d",
  "workspace_name": "default",
  "owner_namespace": "george-7b3c",
  "base_ref": "refs/heads/main",
  "base_change_id": null,
  "base_sha": "abc123",
  "track_ref": "refs/heads/main",
  "track_tip": "def456",
  "updated_at": "2026-01-25T09:12:00Z"
}
```

**Local-only storage (not synced):**
```
.jul/ci/                  # Draft attestations (device-scoped, ephemeral)
.jul/workspaces/<ws>/     # Per-workspace cache (lease, track tip, cached meta)
.jul/local/               # Saved local workspace states
.jul/traces/              # Full prompt text and summaries (local by default)
```

Notes are pushed with explicit refspecs. Draft attestations are local-only by default to avoid multi-device write contention.
Checkpoint attestation sync is **structured only** by default (no raw stdout/stderr; failure snippets
are opt‑in and scrubbed).

### 3.9 Complete Ref Layout

```
refs/
├── heads/                           # Promote targets
│   ├── main
│   ├── staging
│   └── jul/                          # Local workspace branches (HEAD targets; never pushed)
│       └── <workspace>
├── tags/
├── jul/
│   ├── changes/                     # Stable per-change tips (stacking boundary)
│   │   └── <Change-Id>
│   ├── anchors/                     # Change-Id → anchor SHA (first checkpoint)
│   │   └── <Change-Id>
│   ├── workspaces/                  # Canonical workspace base tips (shared truth)
│   │   └── <user>/
│   │       ├── default              # CLI: @
│   │       └── <named>
│   ├── sync/                        # Per-device drafts (draft sync only)
│   │   └── <user>/
│   │       └── <device>/
│   │           ├── default          # CLI: @
│   │           └── <named>
│   ├── traces/                      # Canonical trace tips (advance with checkpoint tip)
│   │   └── <user>/
│   │       ├── default              # Points to trace tip SHA, parent chain provides history (CLI: @)
│   │       └── <named>
│   ├── trace-sync/                  # Device backups for traces (sync-enabled)
│   │   └── <user>/
│   │       └── <device>/
│   │           ├── default          # CLI: @
│   │           └── <named>
│   ├── suggest/
│   │   └── <Change-Id>/             # Change-Id (Iab4f...)
│   │       └── <suggestion_id>
│   └── keep/
│       └── <user>/
│           └── <workspace>/
│               └── <Change-Id>/
│                   └── <sha>
└── notes/jul/
    ├── attestations/
    │   ├── checkpoint
    │   ├── published
    │   └── trace
    ├── repo-meta                    # Repo identity + user namespace (root-commit keyed)
    ├── workspace-meta               # Workspace intent (keyed by canonical workspace tip)
    ├── change-id                    # Reverse index: commit SHA -> Change-Id
    ├── traces                       # Trace metadata (prompt hash, agent, session)
    ├── trace-index                  # Optional blame index (changed paths, patch/hunk hashes)
    ├── agent-review                 # Agent review summaries/results
    ├── cr-comments                  # Review layer: CR comments/threads (keyed by checkpoint SHA)
    ├── cr-state                     # Review layer: CR state (keyed by Change-Id anchor)
    ├── meta
    └── suggestions
```

Notes:
- The CLI alias `@` is normalized to `default` in ref paths and local state directories.
- `<user>` refers to the stable repo-scoped `user_namespace` from `refs/notes/jul/repo-meta`.

**Note:** No `refs/notes/jul/prompts` — prompt data is either:
- Trace-level: stored in `refs/notes/jul/traces` (per-turn, from harness)
- Checkpoint-level: the commit message itself (high-level intent)

---


## 4. Policy Model

### 4.1 Promote Policies

```toml
# .jul/policy.toml
[promote.main]
required_checks = ["compile", "test"]
min_coverage_pct = 80
require_suggestions_addressed = false   # Warn only
strategy = "rebase"                     # rebase | squash | merge
```

### 4.2 Promote Strategies

| Strategy | Behavior |
|----------|----------|
| `rebase` | Each checkpoint becomes a commit on target (linear history) |
| `squash` | All checkpoints squashed into single commit |
| `merge` | Merge commit joining workspace to target |

---

## 5. Git Implementation Details

This section addresses how Jul concepts map to Git.

### 5.1 Draft Representation

**Drafts are real git commits.**

A draft is a commit with:
- A placeholder message (`[draft] WIP`)
- A Change-Id trailer starting with the first draft commit
- Parent = base commit (workspace base tip: latest checkpoint or workspace base marker)
- Pointed to by this device's sync ref (local, and pushed only if draft sync is available)

Draft commits can be detected by the `[draft] WIP` message prefix (and optionally a `Jul-Type:
draft` trailer).

```
commit abc123
Author: george <george@example.com>
Date:   Mon Jan 19 15:30:00 2026

    [draft] Work in progress
    
    Change-Id: Iab4f3c2d1e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b   (from first draft)
```

**Each sync creates a NEW draft commit only if the tree changed:**
- Same parent (base commit)
- New tree (current working directory state)
- Force-updates the per-device sync ref (when draft sync is available)
- Old draft becomes unreachable (ephemeral)

If the tree is unchanged, Jul reuses the existing draft SHA (still fetches the latest workspace base tip and records drift).

This avoids "infinite WIP commit chain" — there's always exactly one draft commit **per device per workspace**, with parent = base commit. Drafts are siblings (same parent), not ancestors of each other.

**Why commits, not sidecar state:**
- Git tools work (diff, log, bisect)
- Push to any compatible git remote
- JJ interop preserved
- Attestations attach via notes

**Shadow index for non-interference:**

Jul uses a shadow index so it doesn't interfere with your normal git staging:

Before writing the draft tree, Jul applies `.gitignore`, `.jul/syncignore`, and the
security‑first draft ignore list **to the shadow index**. Excluded paths are never added to the
draft commit (they are filtered before `git add -A` / `git write-tree`), not merely blocked at
push time.

```bash
# Jul sync implementation
GIT_INDEX_FILE=.jul/draft-index git add -A
GIT_INDEX_FILE=.jul/draft-index git write-tree
# Create commit from tree with parent = base commit
# Force-update per-device draft ref (when allowed)
# Workspace ref is updated only by `jul checkpoint`
# User's .git/index is untouched
```

This means:
- `git add -p` works normally
- `git stash` works normally
- Jul's draft commits are separate from your staged changes

**Crash safety:** Shadow index uses atomic writes (write to temp file, then rename). If Jul crashes mid-sync, the shadow index may be stale but not corrupt — next sync regenerates it from the working tree. The shadow index is purely derived state.

### 5.2 Checkpoint vs Draft

| Aspect | Draft | Checkpoint |
|--------|-------|------------|
| Message | `[draft] WIP` | Agent-generated or user-provided |
| Parent | Base commit | Base commit |
| Mutable | Yes (replaced on each sync) | No (immutable) |
| Checks run | Optional | Always |
| Retention | Ephemeral (no keep-ref) | Keep-ref created |
| Attestations | Temporary | Permanent |

**Checkpoint is a NEW commit, not a rewritten draft:**

```
Before checkpoint:
  parent ◄── draft (abc123, "[draft] WIP")

After checkpoint:
  parent ◄── checkpoint (def456, "feat: add auth") ◄── new draft (ghi789, "[draft] WIP")
```

This keeps checkpoint SHAs stable for attestations.
`jul checkpoint` updates `refs/jul/workspaces/*` and notes, and advances the local workspace head
ref (`refs/heads/jul/<workspace>`).

### 5.3 Sync and Draft Handoff

**Canonical vs device refs (capabilities):**

```
# Checkpoint sync available (custom refs + notes)
refs/jul/workspaces/george/default        ← workspace base tip (CLI: @)

# Draft sync available (adds non-FF updates for refs/jul/sync/*)
refs/jul/sync/george/swift-tiger/default  ← this device's draft (CLI: @)

# Local-only (no compatible sync remote)
refs/jul/workspaces/george/default        ← local workspace base tip
refs/jul/sync/george/swift-tiger/default  ← local draft
```

**Plus local tracking files (per-workspace):**

```
.jul/workspaces/default/lease        ← Last workspace checkpoint incorporated locally (CLI: @)
.jul/workspaces/default/track-tip    ← Last observed tip of tracked branch (e.g., origin/main)
.jul/workspaces/feature-auth/lease  ← Same, for feature-auth workspace
.jul/workspaces/feature-auth/track-tip  ← Same, for feature-auth workspace
```

**Sync capabilities:**
- **Checkpoint sync**: workspace ref + notes (requires custom refs + notes)
- **Draft sync**: per-device draft ref (requires non‑FF updates to `refs/jul/sync/*`)

Checkpoint sync moves the **workspace base ref**, which is usually a checkpoint tip but may be a
workspace base marker immediately after promote.

#### How sync works

```bash
$ jul sync
Syncing...
  ✓ Fetched workspace base tip
  ✓ Draft snapshot updated
  ✓ Draft ref pushed (if available)
  ⚠ Base advanced (if another device checkpointed)
```

**The sync algorithm:**
0. **Fetch notes early and resolve identity** (if a sync remote is configured):
   - Fetch `refs/notes/jul/repo-meta` (and notes generally) from the sync remote.
   - Resolve `<user>` as the repo's stable `user_namespace` from repo-meta when present.
1. **Fetch workspace base tip** → `workspace_tip`:
   - Checkpoint sync available: `workspace_tip = refs/jul/workspaces/<user>/<workspace>`
   - Otherwise: `workspace_tip = local refs/jul/workspaces/<user>/<workspace>`
2. **Snapshot locally** → `local_draft`:
   - Create/update the draft commit as a sibling of the base checkpoint.
2.5. **Draft secret scan (before remote push):**
   - Scan **new/modified blobs** vs the base for high‑signal secret patterns (keys, tokens, creds).
   - If detected: **block draft push** by default, keep the draft local, and surface a warning.
   - Allow explicit override (`jul sync --allow-secrets` or config).
3. **Draft sync (if available)**:
   - Push this device's draft ref with `--force`
   - If draft sync is unavailable, keep the draft ref local
4. **Validate the lease**:
   - If `workspace_lease` is set but is not an ancestor of (or equal to) `workspace_tip`, treat it
     as corrupted and require `jul ws checkout` (or a repair flow).
5. **Detect base advancement**:
   - `local_base = parent(local_draft)`
   - If `workspace_tip` exists and `local_base != workspace_tip`, mark **base_advanced**.
   - Do **not** rewrite the draft base automatically.
6. **Advance `workspace_lease` only when incorporated**:
   - Update `workspace_lease` only after the working tree is based on `workspace_tip`
     (e.g., after `jul ws checkout`, `jul ws restack`, or a new `jul checkpoint` on top of it).
7. **Observe tracked upstream without silently restacking**:
   - Do **not** fetch the publish remote during `jul sync`.
   - Use the last known `track_tip` (updated by `jul ws checkout`, `jul ws restack`, or `jul promote`).
   - Record drift if a newer tip is known; do not rewrite the draft base during sync.

**Note:** `jul sync` never moves the workspace ref. Only checkpoints/restacks/checkout change the
workspace base tip.

**Why the lease matters:** It tracks the last workspace checkpoint you have incorporated locally.
Advancing it without updating the working tree risks clobbering remote changes later.

**Target drift is observed but integrated at restack/promote:** Jul reports drift using the last
known `track_tip`, but does not change the base. Integration happens only via explicit restack or
at promote time.

**Diverged workspace state (multi-device):**
If a checkpoint push is rejected (non‑FF) or `workspace_tip` advances on another device, the
workspace is **diverged** until you `jul ws restack` or `jul ws checkout`.

In diverged state:
- You can keep drafting and checkpointing **locally** (new checkpoints are kept locally).
- `jul sync` continues to update/push the **draft** ref (if draft sync is available).
- `jul promote` is blocked until you restack or checkout to a canonical base.
- `jul status` should clearly report the divergence and next actions.

#### Draft handoff (explicit)

Drafts are **per-device**. To bring a draft from another device, use an explicit adopt flow:

```bash
$ jul draft list --remote
swift-tiger  @  base=def456  updated=2m ago
quiet-mountain  @  base=def456  updated=5m ago

$ jul draft adopt swift-tiger
Adopting draft from swift-tiger...
  ✓ Base matches (def456)
  ✓ Draft merged (no conflicts)
  ✓ Working tree updated
```

**Adopt algorithm (high level):**
1. Fetch `refs/jul/sync/<user>/<device>/<workspace>` for the target device.
2. Verify the base checkpoint matches your `local_base`.
3. If base matches: 3-way merge of draft trees, create a **new local draft** (single parent = base).
4. If base differs: refuse by default and require `--onto <checkpoint>` (explicit restack).
5. If conflicts: defer to `jul merge`.

This keeps draft handoff explicit and avoids silent rebases.

#### Conflicts and `jul merge`

If `jul draft adopt` or `jul ws restack` produces conflicts, `jul merge` applies the agent-assisted
merge flow to produce a new draft commit and update the working tree.

After `jul merge`:

```json
{
  "merge": {
    "status": "resolved",
    "suggestion_id": "01HX..."
  },
  "suggestion": {
    "type": "conflict_resolution",
    "conflicts": [
      {"file": "src/auth.py", "strategy": "combined"}
    ]
  },
  "next_actions": ["apply 01HX...", "reject 01HX..."]
}
```

**This differs from git:**
- Git: conflict blocks you immediately
- Jul: conflicts appear only during explicit adopt/restack; sync itself does not block you

#### Trace Sync Algorithm

Traces mirror the workspace/sync pattern:

```
refs/jul/traces/george/default                    ← canonical trace tip (CLI: @)
refs/jul/trace-sync/george/swift-tiger/default    ← this device's trace backup (CLI: @)
```

**The trace sync algorithm (runs as part of `jul sync`):**

1. **Update** this device's trace backup:
   - Checkpoint sync available: push to `refs/jul/trace-sync/<user>/<device>/<workspace>` (fast‑forward only)
   - Local-only: keep the trace-sync ref local
2. **Resolve** canonical trace tip → `trace_remote`:
   - Checkpoint sync available: fetch `refs/jul/traces/<user>/<workspace>`
   - Local-only: use the local canonical trace tip
3. **Compare** `trace_remote` to local trace tip
4. **If same or fast-forward**: canonical trace = local (simple case)
5. **If diverged** (both devices created traces) **and checkpoint tips match**:
   - Create **trace merge commit** with two parents: `trace_remote` and local tip
   - Tree = **canonical workspace tip after sync** (strategy `ours`)
   - Checkpoint sync available: push trace merge as new canonical tip
   - Local-only: keep the canonical trace ref local
6. **If base is advanced (restack/checkout needed)**:
   - Do **not** update `refs/jul/traces/...`
   - Only update this device’s `trace-sync/...` ref
   - (Optional) create local trace merge, but keep canonical frozen until workspace resolves

```
Before merge:
  Device A trace: t1 ← t2 ← t3
  Device B trace: t1 ← t4 ← t5

After trace merge:
  Canonical:  t1 ← t2 ← t3 ─┐
                             ├─ merge_trace (tree = canonical workspace tip after sync)
              t1 ← t4 ← t5 ─┘
```

**Why strategy `ours` for trace merge tree?** The trace merge exists purely to keep both device histories reachable for `jul blame`. The actual code state is determined by the workspace base tip (often the checkpoint tip) and your local draft, not the trace merge. So we use the canonical checkpoint tree as the trace merge tree.

**Timing:** Trace sync happens alongside checkpoint sync:
- If trace histories diverged but checkpoint tips match → trace merge proceeds
- If checkpoint base advanced (you need restack/checkout) → canonical trace tip waits; device trace backups still update

**Idempotency:** If working tree equals current trace tip tree, no new trace is created. This prevents trace spam from repeated syncs.

### 5.4 Sync Modes

**Local-first:** Jul works with or without a remote.

Without a remote configured:
```bash
$ jul sync
Syncing...
  ✓ Draft snapshot updated
  
(No remote configured — working locally)
```

Everything works locally: drafts, checkpoints, workspaces, attestations. The sync just doesn't push/fetch. Add a remote later via `jul remote set` when you're ready.

**With a remote configured:** Jul syncs to the remote using one of three modes:

```toml
# ~/.config/jul/config.toml or .jul/config.toml
[sync]
mode = "on-command"   # on-command | continuous | explicit
```

#### Mode 1: `on-command` (default)

JJ-style. Sync happens automatically on every `jul` command.

```bash
$ jul status      # Syncs draft first, then shows status
$ jul checkpoint  # Syncs draft, then locks it
$ jul ws switch   # Syncs draft, then switches
```

**Implementation:**
- Every `jul` command starts with "sync current draft"
- Fetch workspace base tip, update local draft snapshot, push draft ref if available
- No daemon needed
- Sync is implicit but predictable

```toml
[sync]
mode = "on-command"
```

**Pros:** No daemon, predictable, sync happens when you're "at the keyboard"
**Cons:** Stale if you don't run jul commands for a while

#### Mode 2: `continuous`

Dropbox-style. Daemon watches filesystem, syncs automatically.

```bash
$ jul sync --daemon &    # Start daemon (or auto-start on jul init)

# Daemon watches files, syncs when stable
# You never think about it
```

**Implementation:**
- Uses inotify (Linux) / FSEvents (macOS) / ReadDirectoryChangesW (Windows)
- Debounce: waits for write burst to settle
- Runs the same sync algorithm: fetch workspace base tip → update local draft → push draft ref if available
- If base advanced, daemon records it and keeps updating the local draft only

**Configuration:**
```toml
[sync]
mode = "continuous"
debounce_seconds = 2        # Wait for writes to settle
min_interval_seconds = 5    # Don't sync more often than this
```

**Pros:** Never lose work, seamless multi-device handoff (when draft sync is available)
**Cons:** Background process, more resource usage, more edge cases

#### Mode 3: `explicit`

Full manual control. Sync only when you say so.

```bash
$ jul sync        # Explicit sync
$ jul checkpoint  # Also syncs (checkpoint implies sync)
```

```toml
[sync]
mode = "explicit"
```

**Pros:** Maximum control, no surprises
**Cons:** Easy to forget, risk of losing work

### 5.4.1 Continuous Sync Implementation Details

For `continuous` mode, the daemon needs careful implementation:

**Debouncing:**
```
file change → wait 2s → no more changes? → sync
                     → more changes? → reset timer
```

**Ignore rules (beyond .gitignore):**

`jul init` must make `.jul/` uncommittable by default:
- Append `.jul/` to `.gitignore` if it is not already ignored
- Also add `.jul/` to `.git/info/exclude` as belt-and-suspenders protection

`.gitignore` should be honored by default. `.jul/syncignore` is optional and should be minimal:

```
# .jul/syncignore (optional)
.jul/              # CRITICAL: ignore Jul's own directory
```

Jul should not ship a broad default ignore list beyond `.jul/**`, because some repos intentionally
track paths like `dist/` or `node_modules/`. However, Jul **does** ship a small, security‑first
default ignore set for **draft sync only** (to avoid accidental secret exfiltration). Suggested
defaults:

```
.env
.env.*
*.pem
*.key
id_rsa
.aws/credentials
.npmrc
.pypirc
.netrc
```

This ignore set is applied only to **draft snapshots** before pushing `refs/jul/sync/*`. It does
not prevent you from intentionally committing or promoting tracked files. Users can override with
`.jul/syncignore` or config.

**CRITICAL: `.jul/**` must always be ignored** or the daemon will sync its own agent workspace, local saves, and indexes in an infinite loop.

**Burst detection:**
- Large file copies (e.g., `npm install`) generate thousands of events
- Wait until event rate drops below threshold
- Avoid syncing half-written node_modules

**Implementation sketch:**
```go
func syncDaemon() {
    var pending bool
    var lastChange time.Time
    
    watcher.OnChange(func(path string) {
        if shouldIgnore(path) { return }
        pending = true
        lastChange = time.Now()
    })
    
    ticker := time.NewTicker(500 * time.Millisecond)
    for range ticker.C {
        if pending && time.Since(lastChange) > debounceSeconds {
            pending = false
            syncDraft()
        }
    }
}
```

**Multi-device workflow:**

On a fresh device:
```bash
$ git clone git@github.com:george/myproject.git
$ cd myproject
$ jul init                    # Generates device ID (e.g., "quiet-mountain")
$ jul ws checkout @           # Establishes baseline: canonical snapshot + lease + workspace intent
```

The checkout establishes your baseline: it sets `workspace_lease`, initializes your sync ref, and
repairs base/track intent from workspace-meta notes. Now `jul sync` knows where you started.

Ongoing work across devices:
```bash
# Device A (swift-tiger): daemon running, syncing continuously
# ... edit files ...

# Device B (quiet-mountain):
$ jul sync                    # Fetch workspace base tip, push draft if available
# If another device checkpointed: base advanced → restack/checkout when ready
# If you want their draft: jul draft list --remote && jul draft adopt <device>

$ jul ws checkout @           # If needed: re-materialize working tree
```

Note: `jul ws checkout` restores working tree + establishes baseline. Staging area (`git add` state) is local to each device and not synced.

### 5.5 Retention and Fetchability

**Problem:** `gc.pruneExpire=never` keeps objects on disk, but doesn't make them fetchable. Unreachable commits can't be fetched by clients unless:
- (a) They're reachable via refs (keep-refs), or
- (b) Remote allows fetching by SHA (`uploadpack.allowAnySHA1InWant`)

**Solution: Keep-refs at checkpoint boundaries only.**

```
refs/jul/keep/<user>/<workspace>/<change-id>/<checkpoint-sha>
```

Example:
```
refs/jul/keep/george/default/Iab4f3c2d/abc123
refs/jul/keep/george/default/Iab4f3c2d/def456   # Amended checkpoint (CLI: @)
refs/jul/keep/george/feature/Icd5e6f7a/ghi789
```

**Lifecycle:**
- Created when checkpoint is locked
- TTL-based expiration (configurable, default 90 days) based on **last-touched**
- **Pinned while CR open:** keep‑refs for CR anchors do not expire until the CR is closed/promoted
- Expired keep-refs deleted by Jul maintenance job
- **Cascade cleanup:** When keep-ref expires, also delete:
  - Associated suggestion refs (`refs/jul/suggest/<change-id>/*`)
  - Associated notes (attestations, CR comments)
- Objects become unreachable after ref deletion, eventually GC'd

```toml
# Jul remote config (optional)
[retention]
checkpoint_keep_days = 90    # Keep-refs for checkpoints
draft_keep_days = 0          # No keep-refs for drafts (ephemeral)
```

**Drafts are intentionally ephemeral:**
- Only the latest draft commit matters
- Previous draft states are overwritten (force-push)
- If you need to recover old draft state, that's what checkpoints are for

**Git-only remote note:** keep-refs are just normal refs you can push; retention/GC depends on the host’s policies.

**For personal use with full time-travel:**
```toml
[retention]
checkpoint_keep_days = -1    # Never expire (infinite)
```

**Future multi-user consideration:** Don't enable `uploadpack.allowAnySHA1InWant` in multi-tenant scenarios. Keep-refs are the safe path.

#### 5.5.1 Cleanup and Archiving (Default: none)

Promote does **not** delete or rewrite Jul refs. By default, all workspace refs, sync refs, trace refs, and notes remain intact for provenance and recovery.

Optional cleanup (explicit user intent) can be added later:
- `jul ws close` — archive a workspace (move refs to `refs/jul/archive/...`)
- `jul prune` — remove expired keep-refs and clean related suggestion/notes per retention policy

The guiding rule is **no implicit data loss**. Cleanup should be manual and conservative.

### 5.6 Four Classes of Attestations

**Problem:** Rebase/squash changes SHAs. An attestation for checkpoint `abc123` doesn't apply to the rebased commit `xyz789` on main.

**Solution: Separate attestations by lifecycle.**

**Trust model (important):** Policy decisions are based on **locally computed checks** for the
exact SHA being evaluated. Synced attestations in notes are **informational caches** only. Jul
must not trust remote‑synced attestations for gating unless they were computed locally on the same
machine for the same SHA.

| Attestation Type | Attached To | Scope | Checks Level | Purpose |
|------------------|-------------|-------|----------|---------|
| **Trace** | Trace SHA | Synced | Cheap (lint, typecheck) | Per-trace provenance |
| **Draft** | Current draft SHA | Device-local | Full (optional) | Continuous feedback, ephemeral |
| **Checkpoint** | Original checkpoint SHA | Synced | Full (required) | Pre-integration checks, review |
| **Published** | Post-rebase SHA on target | Synced | Full (optional) | Final verification on main |

**Trace attestations (lightweight, per-trace):**

Each trace gets cheap, fast checks:

```bash
$ jul trace --prompt "add auth" --agent claude-code
Trace created (sha:abc1234).
  ⚡ Running lint, typecheck...
  ✓ lint: pass
  ✓ typecheck: pass
```

Trace attestations are:
- **Cheap checks only** — lint, typecheck, maybe fast unit tests
- **Synced** — stored in notes, available for `jul blame`
- **Provenance data** — "what were the checks saying when this line was written?"

```toml
[ci]
run_on_trace = true                    # Default: cheap checks on each trace
trace_checks = ["lint", "typecheck"]   # What to run per-trace
```

**Draft attestations (full checks, ephemeral):**

By default, checks run in the background every time the **local draft SHA changes**:

```bash
$ jul sync
Syncing...
  ✓ Draft snapshot updated
  ⚡ Checks running in background...

# Later, or immediately if fast:
$ jul status
Draft abc123 (change Iab4f...) (2 files changed)
  ✓ lint: pass
  ✓ test: pass (48/48)
  ✓ coverage: 84%
```

Draft attestations are:
- **Non-blocking** — you keep working, results appear when ready
- **Ephemeral** — replaced on each sync (only current draft matters)
- **Device-local by default** — stored in `.jul/ci/`, not pushed to remote
- **Coalesced** — only the latest draft SHA runs; older runs are cancelled/ignored

**Why device-local?** Two devices syncing to the same workspace would conflict on a shared draft notes ref. Since drafts are device-scoped anyway (via sync refs), their attestations should be too.

```toml
[ci]
run_on_draft = true           # Default: run checks on every draft update
run_on_checkpoint = true      # Always run checks on checkpoint
draft_ci_blocking = false     # Default: non-blocking background run
sync_draft_attestations = false  # Default: local-only (avoid multi-device conflicts)
```

**Local storage (device-scoped):**
```
.jul/ci/
├── current_draft_sha         # SHA of draft being tested (or last tested)
├── current_run_pid           # PID of running checks (for cancellation)
├── results.json              # Latest results
└── logs/
    └── 2026-01-19-153045.log
```

**Checks coalescing rules:**
1. New sync → cancel any in-progress checks run for old draft
2. Start checks for new draft SHA
3. `jul ci status` reports: (a) latest completed SHA, (b) whether it matches current draft
4. If results are for old draft: show with warning "⚠ results for old draft"

**Run types and visibility:**
- **Background draft checks (sync‑triggered)**: one at a time, coalesced per device. Visible via `jul ci status` (current draft + running PID).
- **Foreground checks (`jul ci run` / `jul ci run --watch`)**: runs immediately and streams output; results are recorded for the target SHA.
- **Checkpoint checks (`jul checkpoint`)**: runs after a checkpoint and writes an attestation note for that checkpoint.
- **Manual checks (`jul ci run --target/--change`)**: attaches to the requested revision; does not replace draft checks unless it targets the draft SHA.

**Multiple runs:** Draft checks are single‑flight per device (latest draft wins). Manual/foreground runs can be started while draft checks are idle, but if they target the draft SHA they will supersede the previous draft result.

```bash
$ jul status
Draft def456 (change Iab4f...) (3 files changed)
  ⚠ Check results for previous draft (abc123)
  ⚡ Checks running for current draft (def456)...
```

**Synced storage (checkpoint/published only):**
```
refs/notes/jul/attestations/checkpoint   # Keyed by original SHA
refs/notes/jul/attestations/published    # Keyed by published SHA
```

**Workflow:**
```
draft update
    │
    ├── Checks run (background, local) → .jul/ci/results.json
    │
    ▼
checkpoint abc123 (change Iab4f...)
    │
    ├── Checks run → checkpoint attestation (synced via notes)
    │
    ▼
jul promote --to main --rebase
    │
    ├── Rebase creates new SHA xyz789
    ├── (Optional) checks run on xyz789 → published attestation
    │
    ▼
main now at xyz789
    │
    └── Query: "is main green?"
        └── Check published attestation for xyz789
        
    └── Query: "what was the checkpoint status?"
        └── Check checkpoint attestation for abc123 (via Change-Id mapping)
```

**Change-Id provides the link:**
```json
// In refs/notes/jul/meta
{
  "change_id": "Iab4f3c2d...",
    "anchor_sha": "abc123",
    "checkpoints": [
      {"sha": "abc123", "message": "feat: add auth"},
      {"sha": "def456", "message": "feat: add auth (v2)"}
    ],
  "promote_events": [
    {
      "event_id": 3,
      "target": "main",
      "strategy": "rebase",
      "timestamp": "2026-01-19T15:42:00Z",
      "checkpoint_shas": ["abc123", "def456"],
      "published_shas": ["xyz789"],
      "published_map": [
        {"published_sha": "xyz789", "checkpoint_sha": "def456"}
      ],
      "merge_commit_sha": null,
      "mainline": null
    }
  ]
}
```

**Anchor lookup (O(1)):**
```
refs/jul/anchors/<change-id> -> <anchor_sha>
```
Created at the first checkpoint and never moves. Used for fast Change‑Id → anchor resolution.

Reverse index note (per published commit):
```json
// In refs/notes/jul/change-id keyed by published SHA xyz789
{
  "change_id": "Iab4f3c2d...",
  "promote_event_id": 3,
  "strategy": "rebase",
  "target": "main",
  "source_checkpoint_shas": ["def456"],
  "checkpoint_anchors": [
    {
      "checkpoint_sha": "def456",
      "trace_base": "t0_sha",
      "trace_head": "t3_sha"
    }
  ],
  "trace_base": "t0_sha",
  "trace_head": "t3_sha"
}
```

This reverse index makes commit -> Change-Id lookup cheap and provides blame anchors for
`jul log <branch>` and `jul blame`.

For squash/merge promotes, `trace_base` / `trace_head` should span the entire change (earliest
checkpoint base to latest checkpoint head), and `checkpoint_anchors[]` allows blame to narrow
within that range when possible.

### 5.7 Notes: Storage, Sync, and Portability

**Metadata travels with git... mostly.**

Jul stores metadata as git objects (notes, refs). This means it can be synced via git push/pull,
but the outcome depends on what the remote allows:

- Different hosts have different ref policies (some block custom refs)
- Some hosts reject non-fast-forward updates for `refs/jul/sync/*`
- Size limits vary (GitHub has push size limits)
- Retention varies (some hosts GC aggressively)

**Sync compatibility (auto-detected via `jul doctor`):**
- Checkpoint sync available: custom refs + notes accepted; workspace refs are canonical across devices
- Draft sync available: non‑FF updates allowed for `refs/jul/sync/*`; per-device draft refs can push
- No custom refs: Jul refs/notes stay local (promote still targets normal branches)

The recommended setup is a separate sync remote (for example, `jul`) so `origin` can remain
locked down while Jul still gets the best available sync capabilities.


**Size limits to prevent repo bloat:**

Continuous sync can balloon your repo if attestations contain full logs or coverage reports. Rules:

| Data | Storage | Size Limit |
|------|---------|------------|
| Attestation summary | Notes | ≤ 16 KB |
| Suggestion metadata | Notes | ≤ 16 KB |
| Full check logs | Local only (`.jul/logs/`) | No limit |
| Coverage reports | Local only (`.jul/coverage/`) | No limit |
| Suggestion patches | Commits | No limit (git handles) |

Notes store summaries; big artifacts stay local by default.

**Refspec configuration:**

Git notes are not fetched by default. Jul configures explicit refspecs. The recommended setup is a
separate sync remote (for example, `jul`) so `origin` can stay locked down:

```ini
[remote "origin"]
    url = git@github.com:george/myproject.git
    fetch = +refs/heads/*:refs/remotes/origin/*

[remote "jul"]
    url = git@github.com:george/myproject-jul.git
    fetch = +refs/jul/workspaces/*:refs/jul/workspaces/*
    fetch = +refs/jul/sync/*:refs/jul/sync/*
    fetch = +refs/jul/traces/*:refs/jul/traces/*
    fetch = +refs/jul/trace-sync/*:refs/jul/trace-sync/*
    fetch = +refs/jul/changes/*:refs/jul/changes/*
    fetch = +refs/jul/anchors/*:refs/jul/anchors/*
    fetch = +refs/jul/keep/*:refs/jul/keep/*
    fetch = +refs/jul/suggest/*:refs/jul/suggest/*
    fetch = +refs/notes/jul/*:refs/notes/jul/*
```

These refspecs apply to the sync remote. `track_ref` and `jul promote` use the publish remote
(often `origin`) to fetch target tips and update branches.

**Namespace resolution ordering (cross-device intent):**
1. Fetch notes early (especially `refs/notes/jul/repo-meta`).
2. Resolve `user_namespace` from repo-meta when present (otherwise use cached/local and publish it
   when possible).
3. Fetch user-scoped refs under `refs/jul/*/<user>/*`.

**Capability-dependent push rules:**
- Checkpoint sync available: push workspace refs, trace refs, and notes.
- Draft sync available: also push per-device sync refs (`refs/jul/sync/*`).
- No custom refs: do not push any `refs/jul/*` or `refs/notes/jul/*`.

If you must use a single remote, Jul can attach these refspecs to `origin` instead.

Refspecs must exclude `refs/heads/jul/*` (local-only workspace branches).

**Recommended writers per namespace:**

Notes refs can have non-fast-forward conflicts. Prefer clear ownership, but **multiple devices may still write** (e.g., two laptops). Use the notes sync algorithm below to merge safely.

| Namespace | Typical writer | Content |
|-----------|----------------|---------|
| `refs/notes/jul/repo-meta` | Init / doctor | Repo identity + stable user namespace |
| `refs/notes/jul/workspace-meta` | Workspace ops / sync | Workspace intent (base/track/pinning/owner) |
| `refs/notes/jul/meta` | Client | Change-Id mappings |
| `refs/notes/jul/change-id` | Promote | Reverse index: published SHA -> Change-Id (+ promote context) |
| `refs/notes/jul/attestations` | check runner | Test results (summaries only) |
| `refs/notes/jul/trace-index` | Trace writer | Optional blame index (changed paths, patch/hunk hashes) |
| `refs/notes/jul/cr-comments` | Client | Review layer: CR comments/threads (checkpoint-scoped) |
| `refs/notes/jul/cr-state` | Client | Review layer: CR state (Change-Id anchor, latest checkpoint) |
| `refs/notes/jul/suggestions` | Review agent | Suggestion metadata |
| `refs/notes/jul/agent-review` | Review agent | Agent review summaries/results |
| `refs/notes/jul/traces` | Client | Trace metadata (prompt hash, agent, session) |

**Notes payload format (multi-writer safe):**
- For notes that may be written by multiple devices (e.g., `meta`, `cr-comments`), store **append‑only
  NDJSON** (one JSON object per line) with a stable `event_id`.
- Merges then become line‑level unions instead of JSON object conflicts.
- Avoid in‑place mutation of nested JSON objects in notes.
  - `event_id` format: ULID (lexicographically sortable, time‑ordered).
  - Dedup by `event_id` uniqueness.
  - "Current state" is derived by **max(event_id)** (or highest ULID) rather than mutation.

**Notes sync algorithm (multi-device):**

Notes can diverge between devices. Jul syncs notes like this:
1. Fetch remote notes ref into a temporary ref
2. For append‑only NDJSON notes, merge with:  
   `git notes --ref <ref> merge -s cat_sort_uniq <temp_ref>`
3. For single‑truth notes (e.g., `repo-meta`), **do not auto‑merge** on conflict; require explicit repair.
4. Push merged notes ref with lease

This avoids flaky push failures when two devices append notes in parallel.

**Repo/workspace meta conflict rules (load-bearing):**
- `repo-meta.user_namespace` should be stable. If two values appear, treat it as a repo identity
  conflict and require explicit repair/confirmation rather than silently forking refs.
- `workspace-meta` should be merged by `workspace_id` + `owner_namespace`:
  - If those match, prefer the note with the newest `updated_at` for base/track fields.
  - If they differ for the same workspace name, treat it as a conflict and require repair.

**Concurrency rule:** When multiple devices might update the same note entry, prefer **append-only events** (e.g., CR comment events, suggestion status events) and derive current state from the latest event. This avoids conflicts from “last writer wins” overwrites.

**Suggestions storage:**

Suggestions have two parts:
- **Patch commits**: `refs/jul/suggest/<Change-Id>/<suggestion_id>` — actual code
- **Metadata**: `refs/notes/jul/suggestions` — reasoning, confidence, status

Commits carry the heavy diffs; notes stay small.

**Trace prompt privacy:**

See section 2.6 for full privacy settings. Summary:
- `prompt_hash_mode = "hmac"` (default) — HMAC hash syncs by default; low risk, no cross‑device correlation
- `sync_prompt_summary = false` (default) — summaries stay local (can leak paraphrased secrets)
- `sync_prompt_full = false` (default) — full text stays local

Local storage for prompts and summaries: `.jul/traces/`

### 5.8 Summary: Git Object Model

```
                            refs/heads/main
                                   │
                                   ▼
           ┌─────────── xyz789 (published, rebased) ◄─── attestation
           │
           │   refs/jul/keep/george/default/Iab4f.../def456   (CLI: @)
           │                        │
           │                        ▼
           │           def456 (checkpoint, immutable) ◄─── attestation
           │               │
           │               │   refs/jul/workspaces/george/default        ◄─ workspace base tip (CLI: @)
           │               │
           │               └──── ghi789 (draft, ephemeral)
           │                      │
           │                      │   refs/jul/sync/george/swift-tiger/default  ◄─ this device (CLI: @)
           │                       │
           │                       └── [draft] WIP
           │                           Change-Id: Iab4f...
           │
    (parent chain)
           │
           ▼
      earlier commits
```

**Ref purposes:**
- `refs/heads/*` — Promote targets (main, staging)
- `refs/heads/jul/<workspace>` — Local workspace branches (HEAD targets; local-only)
- `refs/jul/changes/<change-id>` — Per-change tips (stacking boundary; stable after promote)
- `refs/jul/anchors/<change-id>` — Change-Id → anchor SHA (first checkpoint)
- `refs/jul/workspaces/<user>/<ws>` — Canonical **workspace base** tip (shared when checkpoint sync is available)
- `refs/jul/sync/<user>/<device>/<ws>` — This device's **draft** ref (remote-pushed only when draft sync is available)
- `refs/jul/keep/*` — Checkpoint retention anchors
- `refs/jul/suggest/*` — Suggestion patch commits
- `refs/notes/jul/*` — Metadata (attestations, change-id reverse index, cr-state/comments, suggestions, traces)

**Local state (per workspace):**
- `.jul/workspaces/<ws>/lease` — last workspace base tip incorporated into the local working tree (the semantic lease)
- `.jul/workspaces/<ws>/track-tip` — last observed tip of `track_ref` (for upstream drift reporting)
- `.jul/workspaces/<ws>/...` — cached workspace intent (repairable from `workspace-meta` notes)

**Invariants:**
- Canonical remote snapshot = workspace ref (workspace base tip) when checkpoint sync is available
- `workspace_tip == workspace_lease` → base is current
- `workspace_tip != workspace_lease` → base advanced; require `jul ws checkout` or `jul ws restack` (no auto-merge)
- Lease advances only after the local working tree reflects the canonical tip
- `jul ws checkout` establishes baseline (canonical snapshot + workspace_lease + workspace intent)
- Sync may observe upstream advancement but must not rewrite the draft base after checkpoints exist
- Promote may restack onto the latest checkpoint if needed

---


## 6. CLI Design (`jul`)

### 6.1 Setup Commands

#### `jul configure`

Interactive setup wizard for global configuration.

```bash
$ jul configure
Jul Configuration
─────────────────
Preferred sync remote name [jul]: jul
Publish remote name [origin]: origin
Run jul doctor on init? [Y/n]: Y

Agent Provider:
  [1] opencode (bundled)
  [2] claude-code
  [3] codex
  [4] custom
Select [1]: 1

Configuration saved to ~/.config/jul/config.toml
```

Creates:
- `~/.config/jul/config.toml` — Remote defaults, publish defaults, init preferences
- `~/.config/jul/agents.toml` — Agent provider settings

#### `jul init`

Initialize a repository with Jul.

```bash
# In a repo with a dedicated sync remote
$ git remote add jul git@github.com:george/myproject-jul.git
$ cd my-project
$ jul init
Using sync remote 'jul' (git@github.com:george/myproject-jul.git)
Device ID: swift-tiger
Workspace '@' ready

# In a cloned repo (origin exists, no jul remote)
$ jul init
Using sync remote 'origin' (git@github.com:george/myproject.git)
Tip: add a separate sync remote: git remote add jul <url> && jul remote set jul
Device ID: swift-tiger
Workspace '@' ready

# In a repo with multiple remotes (no jul, no origin)
$ jul init
Multiple remotes found: upstream, github, personal
Run 'jul remote set <name>' to choose one.
Device ID: swift-tiger
Workspace '@' ready (local only)

# Fresh repo (no remotes)
$ jul init
No remote configured. Working locally.
Device ID: swift-tiger
Workspace '@' ready
```

**Sync remote selection logic:**
1. If remote `jul` exists → use it (recommended)
2. Else if `origin` exists → use it
3. Else if exactly one remote exists → use it
4. Else if multiple remotes exist → require explicit `jul remote set`
5. If no remotes → work locally
6. After selection, probe capabilities; if custom refs are blocked, disable checkpoint sync.
   If non‑FF updates are blocked, disable draft sync. Suggest a dedicated `jul` remote when needed.

**Publish remote selection logic (for promote/track_ref):**
1. If `origin` exists → use it
2. Else if a sync remote exists → use the sync remote
3. Else publish is local-only until configured

Override publish remote with `jul remote publish set <name>`.

**User namespace selection logic (repo-scoped identity):**
1. If a sync remote is available, fetch notes early (especially `refs/notes/jul/repo-meta`)
2. If repo-meta contains `user_namespace` → adopt it
3. Else if local cached `user_namespace` exists → use it
4. Else generate a new `user_namespace` and publish it to repo-meta when possible

What it does:
1. `git init` (if new)
2. Generate device ID (e.g., "swift-tiger") → `~/.config/jul/device`
3. Ensure `.jul/` is ignored in both `.gitignore` and `.git/info/exclude`
4. Select sync remote (if available)
5. Fetch notes early and resolve `user_namespace`
6. Select publish remote (if available)
7. Add Jul refspecs to the sync remote (if configured)
8. Probe sync compatibility (`jul doctor` or inline) and enable/disable sync accordingly
9. Create default workspace `@` (normalized to `default` internally)
10. Write/update repo-meta and workspace-meta notes when possible
11. Start first draft

#### `jul remote`

View or set remotes used for Jul sync and publish.

```bash
# View current remotes
$ jul remote
Sync remote:    jul    (git@github.com:george/myproject-jul.git)
Publish remote: origin (git@github.com:george/myproject.git)
Checkpoint sync: ok
Draft sync:      ok

# Set sync remote
$ jul remote set jul
Now using 'jul' for sync.

# Set publish remote
$ jul remote publish set origin
Now using 'origin' for publish/track_ref.

# Clear remote (work locally)
$ jul remote clear
Sync remote cleared. Working locally (publish remote unchanged).
```

Use `jul doctor` to detect sync compatibility. Jul should record the detected capabilities in repo
config and skip draft sync or checkpoint sync when needed. Sync remote capability does not change
publish semantics: `jul promote` always fetches the publish remote's target tip.

Jul configures refspecs and notes once a sync remote is set, but it does **not** create the remote
server for you. Add it with `git remote add ...` or your hosting provider, then run `jul remote set`.

### 6.2 Core Workflow Commands

#### `jul sync`

Sync current draft and fetch the latest workspace base tip. Sync never auto-merges drafts.
It does **not** fetch the publish remote; upstream tips refresh only on `jul ws restack` or
`jul promote`.

```bash
# With compatible sync remote (checkpoint + draft sync)
$ jul sync
Syncing...
  ✓ Fetched workspace base tip
  ✓ Draft snapshot updated
  ✓ Draft ref pushed

# With sync remote but no draft sync (FF-only)
$ jul sync
Syncing...
  ✓ Fetched workspace base tip
  ✓ Draft snapshot updated (local only)
  ⚠ Draft sync unavailable (non-FF blocked)

# Without remote (local only)
$ jul sync
Syncing...
  ✓ Draft snapshot updated
```

If another device checkpointed:
```bash
$ jul sync
Syncing...
  ✓ Fetched workspace base tip
  ⚠ Base advanced (local base abc123 → workspace def456)
Run `jul ws restack` or `jul ws checkout @` when ready.
```

Upstream drift is reported using the **last known** `track_tip` (refreshed by restack/promote),
not by fetching during sync.

Flags:
- `--allow-secrets` — Allow draft push even if the secret scanner flags new/modified blobs

```bash
# Run as daemon (watches filesystem, syncs automatically)
$ jul sync --daemon
Watching for changes...
  15:30:01 synced (2 files)
  15:30:45 synced (1 file)
```

Flags:
- `--daemon` — Run as background watcher (continuous mode)
- `--json` — JSON output

**Upstream rule:** Sync can safely auto-transplant only before the first checkpoint. Once checkpoints
exist, sync reports upstream drift but does not rewrite the draft base.

#### `jul draft`

Manage per-device drafts and explicit draft handoff.

```bash
# See drafts available from other devices
$ jul draft list --remote
swift-tiger     @  base=def456  updated=2m ago
quiet-mountain  @  base=def456  updated=5m ago

# Inspect a draft (summary)
$ jul draft show swift-tiger

# Adopt another device's draft into your workspace
$ jul draft adopt swift-tiger
```

Flags:
- `--remote` — List drafts from other devices (requires draft sync capability)
- `--onto <checkpoint>` — Adopt onto a specific checkpoint (explicit restack)
- `--replace` — Discard local draft and take the remote draft as-is
- `--json` — JSON output

If draft sync is unavailable, `jul draft list --remote` will show nothing and `jul draft adopt`
will refuse; use `jul checkpoint` to hand off work instead.

#### `jul trace`

Create a trace (provenance record) with optional prompt/agent metadata.

```bash
# Explicit trace (no prompt)
$ jul trace
Trace created (sha:abc1234).

# With prompt (harness calls this)
$ jul trace --prompt "add user authentication" --agent claude-code
Trace created (sha:def5678).
  Agent: claude-code
  Prompt: "add user authentication" [synced as hash]

# With full session context
$ jul trace \
  --prompt "fix the failing test" \
  --agent claude-code \
  --session-id abc123 \
  --turn 5
Trace created (sha:ghi9012).
  Agent: claude-code (session abc123, turn 5)
  Prompt: "fix the failing test"
```

For long prompts, use stdin:
```bash
$ echo "$PROMPT" | jul trace --prompt-stdin --agent claude-code
```

**When to use:**

| Scenario | Command |
|----------|---------|
| Harness integration | Harness calls `jul trace --prompt "..." --agent ...` after each turn |
| Manual trace boundary | User calls `jul trace` |
| No explicit traces (implicit enabled) | `jul sync` / `jul checkpoint` create traces implicitly (no prompt attached), subject to throttles |

Traces are stored as side history (not in checkpoint ancestry). Use `jul blame` to query.

Flags:
- `--prompt <text>` — Attach prompt text
- `--prompt-stdin` — Read prompt from stdin
- `--agent <name>` — Agent that created this trace
- `--session-id <id>` — Session identifier (for multi-turn tracking)
- `--turn <n>` — Turn number within session
- `--json` — JSON output

#### `jul merge`

Resolve conflicts from `jul draft adopt` or `jul ws restack`. Agent handles conflicts automatically. See [6.7 Merge Command](#67-merge-command) for full details.

```bash
$ jul merge
Agent resolving conflicts...

Conflicts resolved:
  src/auth.py — combined both changes

Resolution ready as suggestion [01HX...].
Accept? [y/n] y

  ✓ Merged
  ✓ Draft updated
  ✓ Working tree updated
```

#### `jul checkpoint`

Lock current draft, generate message, start new draft.
Also advances the workspace base tip and change ref:
`refs/jul/workspaces/<user>/<ws> → <new_checkpoint_sha>`,
`refs/jul/changes/<change-id> → <new_checkpoint_sha>`.

```bash
$ jul checkpoint
Locking draft abc123 (change Iab4f...)

Generating message... (opencode)
  "feat: add JWT validation with refresh token support"

Accept? [y/n/edit] y

Syncing... done
Running checks...
  ✓ lint
  ✓ compile  
  ✓ test (48/48)
  ✓ coverage (84%)

Running review...
  ⚠ 1 suggestion created

Checkpoint def456 locked (change Iab4f...).
New draft ghi789 started (change Iab4f...).
```

Checkpoint sync (if available) fast‑forwards the workspace ref to the new checkpoint. If no
compatible sync remote exists, the workspace ref updates locally only.

If the workspace ref push is rejected (non‑FF because another device checkpointed), Jul keeps the
new checkpoint locally and marks the workspace as **diverged**. The user must run `jul ws restack`
or `jul ws checkout` explicitly; no auto‑merge is attempted.

If this is the **first checkpoint** for a Change‑Id, Jul creates the anchor ref:
`refs/jul/anchors/<change-id> -> <checkpoint_sha>` (never moves).

Flags:
- `-m "message"` — Provide message (skip agent)
- `--prompt "..."` — Store the prompt that led to this checkpoint (optional metadata)
- `--adopt` — Adopt the current `HEAD` commit as a checkpoint (opt‑in; doesn’t move branches)
- `--no-review` — Skip review
- `--json` — JSON output

**Git commit adoption (opt‑in):**

```toml
[checkpoint]
adopt_on_commit = false   # default: off
adopt_run_ci = false
adopt_run_review = false
```

When enabled, the post‑commit hook runs `jul checkpoint --adopt`, which:
1) adds a keep‑ref for `HEAD`,
2) records metadata, and
3) starts a new draft parented at `HEAD`.

This preserves continuity without moving `refs/heads/*`.

**Change boundary (adopt):** An adopted commit becomes the **next checkpoint in the current Change‑Id** (or starts a Change‑Id if none exists yet). Jul does not create a new Change‑Id just because a git commit was adopted.

**Change-Id note for adopted commits:** Adopted commits may not have a `Change-Id:` trailer. In that
case, Jul records the Change-Id mapping in `refs/notes/jul/meta` (no history rewrite). On promote,
Jul writes the reverse index (`refs/notes/jul/change-id`) for published commits and adds a
`Change-Id:` trailer when possible. If you want Change-Id embedded in normal git commits from the
start, install a `commit-msg` hook that injects the trailer.

**Adopt + promote behavior (edge case example):**
```text
Case A: git commit on target branch (already published)
  main: A──B  (B is adopted)
  jul promote → only checkpoints after B are published (B is base)

Case B: git commit off target (not published yet)
  main: A
  workspace: A──B  (B is adopted)
  jul promote → publishes B along with later checkpoints
```

#### `jul status`

Show current workspace status.

```bash
$ jul status

Workspace: @ (default)
Draft: def456 (change Iab4f...) (2 files changed)
Sync: checkpoint ok; draft blocked (non-FF rejected)
Last sync: 2m ago

Checkpoints (not yet promoted):
  abc123 (change Iab4f...) "feat: add JWT validation" ✓ Checks passed
    └─ 1 suggestion pending

Tracked target: main (last seen ghi789)
Pinned base: abc123 (run `jul ws restack` to integrate now; promote will restack automatically)
```

With `--json` for agents:
```json
{
  "workspace": "@",
  "draft": {
    "sha": "def456",
    "change_id": "Iab4f...",
    "files_changed": 2
  },
  "sync": {
    "checkpoint": "ok",
    "draft": "blocked",
    "last_sync_at": "2026-01-26T01:24:00Z"
  },
  "checkpoints": [...],
  "suggestions": [...],
  "promote_status": {
    "target": "main",
    "track_tip_sha": "ghi789",
    "upstream_advanced_by": null,
    "eligible": true,
    "checkpoints_ahead": 1
  }
}
```

**Upstream reporting:** `jul status` uses the **last known** `track_tip` (updated by `jul ws checkout`,
`jul ws restack`, or `jul promote`). It does **not** fetch the publish remote. If you need a fresh
view, run `jul ws restack --onto <branch>` to refresh the tip. If the target was rewritten (the
old tip is not an ancestor of the new tip), restack/promote must warn loudly and require explicit
confirmation.

**Manual branch switch detection:** If `HEAD` is not on `refs/heads/jul/<current_workspace>`,
`jul status` should warn and suggest `jul ws checkout @` to re‑establish the workspace head.

**Unadopted commits warning:** If
- `HEAD == refs/heads/jul/<workspace>` **and**
- `refs/heads/jul/<workspace> != refs/jul/workspaces/<user>/<workspace>` **and**
- `refs/heads/jul/<workspace>` is **not** a draft commit

then `jul status` should warn that commits exist on the workspace branch that are not yet adopted
as checkpoints, and offer:

```
jul checkpoint --adopt    # adopt these commits as checkpoints
jul ws checkout @         # discard and reset to canonical workspace tip
```

**Callout: Git changed the target behind Jul's back (force-push / rewrite):**
```bash
# Someone rewrites main outside Jul
$ git push --force origin main

# Jul observes but does not restack silently (at restack/promote time)
$ jul promote --to main
Fetching target...
  ⚠ Tracked target main was rewritten (last seen abc123, now def456)
  ⚠ No restack performed - run `jul ws restack --onto main` (or confirm at promote)

$ jul status
Workspace: @ (default)
Tracked target: main (rewritten on remote)
  last_seen: abc123
  current:   def456
  action: restack explicitly or confirm at promote
```

#### `jul promote`

Promote checkpoints to a target branch.

Promote updates the **publish remote** and advances the local workspace head ref to the new base.

```bash
$ jul promote --to main

Promoting 2 checkpoints to main...
  abc123 "feat: add JWT validation" (change Iab4f...)
  def456 "fix: null check on token" (change Iab4f...)

Policy check (main):
  ✓ compile: pass
  ✓ test: pass (48/48)
  ✓ coverage: 84% (≥80%)
  ⚠ 1 suggestion not addressed (warning)

Strategy: rebase

Promote? [y/n] y

Rebased onto main.
Workspace base marker wbase123 created (tree = main@ghi789)
Workspace '@' now based on wbase123
New draft started.
```

If the tracked target was rewritten (force-push / history edit) and you choose to continue:
```bash
$ jul promote --to main
Promoting 2 checkpoints to main...
  abc123 "feat: add JWT validation" (change Iab4f...)
  def456 "fix: null check on token" (change Iab4f...)

⚠ Tracked target main was rewritten on remote
  last_seen: abc123
  remote:    def456
This may indicate a force-push. Restack onto remote and continue? [y/N] y

Restacking onto origin/main@def456...
  ✓ Checkpoint abc123 -> xyz111
  ✓ Checkpoint def456 -> xyz222
Policy check (main): ✓
Fast-forward push: ✓
New draft started.
```

If the tracked target was rewritten and you decline:
```bash
$ jul promote --to main
Promoting 2 checkpoints to main...
  abc123 "feat: add JWT validation" (change Iab4f...)
  def456 "fix: null check on token" (change Iab4f...)

⚠ Tracked target main was rewritten on remote
  last_seen: abc123
  remote:    def456
This may indicate a force-push. Restack onto remote and continue? [y/N] n

Aborted. No restack or publish performed (workspace remains pinned).
Next: jul ws restack --onto main
```

**Note:** Promoted history on `refs/heads/*` is **published commits** (normal Git commits), not checkpoints. A new Change‑Id starts for the next workspace draft after promote.

**Workspace base marker (post‑promote):**
- After a successful promote, Jul creates a **workspace base marker** commit:
  - Tree = published tip tree
  - Parent = last workspace checkpoint tip
  - Trailer: `Jul-Type: workspace-base`
- `refs/jul/workspaces/<user>/<ws>` advances to this marker.
- The new draft uses the marker as its base, so `workspace_tip == parent(current_draft)` stays true across devices.
- `jul log` hides base markers by default (show with `--verbose`).

**Safety invariant:** `jul promote` always fetches the current target tip from the **publish remote**
(for example, `origin/main`), constructs published commits that are descendants of that tip, and
updates the target branch via **fast-forward** push (no force). If the remote target has advanced,
promote rebases your checkpoint chain onto that new tip (surfacing conflicts if needed). The target
branch is never force-updated unless `--force-target` is explicitly passed.

After fetching, `jul promote` must update `.jul/workspaces/<ws>/track-tip` to the fetched target tip
so `jul status` reflects the latest known upstream state.

If `.jul/workspaces/<ws>/track-tip` is not an ancestor of the fetched remote target tip (target
rewrite / force-push), promote must warn and require explicit confirmation before restacking and
publishing.

After a successful promote, update `.jul/workspaces/<ws>/track-tip` to the fetched remote target tip
that was used for the publish.

Promote must also update `refs/notes/jul/workspace-meta` so workspace intent stays portable:
- `base_ref = <target branch>`
- `base_change_id = null` (unless explicitly retargeted to a change ref)
- `base_sha = <published tip>` (the new base for the next change)
- `track_ref = <target branch>` and `track_tip = <published tip>`

**Stacked promote (auto-land stack):** If `base_ref` points to a parent change ref
(`refs/jul/changes/<change-id>`), `jul promote` automatically lands the **entire stack** bottom-up:
1. Identify the stack chain by following `base_change_id` / change refs up to the first branch base.
2. For each layer (bottom-up):
   - fetch and restack onto the current publish remote target tip,
   - evaluate promote policy for that layer (run checks if required, especially after restack), and
   - publish the layer.
3. Rebase the child layer onto the newly published parent tip and continue.
4. Start new drafts for each workspace in the stack (each gets a new Change-Id).

Each restack/promote in the stack must update that workspace's `workspace-meta` note with the new
base/track tip.

If a layer hits conflicts or fails policy, stack promote stops at that layer, keeps already
published layers, leaves children untouched, and reports next actions.

**Mapping rule:** `jul promote` records both forward and reverse mappings (so published commits can
be resolved to a Change-Id in O(1)):
- Update the change ref: `refs/jul/changes/<change-id> → <latest_checkpoint_sha>`
- Append a promote event in `refs/notes/jul/meta` (keyed by anchor SHA) with:
  - `checkpoint_shas`, `published_shas`, `target`, `strategy`, `timestamp`
  - `published_map[]` entries when the mapping is not 1:1
  - For merge: `merge_commit_sha` + `mainline` (for deterministic revert)
- `promote_event_id` is the promote event's monotonic index within the Change-Id.
- Write a reverse index note in `refs/notes/jul/change-id` for each published SHA with:
  - `change_id`, `promote_event_id`, `strategy`
  - Checkpoint context: `source_checkpoint_sha(s)` and optional `checkpoint_anchors[]`
  - Blame anchors: `trace_base`, `trace_head` (full-change range for squash/merge)
- Add a `Change-Id:` trailer to published commits when possible (even for squash).

Strategy mapping defaults:
- `rebase`: published commits map 1:1 to checkpoints in order; anchors come from that checkpoint.
- `squash`: the published commit maps to the change as a whole; anchors must span the change
  (`trace_base = earliest_checkpoint.trace_base`, `trace_head = latest_checkpoint.trace_head`).
- `merge`: the merge commit maps to the change as a whole; anchors must span the change.

This makes `jul log <branch>`, `jul blame <branch>`, and `jul revert <change-id>` deterministic on
published branches while keeping CR status tied to the latest checkpoint SHA.

Flags:
- `--to <branch>` — Target branch (required)
- `--squash` — Override strategy to squash
- `--rebase` — Override strategy to rebase
- `--merge` — Override strategy to merge
- `--no-policy` — Skip policy checks (tests/coverage/etc.)
- `--force-target` — **Dangerous**: allow non-fast-forward update of target branch (should be rare)
- `--auto` — Auto-checkpoint draft first if needed
- `--json` — JSON output

#### `jul revert`

Revert a logical change by Change-Id using the recorded promote mapping.

```bash
$ jul revert Iab4f... --to main

Reverting change Iab4f... on main:
  def456 "fix: null check on token"
  abc123 "feat: add JWT validation"

Checkpoint xyz123 "revert: Iab4f..." created.
Next: jul promote --to main
```

**Rules:**
- Resolves the Change‑Id to its anchor via `refs/jul/anchors/<change-id>`, then finds the promote
  event in `refs/notes/jul/meta` (defaults to the most recent promote).
- Reverts the recorded `published_shas` in reverse order; for merge promotes, uses the stored `mainline`.
- Creates the revert in the current workspace (does not update the target branch directly).
- If the revert is empty (already reverted / no-op), Jul reports it and does not checkpoint unless `--force` is set.
- If a revert hits conflicts, Jul stops and routes to the normal conflict flow (`jul merge`) with revert context.

Flags:
- `--to <branch>` — Target branch to revert against (defaults to last promote target)
- `--event <n>` — Select a specific promote event for this Change-Id
- `--force` — Allow an empty revert checkpoint (`git commit --allow-empty`) when the revert is a no-op
- `--json` — JSON output

### 6.3 Workspace Commands

#### `jul ws new`

Create a named workspace.

```bash
$ jul ws new feature-auth
Created workspace 'feature-auth'
Draft abc123 started.
```

**Base ref:** `refs/heads/main` by default (or the current branch). `track_ref` defaults to the
same branch.

On creation, Jul pins `base_sha` to the base ref tip and initializes `.jul/workspaces/<ws>/track-tip`
to the current `track_ref` tip.

Jul also generates a stable `workspace_id` and writes workspace intent to
`refs/notes/jul/workspace-meta` keyed by the canonical workspace tip (so other devices inherit the
same base/track meaning, not just the tree).

#### `jul ws stack`

Create a new workspace stacked on the current Change-Id’s **latest checkpoint** (not its draft).
Stacking pins to a change ref, not a workspace stream:
- Parent base: `refs/jul/changes/<parent_change_id>`
- Child records: `base_ref = refs/jul/changes/<parent_change_id>` and `base_change_id = <parent_change_id>`
- Child inherits `track_ref` from the parent workspace at stack time

The base tip resolves to the parent change ref tip, even if the parent workspace later rolls to a
new Change-Id.

Jul initializes `.jul/workspaces/<ws>/track-tip` to the current `track_ref` tip so upstream drift
is measured from the moment the stack layer is created.

Stacking must also update `refs/notes/jul/workspace-meta` so the base change pin
(`base_change_id`) and inherited `track_ref` survive across devices.

```bash
$ jul ws stack feature-b
Created workspace 'feature-b' (stacked on feature-auth)
Draft def456 started.
```

Use this when you want dependent work that should review/land after the current workspace.

**V1 rule:** stacking requires a checkpoint. If the current workspace has no checkpoint yet, Jul asks you to checkpoint first.

#### `jul ws restack`

Restack the workspace onto the latest tip of its base ref (branch or parent change ref) by
recomputing the workspace state on top of the new base and **creating a new checkpoint**
(fast‑forward).
Optionally, **retarget** the base ref.

This updates the local workspace head ref (`refs/heads/jul/<workspace>`) and the working tree.

When `base_ref` is a change ref (`refs/jul/changes/<change-id>`), restack resolves that change's
latest checkpoint tip (not the parent workspace's current change).

Restack must update `base_sha` (and `base_ref` / `base_change_id` when retargeting) and write the
new workspace intent to `refs/notes/jul/workspace-meta`.

```bash
$ jul ws restack
Fetching base...
Restacking workspace onto base@def456...
  ✓ Restack checkpoint xyz789 created (parent abc123)
Workspace restacked. New base: def456
```

Retarget base (change the base ref):

```bash
$ jul ws restack --onto main
Restacking workspace onto main@def456...
  ✓ Restack checkpoint xyz789 created (parent abc123)
Workspace restacked. New base_ref: refs/heads/main
```

**Retarget semantics:** `base_ref` is set to the new ref, and `base_sha` is set to that ref's
current tip at the time of restack. If the new ref is a change ref, set `base_change_id` to that
Change-Id; otherwise clear `base_change_id`. If the new ref is a branch, also set `track_ref` to
that branch.

After a successful restack, update `.jul/workspaces/<ws>/track-tip` to the current fetched
`track_ref` tip.

Restack/retarget must also update `refs/notes/jul/workspace-meta` so other devices see the new
base/track intent.

`--onto` accepts branch refs or Change-Ids. Change-Ids resolve to `refs/jul/changes/<change-id>`.

If conflicts:

```bash
$ jul ws restack
Restacking workspace onto base@def456...
  ⚠ Conflict in src/auth.py
Run 'jul merge' to resolve.
```

**Restack semantics:**
- Creates a **new restack checkpoint** (new SHA) whose parent is the previous checkpoint tip.
- **Preserves Change‑Id** (same logical change).
- Advances the change ref to the new checkpoint tip.
- Earlier checkpoint SHAs remain reachable as ancestors; keep‑refs govern retention for attestations/provenance.
- Restack emits a **synthetic trace** with `trace_type=restack` so `trace_head` matches the new tree.
  - `jul blame` ignores `trace_type=restack` for attribution (same as merge traces).
- **Suggestions become stale:** restack changes the base; run `jul review` again for fresh suggestions.
- **Checks on restack:** restack checkpoints have no attestations; Jul should run checks (or prompt to run `jul ci run`).

**Restack vs Promote (difference in intent):**

| Command | What it does | When to use |
|---------|--------------|-------------|
| `jul ws restack` | Restack onto latest base ref tip; create a new checkpoint and update `base_sha` | “I want upstream changes now, before I’m done.” |
| `jul promote` | Rebase onto target tip at publish time, then publish | “I’m done, land this.” |

**Stacked workspace drift:** If a parent change ref advances (new checkpoint or restack), child
workspaces keep their pinned `base_sha`. `jul status` should warn “parent change advanced; run
`jul ws restack`”. Children can keep working until they restack.

**Stacked promote rule (Graphite‑style):**
- If the workspace `base_ref` is a change ref (`refs/jul/changes/<change-id>`),
  **`jul promote` auto-lands the stack** bottom-up.
- Jul promotes parents first, rebasing children as needed, and lands each layer onto the target branch.
- Promote policy is evaluated per layer. If a layer fails policy or hits conflicts, stack promote
  stops at that layer, leaves children untouched, and reports next actions.
- The result is the same as “land stack” in Graphite, but with a single command.

#### `jul ws switch`

Switch to another workspace.

This updates the local workspace head ref (`refs/heads/jul/<workspace>`) and the working tree.

```bash
$ jul ws switch feature-auth
Saving current workspace '@'...
  ✓ Working tree saved
  ✓ Staged changes saved
Restoring 'feature-auth'...
  ✓ Working tree restored
  ✓ workspace_lease updated
  ✓ track-tip updated
Switched to workspace 'feature-auth'
```

**What happens:**
1. Auto-saves current workspace (working tree + staging area) via `jul local save`
2. Syncs current draft (pushes draft ref if available)
3. Fetches target workspace's canonical state (workspace ref) plus workspace intent notes
4. Repairs local base/track/pinning intent from `refs/notes/jul/workspace-meta` when present
5. Restores target workspace's saved state (working tree + staging area)
6. Updates `workspace_lease` for target workspace to the canonical SHA once materialized locally
7. Updates `.jul/workspaces/<ws>/track-tip` to the current fetched `track_ref` tip

This makes "no dirty state concerns" actually true — your uncommitted work is preserved per-workspace.

#### `jul ws checkout`

Fetch and materialize a workspace's **base tip** into the working tree, then start a draft.
Establishes this device's baseline for future syncs.

This updates the local workspace head ref (`refs/heads/jul/<workspace>`) and the working tree.

```bash
$ jul ws checkout @
Fetching workspace '@'...
  ✓ Workspace ref: abc123
  ✓ Working tree updated
  ✓ Workspace intent repaired from notes
  ✓ Sync ref initialized
  ✓ workspace_lease set
  ✓ track-tip set
```

**What happens:**
1. Fetch the canonical workspace base tip (workspace ref) plus repo/workspace meta notes
2. Materialize working tree to match
3. Repair local base/track/pinning from `refs/notes/jul/workspace-meta` when present
4. Initialize this device's sync ref to the same commit (remote-pushed when draft sync is available; local-only otherwise)
5. Set `workspace_lease` to the canonical tip SHA
6. Set `.jul/workspaces/<ws>/track-tip` to the current fetched `track_ref` tip

This establishes the baseline: checkout sets up base + sync ref, so future `jul sync` commands know where they started.

Local-only: checkout uses local refs only; no remote fetch is required.

Use this when:
- Setting up a fresh device
- Pulling in another device's latest work
- Recovering after `git reset` or other working tree changes

Note: Only restores working tree. Staging area is local to each device and not synced.

#### `jul ws list`

List all workspaces.

```bash
$ jul ws list
* @ (default)           abc123 (2 files changed)
  feature-auth          def456 (clean)
  bugfix-123            ghi789 (5 files changed)
```

#### `jul ws rename`

Rename current workspace.

```bash
$ jul ws rename auth-feature
Renamed '@' → 'auth-feature'
```

#### `jul ws delete`

Delete a workspace.

```bash
$ jul ws delete bugfix-123
Delete workspace 'bugfix-123'? [y/n] y
Deleted.
```

Can't delete current workspace.

#### `jul transplant` (Future)

Rebase a draft from one base commit to another. Used when bases have diverged.

```bash
$ jul sync
  ⚠ Base advanced
  Your draft is based on checkpoint1, but workspace is now at checkpoint2.

# Future command to carry changes forward:
$ jul transplant
Rebasing draft from checkpoint1 onto checkpoint2...
  ⚠ Conflicts in src/auth.py
  
Run 'jul merge' to resolve.
```

**V1:** Not implemented. Use `jul ws restack` or `jul ws checkout @` to start fresh.

### 6.4 Submit Command

#### `jul submit`

Create or update the **single** change request (CR) for the current Change-Id.
This is part of the optional review layer.

```bash
$ jul submit
CR updated for change Iab4f... (workspace 'feature-auth')
  Checkpoint: def456...
```

**Rules:**
- One Change-Id = one CR (no CR IDs).
- Uses the **latest checkpoint** for the Change-Id.
- Writes CR state to `refs/notes/jul/cr-state` (review layer; keyed by Change-Id anchor; stores latest checkpoint).
- Subsequent `jul submit` updates the same CR.
- Optional: stacked workspaces include the parent Change-Id (and workspace name) in CR metadata.

If you don't use CRs, skip `jul submit` entirely and go checkpoint -> promote.

### 6.5 Suggestion Commands

Suggestion and review commands are part of the optional review layer.

#### `jul suggestions`

List pending suggestions for current checkpoint.

Suggestions are created by `jul review` (agent-generated); there is no manual `jul suggest` command.

```bash
$ jul suggestions

Pending for change Iab4f... (checkpoint abc123) "feat: add JWT validation":

  [01HX7Y9A] potential_null_check (92%) ✓
             src/auth.py:42 - Missing null check on token
             
  [01HX7Y9B] test_coverage (78%) ✓
             src/auth.py:67-73 - Uncovered error path

Actions:
  jul show <id>      Show diff
  jul apply <id>     Apply to draft
  jul reject <id>    Reject
```

If the base commit changed (new checkpoint or restack), stale suggestions are marked:

```bash
$ jul suggestions

Pending for change Iab4f... (checkpoint def456) "feat: add JWT validation":

  [01HX7Y9A] potential_null_check (92%) ⚠ stale
             Created for abc123, current is def456
             
  [01HX7Y9B] test_coverage (78%) ⚠ stale
             Created for abc123, current is def456

Run 'jul review' to generate fresh suggestions.
```

#### `jul show`

Show suggestion details.

```bash
$ jul show 01HX7Y9A

Suggestion: potential_null_check
Confidence: 92%
Checkpoint: abc123 "feat: add JWT validation"
Change-Id:  Iab4f...
Base SHA:   abc123

src/auth.py:
  @@ -40,6 +40,9 @@
   def validate_token(request):
       token = request.headers.get("Authorization")
  +    if not token:
  +        raise AuthError("Missing authorization token")
  +
       user = validate_jwt(token)

Reasoning:
  "validate_jwt will raise unclear KeyError if token is None.
   Explicit check provides better error message."
```

#### `jul apply`

Apply a suggestion to current draft.

```bash
$ jul apply 01HX7Y9A
Applied to draft.

# Or apply and checkpoint immediately
$ jul apply 01HX7Y9A --checkpoint
Applied and checkpointed as def456 (change Iab4f...) "fix: add null check for auth token"
```

If suggestion is stale:

```bash
$ jul apply 01HX7Y9A
⚠ Suggestion is stale (created for abc123, current is def456)

Options:
  --force    Apply anyway (may not apply cleanly)
  
Run 'jul review' to generate fresh suggestions.
```

#### `jul reject`

Reject a suggestion.

```bash
$ jul reject 01HX7Y9B -m "covered by integration tests"
Rejected.
```

### 6.6 Review Command

#### `jul review`

Manually trigger review on current draft.

```bash
$ jul review
Running review on draft def456 (change Iab4f...)
  Analyzing 3 changed files...
  
  ⚠ 1 suggestion created
  
Run 'jul suggestions' to see details.
```

Useful before checkpoint to catch issues early.

### 6.7 Merge Command

#### `jul merge`

Resolve conflicts from `jul draft adopt` or `jul ws restack`. Agent handles conflicts automatically.

```bash
$ jul merge
Agent resolving conflicts...

Conflicts resolved:
  src/auth.py — combined both changes (local validation + remote caching)
  src/config.py — kept remote, applied local additions

Resolution ready as suggestion [01HX...].
Accept? [y/n] y

  ✓ Merged (draft updated)
```

The resolution is a suggestion, so you can:
- Accept it (`y` or `jul apply 01HX...`)
- Reject it and resolve manually (`n` or `jul reject 01HX...`)
- See the diff first (`jul show 01HX...`)

With `--json` for agents:
```json
{
  "merge": {
    "status": "resolved",
    "suggestion_id": "01HX...",
    "conflicts": [
      {"file": "src/auth.py", "strategy": "combined"},
      {"file": "src/config.py", "strategy": "both"}
    ]
  },
  "next_actions": ["apply 01HX...", "reject 01HX..."]
}
```

### 6.8 Checks Command (`jul ci`)

#### `jul ci run`

Run checks and show results.

```bash
$ jul ci run
Running checks...
  ✓ lint: pass (1.2s)
  ✓ test: pass (8.4s) — 48/48
  ✓ coverage: 84%

All checks passed.
```

If tests fail:
```bash
$ jul ci run
Running checks...
  ✓ lint: pass (1.2s)
  ✗ test: fail (6.1s) — 45/48
    FAIL tests/test_auth.py::test_token_refresh
    FAIL tests/test_auth.py::test_expired_token
    FAIL tests/test_jwt.py::test_invalid_signature
  ⚠ coverage: 79% (below 80% threshold)

3 checks failed.
```

**Subcommands:**

```bash
$ jul ci run              # Run checks now, wait for results
$ jul ci run --watch      # Run checks now, stream output
$ jul ci run --target <rev>   # Attach results to a specific revision
$ jul ci run --change Iab4f3c2d...  # Attach results to latest checkpoint for a change
$ jul ci status       # Show latest results (don't re-run)
$ jul ci list         # List recent check runs
$ jul ci config       # Show checks configuration
$ jul ci config --show  # Show resolved commands (file or inferred)
$ jul ci cancel       # Cancel in-progress background checks
```

**`jul ci status` reports current vs completed:**

```bash
$ jul ci status
Checks Status:
  Current draft: def456
  Last completed: def456 ✓ (results current)
  
  ✓ lint: pass
  ✓ test: pass (48/48)
  ✓ coverage: 84%
```

If you've edited since the last checks run:

```bash
$ jul ci status
Checks Status:
  Current draft: ghi789
  Last completed: def456 ⚠ (stale)
  ⚡ Checks running for ghi789...
  
  Previous results (def456):
    ✓ lint: pass
    ✓ test: pass (48/48)
```

**For agents (JSON):**

```bash
$ jul ci run --json
```

```json
{
  "ci": {
    "status": "pass",
    "current_draft_sha": "def456",
    "completed_sha": "def456",
    "results_current": true,
    "duration_ms": 9600,
    "results": [
      {"name": "lint", "status": "pass", "duration_ms": 1200},
      {"name": "test", "status": "pass", "duration_ms": 8400, "passed": 48, "failed": 0},
      {"name": "coverage", "status": "pass", "value": 84, "threshold": 80}
    ]
  }
}
```

When results are stale:

```json
{
  "ci": {
    "status": "stale",
    "current_draft_sha": "ghi789",
    "completed_sha": "def456",
    "results_current": false,
    "running_sha": "ghi789",
    "results": [...]
  }
}
```

**Difference from background checks:**
- Background checks run automatically on sync (non-blocking)
- `jul ci run` runs explicitly and waits for results (blocking)

Use `jul ci run` when you want to explicitly verify before checkpointing:
```bash
$ jul ci run && jul checkpoint   # Only checkpoint if checks pass
```

### 6.9 History, Diff, and Git Interop Commands

#### `jul log`

Show change-aware history (checkpoints and published commits), grouped by Change-Id. Default is the current workspace; pass a ref to inspect a published branch.

Workspace base markers created after promote are hidden by default. Use `jul log --verbose` to show them.

```bash
$ jul log

def456 (change Iab4f...) (2h ago) "fix: null check on token"
        Author: george
        ✓ Checks passed

abc123 (change Iab4f...) (4h ago) "feat: add JWT validation"
        Author: george
        ✓ Checks passed, 1 suggestion

ghi789 (change Ief6a...) (1d ago) "initial project structure"
        Author: george
        ✓ Checks passed
```

Change-aware log for a published branch:
```bash
$ jul log main
```

For published refs, Jul resolves commit -> Change-Id via the `Change-Id:` trailer when present,
else via `refs/notes/jul/change-id` (reverse index).

With trace history (provenance):
```bash
$ jul log --traces

def456 (change Iab4f...) (2h ago) "fix: null check on token"
        Author: george
        ✓ Checks passed
  └── 1 trace:
      (sha:abc1) claude-code "fix the failing test" (auth.py)
          ✓ lint, ✓ typecheck

abc123 (change Iab4f...) (4h ago) "feat: add JWT validation"
        Author: george
        ✓ Checks passed, 1 suggestion
  └── 3 traces:
      (sha:def2) george (manual) (auth.py, models.py)
      (sha:ghi3) claude-code "use JWT instead" (auth.py)
          ✓ lint, ✗ typecheck
      (sha:jkl4) claude-code "fix type error" (auth.py)
          ✓ lint, ✓ typecheck
```

Flags:
- `--limit <n>` — Show last n checkpoints
- `--change-id <id>` — Filter by Change-Id
- `--traces` — Show trace history (prompts, agents, per-trace checks)
- `--json` — JSON output

#### `jul diff`

Show diff between checkpoints, changes, or against draft.

```bash
# Diff current draft against base commit
$ jul diff

# Diff a logical change (Change-Id)
$ jul diff Iab4f...

# Diff between two checkpoints
$ jul diff abc123 def456

# Diff specific checkpoint against its parent
$ jul diff abc123
```

If the argument is a Change-Id, Jul computes the net diff for that logical change (from its base
to the latest checkpoint or last published commit). On published branches, Change-Id resolution
uses `refs/jul/changes/<change-id>` plus `refs/notes/jul/change-id`.

Flags:
- `--stat` — Show diffstat only
- `--name-only` — Show changed filenames only
- `--json` — JSON output

#### `jul show`

Show details of a checkpoint, change-id, or suggestion.

```bash
$ jul show Iab4f...

$ jul show abc123

Checkpoint: abc123
Message: "feat: add JWT validation with refresh tokens"
Author: george
Date: Mon Jan 19 15:30:00 2026
Change-Id: Iab4f3c2d1e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b

Attestation:
  ✓ lint: pass
  ✓ compile: pass
  ✓ test: pass (48/48)
  ✓ coverage: 84%

Suggestions: 1 pending

Files changed:
  M src/auth.py (+42 -8)
  A src/jwt_utils.py (+128)
  M tests/test_auth.py (+67 -12)
```

If passed a Change-Id, Jul shows the change summary: base, checkpoints, published commits,
attestations, and suggestions. On published branches, commit -> Change-Id resolution uses the
`Change-Id:` trailer when present, else `refs/notes/jul/change-id`.

#### `jul blame`

Show line-by-line provenance: checkpoint, trace, prompt, agent.

**V1 promise:** best-effort provenance with safe fallbacks. When attribution is ambiguous or too
expensive, Jul should fall back to checkpoint/commit-level blame rather than guess confidently.

**High-level algorithm (heuristic by design):**
1. Run `git blame <ref>` to find the commit SHA that last touched each line (published or checkpoint).
2. Resolve commit -> Change-Id:
   - Prefer the `Change-Id:` trailer when present.
   - Otherwise, look up `refs/notes/jul/change-id` (reverse index) keyed by that commit SHA.
3. Resolve blame anchors:
   - Prefer anchors from the reverse index note (`refs/notes/jul/change-id`).
   - If `checkpoint_anchors[]` is present, keep it as a narrowing hint.
   - If only a Change-Id is known, resolve its anchor via `refs/jul/anchors/<change-id>` and
     derive anchors from promote metadata when possible.
   - Else fall back to the change ref tip (`refs/jul/changes/<change-id>`).
4. Build candidate traces between `trace_base..trace_head`:
   - If `checkpoint_anchors[]` is available, try those anchors first, then widen to the full range.
   - If `refs/notes/jul/trace-index` is available, filter to traces that touched the file/hunks.
   - Otherwise, use a bounded scan and diff only along the trace chain for that file.
5. Attribution heuristic:
   - Prefer the nearest non-connective trace that touched the line/hunk.
   - If needed, fall back to "first trace where the line appears" within the anchors.
6. **Skip `trace_type=merge` and `trace_type=restack` nodes** for attribution (they're connective, not edits).
   - Blame must **not** diff across merge/restack nodes; it should traverse parents directly.
   - If `trace-index` is used, merge/restack nodes should have empty/ignored `changed_paths`.
7. If no confident attribution is found, return checkpoint/commit-level blame for that line.

Deriving anchors from promote metadata means widening to the change-level range: earliest
checkpoint `trace_base` through latest checkpoint `trace_head` for the selected promote event.

If a commit cannot be resolved to a Change-Id, Jul falls back to commit-level blame for that line.

**Known hard cases (v1 expectations):**
- Renames/moves, partial-line edits, and large conflict resolutions may produce approximate results.
- Very long trace chains can be slow without `trace-index` hints.
- The correct fallback is "less specific but correct" (checkpoint/commit) rather than over-precise.

```bash
$ jul blame src/auth.py

42 │ def validate_token(request):     change Iab4f... (checkpoint abc123) trace:abc1 george (manual)
43 │     token = request.headers...   change Iab4f... (checkpoint abc123) trace:abc1 george (manual)
44 │     if not token:                 change Iab4f... (checkpoint abc123) trace:def2 claude-code
45 │         raise AuthError(...)      change Iab4f... (checkpoint abc123) trace:def2 claude-code
46 │     user = validate_jwt(token)    change Iab4f... (checkpoint abc123) trace:abc1 george (manual)
```

With prompts:
```bash
$ jul blame src/auth.py --prompts

42-43 │ change Iab4f... (checkpoint abc123) trace:abc1 george (manual)
      │ No prompt (manual edit)

44-45 │ change Iab4f... (checkpoint abc123) trace:def2 claude-code
      │ Prompt: [hash only, summary stored locally]
```

With full prompt text (if available locally):
```bash
$ jul blame src/auth.py --prompts --local

44-45 │ change Iab4f... (checkpoint abc123) trace:def2 claude-code
      │ Summary: "Added null check for auth token"
      │ Prompt: "add null check for missing auth token"
```

With full context:
```bash
$ jul blame src/auth.py --verbose

44-45 │ change Iab4f... (checkpoint abc123) trace:def2 claude-code
      │ Checkpoint: "feat: add JWT validation"
      │ Trace: def2... (2026-01-19 15:32:00)
      │ Agent: claude-code
      │ Session: abc123, turn 5
      │ Summary: "Added null check for auth token"
      │ Prompt: [stored locally]
      │ Checks at trace: ✓ lint, ✓ typecheck
```

Line range:
```bash
$ jul blame src/auth.py:40-50
```

Flags:
- `--prompts` — Show prompts/summaries that led to each line
- `--local` — Include full prompt text from local storage
- `--verbose` — Show full context (session, checks state)
- `--json` — JSON output (for tooling integration)
- `--no-trace` — Show only checkpoint, not trace-level detail

**JSON output** (for IDE integration):
```json
{
  "file": "src/auth.py",
  "lines": [
    {
      "line": 44,
      "content": "    if not token:",
      "checkpoint_change_id": "Iab4f...",
      "trace_sha": "def2abc...",
      "agent": "claude-code",
      "prompt_hash": "hmac:abc123...",
      "prompt_summary": null,
      "session_id": "abc123",
      "turn": 5
    }
  ]
}
```

Note: `prompt_summary` is null by default (stays local). Only populated if `sync_prompt_summary = true` and the summary was synced.

#### `jul query`

Query checkpoints by criteria.

```bash
$ jul query --test=pass --coverage-min=80 --limit=5

abc123 (change Iab4f...) (2h ago) "feat: add JWT validation"
        ✓ tests, 84% coverage
        
def456 (change Icd5e...) (1d ago) "refactor: extract auth utils"
        ✓ tests, 82% coverage
```

Note: different Change‑Ids in query output represent different logical changes (often after a promote boundary).

#### `jul reflog`

Show workspace history (including draft updates).

```bash
$ jul reflog --limit=10

def456 checkpoint "fix: null check" (2h ago)
abc123 checkpoint "feat: add JWT validation" (4h ago)
        └─ draft update (4h ago)
        └─ draft update (5h ago)
ghi789 checkpoint "initial structure" (1d ago)
```

#### `jul git`

Passthrough to `git`. All arguments are forwarded unchanged.

```bash
$ jul git status
$ jul git log --oneline main
$ jul git diff main~3..main
```

Use this when you want a standard Git command without leaving the Jul CLI.

### 6.10 Local Workspaces (Client-Side)

Local workspaces enable instant context switching for uncommitted work.

**Note**: This is separate from synced workspaces. It's a client-only feature for managing local state.

#### `jul local save`

Save current local state.

```bash
$ jul local save wip-experiment
Saved local state 'wip-experiment'
  3 modified files
  1 untracked file
```

#### `jul local restore`

Restore local state.

```bash
$ jul local restore wip-experiment
Restored local state 'wip-experiment'
```

#### `jul local list`

List saved local states.

```bash
$ jul local list
  wip-experiment (3 files, 2h ago)
  scratch-idea (1 file, 1d ago)
```

#### `jul local delete`

Delete saved local state.

```bash
$ jul local delete scratch-idea
Deleted.
```

**Storage**: `.jul/local/<name>/` contains saved files, index, manifest.

---

## 7. Configuration

### 7.1 Global Config

```toml
# ~/.config/jul/config.toml

[user]
name = "george"

[workspace]
default_name = "@"

[remote]
sync_remote = "jul"              # Preferred sync remote name
publish_remote = "origin"        # Preferred publish remote name
run_doctor_on_init = true        # Probe sync compatibility automatically

[sync]
mode = "on-command"              # on-command | continuous | explicit
allow_secrets = false            # Block draft push when secrets are detected

[checkpoint]
auto_message = true              # Agent generates message

[promote]
default_target = "main"
strategy = "rebase"              # rebase | squash | merge

[ci]
run_on_checkpoint = true         # Always run checks on checkpoint
run_on_draft = true              # Run checks on draft update (background)
draft_ci_blocking = false        # Draft checks don't block sync

[review]
enabled = true
run_on_checkpoint = true
min_confidence = 70

[traces]
prompt_hash_mode = "hmac"        # hmac (default) | sha256 | off
sync_prompt_summary = false      # Summaries stay local by default
sync_prompt_full = false         # Full text stays local by default
implicit_mode = "checkpoint"     # off | checkpoint | sync (implicit traces)
min_seconds_between = 2          # throttle implicit traces
min_diff_bytes = 2048            # throttle implicit traces
```

### 7.2 Device Config

```
# ~/.config/jul/device
swift-tiger
```

Auto-generated on first `jul init`. Two random words (adjective-noun). Used for device-scoped sync refs.

### 7.3 Agent Config

```toml
# ~/.config/jul/agents.toml

[default]
provider = "opencode"

[providers.opencode]
command = "opencode"
protocol = "jul-agent-v1"

[providers.claude-code]
command = "claude"
protocol = "jul-agent-v1"

[providers.codex]
command = "codex"
protocol = "jul-agent-v1"
```

### 7.4 Repo Config

```toml
# .jul/config.toml (per-repo)

[remote]
name = "jul"                     # Git remote to use for Jul sync (can be different from origin)
checkpoint_sync = "auto"         # auto | enabled | disabled (detected via jul doctor)
draft_sync = "auto"              # auto | enabled | disabled (detected via jul doctor)

[publish]
remote = "origin"                # Remote used for promote targets and upstream tracking (track_ref)

[identity]
user_namespace = "george-7b3c"   # Stable repo-scoped user namespace (canonical in repo-meta notes)
repo_id = "jul:8b1f4c2d"         # Optional cache of repo identity

[workspace]
name = "feature-auth"            # Override default workspace name
base_ref = "refs/heads/main"     # Branch or parent change ref (refs/jul/changes/<id>)
base_change_id = "Iab4f..."      # When stacked, pins the parent Change-Id
base_sha = "abc123"              # Pinned base commit (updated on restack/promote)
track_ref = "refs/heads/main"    # Tracked publish target for upstream drift + status
# Workspace fields are a local cache; canonical intent lives in refs/notes/jul/workspace-meta.

[ci]
# Agent-assisted checks setup (future)
# First checkpoint without config triggers setup wizard
```

**Sync remote selection (auto-detected on `jul init`):**
1. If `jul` remote exists → use it
2. Else if `origin` exists → use it
3. Else if exactly one remote exists → use it
4. Else if multiple remotes exist → must set explicitly via `jul remote set`
5. If no remotes → work locally (no `[remote]` section)
6. After selection, probe capabilities; if custom refs are blocked, disable checkpoint sync.
   If non‑FF updates are blocked, disable draft sync. Suggest a dedicated `jul` remote when needed.

**Publish remote selection (for `jul promote` and `track_ref`):**
1. If `origin` exists → use it
2. Else if a sync remote exists → use the sync remote
3. Else publish is local-only until configured

**User namespace selection (repo-scoped identity):**
1. Fetch notes early (especially `refs/notes/jul/repo-meta`) when a sync remote is available
2. If repo-meta contains `user_namespace` → adopt it
3. Else if local cached `user_namespace` exists → use it
4. Else generate a new `user_namespace` and publish it to repo-meta when possible

### 7.5 Policy Config

```toml
# .jul/policy.toml (per-repo)

[promote.main]
required_checks = ["compile", "test"]
min_coverage_pct = 80
require_suggestions_addressed = false

[promote.staging]
required_checks = ["compile"]
min_coverage_pct = 0
```

---

## 8. Agent Integration

Jul is designed for two types of agents:

1. **External agents** (Codex, Claude Code, etc.) — Build your application, use Jul for feedback
2. **Internal agent** (configured provider) — Reviews your code, generates suggestions

### 8.1 External Agent Integration

External agents use Jul as infrastructure. The key principle: **every command returns structured feedback that agents can act on.**

#### 8.1.1 The Feedback Loop

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    External Agent (Codex, Claude Code)                  │
│                                                                         │
│    edit ──► jul checkpoint --json ──► parse response ──► decide         │
│                      │                                      │           │
│                      ▼                                      ▼           │
│              ┌──────────────────┐                  ┌────────────────┐   │
│              │ Check results    │                  │ Apply fix      │   │
│              │ Suggestions      │                  │ Reject         │   │
│              │ Coverage gaps    │                  │ Edit manually  │   │
│              └──────────────────┘                  │ Iterate        │   │
│                                                    └────────────────┘   │
└─────────────────────────────────────────────────────────────────────────┘
```

#### 8.1.2 Checkpoint Response

When an external agent runs `jul checkpoint --json`:

```json
{
  "checkpoint": {
    "change_id": "Iab4f3c2d...",
    "message": "feat: add user authentication",
    "sha": "abc123..."
  },
  "ci": {
    "status": "fail",
    "signals": {
      "lint": {"status": "pass", "warnings": 2},
      "compile": {"status": "pass"},
      "test": {
        "status": "fail",
        "passed": 46,
        "failed": 2,
        "failures": [
          {
            "name": "test_token_refresh",
            "file": "tests/test_auth.py",
            "line": 87,
            "message": "AssertionError: expected True",
            "trace": "..."
          }
        ]
      },
      "coverage": {"line_pct": 78.5, "uncovered": ["src/auth.py:45-52"]}
    }
  },
  "suggestions": [
    {
      "id": "01HX7Y9A",
      "reason": "fix_failing_test",
      "confidence": 0.89,
      "description": "Test expects old behavior. Update assertion.",
      "diff": "--- a/tests/test_auth.py\n+++ b/tests/test_auth.py\n@@ -87..."
    }
  ],
  "next_actions": [
    {"action": "apply", "command": "jul apply 01HX7Y9A --json"},
    {"action": "show", "command": "jul show 01HX7Y9A --json"},
    {"action": "reject", "command": "jul reject 01HX7Y9A --json"}
  ]
}
```

The external agent can then:
- Parse the failures
- Decide to apply suggestions: `jul apply 01HX7Y9A --json`
- Or make its own fix and checkpoint again
- Or ask the user for help

#### 8.1.3 Apply Response

When agent runs `jul apply <id> --json`:

```json
{
  "applied": true,
  "suggestion_id": "01HX7Y9A",
  "files_changed": ["tests/test_auth.py"],
  "draft": {
    "change_id": "Icd5e6f7a...",
    "files_changed": 1
  },
  "next_actions": [
    {"action": "checkpoint", "command": "jul checkpoint --json"},
    {"action": "review", "command": "jul review --json"}
  ]
}
```

#### 8.1.4 Full External Agent Loop

```python
# Example: External agent using Jul
import subprocess
import json

def jul(cmd):
    result = subprocess.run(f"jul {cmd} --json", shell=True, capture_output=True)
    return json.loads(result.stdout)

# Agent makes changes
edit_files(...)

# Checkpoint and get feedback
response = jul("checkpoint")

while response["ci"]["status"] == "fail":
    # Check for suggestions
    if response["suggestions"]:
        for suggestion in response["suggestions"]:
            if suggestion["confidence"] > 0.8:
                jul(f"apply {suggestion['id']}")
        response = jul("checkpoint")
    else:
        # No suggestions, agent needs to fix manually
        failures = response["ci"]["signals"]["test"]["failures"]
        fix_failures(failures)
        response = jul("checkpoint")

# Checks pass, promote
jul("promote --to main")
```

### 8.2 Internal Agent (Review Agent)

The internal agent is your configured provider (OpenCode bundled, or Claude Code/Codex if configured). It runs reviews, resolves conflicts, and generates suggestions.

**Key principle**: The internal agent works in an isolated git worktree, never touching your files directly.

#### 8.2.1 Agent Workspace (Git Worktree)

Jul uses a full git worktree for agent isolation:

```bash
# Jul creates worktree automatically
git worktree add .jul/agent-workspace/worktree <checkpoint-sha>
```

```
.jul/
├── agent-workspace/              # Isolated agent sandbox
│   ├── worktree/                 # Full git worktree (separate checkout)
│   │   └── ... (all project files)
│   ├── suggestions/
│   │   ├── 01HX7Y9A/            # Each suggestion is a commit
│   │   │   ├── commit           # SHA of suggestion commit
│   │   │   ├── base             # SHA it applies to
│   │   │   └── metadata.json    # Reason, confidence, etc.
│   │   └── 01HX7Y9B/
│   └── logs/
│       └── review-2026-01-19.log
```

**Why git worktree?**
- Full checkout — agent sees all files, can run tests
- Isolated — agent changes don't affect user's working tree
- Git-native — commits become suggestion refs naturally
- Standard tooling — agent can use normal git commands

#### 8.2.2 How Internal Agent Creates Suggestions

When `jul checkpoint` triggers review:

1. **Create agent workspace** (if not exists)
   ```bash
   git worktree add .jul/agent-workspace/worktree <checkpoint-sha>
   ```

2. **Invoke agent** with context
   ```json
   {
     "action": "review",
     "workspace": ".jul/agent-workspace/worktree",
     "change_id": "Iab4f3c2d...",
     "checkpoint_sha": "abc123...",
     "diff": "...",
     "ci_results": {...}
   }
   ```

3. **Agent makes changes** in its workspace
   - Edits files in `.jul/agent-workspace/worktree/`
   - Runs tests to verify fixes
   - Can iterate multiple times

4. **Agent commits** its changes
   ```bash
   cd .jul/agent-workspace/worktree
   git add -A
   git commit -m "fix: add null check for auth token"
   # SHA becomes the suggestion
   ```

5. **Suggestion created** pointing to agent's commit
   ```json
   {
     "id": "01HX7Y9A",
     "change_id": "Iab4f3c2d...",
     "base": { "kind": "checkpoint", "sha": "abc123..." },
     "commit": "def456...",
     "status": "pending",
     "reason": "potential_null_check",
     "confidence": 0.92
   }
   ```

The `base` field records whether the suggestion was created against a checkpoint or a draft. If the base commit changes (new checkpoint or restack), the suggestion becomes stale.

#### 8.2.3 Applying Suggestions

When user runs `jul apply 01HX7Y9A`:

1. **Check staleness**:
   - If `base.kind == "checkpoint"`, compare `base.sha` with `parent(current_draft)`
   - If `base.kind == "draft"`, compare `base.sha` with `current_draft`
   - If mismatch: warn "stale", require `--force` or fresh review

2. **Get the suggestion's commit SHA**

3. **Cherry-pick or patch** into user's draft
   ```bash
   git cherry-pick --no-commit def456
   ```

4. **Changes appear** in user's working directory

5. **User can review**, edit, then checkpoint

**The user never sees the agent workspace.** Suggestions appear as "proposed changes" that can be previewed and applied.

**Staleness handling:**
```bash
# Normal case (not stale)
$ jul apply 01HX7Y9A
Applied to draft.

# Stale case
$ jul apply 01HX7Y9A
⚠ Suggestion is stale (created for abc123, current is def456)
  Run 'jul review' to generate fresh suggestions.
  Or use --force to apply anyway.

# Force apply (may conflict)
$ jul apply 01HX7Y9A --force
⚠ Applying stale suggestion...
Applied to draft (check for conflicts).
```

#### 8.2.4 Agent Workspace Lifecycle

```
jul checkpoint
    │
    ├── Trigger review
    │
    ▼
Agent workspace created/updated
    │
    ├── Agent invoked
    ├── Agent edits files (in sandbox)
    ├── Agent runs tests (in sandbox)
    ├── Agent commits (in sandbox)
    │
    ▼
Suggestion refs created
    │
    ├── refs/jul/suggest/Iab4f.../01HX7Y9A → agent's commit
    │
    ▼
User sees: "⚠ 1 suggestion created"
    │
    ├── jul show 01HX7Y9A  → see diff
    ├── jul apply 01HX7Y9A → cherry-pick to draft
    └── jul reject 01HX7Y9A → mark rejected
```

### 8.3 Agent Protocol (v1)

Communication with internal agent via stdin/stdout JSON.

**Request:**
```json
{
  "version": 1,
  "action": "review",
  "workspace_path": "/path/to/.jul/agent-workspace/worktree",
  "context": {
    "checkpoint_sha": "abc123...",
    "change_id": "Iab4f3c2d...",
    "diff": "...",
    "files": [
      {"path": "src/auth.py", "content": "..."}
    ],
    "ci_results": {
      "test": {"status": "fail", "failures": [...]}
    }
  }
}
```

**Response:**
```json
{
  "version": 1,
  "status": "completed",
  "suggestions": [
    {
      "id": "01HX7Y9A",
      "commit": "def456...",
      "reason": "fix_failing_test",
      "description": "Updated test assertion to match new behavior",
      "confidence": 0.89,
      "files_changed": ["tests/test_auth.py"]
    }
  ]
}
```

### 8.4 Agent Actions

| Action | Triggered by | Agent Workspace | Blocking | Purpose |
|--------|--------------|-----------------|----------|---------|
| `generate_message` | `jul checkpoint` | No | Yes | Create commit message |
| `review` | After checkpoint | Yes (worktree) | No | Analyze code, create suggestions |
| `resolve_conflict` | `jul merge` | Yes (worktree) | Yes | 3-way merge resolution |
| `setup_ci` | First checkpoint (no config) | No | Yes | Auto-configure checks |

**Workspace = git worktree** at `.jul/agent-workspace/worktree/` — full checkout, isolated from user's files.

### 8.5 Agent Providers

Jul can use any compatible coding agent. OpenCode is bundled for zero-config setup; others can be configured.

#### Bundled: OpenCode

Jul bundles OpenCode, so it works out of the box:

```bash
$ jul init
Device ID: swift-tiger
Agent: opencode (bundled)
Workspace '@' ready
```

No API keys needed if using OpenCode's free tier or your own provider keys configured in `~/.config/opencode/`.

#### External Agents

Users can configure Claude Code, Codex, or other agents:

```toml
# ~/.config/jul/agents.toml

[default]
provider = "opencode"              # Bundled, works out of box

[providers.opencode]
command = "opencode"
bundled = true
headless = "opencode run --model $MODEL \"$PROMPT\" -f json"
timeout_seconds = 300

[providers.claude-code]
command = "claude"
bundled = false                    # User must install
headless = "claude -p \"$PROMPT\" --output-format json --permission-mode acceptEdits"
timeout_seconds = 300

[providers.codex]
command = "codex"
bundled = false                    # User must install
headless = "codex exec \"$PROMPT\" --output-format json --full-auto"
timeout_seconds = 300

[sandbox]
enable_network = false             # Agent can't make network calls
enable_exec = true                 # Agent can run tests in sandbox
max_iterations = 5                 # Max edit-test cycles per review
```

#### Headless Invocation

Jul invokes agents in headless mode (non-interactive) for automated tasks:

| Agent | Headless Command | Key Flags |
|-------|------------------|-----------|
| **OpenCode** | `opencode run --model <p/m> "task"` | `-f json` for output |
| **Claude Code** | `claude -p "task"` | `--output-format json`, `--permission-mode acceptEdits` |
| **Codex** | `codex exec "task"` | `--output-format json`, `--full-auto` |

**Example: Jul invoking agent for review**

```bash
# Jul creates worktree, then invokes agent
cd .jul/agent-workspace/worktree

# OpenCode (bundled)
opencode run --model opencode/claude-sonnet "Review this code for bugs. \
  Checks failed with: $FAILURES. Create fixes in this directory." -f json

# Claude Code (if configured)
claude -p "Review this code for bugs. Checks failed with: $FAILURES. \
  Create fixes in this directory." \
  --output-format json \
  --permission-mode acceptEdits \
  --allowedTools "Read,Write,Edit,Bash(npm test)"
```

### 8.6 Checks Auto-Setup

When no checks configuration exists, the agent proposes one:

```bash
$ jul checkpoint
No checks configuration found.
Agent analyzing project...

Detected: Python 3.11, pytest, ruff
Proposed checks config:

  [ci]
  lint = "ruff check ."
  test = "pytest"
  coverage = "pytest --cov --cov-report=json"

Accept? [y/n/edit] y

  ✓ Checks configuration saved to .jul/ci.toml
  ✓ Running checks...
```

**Jul's checks are for fast local feedback**, separate from project CI (GitHub Actions, etc.):

```toml
# .jul/ci.toml (auto-generated or manual)

[commands]
lint = "ruff check ."
test = "pytest"
coverage = "pytest --cov --cov-report=json"

[thresholds]
min_coverage_pct = 80

[options]
timeout_seconds = 300
parallel = true
```

If the project already has standard tooling (package.json scripts, Makefile, pyproject.toml), the agent detects and uses it:

```bash
$ jul checkpoint
Detected checks from pyproject.toml:
  lint: ruff check .
  test: pytest

Use detected config? [y/n/edit] y
```

### 8.7 Structured Output

All commands support `--json` for external agent consumption:

```bash
$ jul status --json
$ jul sync --json
$ jul trace --json
$ jul merge --json
$ jul checkpoint --json  
$ jul log --json
$ jul diff --json
$ jul show <id> --json
$ jul blame <file> --json
$ jul suggestions --json
$ jul apply 01HX... --json
$ jul reject 01HX... --json
$ jul review --json
$ jul promote --to main --json
```

Every response includes `next_actions` suggesting what the agent might do next.

---

## 9. Example Workflows

### 9.1 Full Jul Flow

```bash
# Setup (once)
$ jul configure
$ jul init my-project
$ git remote add origin git@github.com:you/my-project.git
$ jul remote publish set origin
# Optional but recommended: separate sync remote
$ git remote add jul git@github.com:you/my-project-jul.git
$ jul remote set jul
$ jul doctor

# Daily work
$ # ... edit files ...
$ jul checkpoint
  "feat: add user authentication"
  ✓ Checks passed
  ⚠ 1 suggestion

$ jul apply 01HX7Y9A --checkpoint
  "fix: add null check"

$ jul promote --to main
  Promoted 2 checkpoints
```

### 9.2 Git + Jul Flow

```bash
# Setup
$ git init
$ git remote add origin git@github.com:you/my-project.git
$ jul init
$ git remote add jul git@github.com:you/my-project-jul.git
$ jul remote set jul
$ jul remote publish set origin
$ jul hooks install

# Daily work (normal git)
$ git add . && git commit -m "feat: add auth"
  [jul] synced, Checks queued

$ jul status
$ jul promote --to main
```

### 9.3 Agent-Driven Flow

```bash
# Agent queries state
$ jul status --json | agent-process

# Agent makes changes, checkpoints
$ jul checkpoint --json

# Agent handles suggestions
$ jul suggestions --json
$ jul apply 01HX7Y9A --json

# Agent promotes
$ jul promote --to main --json
```

---

## 10. Git Remote Compatibility

Remote compatibility is the main portability risk for Jul.

**Checkpoint sync requirements:**
- Accept custom refs under `refs/jul/*`
- Accept notes under `refs/notes/jul/*` (read/write/delete)

**Draft sync requirements (additional):**
- Allow non-fast-forward updates to `refs/jul/sync/*` (branches can remain protected)

If custom refs are blocked (or no remote is configured), Jul runs local-only (promote still works).
Jul should probe this with `jul doctor` and choose capabilities automatically.

Sync capabilities:

| Remote capability | What syncs |
|---|---|
| Custom refs + notes | **Checkpoints + metadata** |
| + Non-FF updates allowed on `refs/jul/sync/*` | **Drafts** (per-device) |
| No custom refs | No Jul ref/notes sync (local-only) |

These capabilities describe the sync remote. Publish remote behavior remains standard Git branching.

### 10.1 Recommended: Two Remotes (origin + jul)

```bash
$ jul init myproject
$ git remote add origin git@github.com:george/myproject.git
$ git remote add jul git@github.com:george/myproject-jul.git
$ jul remote set jul
$ jul remote publish set origin
$ jul doctor
$ jul sync
```

This lets `origin` remain strict while Jul uses the best available sync capabilities on the sync remote. `origin`
remains the publish remote for branches and upstream tracking.

Once a sync remote is set, Jul configures the required refspecs and notes automatically. You
provide the remote URL (Jul does not create hosting endpoints).

### 10.2 Compatibility Breakdown

#### 10.2.1 Checkpoint Sync Available

Requirements:
- Custom refs accepted under `refs/jul/*`
- Notes accepted under `refs/notes/jul/*`

Behavior:
- `refs/jul/workspaces/*` (workspace base tips) are canonical across devices
- Trace refs and notes sync normally

#### 10.2.2 Draft Sync Available (Additional)

Requirements:
- Non-fast-forward updates accepted for `refs/jul/sync/*`

Behavior:
- Per-device draft refs can push (`refs/jul/sync/*`)
- Draft handoff across devices becomes possible (`jul draft adopt`)

#### 10.2.3 No Custom Refs (Local-Only)

Requirements:
- Custom refs rejected or no remote configured

Behavior:
- Jul refs/notes stay local
- `jul promote --to <branch>` still works with standard remotes/branches

**Probe strategy:** `jul doctor` should probe in two steps:
1. Create/delete a temp ref under `refs/jul/doctor/<device>` and a temp note under
   `refs/notes/jul/doctor` to verify **checkpoint sync** capability.
2. Attempt a non-fast-forward update on the temp ref to verify **draft sync** capability.
This makes compatibility checks explicit rather than assumed.

### 10.3 Jul-Optimized Server (Future)

A git server optimized for Jul compatibility would provide:

- Guaranteed ref acceptance (all jul/* refs)
- Keep-ref retention (no premature GC)
- Optimized for continuous sync patterns
- Optional: server-side indexing for fast queries and blame acceleration

This is future work. For v1, Jul disables checkpoint sync if custom refs are blocked, and disables
draft sync if non‑FF updates are blocked.

---

## 11. Glossary

| Term | Definition |
|------|------------|
| **Agent Workspace** | Isolated git worktree (`.jul/agent-workspace/worktree/`) where internal agent works |
| **Attestation** | Check results (tests/coverage) attached to a commit (trace, draft, checkpoint, or published) |
| **Change-Id** | Stable identifier (`Iab4f...`) for a logical change. Survives rewrites and promote; enables `jul diff/show/revert <change-id>` on published code |
| **Change Ref** | Stable per-change tip ref (`refs/jul/changes/<change-id>`); used for stacking and published lookup |
| **Change Anchor SHA** | The first checkpoint SHA of a Change-Id; fixed lookup key for cr-state/metadata |
| **Base Change-Id** | When stacked, the parent Change-Id this workspace is pinned to |
| **Base Commit** | Parent of the current draft (workspace base tip: latest checkpoint or workspace base marker) |
| **Checkpoint** | Locked unit of work with message, Change-Id, and trace_base/trace_head refs |
| **Base Divergence** | When one device advanced the base while another has a draft on the old base |
| **Checkpoint Flush** | Rule that `jul checkpoint` must create final trace so trace_head tree = checkpoint tree |
| **Checks Coalescing** | Only latest draft SHA runs checks; older runs cancelled/ignored |
| **Device ID** | Random word pair (e.g., "swift-tiger") identifying this machine |
| **Draft** | Ephemeral commit capturing working tree (parent = base commit) |
| **Draft Attestation** | Device-local check results for current draft (ephemeral, not synced) |
| **External Agent** | Coding agent (Claude Code, Codex) that uses Jul for feedback |
| **Harness Integration** | Agent harness calls `jul trace --prompt "..."` to attach rich provenance |
| **Headless Mode** | Non-interactive agent invocation for automation |
| **Internal Agent** | Configured provider (OpenCode bundled) that runs reviews/merge resolution |
| **jul blame** | Command showing line-by-line provenance: checkpoint → trace → prompt → agent |
| **Keep-ref** | Ref that anchors a checkpoint for retention |
| **Local Workspace** | Client-side saved state for fast context switching |
| **Merge** | Agent-assisted resolution used by `jul draft adopt` or restack conflicts |
| **Promote** | Move checkpoints to a target branch (main) |
| **Prompt Hash** | HMAC‑SHA256 hash of prompt text (synced by default; low risk, no cross‑device correlation). `sha256` mode reveals equality/low‑entropy prompts |
| **Prompt Summary** | AI-generated summary of prompt (local-only by default, opt-in sync with scrubbing) |
| **Secret Scrubber** | Pre-sync filter that detects API keys, passwords, tokens (prompt summaries + draft blobs) |
| **Session Summary** | AI-generated summary of multi-turn conversation that produced a checkpoint |
| **Shadow Index** | Separate index file so Jul doesn't interfere with git staging |
| **Side History** | Trace refs stored separately from main commit ancestry (for provenance without pollution) |
| **Stale Suggestion** | Suggestion created against an old base commit (base changed due to new checkpoint or restack) |
| **Suggestion** | Agent-proposed fix tied to a Change-Id and base SHA, with apply/reject lifecycle |
| **Suggestion Base** | The base a suggestion was created against: `{kind: checkpoint|draft, sha: <sha>}` |
| **Sync Remote** | Git remote used for Jul refs/notes sync (often a remote named `jul`) |
| **Publish Remote** | Git remote used for branches and upstream tracking (often `origin`) |
| **User Namespace** | Stable repo-scoped owner segment used in ref paths (`<user>`), canonicalized in repo-meta notes |
| **Repo Meta** | Note (`refs/notes/jul/repo-meta`) storing repo identity and `user_namespace` |
| **Sync** | Fetch workspace base tip, update local draft snapshot, push draft ref if allowed (no publish-remote fetch) |
| **Sync Ref** | Per-device draft ref (`refs/jul/sync/<user>/<device>/...`), remote-pushed only when draft sync is available |
| **Trace** | Fine-grained provenance unit with prompt/agent/session metadata (side history), keyed by SHA |
| **Trace Attestation** | Lightweight check results (lint, typecheck) attached to a trace |
| **Trace Merge** | Merge commit in trace side-history with two parents; uses strategy `ours` for tree |
| **Trace Sync Ref** | Device's trace backup (`refs/jul/trace-sync/<user>/<device>/...`), remote-pushed when checkpoint sync is available |
| **trace_base** | Checkpoint metadata: previous checkpoint's trace tip SHA (or null) |
| **trace_head** | Checkpoint metadata: current trace tip SHA |
| **Trace Tip** | Canonical trace ref (`refs/jul/traces/<user>/<ws>`), advances with checkpoint tip |
| **Transplant** | (Future) Rebase draft from one base commit to another |
| **Workspace** | Named stream of work (replaces feature branches); can hold multiple Change-Ids over time |
| **Workspace Meta** | Note (`refs/notes/jul/workspace-meta`) storing workspace intent (base/track/pinning/owner) keyed by canonical workspace tip |
| **Workspace Track Ref** | Target branch a workspace tracks for upstream drift (usually `refs/heads/main`) |
| **Workspace Track Tip** | Local record of the last observed `track_ref` tip (updated on checkout/restack/promote; used for drift/rewrite detection) |
| **Workspace Lease** | Per-workspace file (`.jul/workspaces/<ws>/lease`) tracking the last workspace base tip incorporated locally |
| **Workspace Ref** | Canonical workspace base tip (`refs/jul/workspaces/...`) — shared when checkpoint sync is available |
| **Workspace Base Marker** | Synthetic commit created after promote; tree matches published tip and parent is the last checkpoint tip |
| **Workspace Head Ref** | Local base branch (`refs/heads/jul/<workspace>`) that `HEAD` points to |

**Note:** "Trace ID" (e.g., "t1", "t2") is display-only for human readability. Internally, everything is keyed by trace commit SHA.

---

## Appendix A: Why Not Just Use X?

| Alternative | Why Jul is different |
|-------------|---------------------|
| **GitHub/GitLab** | No continuous sync, no checkpoint model, no agent feedback loop |
| **Gerrit** | Change-centric but complex, not agent-native |
| **JJ** | Great local UX but no built-in checks/review/suggestions |
| **Git + hooks** | No rich metadata, no suggestions, no agent integration |

Jul = Git + continuous sync + checkpoints + checks/review + agent-native feedback loop.

### Mental Load Comparison (Git vs JJ vs Jul)

Jul intentionally shifts mental load from manual coordination to automation.  
This is the trade‑off: fewer “decisions per command,” but more underlying machinery.

| Aspect | Git | JJ | Jul |
|--------|-----|----|-----|
| **Daily commands** | Medium (commit/push/pull) | Low | **Low** (checkpoint/promote) |
| **Concept count** | Low | Medium | **High** (workspaces, drafts, traces, suggestions) |
| **Sync coordination** | Manual | Manual | **Automatic** |
| **Conflict handling** | Manual merges | Manual merges | **Agent‑assisted** (suggestions) |
| **Provenance** | Commit history only | Commit history | **Prompt/trace‑aware** |
| **Operational risk** | Low (explicit ops) | Low | Medium (automation must be correct) |

**Summary:** Jul reduces *day‑to‑day* mental load for humans at the cost of a more complex
implementation model. Git/JJ are simpler systems; Jul is optimized for agent‑assisted workflows.

---

## Appendix B: Migration from Git Workflow

| Git habit | Jul equivalent |
|-----------|----------------|
| `git checkout -b feature` | `jul ws new feature` |
| `git add . && git commit` | `jul checkpoint` |
| `git push` | `jul sync` (workspace base tip + optional draft ref; automatic in on-command mode) |
| `git pull` | `jul ws restack` (update workspace base to latest target) |
| `git fetch` | `git fetch` (published refs) + `jul sync` (Jul refs/notes) |
| Merge conflicts | `jul merge` (agent resolves) |
| `git merge main` | `jul promote --to main` |
| `git stash` | `jul local save` |
| `git stash pop` | `jul local restore` |
| `git log` | `jul log` |
| `git diff` | `jul diff` |
| `git show <commit>` | `jul show <checkpoint>` |
| `git status` | `jul status` |
| `git branch` | `jul ws list` |
| `git switch <branch>` | `jul ws switch <workspace>` |
| `git checkout <branch>` | `jul ws checkout <workspace>` |

---

## Appendix C: Failure & Recovery (Quick Paths)

**Remote blocks custom refs**
- **What breaks:** No checkpoint/draft sync; notes do not sync.
- **Still works:** Local drafts, checkpoints, promote to publish remote.
- **Recovery:** Configure a sync remote that allows `refs/jul/*` + notes, then run `jul doctor` and `jul sync`.

**Remote blocks non‑FF updates**
- **What breaks:** Draft sync + cross‑device draft handoff (`jul draft adopt`) are unavailable.
- **Still works:** Checkpoint sync + metadata sync.
- **Recovery:** Use a sync remote that allows non‑FF updates to `refs/jul/sync/*`, or accept local‑only drafts.

**Diverged workspace (another device checkpointed first)**
- **What breaks:** Remote push of your new checkpoint (non‑FF).
- **Still works:** Local work continues; drafts remain local.
- **Recovery:** `jul ws restack` (recommended) or `jul ws checkout @` to reset to the canonical tip.

**`.jul/` deleted or corrupted**
- **What breaks:** Local caches, shadow index, device state.
- **Still works:** All repo data stored in Git objects/refs/notes remains.
- **Recovery:** Re‑run `jul init` then `jul ws checkout <workspace>` to reconstruct state from refs/notes.

**Manual `git switch` / `git commit` outside Jul**
- **What breaks:** Jul model becomes out‑of‑band.
- **Still works:** Git history remains valid.
- **Recovery:** `jul status` warns. Use `jul ws checkout @` to return, or `jul checkpoint --adopt` to adopt commits.

**Secret scan blocks draft push**
- **What breaks:** Draft sync only (local draft still saved).
- **Still works:** Checkpoints/promote.
- **Recovery:** Add to `.jul/syncignore`, remove the secret file, or `jul sync --allow-secrets` (explicit).

---

## Appendix D: References

- [git-http-backend](https://git-scm.com/docs/git-http-backend) — Smart HTTP server
- [JJ (Jujutsu)](https://github.com/jj-vcs/jj) — Inspiration for working-copy model
- [Change-Id trailer format reference](https://gerrit-review.googlesource.com/Documentation/user-changeid.html)
