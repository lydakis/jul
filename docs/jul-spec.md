# 줄 Jul: AI-First Git Workflow

**Version**: 0.3 (Draft)
**Status**: Design Specification

---

## 0. What Jul Is

Jul is **Git with a built-in agent**. It's a local CLI tool that adds:

- **Rich metadata** on every checkpoint (CI results, coverage, lint, traces)
- **Agent-native feedback loop**: checkpoint → get suggestions → act → repeat
- **Continuous sync**: Every draft pushed to your git remote immediately
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
- A special server (standard git remotes work **if** they accept custom refs and non‑FF updates to `refs/jul/*`)
- A CI service (tests run locally)
- A remote execution platform (agents run locally)

**Metadata travels with Git** via refs and notes — on hosts that accept custom refs **and non‑FF updates**. See Section 5.7 for portability details.

---

## 1. Goals and Non-Goals

### 1.1 Goals

- **Local-first**: Everything runs on your machine
- **Continuous sync**: Every change pushed to git remote automatically
- **Checkpoint model**: Lock work, agent generates message, run CI, get suggestions
- **Agent-native feedback**: Rich JSON responses for agents to act on
- **Workspaces over branches**: Named streams of work
- **Rich metadata**: CI/coverage/lint/traces attached to checkpoints
- **Git compatibility**: Any git remote that accepts custom refs + non‑FF updates to `refs/jul/*` (GitHub, GitLab, etc.)
- **JJ friendliness**: Works with JJ's git backend

### 1.2 Non-Goals (v1)

- Replacing Git (Jul is built on Git)
- Server-side execution (everything runs locally)
- Multi-user / teams (single-player for v1)
- Code review UI (use external tools)
- Issue tracking

---

## 2. Core Concepts

### 2.1 Entities

| Entity | Description |
|--------|-------------|
| **Repo** | A normal Git repository |
| **Device** | A machine running Jul, identified by device ID (e.g., "swift-tiger") |
| **Workspace** | A named stream of work. Replaces branches. Default: `@` |
| **Workspace Ref** | Canonical state (`refs/jul/workspaces/...`) — shared across devices |
| **Workspace Lease** | Per-workspace file (`.jul/workspaces/<ws>/lease`) — the semantic lease |
| **Base Commit** | The parent for the current draft (latest checkpoint or latest published commit after promote) |
| **Sync Ref** | Device backup (`refs/jul/sync/<user>/<device>/...`) — always pushes |
| **Trace Sync Ref** | Device trace backup (`refs/jul/trace-sync/...`) — always pushes |
| **Draft** | Ephemeral commit snapshotting working tree (parent = base commit) |
| **Trace** | Fine-grained provenance unit (prompt, agent, session) — side history, keyed by SHA |
| **Checkpoint** | A locked unit of work with message, Change-Id, and trace_base/trace_head refs |
| **Change-Id** | Stable identifier for a logical change (`Iab4f3c2d...`), created at the first draft commit and renewed after promote |
| **Attestation** | CI/test/coverage results attached to a trace, draft, checkpoint, or published commit |
| **Suggestion** | Agent-proposed fix targeting a checkpoint |
| **Local Workspace** | Client-side saved state for fast context switching |

**Change-Id scope:** A Change‑Id groups multiple checkpoints (and later published commits) until `jul promote` closes it.

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
│    • Lightweight CI: lint, typecheck                                    │
│    • Powers `jul blame` for "how did this line come to exist?"          │
├─────────────────────────────────────────────────────────────────────────┤
│  DRAFT (main ancestry)                                                  │
│    • Shadow snapshot of your working tree                               │
│    • Continuously updated (every save)                                  │
│    • Synced to remote automatically                                     │
│    • Change-Id assigned at first draft commit (carried forward)         │
│    • No commit message yet                                              │
├─────────────────────────────────────────────────────────────────────────┤
│                           jul checkpoint                                │
├─────────────────────────────────────────────────────────────────────────┤
│  CHECKPOINT                                                             │
│    • Locked, immutable                                                  │
│    • Agent generates commit message (or user provides with -m)          │
│    • Records trace_base + trace_head (for blame)                        │
│    • Session summary: AI-generated summary of multi-turn work           │
│    • CI runs, attestation created                                       │
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

**Key insight**: Your working tree can still be "dirty" relative to HEAD (normal git). But Jul continuously snapshots your dirty state as a draft commit and syncs it. You can always recover. The draft is your safety net, not your workspace. `jul checkpoint` is when you say "this is a logical unit." Traces track *how* you got there.

### 2.3 Workspaces Replace Branches

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
- `refs/heads/*` exist only as **promote targets** (main, staging, etc.)
- You never work directly on `refs/heads/main`
- Workspaces are where work happens: `refs/jul/workspaces/<user>/<name>`
- Default workspace `@` means you don't need to name anything upfront

### 2.4 Integration Modes

Jul works at multiple levels. Choose your porcelain:
All modes work offline; add a remote only when you want sync/collaboration.

#### 2.4.1 Full Jul Mode

Jul is your primary interface.

```bash
$ jul configure                         # One-time setup
$ jul init my-project --create-remote   # Create repo + server remote (optional)
# ... edit ...
$ jul checkpoint                        # Lock + message + CI + review
$ jul promote --to main                 # Publish
```

#### 2.4.2 Git + Jul (Invisible Infrastructure)

Git is your porcelain. Jul can sync in background via hooks when a remote is configured.

```bash
$ git init && jul init --server https://jul.example.com
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
$ jul init --server https://jul.example.com
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
CI + review runs
         │
         ▼
suggestions created (base_sha: abc123)
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
  "base_sha": "abc123",      // Exact SHA this was created against
  "commit": "def456",         // The suggestion's commit
  "status": "pending",        // pending | applied | rejected | stale
  "reason": "fix_failing_test",
  "confidence": 0.89
}
```

**Staleness:** A suggestion is **fresh** iff `suggestion.base_sha == parent(current_draft)`.  
If the base commit changes (amend **or** new checkpoint in the same change), existing suggestions become stale:

> Example: create a new checkpoint in the same Change‑Id without amending; prior suggestions become stale because the draft’s base commit advanced.

```
checkpoint abc123 (change Iab4f...)
         │
         ├── suggestion created (base_sha: abc123)
         │
         ▼
amend → checkpoint def456 (same Iab4f...)
         │
         ├── suggestion marked "stale" (base mismatch)
         │
         ▼
$ jul apply 01HX7Y9A
⚠ Suggestion is stale (created for abc123, current is def456)
  Run 'jul review' to generate fresh suggestions.
```

**Why track base_sha?** Change-Id survives amends, but the code changed. A suggestion that fixed line 45 in abc123 might not apply cleanly to def456 if you edited that area.

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

#### 2.5.1 Review Lifecycle (One Workspace = One Review)

Jul keeps reviews simple: **one workspace equals one review**.

- `jul submit` **creates or updates** the review for the current Change-Id.
- There are **no review IDs** and no `submit --new`.
- Each submit points the review at the **latest checkpoint** (a new revision).
- If you created multiple checkpoints before the first submit, the review simply reflects the latest one (cumulative diff from the base commit).
- After `jul promote`, the next draft starts a **new Change-Id**, and the next submit opens a **new review**.
- Submit is **optional** — solo workflows can go straight from checkpoint → promote.

Review state lives in Git notes so it works offline and syncs with the repo:
- `refs/notes/jul/review-state` — keyed by the **Change-Id anchor SHA** (the first checkpoint SHA for the change); stores Change-Id, status, and latest checkpoint
- `refs/notes/jul/review-comments` — keyed by checkpoint SHA; stores review comments/threads with `change_id` and optional file/line
  - The anchor SHA is also recorded in `refs/notes/jul/meta` for lookup by Change-Id

Comments can be **review-level** (no file/line, applies to the whole Change-Id) or **checkpoint-level** (anchored to a specific checkpoint/file/line). Threads can span multiple checkpoints by reusing the same `thread_id`.

