# 줄 Jul: AI-First Git Workflow

**Version**: 0.3 (Draft)
**Status**: Design Specification

---

## 0. What Jul Is

Jul is **Git with a built-in agent**. It's a local CLI tool that adds:

- **Rich metadata** on every checkpoint (CI results, coverage, lint, prompts)
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
- A special server (any git remote works: GitHub, GitLab, etc.)
- A CI service (tests run locally)
- A remote execution platform (agents run locally)

**Metadata travels with Git** via refs and notes — on hosts that accept custom refs. See Section 5.7 for portability details.

---

## 1. Goals and Non-Goals

### 1.1 Goals

- **Local-first**: Everything runs on your machine
- **Continuous sync**: Every change pushed to git remote automatically
- **Checkpoint model**: Lock work, agent generates message, run CI, get suggestions
- **Agent-native feedback**: Rich JSON responses for agents to act on
- **Workspaces over branches**: Named streams of work
- **Rich metadata**: CI/coverage/lint/prompts attached to checkpoints
- **Git compatibility**: Any git remote works (GitHub, GitLab, etc.)
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
| **Workspace Base** | Per-workspace file (`.jul/workspaces/<ws>/base`) — the semantic lease |
| **Sync Ref** | Device backup (`refs/jul/sync/<user>/<device>/...`) — always pushes |
| **Draft** | Ephemeral commit snapshotting working tree (parent = last checkpoint) |
| **Checkpoint** | A locked unit of work with Change-Id and generated message |
| **Change-Id** | Stable identifier that survives amend/rebase (`Iab4f3c2d...`) |
| **Attestation** | CI/test/coverage results attached to a checkpoint |
| **Suggestion** | Agent-proposed fix targeting a checkpoint |
| **Local Workspace** | Client-side saved state for fast context switching |

### 2.2 The Draft → Checkpoint → Promote Model

Jul uses a three-stage model:

```
┌─────────────────────────────────────────────────────────────────────────┐
│  DRAFT                                                                  │
│    • Shadow snapshot of your working tree                               │
│    • Continuously updated (every save)                                  │
│    • Synced to remote automatically                                     │
│    • Has a Change-Id from creation                                      │
│    • No commit message yet                                              │
├─────────────────────────────────────────────────────────────────────────┤
│                           jul checkpoint                                │
├─────────────────────────────────────────────────────────────────────────┤
│  CHECKPOINT                                                             │
│    • Locked, immutable                                                  │
│    • Agent generates commit message (or user provides with -m)          │
│    • Optional: prompt that led to this checkpoint                       │
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

**Key insight**: Your working tree can still be "dirty" relative to HEAD (normal git). But Jul continuously snapshots your dirty state as a draft commit and syncs it. You can always recover. The draft is your safety net, not your workspace. `jul checkpoint` is when you say "this is a logical unit." Checkpoints are real git commits, but they do **not** move `refs/heads/*`. Only `jul promote` updates branches.

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

#### 2.5.1 Full Jul Mode

Jul is your primary interface.

```bash
$ jul configure                         # One-time setup
$ jul init my-project --create-remote   # Create repo + server remote (optional)
# ... edit ...
$ jul checkpoint                        # Lock + message + CI + review
$ jul promote --to main                 # Publish
```

#### 2.5.2 Git + Jul (Invisible Infrastructure)

Git is your porcelain. Jul can sync in background via hooks when a remote is configured.

```bash
$ git init && jul init --server https://jul.example.com
$ jul hooks install
# ... use normal git commands ...
# post-commit hook auto-syncs
$ jul status                            # Check attestations
$ jul promote --to main                 # When ready
```

#### 2.5.3 JJ + Jul

JJ handles local workflow. Jul handles optional remote sync/policy.

```bash
$ jj git init --colocate
$ jul init --server https://jul.example.com
$ jul sync --daemon &                   # Background sync
# ... use jj commands ...
$ jul promote --to main
```

#### 2.5.4 Agent Mode

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
checkpoint abc123 (Iab4f...)
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

**Staleness:** If you amend the checkpoint (same Change-Id, new SHA), existing suggestions become stale:

```
checkpoint abc123 (Iab4f...)
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
- `pending` → `stale` (checkpoint amended)
- `stale` → stays stale (must run fresh review)

**Result**: Clean history with your work and agent fixes as separate checkpoints.

```
main:
  Iab4f... "feat: add auth"              ← your work
  Icd5e... "fix: null check"             ← agent fix  
  Ief6a... "feat: add refresh tokens"    ← your work
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

### 3.4 Suggestion Refs

```
refs/jul/suggest/<change_id>/<suggestion_id>
```

- Points to suggested commit — the actual code changes
- Tied to a Change-Id (checkpoint) AND a specific base SHA
- Immutable once created
- Can be fetched, inspected, cherry-picked
- Metadata (reasoning, confidence, base_sha) stored in notes

**Staleness:** If the checkpoint is amended (same Change-Id, new SHA), existing suggestions become stale because they were created against the old SHA.

**Cleanup:** Suggestion refs are deleted when their parent checkpoint's keep-ref expires. This prevents ref accumulation:

```
refs/jul/keep/george/@/Iab4f.../abc123  expires
    → delete refs/jul/suggest/Iab4f.../*
    → delete associated notes
```

Without this, suggestion refs would accumulate forever even after their checkpoints are GC'd.

### 3.5 Keep Refs

```
refs/jul/keep/<workspace>/<change_id>/<sha>
```

Anchors checkpoints for retention/fetchability. Without a ref, git may GC unreachable commits.

### 3.6 Notes Namespaces

**Synced notes (pushed to remote):**
```
refs/notes/jul/attestations/checkpoint   # Checkpoint CI results (keyed by SHA)
refs/notes/jul/attestations/published    # Published CI results (keyed by SHA)
refs/notes/jul/review                    # Review comments  
refs/notes/jul/meta                      # Change-Id mappings
refs/notes/jul/suggestions               # Suggestion metadata
refs/notes/jul/prompts                   # Optional: prompts that led to checkpoints
```

**Local-only storage (not synced):**
```
.jul/ci/                  # Draft attestations (device-scoped, ephemeral)
.jul/workspaces/<ws>/     # Per-workspace tracking (workspace_base)
.jul/local/               # Saved local workspace states
```

Notes are pushed with explicit refspecs. Draft attestations are local-only by default to avoid multi-device write contention.

### 3.7 Complete Ref Layout

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
│   ├── sync/                        # Device backups (per-device)
│   │   └── <user>/
│   │       └── <device>/
│   │           ├── @
│   │           └── <named>
│   ├── suggest/
│   │   └── <change_id>/
│   │       └── <suggestion_id>
│   └── keep/
│       └── <workspace>/
│           └── <change_id>/
│               └── <sha>
└── notes/jul/
    ├── attestations
    ├── review
    ├── meta
    ├── suggestions
    └── prompts
```

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
- A placeholder message (e.g., `[draft] Iab4f3c2d`)
- A stable Change-Id in the message trailer
- Parent = last checkpoint (always)
- Always pointed to by this device's sync ref
- Pointed to by workspace ref only when canonical (not diverged)

```
commit abc123
Author: george <george@example.com>
Date:   Mon Jan 19 15:30:00 2026

    [draft] Work in progress
    
    Change-Id: Iab4f3c2d1e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b
```

**Each sync creates a NEW draft commit:**
- Same parent (last checkpoint)
- New tree (current working directory state)
- Force-updates sync ref
- Old draft becomes unreachable (ephemeral)

This avoids "infinite WIP commit chain" — there's always exactly one draft commit per workspace, with parent = last checkpoint. Drafts are siblings (same parent), not ancestors of each other.

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
# Create commit from tree with parent = last checkpoint
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
| Parent | Last checkpoint | Last checkpoint |
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
.jul/workspaces/@/base              ← SHA of last workspace state we merged
.jul/workspaces/feature-auth/base  ← Same, for feature-auth workspace
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
3. **Compare** `workspace_remote` to local `workspace_base` (per-workspace)
4. **If** `workspace_remote == workspace_base`:
   - Not diverged, safe to update
   - Update workspace ref with `--force-with-lease=<workspace_ref>:<workspace_remote>`
   - Set `workspace_base = new_sha`
5. **If** `workspace_remote != workspace_base`:
   - Another device pushed since we last merged
   - **Try auto-merge:**
     - Merge base = merge-base of the two draft commits (typically the shared checkpoint)
     - 3-way merge: merge_base ↔ workspace_remote (theirs) ↔ sync (ours)
   - **If no conflicts**: 
     - Update workspace ref with `--force-with-lease=<workspace_ref>:<workspace_remote>`
     - If lease fails, re-fetch and retry (or fall back to "conflicts pending")
     - Set `workspace_base = new_sha`
   - **If conflicts**: mark diverged, defer to `jul merge`

**Why workspace_base matters:** It's the semantic lease — it tracks the last workspace state we incorporated, so we know when we're behind.

**Why lease against workspace_remote:** When updating workspace ref after auto-merge, we guard against "someone else pushed while we were merging." If the lease fails, we re-fetch and try again.

**Why merge-base of drafts:** Since drafts have parent = last checkpoint, the merge-base is the shared checkpoint. This avoids relying on old ephemeral draft commits.

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

**Important:** Auto-merge produces a **new draft commit with single parent** (the checkpoint), NOT a 2-parent merge commit. Jul uses `git merge-tree` or equivalent to compute the merged tree, then creates a new draft commit:

```
parent = checkpoint (single parent, preserving "draft parent = checkpoint" invariant)
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

**Failure mode:** What if Device A checkpointed while Device B has a local draft based on the OLD checkpoint?

```
Device A: checkpoint1 → checkpoint2 (pushed)
Device B: checkpoint1 → draft (still on old base)
```

This is different from normal divergence (both on same checkpoint, different drafts). Here, the checkpoint histories have forked.

**Detection:** When auto-merge runs, the merge-base of the two drafts will be `checkpoint1`, not `checkpoint2`. If Device B's draft parent ≠ `checkpoint2`, we've got checkpoint base divergence.

**V1 behavior:** Fail with clear error:

```bash
$ jul sync
Syncing...
  ✓ Pushed to sync ref (your work is safe!)
  ✗ Checkpoint base diverged
  
Your draft is based on checkpoint1, but workspace is now at checkpoint2.
Your work is safe on your sync ref.

Options:
  jul ws checkout @     # Discard local changes, start fresh from checkpoint2
  jul transplant        # (future) Rebase your draft onto checkpoint2
```

**Why not auto-fix?** Transplanting a draft from one checkpoint base to another is a rebase operation that can have complex conflicts. V1 takes the safe path: your work is preserved on sync ref, but you must explicitly decide how to proceed.

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
  ✓ workspace_base updated
```

**The merge algorithm:**
1. Merge base = merge-base of workspace ref and sync ref (the shared checkpoint)
2. 3-way merge: merge_base ↔ workspace (theirs) ↔ sync (ours)
3. Agent resolves conflicts automatically
4. Create resolution as suggestion
5. If accepted: new draft commit (parent = last checkpoint, NOT a 2-parent merge)
6. Update workspace ref, sync ref, AND `workspace_base`

**V1 constraint:** Both sides must share the same checkpoint base. If checkpoint histories have diverged, manual intervention required.

**The invariants:**
- Sync ref = this device's backup (always pushes, device-scoped)
- Workspace ref = canonical state (shared across devices)
- `workspace_base` = last workspace SHA we incorporated (per-workspace)
- Auto-merge produces single-parent commit (parent = checkpoint), not 2-parent merge
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

Or when checkpoint bases have diverged:

```json
{
  "sync": {
    "status": "checkpoint_base_diverged",
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
$ jul ws checkout @           # Establishes baseline: sync ref + workspace_base
```

The checkout establishes your baseline: it sets `workspace_base` and initializes your sync ref. Now `jul sync` knows where you started.

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
refs/jul/keep/<workspace>/<change-id>/<checkpoint-sha>
```

Example:
```
refs/jul/keep/george/@/Iab4f3c2d/abc123
refs/jul/keep/george/@/Iab4f3c2d/def456   # Amended checkpoint
refs/jul/keep/george/feature/Icd5e6f7a/ghi789
```

**Lifecycle:**
- Created when checkpoint is locked
- TTL-based expiration (configurable, default 90 days)
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

### 5.6 Three Classes of Attestations

**Problem:** Rebase/squash changes SHAs. An attestation for checkpoint `abc123` doesn't apply to the rebased commit `xyz789` on main.

**Solution: Separate attestations by lifecycle.**

| Attestation Type | Attached To | Scope | Purpose |
|------------------|-------------|-------|---------|
| **Draft** | Current draft SHA | Device-local | Continuous feedback, ephemeral |
| **Checkpoint** | Original checkpoint SHA | Synced | Pre-integration CI, review |
| **Published** | Post-rebase SHA on target | Synced | Final verification on main |

**Draft attestations (continuous CI):**

By default, CI runs in the background every time the workspace ref updates:

```bash
$ jul sync
Syncing...
  ✓ Workspace ref updated
  ⚡ CI running in background...

# Later, or immediately if fast:
$ jul status
Draft Iab4f... (2 files changed)
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
- **Foreground CI (`jul ci` / `jul ci watch`)**: runs immediately and streams output; results are recorded for the target SHA.
- **Checkpoint CI (`jul checkpoint`)**: runs after a checkpoint and writes an attestation note for that checkpoint.
- **Manual CI (`jul ci --target/--change`)**: attaches to the requested revision; does not replace draft CI unless it targets the draft SHA.

**Multiple runs:** Draft CI is single‑flight per device (latest draft wins). Manual/foreground runs can be started while draft CI is idle, but if they target the draft SHA they will supersede the previous draft result.

```bash
$ jul status
Draft Iab4f... (3 files changed)
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
checkpoint Iab4f... (sha: abc123)
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
  "checkpoints": [
    {"sha": "abc123", "message": "feat: add auth"},
    {"sha": "def456", "message": "feat: add auth (amended)"}
  ],
  "published": [
    {"sha": "xyz789", "target": "main", "strategy": "rebase"}
  ]
}
```

### 5.7 Notes: Storage, Sync, and Portability

**Metadata travels with git... mostly.**

Jul stores all metadata as git objects (notes, refs). This means it *can* be synced via git push/pull. However:

- Different hosts have different ref policies (some block custom refs)
- Size limits vary (GitHub has push size limits)
- Retention varies (some hosts GC aggressively)

**The right expectation:** Jul metadata syncs on hosts that allow it. If a host blocks some refs, Jul degrades gracefully to local-only for those namespaces. Jul config sets up the refspecs; check `jul doctor` to see what's actually syncing.

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

**Single-writer rule per namespace:**

Notes refs can have non-fast-forward conflicts. Avoid with clear ownership:

| Namespace | Writer | Content |
|-----------|--------|---------|
| `refs/notes/jul/meta` | Client | Change-Id mappings |
| `refs/notes/jul/attestations` | CI runner | Test results (summaries only) |
| `refs/notes/jul/review` | Review agent | Review comments |
| `refs/notes/jul/suggestions` | Review agent | Suggestion metadata |
| `refs/notes/jul/prompts` | Client | Prompt metadata |

**Suggestions storage:**

Suggestions have two parts:
- **Patch commits**: `refs/jul/suggest/<change_id>/<suggestion_id>` — actual code
- **Metadata**: `refs/notes/jul/suggestions` — reasoning, confidence, status

Commits carry the heavy diffs; notes stay small.

**Prompts privacy:**

Prompts often contain secrets. By default, prompts are local-only:

```toml
[prompts]
storage = "local"   # local | sync
```

`local`: stored in notes but never pushed
`sync`: pushed to remote (opt-in only)

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
           │                           Change-Id: Icd5e...
           │
    (parent chain)
           │
           ▼
      earlier commits
```

**Ref purposes:**
- `refs/heads/*` — Promote targets (main, staging)
- `refs/jul/workspaces/<user>/<ws>` — Canonical draft (shared across devices)
- `refs/jul/sync/<user>/<device>/<ws>` — This device's backup (never clobbered)
- `refs/jul/keep/*` — Checkpoint retention anchors
- `refs/jul/suggest/*` — Suggestion patch commits
- `refs/notes/jul/*` — Metadata (attestations, review, suggestions, prompts)

**Local state (per workspace):**
- `.jul/workspaces/<ws>/base` — SHA of last workspace state we merged (the semantic lease)

**Invariants:**
- `workspace_remote == workspace_base` → not diverged, update workspace directly
- `workspace_remote != workspace_base` → try auto-merge; only defer if actual conflicts
- `jul ws checkout` establishes baseline (sync ref + workspace_base)
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
  ✓ workspace_base updated

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

#### `jul merge`

Resolve diverged state. Agent handles conflicts automatically.

```bash
$ jul merge
Agent resolving conflicts...

Conflicts resolved:
  src/auth.py — combined both changes

Resolution ready as suggestion [01HX...].
Accept? [y/n] y

  ✓ Merged
  ✓ Workspace ref updated
  ✓ workspace_base updated
```

Flags:
- `--json` — JSON output

#### `jul checkpoint`

Lock current draft, generate message, start new draft.

```bash
$ jul checkpoint
Locking draft Iab4f...

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

Checkpoint Iab4f... locked.
New draft Icd5e... started.
```

Flags:
- `-m "message"` — Provide message (skip agent)
- `--amend` — Amend previous checkpoint instead of creating new one
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

#### `jul status`

Show current workspace status.

```bash
$ jul status

Workspace: @ (default)
Draft: Icd5e... (2 files changed)

Checkpoints (not yet promoted):
  Iab4f... "feat: add JWT validation" ✓ CI passed
    └─ 1 suggestion pending

Promote target: main (3 checkpoints behind)
```

With `--json` for agents:
```json
{
  "workspace": "@",
  "draft": {
    "change_id": "Icd5e...",
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
  Iab4f... "feat: add JWT validation"
  Icd5e... "fix: null check on token"

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
Draft Ief6a... started.
```

#### `jul ws switch`

Switch to another workspace.

```bash
$ jul ws switch feature-auth
Saving current workspace '@'...
  ✓ Working tree saved
  ✓ Staged changes saved
Restoring 'feature-auth'...
  ✓ Working tree restored
  ✓ workspace_base updated
Switched to workspace 'feature-auth'
```

**What happens:**
1. Auto-saves current workspace (working tree + staging area) via `jul local save`
2. Syncs current draft to remote
3. Fetches target workspace's canonical state
4. Restores target workspace's saved state (working tree + staging area)
5. Updates `workspace_base` for target workspace to the fetched canonical SHA

This makes "no dirty state concerns" actually true — your uncommitted work is preserved per-workspace.

#### `jul ws checkout`

Fetch and materialize a workspace's draft into the working tree. Establishes this device's baseline for future syncs.

```bash
$ jul ws checkout @
Fetching workspace '@'...
  ✓ Workspace ref: abc123
  ✓ Working tree updated
  ✓ Sync ref initialized
  ✓ workspace_base set
```

**What happens:**
1. Fetch workspace ref from remote
2. Materialize working tree to match
3. Initialize this device's sync ref to the same commit
4. Set `workspace_base` to the fetched SHA

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
* @ (default)           Icd5e... (2 files changed)
  feature-auth          Ief6a... (clean)
  bugfix-123            Igh7b... (5 files changed)
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

Rebase a draft from one checkpoint base to another. Used when checkpoint bases have diverged.

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

### 6.4 Suggestion Commands

#### `jul suggestions`

List pending suggestions for current checkpoint.

```bash
$ jul suggestions

Pending for Iab4f... (abc123) "feat: add JWT validation":

  [01HX7Y9A] potential_null_check (92%) ✓
             src/auth.py:42 - Missing null check on token
             
  [01HX7Y9B] test_coverage (78%) ✓
             src/auth.py:67-73 - Uncovered error path

Actions:
  jul show <id>      Show diff
  jul apply <id>     Apply to draft
  jul reject <id>    Reject
```

If checkpoint was amended, stale suggestions are marked:

```bash
$ jul suggestions

Pending for Iab4f... (def456) "feat: add JWT validation":

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
Checkpoint: Iab4f... "feat: add JWT validation"
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
Applied and checkpointed as Icd5e... "fix: add null check for auth token"
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

### 6.5 Review Command

#### `jul review`

Manually trigger review on current draft.

```bash
$ jul review
Running review on draft Icd5e...
  Analyzing 3 changed files...
  
  ⚠ 1 suggestion created
  
Run 'jul suggestions' to see details.
```

Useful before checkpoint to catch issues early.

### 6.6 Merge Command

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

### 6.7 CI Command

#### `jul ci`

Run CI and show results.

```bash
$ jul ci
Running CI...
  ✓ lint: pass (1.2s)
  ✓ test: pass (8.4s) — 48/48
  ✓ coverage: 84%

All checks passed.
```

If tests fail:
```bash
$ jul ci
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
$ jul ci              # Run CI now, wait for results
$ jul ci --target <rev>   # Attach results to a specific revision
$ jul ci --change Iab4f3c2d...  # Attach results to latest checkpoint for a change
$ jul ci status       # Show latest results (don't re-run)
$ jul ci watch        # Run and stream output
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
$ jul ci --json
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
- `jul ci` runs explicitly and waits for results (blocking)

Use `jul ci` when you want to explicitly verify before checkpointing:
```bash
$ jul ci && jul checkpoint   # Only checkpoint if CI passes
```

### 6.8 History and Diff Commands

#### `jul log`

Show checkpoint history.

```bash
$ jul log

Icd5e... (2h ago) "fix: null check on token"
        Author: george
        ✓ CI passed

Iab4f... (4h ago) "feat: add JWT validation"
        Author: george
        ✓ CI passed, 1 suggestion

Ief6a... (1d ago) "initial project structure"
        Author: george
        ✓ CI passed
```

Flags:
- `--limit <n>` — Show last n checkpoints
- `--change-id <id>` — Filter by Change-Id
- `--json` — JSON output

#### `jul diff`

Show diff between checkpoints or against draft.

```bash
# Diff current draft against last checkpoint
$ jul diff

# Diff between two checkpoints
$ jul diff Iab4f... Icd5e...

# Diff specific checkpoint against its parent
$ jul diff Iab4f...
```

Flags:
- `--stat` — Show diffstat only
- `--name-only` — Show changed filenames only
- `--json` — JSON output

#### `jul show`

Show details of a checkpoint or suggestion.

```bash
$ jul show Iab4f...

Checkpoint: Iab4f...
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

#### `jul query`

Query checkpoints by criteria.

```bash
$ jul query --test=pass --coverage-min=80 --limit=5

Iab4f... (2h ago) "feat: add JWT validation"
        ✓ tests, 84% coverage
        
Icd5e... (1d ago) "refactor: extract auth utils"
        ✓ tests, 82% coverage
```

#### `jul reflog`

Show workspace history (including draft syncs).

```bash
$ jul reflog --limit=10

Icd5e... checkpoint "fix: null check" (2h ago)
Iab4f... checkpoint "feat: add JWT validation" (4h ago)
        └─ draft sync (4h ago)
        └─ draft sync (5h ago)
Ief6a... checkpoint "initial structure" (1d ago)
```

### 6.9 Local Workspaces (Client-Side)

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

[prompts]
storage = "local"                # local | sync
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
headless = "opencode run --format json --file $ATTACHMENT $PROMPT"

[providers.claude-code]
command = "claude"
protocol = "jul-agent-v1"
headless = "claude -p $PROMPT --output-format json --permission-mode acceptEdits"

[providers.codex]
command = "codex"
protocol = "jul-agent-v1"
headless = "codex exec --output-format json --full-auto $PROMPT"

[providers.codex.actions.review]
headless = "codex exec \"/review $PROMPT\" --output-format json --full-auto"
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
        jul("checkpoint")
    else:
        # No suggestions, agent needs to fix manually
        failures = response["ci"]["signals"]["test"]["failures"]
        fix_failures(failures)
        jul("checkpoint")
    
    response = jul("status")

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

The `base_sha` tracks which exact checkpoint SHA the suggestion was created against. If the checkpoint is amended (same Change-Id, new SHA), the suggestion becomes stale.

#### 8.2.3 Applying Suggestions

When user runs `jul apply 01HX7Y9A`:

1. **Check staleness**: Compare suggestion's `base_sha` with current checkpoint SHA
   - If match: proceed
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
    "checkpoint": "Iab4f3c2d...",
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
headless = "opencode run --format json --file $ATTACHMENT $PROMPT"
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

[providers.codex.actions.review]
headless = "codex exec \"/review $PROMPT\" --output-format json --full-auto"

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
$ jul merge --json
$ jul checkpoint --json  
$ jul log --json
$ jul diff --json
$ jul show <id> --json
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

Jul works with any git remote. However, some features work better with a Jul-optimized server.

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
- Some hosts may GC unreachable commits
- Size limits vary

Use `jul doctor` to check what's syncing.

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
| **Attestation** | CI/test/coverage results attached to a commit (draft, checkpoint, or published) |
| **Auto-merge** | 3-way merge producing single-parent draft commit (NOT a 2-parent merge commit) |
| **Change-Id** | Stable identifier (`Iab4f...`) for a checkpoint |
| **Checkpoint** | Locked unit of work with message and Change-Id |
| **Checkpoint Base Divergence** | When one device checkpointed while another has draft on old base |
| **CI Coalescing** | Only latest draft SHA runs CI; older runs cancelled/ignored |
| **Device ID** | Random word pair (e.g., "swift-tiger") identifying this machine |
| **Draft** | Ephemeral commit snapshotting working tree (parent = last checkpoint) |
| **Draft Attestation** | Device-local CI results for current draft (ephemeral, not synced) |
| **External Agent** | Coding agent (Claude Code, Codex) that uses Jul for feedback |
| **Headless Mode** | Non-interactive agent invocation for automation |
| **Internal Agent** | Configured provider (OpenCode bundled) that runs reviews/merge resolution |
| **Keep-ref** | Ref that anchors a checkpoint for retention |
| **Local Workspace** | Client-side saved state for fast context switching |
| **Merge** | Agent-assisted resolution when sync has actual conflicts |
| **Promote** | Move checkpoints to a target branch (main) |
| **Prompt** | Optional metadata: the instruction that led to a checkpoint |
| **Shadow Index** | Separate index file so Jul doesn't interfere with git staging |
| **Stale Suggestion** | Suggestion created against an old checkpoint SHA (checkpoint was amended) |
| **Suggestion** | Agent-proposed fix tied to a Change-Id and base SHA, with apply/reject lifecycle |
| **Suggestion Base SHA** | The exact checkpoint SHA a suggestion was created against |
| **Sync** | Fetch, push to sync ref, auto-merge if no conflicts, defer if conflicts |
| **Sync Ref** | Device's backup stream (`refs/jul/sync/<user>/<device>/...`) |
| **Transplant** | (Future) Rebase draft from one checkpoint base to another |
| **Workspace** | Named stream of work (replaces branches) |
| **Workspace Base** | Per-workspace file (`.jul/workspaces/<ws>/base`) tracking last merged SHA |
| **Workspace Ref** | Canonical state (`refs/jul/workspaces/...`) — shared across devices |

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
- [Gerrit Change-Id](https://gerrit-review.googlesource.com/Documentation/user-changeid.html) — Change identity spec