**Review anchor retention:** The Change‑Id anchor SHA never changes (even if the first checkpoint is amended). While a review is open, that anchor commit is pinned (its keep‑ref does not expire). Retention is based on **last‑touched** for open reviews.

Example:
```bash
$ jul ws new feature-auth
$ jul checkpoint
$ jul checkpoint
$ jul submit            # opens review for feature-auth
$ jul checkpoint
$ jul submit            # updates the same review

$ jul ws stack feature-b  # create dependent workspace
$ jul checkpoint
$ jul submit            # opens review for feature-b (stacked)
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
  refs/jul/traces/george/@  →  t3 ← t2 ← t1
                               (single tip ref, parent chain provides history)
```

**Naming clarity:**
- **Change-Id** (`Iab4f...`): Stable identifier for a review/change (workspace)
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
  → updates refs/jul/traces/george/@ to point to t1
  
Agent prompt: "use JWT instead"
  │
  ▼
jul trace --prompt "use JWT instead" --agent claude-code
  → creates trace t2 (parent = t1)
  → updates refs/jul/traces/george/@ to point to t2

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
refs/jul/traces/george/@/t1   # Bad: ref explosion
refs/jul/traces/george/@/t2
refs/jul/traces/george/@/t3
```

Single tip ref with parent chain:
```
refs/jul/traces/george/@      # Points to t3
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

After workspace merge, trace history also merges:
  t1 ← t2 ← t3 ─┐
                ├─ t6 (merge trace, two parents)
  t1 ← t4 ← t5 ─┘

refs/jul/traces/george/@ now points to t6
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

Blame walks from head(s) to base. Tiny metadata, same power. Avoids blowing Jul’s 16KB per-note cap.

**Privacy defaults (secrets can leak in summaries too!):**

```toml
[traces]
sync_prompt_hash = true       # Always sync (cannot leak secrets)
sync_prompt_summary = false   # Opt-in — summaries CAN leak paraphrased secrets!
sync_prompt_full = false      # Opt-in — definitely can leak
```

| Setting | Default | What syncs | Risk |
|---------|---------|-----------|------|
| `sync_prompt_hash` | true | SHA-256 hash | None |
| `sync_prompt_summary` | false | AI summary | Medium (can paraphrase secrets) |
| `sync_prompt_full` | false | Full text | High |

If `sync_prompt_summary = true`, Jul runs a secret scrubber before syncing (detects API keys, passwords, tokens). But scrubbing isn't perfect — if you're paranoid, keep summaries local.

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

**CI on traces:**

Traces get cheap, fast checks (lint, typecheck). Full CI runs on checkpoint.

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

### 3.2 Workspace Refs

```
refs/jul/workspaces/<user>/<workspace>
```

The **canonical** state of a workspace — the "merged truth" shared across devices.

Examples:
```
refs/jul/workspaces/george/@              # Default workspace
refs/jul/workspaces/george/feature-auth   # Named workspace
```

Updated only when:
- Sync succeeds (not diverged, force-with-lease passes)
- Merge completes (after resolving divergence)

### 3.3 Sync Refs

```
refs/jul/sync/<user>/<device>/<workspace>
```

Your **personal backup stream per device** — always pushes, never blocked.

Examples:
```
refs/jul/sync/george/swift-tiger/@           # Laptop backup
refs/jul/sync/george/quiet-mountain/@        # Desktop backup
refs/jul/sync/george/swift-tiger/feature-auth
```

**Device ID:**
- Auto-generated on first `jul init` (e.g., "swift-tiger", "quiet-mountain")
- Stored in `~/.config/jul/device`
- Two random words, memorable and unique enough for personal use

**The relationship:**
- Workspace ref = canonical truth (shared across devices)
- Sync ref = your backup on THIS device (safe from other devices)
- When not diverged: workspace = sync (same commit)
- When diverged: workspace ≠ sync (must merge to reunify)

### 3.4 Trace Refs (Provenance Side History)

Traces mirror the workspace/sync pattern with two ref levels:

```
refs/jul/traces/<user>/<workspace>              # Canonical tip (advances with workspace)
refs/jul/trace-sync/<user>/<device>/<workspace> # Device backup (always pushes)
```

**Why two levels?** Same reason as workspace/sync: canonical tip advances only when workspace does, but device backup never loses work even during "conflicts pending" state.

Examples:
```
refs/jul/traces/george/@                    # Canonical trace tip
refs/jul/trace-sync/george/swift-tiger/@    # Laptop's trace backup
refs/jul/trace-sync/george/quiet-mountain/@ # Desktop's trace backup
```

**Not N refs per trace:** To avoid ref explosion (fetch negotiation, packed-refs, host limits), we store one tip ref per workspace, not one ref per trace commit.

**Trace chain structure:**
```
refs/jul/traces/george/@  →  abc123  (tip SHA)
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
refs/jul/traces/george/@  →  merge_sha  (merge trace)
                               /   \
                          abc123   xyz789
                             │       │
                          def456   uvw012
```

The merge trace commit uses strategy `ours` for its tree (tree = the **canonical workspace tip after sync**, i.e., the workspace ref we just updated). This keeps both device histories reachable without requiring code conflict resolution just to unify traces.

This lets `jul blame` traverse the DAG to find the real origin trace.

**Trace metadata** (stored in notes keyed by trace commit SHA):
```json
{
  "prompt_hash": "sha256:abc123...",
  "agent": "claude-code",
  "trace_type": "prompt",       // prompt | sync | merge
  "session_id": "abc123",
  "turn": 5,
  "device": "swift-tiger",
  "prompt_summary": "Added null check for auth token"
}
```

`prompt_summary` and `prompt_full` are **only present in synced notes** when `traces.sync_prompt_summary` / `traces.sync_prompt_full` are enabled. With defaults, only the hash is synced.

Note: No "trace_id" field — use short SHA for display. `trace_type=merge` marks connective merge traces; `jul blame` skips attribution to merge traces.

**Privacy defaults (secrets can leak in summaries too):**

```toml
[traces]
sync_prompt_hash = true       # Always (cannot leak secrets)
sync_prompt_summary = false   # Opt-in (summaries CAN leak secrets!)
sync_prompt_full = false      # Opt-in (definitely can leak)
```

By default, only the prompt hash syncs. Summaries are generated locally and stay local unless explicitly opted in. If `sync_prompt_summary = true`, Jul runs a secret scrubber before syncing (detects API keys, passwords, tokens).

**Local storage:**
```
.jul/traces/
├── prompts/           # Full prompt text (keyed by trace SHA)
└── summaries/         # AI summaries (keyed by trace SHA)
```

**Lifecycle:**
- Created by `jul trace` (explicit) or `jul sync` (implicit, if tree changed)
- Device trace-sync ref always pushes
- **Canonical trace tip advances only when canonical workspace advances**  
  If workspace is diverged (conflicts pending), **do not update `refs/jul/traces/...`** (only update `trace-sync/...`)
- Referenced by checkpoint metadata: `{trace_base, trace_head}`
- Cleaned up when associated checkpoint expires

**Idempotency:** `jul sync` does NOT create a new trace if working tree equals current trace tip tree. This prevents trace spam from repeated syncs with no actual changes.

**Not part of main ancestry:** Drafts and checkpoints do NOT have traces as parents. Traces are queryable side data.

### 3.5 Suggestion Refs

```
refs/jul/suggest/<Change-Id>/<suggestion_id>
```

- Points to suggested commit — the actual code changes
- Tied to a Change-Id **and** a specific base checkpoint SHA
- Immutable once created
- Can be fetched, inspected, cherry-picked
- Metadata (reasoning, confidence, base_sha) stored in notes

**Staleness:** A suggestion is **fresh** iff `suggestion.base_sha == parent(current_draft)`.  
If the base commit changes (amend **or** new checkpoint in the same change), existing suggestions become stale.

**Cleanup:** Suggestion refs are deleted when their parent checkpoint's keep-ref expires. This prevents ref accumulation:

```
refs/jul/keep/george/@/Iab4f.../abc123  expires
    → delete refs/jul/suggest/Iab4f.../*
    → delete associated notes
```

Without this, suggestion refs would accumulate forever even after their checkpoints are GC'd.

### 3.6 Keep Refs

```
refs/jul/keep/<user>/<workspace>/<change_id>/<sha>
```

Anchors checkpoints for retention/fetchability. Without a ref, git may GC unreachable commits.

### 3.7 Notes Namespaces

**Synced notes (pushed to remote):**
```
refs/notes/jul/attestations/checkpoint   # Checkpoint CI results (keyed by SHA)
refs/notes/jul/attestations/published    # Published CI results (keyed by SHA)
refs/notes/jul/attestations/trace        # Trace CI results (keyed by trace SHA)
refs/notes/jul/traces                    # Trace metadata (prompt hash, summary, agent)
refs/notes/jul/review-comments           # Review comments/threads (keyed by checkpoint SHA)
refs/notes/jul/review-state              # Review state (keyed by Change-Id anchor)
refs/notes/jul/meta                      # Change-Id mappings
refs/notes/jul/suggestions               # Suggestion metadata
```

**Local-only storage (not synced):**
```
.jul/ci/                  # Draft attestations (device-scoped, ephemeral)
.jul/workspaces/<ws>/     # Per-workspace tracking (workspace_lease)
.jul/local/               # Saved local workspace states
.jul/traces/              # Full prompt text and summaries (local by default)
```

Notes are pushed with explicit refspecs. Draft attestations are local-only by default to avoid multi-device write contention.

### 3.8 Complete Ref Layout

```
refs/
├── heads/                           # Promote targets
│   ├── main
│   └── staging
├── tags/
├── jul/
│   ├── workspaces/                  # Canonical state (shared truth)
│   │   └── <user>/
│   │       ├── @
│   │       └── <named>
│   ├── sync/                        # Device backups for drafts (per-device)
│   │   └── <user>/
│   │       └── <device>/
│   │           ├── @
│   │           └── <named>
│   ├── traces/                      # Canonical trace tips (advances with workspace)
│   │   └── <user>/
│   │       ├── @                    # Points to trace tip SHA, parent chain provides history
│   │       └── <named>
│   ├── trace-sync/                  # Device backups for traces (always pushes)
│   │   └── <user>/
│   │       └── <device>/
│   │           ├── @
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
    ├── traces                       # Trace metadata (prompt hash, agent, session)
    ├── review-comments              # Review comments/threads (keyed by checkpoint SHA)
    ├── review-state                 # Review state (keyed by Change-Id anchor)
    ├── meta
    └── suggestions
```

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
- Parent = base commit (latest checkpoint or latest published commit)
- Always pointed to by this device's sync ref
- Pointed to by workspace ref only when canonical (not diverged)

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
- Force-updates sync ref
- Old draft becomes unreachable (ephemeral)

If the tree is unchanged, Jul reuses the existing draft SHA (still fetches/merges remote changes).

This avoids "infinite WIP commit chain" — there's always exactly one draft commit per workspace, with parent = base commit. Drafts are siblings (same parent), not ancestors of each other.

**Why commits, not sidecar snapshots:**
- Git tools work (diff, log, bisect)
- Push to any git remote
- JJ interop preserved
- Attestations attach via notes

**Shadow index for non-interference:**

Jul uses a shadow index so it doesn't interfere with your normal git staging:

```bash
# Jul sync implementation
GIT_INDEX_FILE=.jul/draft-index git add -A
GIT_INDEX_FILE=.jul/draft-index git write-tree
# Create commit from tree with parent = base commit
# Force-update sync ref
# Update workspace ref only if not diverged (via force-with-lease)
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
| CI runs | Optional | Always |
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

### 5.3 Sync and Merge

**Two ref types, different scopes:**

```
refs/jul/workspaces/george/@              ← canonical (shared across devices)
refs/jul/sync/george/swift-tiger/@        ← this device's backup
```

**Plus local tracking files (per-workspace):**

```
.jul/workspaces/@/lease              ← SHA of last workspace state we merged
.jul/workspaces/feature-auth/lease  ← Same, for feature-auth workspace
```

#### How sync works

```bash
$ jul sync
Syncing...
  ✓ Fetched workspace ref
  ✓ Pushed to sync ref
  ✓ Workspace ref updated
```

**The sync algorithm:**
1. **Fetch** workspace ref → `workspace_remote`
2. **Push** to device's sync ref with `--force` (always succeeds, it's yours)
3. **Detect base divergence early**:
   - `local_base = parent(local_draft)`
   - `remote_base = parent(workspace_remote)`
   - If both exist and `local_base != remote_base` → **base_diverged** (abort; require `jul ws checkout`/`jul transplant`)
4. **Compare** `workspace_remote` to local `workspace_lease` (per-workspace)
5. **If** `workspace_remote == workspace_lease`:
   - Not diverged, safe to update
   - Update workspace ref with `--force-with-lease=<workspace_ref>:<workspace_remote>`
   - Set `workspace_lease = new_sha`
6. **If** `workspace_remote != workspace_lease`:
   - Another device pushed since we last merged
   - **Try auto-merge:**
     - Merge base = merge-base of the two draft commits (typically the shared base commit)
     - 3-way merge: merge_base ↔ workspace_remote (theirs) ↔ sync (ours)
   - **If no conflicts**: 
     - Update workspace ref with `--force-with-lease=<workspace_ref>:<workspace_remote>`
     - If lease fails, re-fetch and retry (or fall back to "conflicts pending")
     - Set `workspace_lease = new_sha`
   - **If conflicts**: mark diverged, defer to `jul merge`

**Why workspace_lease matters:** It's the semantic lease — it tracks the last workspace state we incorporated, so we know when we're behind.

**Why lease against workspace_remote:** When updating workspace ref after auto-merge, we guard against "someone else pushed while we were merging." If the lease fails, we re-fetch and try again.

**Why merge-base of drafts:** Since drafts have parent = base commit, the merge-base is the shared base commit (checkpoint or published commit). This avoids relying on old ephemeral draft commits.

#### Sync with auto-merge (no conflicts)

If another device pushed but the changes don't overlap:

```bash
$ jul sync
Syncing...
  ✓ Fetched workspace ref (another device pushed)
  ✓ Pushed to sync ref
  ✓ Auto-merged (no conflicts)
  ✓ Workspace ref updated
```

No user action needed. Git's 3-way merge handles it.

**Important:** Auto-merge produces a **new draft commit with single parent** (the base commit), NOT a 2-parent merge commit. Jul uses `git merge-tree` or equivalent to compute the merged tree, then creates a new draft commit:

```
parent = base commit (single parent, preserving "draft parent = base commit" invariant)
tree = result of 3-way merge
```

This is NOT `git merge` which would create a 2-parent commit.

#### Sync with conflicts (deferred)

If the changes actually conflict:

```bash
$ jul sync
Syncing...
  ✓ Fetched workspace ref (another device pushed)
  ✓ Pushed to sync ref (your work is safe!)
  ⚠ Conflicts detected — merge pending
  
Continue working. Run 'jul merge' when ready.
```

**You keep coding.** Your sync ref keeps updating. Deal with the conflict when you're ready.

#### Checkpoint base divergence

**Failure mode:** What if Device A advanced the base (new checkpoint or promote) while Device B has a local draft based on the OLD base commit?

```
Device A: checkpoint1 → checkpoint2 (pushed)
Device B: checkpoint1 → draft (still on old base)
```

This is different from normal divergence (both on same base, different drafts). Here, the base histories have forked.

**Detection:** Compare draft parents directly:
`local_base = parent(local_draft)`, `remote_base = parent(workspace_remote)`.  
If they differ, we've got base divergence.

**V1 behavior:** Fail with clear error:

```bash
$ jul sync
Syncing...
  ✓ Pushed to sync ref (your work is safe!)
  ✗ Base diverged
  
Your draft is based on checkpoint1, but workspace is now at checkpoint2.
Your work is safe on your sync ref.

Options:
  jul ws checkout @     # Discard local changes, start fresh from checkpoint2
  jul transplant        # (future) Rebase your draft onto checkpoint2
```

**Why not auto-fix?** Transplanting a draft from one base commit to another is a rebase operation that can have complex conflicts. V1 takes the safe path: your work is preserved on sync ref, but you must explicitly decide how to proceed.

#### Merge when ready

```bash
$ jul merge
Agent resolving conflicts...

Conflicts resolved:
  src/auth.py — combined both changes (you added validation, they added caching)

Resolution ready as suggestion [01HX...].
Accept? [y/n] y

  ✓ Merged
  ✓ Workspace ref updated
  ✓ workspace_lease updated
```

**The merge algorithm:**
1. Merge base = merge-base of workspace ref and sync ref (the shared base commit)
2. 3-way merge: merge_base ↔ workspace (theirs) ↔ sync (ours)
3. Agent resolves conflicts automatically
4. Create resolution as suggestion
5. If accepted: new draft commit (parent = base commit, NOT a 2-parent merge)
6. Update workspace ref, sync ref, AND `workspace_lease`

**V1 constraint:** Both sides must share the same base commit. If base histories have diverged, manual intervention required.

**The invariants:**
- Sync ref = this device's backup (always pushes, device-scoped)
- Workspace ref = canonical state (shared across devices)
- `workspace_lease` = last workspace SHA we incorporated (per-workspace)
- Auto-merge produces single-parent commit (parent = base commit), not 2-parent merge
- Diverged only when: auto-merge fails due to actual conflicts
- Can't promote until divergence is resolved

**For agents (JSON response):**

```json
{
  "sync": {
    "status": "ok",
    "auto_merged": true,
    "workspace_updated": true
  }
}
```

Or when conflicts exist:

```json
{
  "sync": {
    "status": "conflicts",
    "sync_pushed": true,
    "workspace_updated": false,
    "conflicts": ["src/auth.py"],
    "merge_pending": true
  },
  "next_actions": ["continue working", "jul merge"]
}
```

Or when bases have diverged:

```json
{
  "sync": {
    "status": "base_diverged",
    "sync_pushed": true,
    "workspace_updated": false,
    "local_base": "checkpoint1_sha",
    "remote_base": "checkpoint2_sha"
  },
  "next_actions": ["jul ws checkout @", "manual intervention"]
}
```

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
- Jul: no-conflict cases auto-merge; conflicts deferred until you're ready

#### Trace Sync Algorithm

Traces mirror the workspace/sync pattern:

```
refs/jul/traces/george/@                    ← canonical trace tip
refs/jul/trace-sync/george/swift-tiger/@    ← this device's trace backup
```

**The trace sync algorithm (runs as part of `jul sync`):**

1. **Push** to device's trace-sync ref with `--force` (always succeeds)
2. **Fetch** canonical trace tip → `trace_remote`
3. **Compare** `trace_remote` to local trace tip
4. **If same or fast-forward**: canonical trace = local (simple case)
5. **If diverged** (both devices created traces) **and workspace is not diverged**:
   - Create **trace merge commit** with two parents: `trace_remote` and local tip
   - Tree = **canonical workspace tip after sync** (strategy `ours`)
   - Push trace merge as new canonical tip
6. **If workspace is diverged (conflicts pending)**:
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

**Why strategy `ours` for trace merge tree?** The trace merge exists purely to keep both device histories reachable for `jul blame`. The actual code state is determined by the workspace merge, not the trace merge. So we use the canonical workspace tree **after sync** (which may be the result of a separate code merge) as the trace merge tree.

**Timing:** Trace sync happens atomically with workspace sync:
- Workspace diverged + traces diverged → both get merge commits
- Workspace not diverged + traces diverged → trace merge, workspace fast-forward
- "Conflicts pending" state → trace-sync refs still push (nothing lost), canonical trace tip waits until workspace resolves

**Idempotency:** If working tree equals current trace tip tree, no new trace is created. This prevents trace spam from repeated syncs.

### 5.4 Sync Modes

**Local-first:** Jul works with or without a remote.

Without a remote configured:
```bash
$ jul sync
Syncing...
  ✓ Draft committed
  ✓ Workspace ref updated
  
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
- Fetch workspace ref, push to sync ref, update workspace if possible
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
- Runs the same sync algorithm: fetch → push to sync ref → update workspace if not diverged
- If diverged, daemon keeps pushing to sync ref but doesn't update workspace

**Configuration:**
```toml
[sync]
mode = "continuous"
debounce_seconds = 2        # Wait for writes to settle
min_interval_seconds = 5    # Don't sync more often than this
```

**Pros:** Never lose work, seamless multi-device handoff
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
```
# .jul/syncignore
.jul/              # CRITICAL: ignore Jul's own directory
*.tmp
*.swp
*.lock
.idea/
node_modules/
target/
build/
dist/
__pycache__/
*.pyc
.DS_Store
```

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
$ jul ws checkout @           # Establishes baseline: sync ref + workspace_lease
```

The checkout establishes your baseline: it sets `workspace_lease` and initializes your sync ref. Now `jul sync` knows where you started.

Ongoing work across devices:
```bash
# Device A (swift-tiger): daemon running, syncing continuously
# ... edit files ...

# Device B (quiet-mountain):
$ jul sync                    # Fetch + auto-merge if needed
# No conflicts: workspace updated automatically
# Conflicts: shows warning, run 'jul merge' when ready

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
refs/jul/keep/george/@/Iab4f3c2d/abc123
refs/jul/keep/george/@/Iab4f3c2d/def456   # Amended checkpoint
refs/jul/keep/george/feature/Icd5e6f7a/ghi789
```

**Lifecycle:**
- Created when checkpoint is locked
- TTL-based expiration (configurable, default 90 days) based on **last-touched**
- **Pinned while review open:** keep‑refs for review anchors do not expire until review is closed/promoted
- Expired keep-refs deleted by Jul maintenance job
- **Cascade cleanup:** When keep-ref expires, also delete:
  - Associated suggestion refs (`refs/jul/suggest/<change-id>/*`)
  - Associated notes (attestations, review comments)
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

| Attestation Type | Attached To | Scope | CI Level | Purpose |
|------------------|-------------|-------|----------|---------|
| **Trace** | Trace SHA | Synced | Cheap (lint, typecheck) | Per-trace provenance |
| **Draft** | Current draft SHA | Device-local | Full (optional) | Continuous feedback, ephemeral |
| **Checkpoint** | Original checkpoint SHA | Synced | Full (required) | Pre-integration CI, review |
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
- **Provenance data** — "what was CI saying when this line was written?"

```toml
[ci]
run_on_trace = true                    # Default: cheap checks on each trace
trace_checks = ["lint", "typecheck"]   # What to run per-trace
```

**Draft attestations (full CI, ephemeral):**

By default, CI runs in the background every time the **local draft SHA changes**:

```bash
$ jul sync
Syncing...
  ✓ Workspace ref updated
  ⚡ CI running in background...

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
run_on_draft = true           # Default: run CI on every draft sync
run_on_checkpoint = true      # Always run on checkpoint
draft_ci_blocking = false     # Default: non-blocking background run
sync_draft_attestations = false  # Default: local-only (avoid multi-device conflicts)
```

**Local storage (device-scoped):**
```
.jul/ci/
├── current_draft_sha         # SHA of draft being tested (or last tested)
├── current_run_pid           # PID of running CI (for cancellation)
├── results.json              # Latest results
└── logs/
    └── 2026-01-19-153045.log
```

**CI coalescing rules:**
1. New sync → cancel any in-progress CI run for old draft
2. Start CI for new draft SHA
3. `jul ci status` reports: (a) latest completed SHA, (b) whether it matches current draft
4. If results are for old draft: show with warning "⚠ results for old draft"

**Run types and visibility:**
- **Background draft CI (sync‑triggered)**: one at a time, coalesced per device. Visible via `jul ci status` (current draft + running PID).
- **Foreground CI (`jul ci run` / `jul ci run --watch`)**: runs immediately and streams output; results are recorded for the target SHA.
- **Checkpoint CI (`jul checkpoint`)**: runs after a checkpoint and writes an attestation note for that checkpoint.
- **Manual CI (`jul ci run --target/--change`)**: attaches to the requested revision; does not replace draft CI unless it targets the draft SHA.

**Multiple runs:** Draft CI is single‑flight per device (latest draft wins). Manual/foreground runs can be started while draft CI is idle, but if they target the draft SHA they will supersede the previous draft result.

```bash
$ jul status
Draft def456 (change Iab4f...) (3 files changed)
  ⚠ CI results for previous draft (abc123)
  ⚡ CI running for current draft (def456)...
```

**Synced storage (checkpoint/published only):**
```
refs/notes/jul/attestations/checkpoint   # Keyed by original SHA
refs/notes/jul/attestations/published    # Keyed by published SHA
```

**Workflow:**
```
draft sync
    │
    ├── CI runs (background, local) → .jul/ci/results.json
    │
    ▼
checkpoint abc123 (change Iab4f...)
    │
    ├── CI runs → checkpoint attestation (synced via notes)
    │
    ▼
jul promote --to main --rebase
    │
    ├── Rebase creates new SHA xyz789
    ├── (Optional) CI runs on xyz789 → published attestation
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
    {"sha": "def456", "message": "feat: add auth (amended)"}
  ],
  "promote_events": [
    {
      "target": "main",
      "strategy": "rebase",
      "timestamp": "2026-01-19T15:42:00Z",
      "published": ["xyz789"],
      "merge_commit_sha": null,
      "mainline": null
    }
  ]
}
```

### 5.7 Notes: Storage, Sync, and Portability

**Metadata travels with git... mostly.**

Jul stores all metadata as git objects (notes, refs). This means it *can* be synced via git push/pull. However:

- Different hosts have different ref policies (some block custom refs)
- Some hosts reject non‑FF updates for `refs/jul/*`
- Size limits vary (GitHub has push size limits)
- Retention varies (some hosts GC aggressively)

**The right expectation:** Jul metadata syncs on hosts that allow custom refs **and** non‑FF updates for `refs/jul/*`. If a host blocks those, Jul degrades gracefully to local-only for those namespaces. Jul config sets up the refspecs; check `jul doctor` to see what's actually syncing.

**Size limits to prevent repo bloat:**

Continuous sync can balloon your repo if attestations contain full logs or coverage reports. Rules:

| Data | Storage | Size Limit |
|------|---------|------------|
| Attestation summary | Notes | ≤ 16 KB |
| Suggestion metadata | Notes | ≤ 16 KB |
| Full CI logs | Local only (`.jul/logs/`) | No limit |
| Coverage reports | Local only (`.jul/coverage/`) | No limit |
| Suggestion patches | Commits | No limit (git handles) |

Notes store summaries; big artifacts stay local by default.

**Refspec configuration:**

Git notes are not fetched by default. Jul configures explicit refspecs:

```ini
[remote "origin"]
    url = git@github.com:george/myproject.git
    fetch = +refs/heads/*:refs/remotes/origin/*
    fetch = +refs/jul/*:refs/jul/*
    fetch = +refs/notes/jul/*:refs/notes/jul/*
```

**Recommended writers per namespace:**

Notes refs can have non-fast-forward conflicts. Prefer clear ownership, but **multiple devices may still write** (e.g., two laptops). Use the notes sync algorithm below to merge safely.

| Namespace | Typical writer | Content |
|-----------|----------------|---------|
| `refs/notes/jul/meta` | Client | Change-Id mappings |
| `refs/notes/jul/attestations` | CI runner | Test results (summaries only) |
| `refs/notes/jul/review-comments` | Review agent | Review comments/threads (checkpoint-scoped) |
| `refs/notes/jul/review-state` | Client | Review state (Change-Id anchor, latest checkpoint) |
| `refs/notes/jul/suggestions` | Review agent | Suggestion metadata |
| `refs/notes/jul/traces` | Client | Trace metadata (prompt hash, agent, session) |

**Notes sync algorithm (multi-device):**

Even though notes are mergeable, the notes ref itself can reject non‑fast‑forward pushes. Jul syncs notes like this:
1. Fetch remote notes ref into a temporary ref
2. `git notes --ref <ref> merge <temp_ref>`
3. Push merged notes ref with lease

This avoids flaky push failures when two devices append notes in parallel.

**Concurrency rule:** When multiple devices might update the same note entry, prefer **append-only events** (e.g., review comment events, suggestion status events) and derive current state from the latest event. This avoids conflicts from “last writer wins” overwrites.

**Suggestions storage:**

Suggestions have two parts:
- **Patch commits**: `refs/jul/suggest/<Change-Id>/<suggestion_id>` — actual code
- **Metadata**: `refs/notes/jul/suggestions` — reasoning, confidence, status

Commits carry the heavy diffs; notes stay small.

**Trace prompt privacy:**

See section 2.6 for full privacy settings. Summary:
- `sync_prompt_hash = true` (default) — hash always syncs, cannot leak secrets
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
           │   refs/jul/keep/george/@/Iab4f.../def456
           │                        │
           │                        ▼
           │           def456 (checkpoint, immutable) ◄─── attestation
           │               │
           │               │   refs/jul/workspaces/george/@         ◄─ canonical
           │               │   refs/jul/sync/george/swift-tiger/@   ◄─ this device
           │               │              │
           │               │              ▼
           │               └──── ghi789 (draft, ephemeral)
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
- `refs/jul/workspaces/<user>/<ws>` — Canonical **draft** (shared across devices); always points to latest draft commit
- `refs/jul/sync/<user>/<device>/<ws>` — This device's backup (never clobbered)
- `refs/jul/keep/*` — Checkpoint retention anchors
- `refs/jul/suggest/*` — Suggestion patch commits
- `refs/notes/jul/*` — Metadata (attestations, review-state/comments, suggestions, traces)

**Local state (per workspace):**
- `.jul/workspaces/<ws>/lease` — SHA of last workspace state we merged (the semantic lease)

**Invariants:**
- `workspace_remote == workspace_lease` → not diverged, update workspace directly
- `workspace_remote != workspace_lease` → try auto-merge; only defer if actual conflicts
- `jul ws checkout` establishes baseline (sync ref + workspace_lease)
- Can't promote until divergence is resolved

---


## 6. CLI Design (`jul`)

### 6.1 Setup Commands

#### `jul configure`

Interactive setup wizard for global configuration.

```bash
$ jul configure
Jul Configuration
─────────────────
Remote URL (optional): https://jul.example.com
Username: george
Create remote by default? [Y/n]: Y

Agent Provider:
  [1] opencode (bundled)
  [2] claude-code
  [3] codex
  [4] custom
Select [1]: 1

Configuration saved to ~/.config/jul/config.toml
```

Creates:
- `~/.config/jul/config.toml` — Remote, user defaults, init preferences
- `~/.config/jul/agents.toml` — Agent provider settings

#### `jul init`

Initialize a repository with Jul.

```bash
# In a cloned repo (origin exists)
$ cd my-project
$ jul init
Using remote 'origin' (git@github.com:george/myproject.git)
Device ID: swift-tiger
Workspace '@' ready

# In a repo with multiple remotes (no origin)
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

**Remote selection logic:**
1. If `origin` exists → use it
2. If no `origin` but exactly one remote → use that
3. If no `origin` and multiple remotes → require explicit `jul remote set`
4. If no remotes → work locally

What it does:
1. `git init` (if new)
2. Generate device ID (e.g., "swift-tiger") → `~/.config/jul/device`
3. Select remote (if available)
4. Add Jul refspecs to remote (if configured)
5. Create default workspace `@`
6. Start first draft

#### `jul remote`

View or set the git remote used for sync.

```bash
# View current remote
$ jul remote
Using 'origin' (git@github.com:george/myproject.git)

# Set remote
$ jul remote set upstream
Now using 'upstream' for sync.

# Clear remote (work locally)
$ jul remote clear
Remote cleared. Working locally.
```

### 6.2 Core Workflow Commands

#### `jul sync`

Sync current draft. With a remote: fetch, push, auto-merge. Without: local only.

```bash
# With remote configured
$ jul sync
Syncing...
  ✓ Fetched workspace ref
  ✓ Pushed to sync ref
  ✓ Workspace ref updated
  ✓ workspace_lease updated

# Without remote (local only)
$ jul sync
Syncing...
  ✓ Draft committed
  ✓ Workspace ref updated (local)
```

If another device pushed but changes don't conflict:
```bash
$ jul sync
Syncing...
  ✓ Fetched workspace ref (another device pushed)
  ✓ Pushed to sync ref
  ✓ Auto-merged (no conflicts)
  ✓ Workspace ref updated
```

If changes actually conflict:
```bash
$ jul sync
Syncing...
  ✓ Fetched workspace ref (another device pushed)
  ✓ Pushed to sync ref (your work is safe!)
  ⚠ Conflicts detected — merge pending
  
Continue working. Run 'jul merge' when ready.
```

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

#### `jul trace`

Create a trace (provenance snapshot) with optional prompt/agent metadata.

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
| No explicit traces | `jul sync` creates trace implicitly (no prompt attached) |

Traces are stored as side history (not in checkpoint ancestry). Use `jul blame` to query.

Flags:
- `--prompt <text>` — Attach prompt text
- `--prompt-stdin` — Read prompt from stdin
- `--agent <name>` — Agent that created this trace
- `--session-id <id>` — Session identifier (for multi-turn tracking)
- `--turn <n>` — Turn number within session
- `--json` — JSON output

#### `jul merge`

Resolve diverged state. Agent handles conflicts automatically. See [6.7 Merge Command](#67-merge-command) for full details.

```bash
$ jul merge
Agent resolving conflicts...

Conflicts resolved:
  src/auth.py — combined both changes

Resolution ready as suggestion [01HX...].
Accept? [y/n] y

  ✓ Merged
  ✓ Workspace ref updated
  ✓ workspace_lease updated
```

#### `jul checkpoint`

Lock current draft, generate message, start new draft.

```bash
$ jul checkpoint
Locking draft abc123 (change Iab4f...)

Generating message... (opencode)
  "feat: add JWT validation with refresh token support"

Accept? [y/n/edit] y

Syncing... done
Running CI...
  ✓ lint
  ✓ compile  
  ✓ test (48/48)
  ✓ coverage (84%)

Running review...
  ⚠ 1 suggestion created

Checkpoint def456 locked (change Iab4f...).
New draft ghi789 started (change Iab4f...).
```

Flags:
- `-m "message"` — Provide message (skip agent)
- `--amend` — Amend previous checkpoint instead of creating new one
- `--prompt "..."` — Store the prompt that led to this checkpoint (optional metadata)
- `--adopt` — Adopt the current `HEAD` commit as a checkpoint (opt‑in; doesn’t move branches)
- `--no-review` — Skip review
- `--json` — JSON output

**Amend semantics:** `jul checkpoint --amend` creates a **new checkpoint commit** (new SHA) with the **same Change‑Id**. The old checkpoint remains reachable via keep‑refs (pinned while review is open, otherwise subject to retention). No in‑place history rewrite.

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

**Change-Id note for adopted commits:** Adopted commits may not have a `Change-Id:` trailer. In that case, Jul records the Change‑Id mapping only in `refs/notes/jul/meta` and does **not** rewrite the commit. If you want Change‑Id embedded in normal git commits, install a `commit-msg` hook that injects the trailer from the start.

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

Checkpoints (not yet promoted):
  abc123 (change Iab4f...) "feat: add JWT validation" ✓ CI passed
    └─ 1 suggestion pending

Promote target: main (3 checkpoints behind)
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
  "checkpoints": [...],
  "suggestions": [...],
  "promote_status": {
    "target": "main",
    "eligible": true,
    "checkpoints_ahead": 1
  }
}
```

#### `jul promote`

Promote checkpoints to a target branch.

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
Workspace '@' now tracking main (ghi789)
New draft started.
```

**Note:** Promoted history on `refs/heads/*` is **published commits** (normal Git commits), not checkpoints. A new Change‑Id starts for the next workspace draft after promote.

**Mapping rule:** `jul promote` records a mapping in `refs/notes/jul/meta`:
- `change_id → anchor_sha` (first checkpoint SHA, pinned while review open)
- `change_id → [checkpoint_shas...]` (the checkpoints being promoted)
- `change_id → promote_events[]` with:
  - `published_shas` (commits created on target branch)
  - `target`, `strategy`, `timestamp`
  - For merge: `merge_commit_sha` + `mainline` (for deterministic revert)

This makes `jul revert <change-id>` deterministic (revert the published SHAs from the last promote), and keeps review status tied to the latest checkpoint SHA.

Flags:
- `--to <branch>` — Target branch (required)
- `--squash` — Override strategy to squash
- `--rebase` — Override strategy to rebase
- `--merge` — Override strategy to merge
- `--force` — Skip policy checks
- `--auto` — Auto-checkpoint draft first if needed
- `--json` — JSON output

### 6.3 Workspace Commands

#### `jul ws new`

Create a named workspace.

```bash
$ jul ws new feature-auth
Created workspace 'feature-auth'
Draft abc123 started.
```

#### `jul ws stack`

Create a new workspace stacked on the current workspace’s **latest checkpoint** (not its draft).

```bash
$ jul ws stack feature-b
Created workspace 'feature-b' (stacked on feature-auth)
Draft def456 started.
```

Use this when you want dependent work that should review/land after the current workspace.

**V1 rule:** stacking requires a checkpoint. If the current workspace has no checkpoint yet, Jul asks you to checkpoint first.

**Restack (future):** If the parent workspace advances after stacking, you’ll need a manual `jul ws restack` to rebase the child onto the new parent checkpoint.

#### `jul ws switch`

Switch to another workspace.

```bash
$ jul ws switch feature-auth
Saving current workspace '@'...
  ✓ Working tree saved
  ✓ Staged changes saved
Restoring 'feature-auth'...
  ✓ Working tree restored
  ✓ workspace_lease updated
Switched to workspace 'feature-auth'
```

**What happens:**
1. Auto-saves current workspace (working tree + staging area) via `jul local save`
2. Syncs current draft to remote
3. Fetches target workspace's canonical state
4. Restores target workspace's saved state (working tree + staging area)
5. Updates `workspace_lease` for target workspace to the fetched canonical SHA

This makes "no dirty state concerns" actually true — your uncommitted work is preserved per-workspace.

#### `jul ws checkout`

Fetch and materialize a workspace's draft into the working tree. Establishes this device's baseline for future syncs.

```bash
$ jul ws checkout @
Fetching workspace '@'...
  ✓ Workspace ref: abc123
  ✓ Working tree updated
  ✓ Sync ref initialized
  ✓ workspace_lease set
```

**What happens:**
1. Fetch workspace ref from remote
2. Materialize working tree to match
3. Initialize this device's sync ref to the same commit
4. Set `workspace_lease` to the fetched SHA

This establishes the baseline: checkout sets up base + sync ref, so future `jul sync` commands know where they started.

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
  ✗ Checkpoint base diverged
  Your draft is based on checkpoint1, but workspace is now at checkpoint2.

# Future command to carry changes forward:
$ jul transplant
Rebasing draft from checkpoint1 onto checkpoint2...
  ⚠ Conflicts in src/auth.py
  
Run 'jul merge' to resolve.
```

**V1:** Not implemented. Use `jul ws checkout @` to start fresh, or manually cherry-pick from your sync ref.

### 6.4 Submit Command

#### `jul submit`

Create or update the **single** review for this workspace.

```bash
$ jul submit
Review updated for workspace 'feature-auth'
  Change-Id: Iab4f...
  Checkpoint: def456...
```

**Rules:**
- One workspace = one review (no review IDs).
- Uses the **latest checkpoint** for the workspace.
- Writes review state to `refs/notes/jul/review-state` (keyed by Change-Id anchor; stores latest checkpoint).
- Subsequent `jul submit` updates the same review.
- Optional: stacked workspaces include the parent workspace in review metadata.

If you don’t use reviews, skip `jul submit` entirely and go checkpoint → promote.

### 6.5 Suggestion Commands

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

If the base commit changed (amend **or** new checkpoint), stale suggestions are marked:

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

Resolve diverged state with remote. Agent handles conflicts automatically.

```bash
$ jul merge
Fetching remote...
Agent resolving conflicts...

Conflicts resolved:
  src/auth.py — combined both changes (local validation + remote caching)
  src/config.py — kept remote, applied local additions

Resolution ready as suggestion [01HX...].
Accept? [y/n] y

  ✓ Merged and synced
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

### 6.8 CI Command

#### `jul ci run`

Run CI and show results.

```bash
$ jul ci run
Running CI...
  ✓ lint: pass (1.2s)
  ✓ test: pass (8.4s) — 48/48
  ✓ coverage: 84%

All checks passed.
```

If tests fail:
```bash
$ jul ci run
Running CI...
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
$ jul ci run              # Run CI now, wait for results
$ jul ci run --watch      # Run CI now, stream output
$ jul ci run --target <rev>   # Attach results to a specific revision
$ jul ci run --change Iab4f3c2d...  # Attach results to latest checkpoint for a change
$ jul ci status       # Show latest results (don't re-run)
$ jul ci list         # List recent CI runs
$ jul ci config       # Show CI configuration
$ jul ci config --show  # Show resolved commands (file or inferred)
$ jul ci cancel       # Cancel in-progress background CI
```

**`jul ci status` reports current vs completed:**

```bash
$ jul ci status
CI Status:
  Current draft: def456
  Last completed: def456 ✓ (results current)
  
  ✓ lint: pass
  ✓ test: pass (48/48)
  ✓ coverage: 84%
```

If you've edited since the last CI run:

```bash
$ jul ci status
CI Status:
  Current draft: ghi789
  Last completed: def456 ⚠ (stale)
  ⚡ CI running for ghi789...
  
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

**Difference from background CI:**
- Background CI runs automatically on sync (non-blocking)
- `jul ci run` runs explicitly and waits for results (blocking)

Use `jul ci run` when you want to explicitly verify before checkpointing:
```bash
$ jul ci run && jul checkpoint   # Only checkpoint if CI passes
```

### 6.9 History and Diff Commands

#### `jul log`

Show checkpoint history.

```bash
$ jul log

def456 (change Iab4f...) (2h ago) "fix: null check on token"
        Author: george
        ✓ CI passed

abc123 (change Iab4f...) (4h ago) "feat: add JWT validation"
        Author: george
        ✓ CI passed, 1 suggestion

ghi789 (change Ief6a...) (1d ago) "initial project structure"
        Author: george
        ✓ CI passed
```

With trace history (provenance):
```bash
$ jul log --traces

def456 (change Iab4f...) (2h ago) "fix: null check on token"
        Author: george
        ✓ CI passed
  └── 1 trace:
      (sha:abc1) claude-code "fix the failing test" (auth.py)
          ✓ lint, ✓ typecheck

abc123 (change Iab4f...) (4h ago) "feat: add JWT validation"
        Author: george
        ✓ CI passed, 1 suggestion
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
- `--traces` — Show trace history (prompts, agents, per-trace CI)
- `--json` — JSON output

#### `jul diff`

Show diff between checkpoints or against draft.

```bash
# Diff current draft against base commit
$ jul diff

# Diff between two checkpoints
$ jul diff abc123 def456

# Diff specific checkpoint against its parent
$ jul diff abc123
```

Flags:
- `--stat` — Show diffstat only
- `--name-only` — Show changed filenames only
- `--json` — JSON output

#### `jul show`

Show details of a checkpoint or suggestion.

```bash
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

#### `jul blame`

Show line-by-line provenance: checkpoint, trace, prompt, agent.

**High-level algorithm:**
1. Use git blame to find the checkpoint commit that last touched each line.
2. Read `Trace-Head` / `Trace-Base` trailers from that checkpoint.
3. Walk the trace DAG between base→head to find the first trace commit whose tree introduces the line.
4. **Skip `trace_type=merge` nodes** for attribution (they’re connective, not edits).
5. Determinism: traverse in first‑parent order; if multiple candidates remain, pick the nearest non‑merge ancestor (shortest path).

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
      │ CI at trace: ✓ lint, ✓ typecheck
```

Line range:
```bash
$ jul blame src/auth.py:40-50
```

Flags:
- `--prompts` — Show prompts/summaries that led to each line
- `--local` — Include full prompt text from local storage
- `--verbose` — Show full context (session, CI state)
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
      "prompt_hash": "sha256:abc123...",
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

Show workspace history (including draft syncs).

```bash
$ jul reflog --limit=10

def456 checkpoint "fix: null check" (2h ago)
abc123 checkpoint "feat: add JWT validation" (4h ago)
        └─ draft sync (4h ago)
        └─ draft sync (5h ago)
ghi789 checkpoint "initial structure" (1d ago)
```

### 6.10 Local Workspaces (Client-Side)

Local workspaces enable instant context switching for uncommitted work.

**Note**: This is separate from server workspaces. It's a client-only feature for managing local state.

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

[sync]
mode = "on-command"              # on-command | continuous | explicit

[checkpoint]
auto_message = true              # Agent generates message

[promote]
default_target = "main"
strategy = "rebase"              # rebase | squash | merge

[ci]
run_on_checkpoint = true         # Always run CI on checkpoint
run_on_draft = true              # Run CI on draft sync (background)
draft_ci_blocking = false        # Draft CI doesn't block sync

[review]
enabled = true
run_on_checkpoint = true
min_confidence = 70

[traces]
sync_prompt_hash = true          # Always sync (cannot leak)
sync_prompt_summary = false      # Summaries stay local by default
sync_prompt_full = false         # Full text stays local by default
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
name = "origin"                  # Git remote to use for sync
                                 # Default: "origin" if exists, else only remote, else none

[workspace]
name = "feature-auth"            # Override default workspace name

[ci]
# Agent-assisted CI setup (future)
# First checkpoint without config triggers setup wizard
```

**Remote selection (auto-detected on `jul init`):**
1. If `origin` exists → use it
2. If no `origin` but exactly one remote → use that
3. If multiple remotes and no `origin` → must set explicitly via `jul remote set`
4. If no remotes → work locally (no `[remote]` section)

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
│              │ CI results       │                  │ Apply fix      │   │
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

# CI passes, promote
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
     "base_sha": "abc123...",
     "commit": "def456...",
     "status": "pending",
     "reason": "potential_null_check",
     "confidence": 0.92
   }
   ```

The `base_sha` tracks which exact checkpoint SHA the suggestion was created against. If the base commit changes (amend or new checkpoint), the suggestion becomes stale. If no checkpoint exists yet and review was run on a draft, `base_sha` may equal the **current draft SHA**.

#### 8.2.3 Applying Suggestions

When user runs `jul apply 01HX7Y9A`:

1. **Check staleness**: Compare suggestion's `base_sha` with `parent(current_draft)` (current base commit)
   - If match: proceed
   - If no checkpoint exists yet and `base_sha == current_draft`, treat as fresh
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
| `setup_ci` | First checkpoint (no config) | No | Yes | Auto-configure CI |

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
  CI failed with: $FAILURES. Create fixes in this directory." -f json

# Claude Code (if configured)
claude -p "Review this code for bugs. CI failed with: $FAILURES. \
  Create fixes in this directory." \
  --output-format json \
  --permission-mode acceptEdits \
  --allowedTools "Read,Write,Edit,Bash(npm test)"
```

### 8.6 CI Auto-Setup

When no CI configuration exists, the agent proposes one:

```bash
$ jul checkpoint
No CI configuration found.
Agent analyzing project...

Detected: Python 3.11, pytest, ruff
Proposed CI config:

  [ci]
  lint = "ruff check ."
  test = "pytest"
  coverage = "pytest --cov --cov-report=json"

Accept? [y/n/edit] y

  ✓ CI configuration saved to .jul/ci.toml
  ✓ Running CI...
```

**Jul's CI is for fast local feedback**, separate from project CI (GitHub Actions, etc.):

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
Detected CI from pyproject.toml:
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
$ jul init my-project --create-remote

# Daily work
$ # ... edit files ...
$ jul checkpoint
  "feat: add user authentication"
  ✓ CI passed
  ⚠ 1 suggestion

$ jul apply 01HX7Y9A --checkpoint
  "fix: add null check"

$ jul promote --to main
  Promoted 2 checkpoints
```

### 9.2 Git + Jul Flow

```bash
# Setup
$ git init && jul init --server https://jul.example.com
$ jul hooks install

# Daily work (normal git)
$ git add . && git commit -m "feat: add auth"
  [jul] synced, CI queued

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

Jul works with any git remote that accepts **custom refs** and **non‑fast‑forward updates** to `refs/jul/*`. If a host rejects those, Jul degrades to **local‑only** for that namespace.

### 10.1 Any Git Remote (GitHub, GitLab, etc.)

```bash
$ jul init myproject
$ git remote add origin git@github.com:george/myproject.git
$ jul sync
```

**What works:**
- Draft/checkpoint sync (via refs)
- Notes sync (with explicit refspecs)
- Suggestions (stored as commits)
- Everything local (CI, review, attestations)

**Limitations depend on host:**
- Some hosts may reject custom refs
- Some hosts may reject non‑FF updates on `refs/jul/*`
- Some hosts may GC unreachable commits
- Size limits vary

Use `jul doctor` to check what's syncing (custom refs + non‑FF support).  
**Probe strategy:** Jul can push a temporary ref under `refs/jul/doctor/<device>`, attempt a non‑FF update, and then delete it. This makes compatibility checks explicit rather than assumed.

### 10.2 Jul-Optimized Server (Future)

A git server optimized for Jul compatibility would provide:

- Guaranteed ref acceptance (all jul/* refs)
- Keep-ref retention (no premature GC)
- Optimized for continuous sync patterns
- Optional: server-side indexing for fast queries

This is future work. For v1, any git remote works.

---

## 11. Glossary

| Term | Definition |
|------|------------|
| **Agent Workspace** | Isolated git worktree (`.jul/agent-workspace/worktree/`) where internal agent works |
| **Attestation** | CI/test/coverage results attached to a commit (trace, draft, checkpoint, or published) |
| **Auto-merge** | 3-way merge producing single-parent draft commit (NOT a 2-parent merge commit) |
| **Change-Id** | Stable identifier (`Iab4f...`) created at the first draft; new Change-Id starts after promote |
| **Change Anchor SHA** | The first checkpoint SHA of a Change-Id; fixed lookup key for review-state/metadata even if that checkpoint is amended |
| **Base Commit** | Parent of the current draft (latest checkpoint or latest published commit) |
| **Checkpoint** | Locked unit of work with message, Change-Id, and trace_base/trace_head refs |
| **Base Divergence** | When one device advanced the base while another has a draft on the old base |
| **Checkpoint Flush** | Rule that `jul checkpoint` must create final trace so trace_head tree = checkpoint tree |
| **CI Coalescing** | Only latest draft SHA runs CI; older runs cancelled/ignored |
| **Device ID** | Random word pair (e.g., "swift-tiger") identifying this machine |
| **Draft** | Ephemeral commit snapshotting working tree (parent = base commit) |
| **Draft Attestation** | Device-local CI results for current draft (ephemeral, not synced) |
| **External Agent** | Coding agent (Claude Code, Codex) that uses Jul for feedback |
| **Harness Integration** | Agent harness calls `jul trace --prompt "..."` to attach rich provenance |
| **Headless Mode** | Non-interactive agent invocation for automation |
| **Internal Agent** | Configured provider (OpenCode bundled) that runs reviews/merge resolution |
| **jul blame** | Command showing line-by-line provenance: checkpoint → trace → prompt → agent |
| **Keep-ref** | Ref that anchors a checkpoint for retention |
| **Local Workspace** | Client-side saved state for fast context switching |
| **Merge** | Agent-assisted resolution when sync has actual conflicts |
| **Promote** | Move checkpoints to a target branch (main) |
| **Prompt Hash** | SHA-256 hash of prompt text (always synced, cannot leak secrets) |
| **Prompt Summary** | AI-generated summary of prompt (local-only by default, opt-in sync with scrubbing) |
| **Secret Scrubber** | Pre-sync filter that detects API keys, passwords, tokens in summaries |
| **Session Summary** | AI-generated summary of multi-turn conversation that produced a checkpoint |
| **Shadow Index** | Separate index file so Jul doesn't interfere with git staging |
| **Side History** | Trace refs stored separately from main commit ancestry (for provenance without pollution) |
| **Stale Suggestion** | Suggestion created against an old base commit (base changed due to amend or new checkpoint) |
| **Suggestion** | Agent-proposed fix tied to a Change-Id and base SHA, with apply/reject lifecycle |
| **Suggestion Base SHA** | The exact checkpoint SHA a suggestion was created against |
| **Sync** | Fetch, push to sync ref, auto-merge if no conflicts, defer if conflicts |
| **Sync Ref** | Device's backup stream (`refs/jul/sync/<user>/<device>/...`) |
| **Trace** | Fine-grained provenance unit with prompt/agent/session metadata (side history), keyed by SHA |
| **Trace Attestation** | Lightweight CI results (lint, typecheck) attached to a trace |
| **Trace Merge** | Merge commit in trace side-history with two parents; uses strategy `ours` for tree |
| **Trace Sync Ref** | Device's trace backup (`refs/jul/trace-sync/<user>/<device>/...`), always pushes |
| **trace_base** | Checkpoint metadata: previous checkpoint's trace tip SHA (or null) |
| **trace_head** | Checkpoint metadata: current trace tip SHA |
| **Trace Tip** | Canonical trace ref (`refs/jul/traces/<user>/<ws>`), advances with workspace |
| **Transplant** | (Future) Rebase draft from one base commit to another |
| **Workspace** | Named stream of work (replaces branches) |
| **Workspace Lease** | Per-workspace file (`.jul/workspaces/<ws>/lease`) tracking last merged SHA |
| **Workspace Ref** | Canonical state (`refs/jul/workspaces/...`) — shared across devices |

**Note:** "Trace ID" (e.g., "t1", "t2") is display-only for human readability. Internally, everything is keyed by trace commit SHA.

---

## Appendix A: Why Not Just Use X?

| Alternative | Why Jul is different |
|-------------|---------------------|
| **GitHub/GitLab** | No continuous sync, no checkpoint model, no agent feedback loop |
| **Gerrit** | Change-centric but complex, not agent-native |
| **JJ** | Great local UX but no built-in CI/review/suggestions |
| **Git + hooks** | No rich metadata, no suggestions, no agent integration |

Jul = Git + continuous sync + checkpoints + local CI/review + agent-native feedback loop.

---

## Appendix B: Migration from Git Workflow

| Git habit | Jul equivalent |
|-----------|----------------|
| `git checkout -b feature` | `jul ws new feature` |
| `git add . && git commit` | `jul checkpoint` |
| `git commit --amend` | `jul checkpoint --amend` |
| `git push` | `jul sync` (automatic in on-command mode) |
| `git pull` | `jul sync` (includes fetch) |
| `git fetch` | Included in `jul sync` |
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

## Appendix C: References

- [git-http-backend](https://git-scm.com/docs/git-http-backend) — Smart HTTP server
- [JJ (Jujutsu)](https://github.com/jj-vcs/jj) — Inspiration for working-copy model
- [Change-Id trailer format reference](https://gerrit-review.googlesource.com/Documentation/user-changeid.html)
