# 줄 Jul: AI-First Git Hosting

**Version**: 0.2 (Draft)
**Status**: Design Specification

---

## 0. What Jul Is

Jul is **not** a new VCS or transport protocol. It is:

1. A **regular Git remote** (smart HTTP + SSH), and
2. A **sidecar protocol** (JSON over HTTPS + event stream) that makes the remote "agent-aware"

This is deliberate: implementing Git's pack protocol from scratch is a yak farm. Instead, Jul leans on standard server components like `git-http-backend`, which supports smart HTTP fetch/push and Git protocol v2.

Jul adds:
- **Sync-by-default**: Every change is continuously backed up to the server
- **Checkpoints**: Agent-assisted commit messages, clean separation of your work from fixes
- **Workspaces**: Replace branches as the primary unit of work
- **Attestations**: CI/coverage/lint as first-class queryable metadata
- **Suggestions**: Agent-proposed fixes with apply/reject lifecycle
- **Agent-native queries**: "Last green checkpoint," "interdiff since review," structured context
- **Local workspaces**: Instant context switching without stash/worktree overhead

---

## 1. Goals and Non-Goals

### 1.1 Goals

- **Sync-first**: Every change gets backed up to the server continuously
- **Checkpoint model**: Lock work when ready, agent generates messages, clean history
- **Workspaces over branches**: Named streams of work, not branch juggling
- **Agent-native primitives**:
  - Stable change identity across amend/rewrite
  - Built-in CI/coverage/lint/compile metadata
  - Queryable history (file versions, interdiff, trend charts)
  - Suggestions as first-class objects
  - Fast local context switching
- **Git compatibility**: Any Git client can clone/fetch/push published branches
- **JJ friendliness**: JJ users work normally (JJ's Git backend produces regular commits)
- **Flexible integration**: Go all-in with Jul, or use Jul as infrastructure behind Git/JJ

### 1.2 Non-Goals (v1)

- Replacing Git object storage
- Inventing a new merge algorithm
- Silent server-side rewriting of user history (server can *suggest*, not mutate)
- Multi-user / teams (personal use only for v1)
- Code review UI (use external tools)
- Issue tracking

---

## 2. Core Concepts

### 2.1 Entities

| Entity | Description |
|--------|-------------|
| **Repo** | A normal Git repository (bare on server, working copy on client) |
| **Workspace** | A named stream of work. Replaces branches for feature work. Default: `@` |
| **Draft** | Current working state, continuously synced, not yet locked |
| **Checkpoint** | A locked unit of work with Change-Id and generated message |
| **Change-Id** | Stable identifier that survives amend/rebase (`Iab4f3c2d...`) |
| **Attestation** | CI/test/coverage results attached to a checkpoint |
| **Suggestion** | Agent-proposed fix targeting a checkpoint |
| **Local Workspace** | Client-side saved state for fast context switching |

### 2.2 The Draft → Checkpoint → Promote Model

Jul uses a three-stage model inspired by JJ's "working copy is always a commit":

```
┌─────────────────────────────────────────────────────────────────────────┐
│  DRAFT                                                                  │
│    • Always exists in current workspace                                 │
│    • Continuously synced to server                                      │
│    • CI can run on drafts                                               │
│    • Has a Change-Id from creation                                      │
│    • No commit message yet                                              │
├─────────────────────────────────────────────────────────────────────────┤
│                           jul checkpoint                                │
├─────────────────────────────────────────────────────────────────────────┤
│  CHECKPOINT                                                             │
│    • Locked, immutable                                                  │
│    • Agent generates commit message (or user provides with -m)          │
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

**Key insight**: You're always working in a draft. There's no "uncommitted changes" anxiety. `jul checkpoint` is when you say "this is a logical unit."

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
# ... edit, auto-synced ...
jul checkpoint                  # Lock with message
jul promote --to main           # Publish
```

**Key differences**:
- `refs/heads/*` exist only as **promote targets** (main, staging, etc.)
- You never work directly on `refs/heads/main`
- Workspaces are where work happens: `refs/jul/workspaces/<user>/<name>`
- Default workspace `@` means you don't need to name anything upfront

### 2.4 Change Identity

We need a stable ID that survives checkpoint amends. Jul uses Change-Id (Gerrit-compatible):

- Format: `I` + 40 hex characters
- Generated when draft is created
- Preserved across amends within same logical change
- Stored in commit message trailer: `Change-Id: Iab4f3c2d...`

### 2.5 Integration Modes

Jul works at multiple levels. Choose your porcelain:

#### 2.5.1 Full Jul Mode

Jul is your primary interface.

```bash
$ jul configure                         # One-time setup
$ jul init my-project --create-remote   # Create repo + server remote
# ... edit ...
$ jul checkpoint                        # Lock + message + CI + review
$ jul promote --to main                 # Publish
```

#### 2.5.2 Git + Jul (Invisible Infrastructure)

Git is your porcelain. Jul syncs in background via hooks.

```bash
$ git init && jul init --server https://jul.example.com
$ jul hooks install
# ... use normal git commands ...
# post-commit hook auto-syncs
$ jul status                            # Check attestations
$ jul promote --to main                 # When ready
```

#### 2.5.3 JJ + Jul

JJ handles local workflow. Jul handles server.

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

### 2.6 Suggestion Lifecycle

Suggestions are agent-proposed fixes created after checkpoint:

```
checkpoint ──► CI + review runs ──► suggestions created
                                          │
         ┌────────────────────────────────┼────────────────────────┐
         ▼                                ▼                        ▼
   jul apply <id>                  jul reject <id>          ignore (warn on promote)
         │                                │
         ▼                                ▼
   added to current draft          marked rejected
         │
         ▼
   jul checkpoint (locks fix as separate checkpoint)
```

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

These are standard Git refs. Any Git client can interact with them. You never work directly on these—only `jul promote` updates them.

### 3.2 Workspace Refs

```
refs/jul/workspaces/<user>/<workspace>
```

Examples:
```
refs/jul/workspaces/george/@              # Default workspace
refs/jul/workspaces/george/feature-auth   # Named workspace
refs/jul/workspaces/george/bugfix-123     # Another named workspace
```

- Updated continuously (draft sync) and on checkpoint
- Force-push is normal and expected
- Server preserves full history (no GC pruning)

### 3.3 Suggestion Refs

```
refs/jul/suggest/<change_id>/<suggestion_id>
```

- Points to suggested commit(s)
- Immutable once created
- Can be fetched, inspected, applied by client

### 3.4 Notes Namespaces

```
refs/notes/jul/attestations    # CI/test/coverage results
refs/notes/jul/review          # Review comments
refs/notes/jul/meta            # Change-Id mappings, workspace metadata
```

### 3.5 Complete Ref Layout

```
refs/
├── heads/                           # Promote targets only
│   ├── main
│   └── staging
├── tags/
├── jul/
│   ├── workspaces/
│   │   └── <user>/
│   │       ├── @                    # Default workspace
│   │       └── <named-workspace>    # Named workspaces
│   └── suggest/
│       └── <change_id>/
│           └── <suggestion_id>
└── notes/jul/
    ├── attestations
    ├── review
    └── meta
```

---

## 4. Sidecar Protocol: Jul API v1

### 4.1 Transport

- **Base URL**: `https://<host>/<repo>.jul/api/v1/...`
- **Event stream**: Server-Sent Events (SSE) at `/events/stream`

### 4.2 Authentication

```
Authorization: Bearer <token>
```

Tokens have scopes: `git:read`, `git:write`, `jul:read`, `jul:write`, `ci:trigger`, `ai:suggest`

### 4.3 Core API Objects

#### Workspace

```json
{
  "workspace_id": "george/@",
  "user": "george",
  "name": "@",
  "head_sha": "abc123...",
  "draft": {
    "change_id": "Icd5e6f7a...",
    "files_changed": 3
  },
  "checkpoints": [
    {"change_id": "Iab4f3c2d...", "message": "feat: add auth", "sha": "def456..."}
  ],
  "updated_at": "2026-01-19T15:30:00Z"
}
```

#### Checkpoint

```json
{
  "change_id": "Iab4f3c2d1e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b",
  "commit_sha": "abc123...",
  "message": "feat: add user authentication",
  "author": "george",
  "status": "locked",
  "created_at": "2026-01-19T15:30:00Z",
  "attestation": { ... },
  "suggestions": [ ... ]
}
```

#### Attestation

```json
{
  "attestation_id": "01HX7Y8Z...",
  "change_id": "Iab4f3c2d...",
  "commit_sha": "abc123...",
  "status": "pass",
  "signals": {
    "lint": {"status": "pass", "warnings": 2},
    "compile": {"status": "pass"},
    "test": {"status": "pass", "passed": 48, "failed": 0},
    "coverage": {"line_pct": 84.2}
  },
  "finished_at": "2026-01-19T15:31:00Z"
}
```

#### Suggestion

```json
{
  "suggestion_id": "01HX7Y9A...",
  "change_id": "Iab4f3c2d...",
  "reason": "potential_null_check",
  "description": "Missing null check on token before validation",
  "confidence": 0.92,
  "status": "open",
  "diff": {
    "file": "src/auth.py",
    "additions": 3,
    "deletions": 1
  }
}
```

### 4.4 Key Endpoints

```
# Workspaces
GET  /api/v1/workspaces
GET  /api/v1/workspaces/<id>
POST /api/v1/workspaces                    # Create workspace
POST /api/v1/workspaces/<id>/sync          # Sync draft
POST /api/v1/workspaces/<id>/checkpoint    # Lock draft

# Checkpoints
GET  /api/v1/checkpoints/<change_id>
GET  /api/v1/checkpoints/<change_id>/interdiff

# Promote
POST /api/v1/promote
     Body: {"workspace_id": "...", "target": "main", "strategy": "rebase"}

# Suggestions
GET  /api/v1/suggestions?change_id=...
GET  /api/v1/suggestions/<id>
POST /api/v1/suggestions/<id>/apply
POST /api/v1/suggestions/<id>/reject

# Attestations
GET  /api/v1/attestations?change_id=...

# Events
GET  /api/v1/events/stream
```

---

## 5. Policy Model

### 5.1 Promote Policies

```toml
# .jul/policy.toml
[promote.main]
required_checks = ["compile", "test"]
min_coverage_pct = 80
require_suggestions_addressed = false   # Warn only
strategy = "rebase"                     # rebase | squash | merge
```

### 5.2 Promote Strategies

| Strategy | Behavior |
|----------|----------|
| `rebase` | Each checkpoint becomes a commit on target (linear history) |
| `squash` | All checkpoints squashed into single commit |
| `merge` | Merge commit joining workspace to target |

---

## 6. Git Implementation Details

This section addresses the concrete git representation of Jul's concepts.

### 6.1 Draft Representation

**Decision: Drafts are real git commits.**

A draft is a commit with:
- A placeholder message (e.g., `[draft] Iab4f3c2d`)
- A stable Change-Id in the message trailer
- Pointed to by `refs/jul/workspaces/<user>/<workspace>`

```
commit abc123
Author: george <george@example.com>
Date:   Mon Jan 19 15:30:00 2026

    [draft] Work in progress
    
    Change-Id: Iab4f3c2d1e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b
```

**Why commits, not sidecar snapshots:**
- Git tools work (diff, log, bisect)
- Server receives via normal push
- JJ/git interop preserved
- Attestations can attach via notes

**Shadow index to protect user's staging:**

Draft sync must NOT interfere with the user's `git add` state. Use a shadow index:

```bash
# Draft sync implementation
GIT_INDEX_FILE=.jul/draft-index git add -A
GIT_INDEX_FILE=.jul/draft-index git write-tree
# Create commit from tree, update workspace ref
# User's .git/index is untouched
```

This allows:
- User can `git add -p` (selective staging) while draft syncs
- User's staging is preserved
- Draft always reflects full working tree

### 6.2 Checkpoint vs Draft

| Aspect | Draft | Checkpoint |
|--------|-------|------------|
| Message | `[draft] WIP` | Agent-generated or user-provided |
| Mutable | Yes (amended on each sync) | No (immutable after creation) |
| CI runs | Optional (configurable) | Always |
| Retention | Ephemeral (no keep-ref) | Keep-ref created |
| Attestations | Temporary | Permanent |

When `jul checkpoint` runs:
1. Current draft commit is finalized with real message
2. A new draft commit is created as child
3. Keep-ref created for the checkpoint (see 6.4)

### 6.3 Sync Modes

Jul supports three sync modes, configurable per-repo or globally:

```toml
# ~/.config/jul/config.toml or .jul/config.toml
[sync]
mode = "continuous"   # continuous | on-command | explicit
```

#### Mode 1: `continuous` (default)

Dropbox-style. Daemon watches filesystem, syncs automatically.

```bash
$ jul sync --daemon &    # Start daemon (or auto-start on jul init)

# Daemon watches files, syncs when stable
# You never think about it
```

**Implementation:**
- Uses inotify (Linux) / FSEvents (macOS) / ReadDirectoryChangesW (Windows)
- Debounce: waits for write burst to settle
- Batches changes, creates draft commit, pushes

**Configuration:**
```toml
[sync]
mode = "continuous"
debounce_seconds = 2        # Wait for writes to settle
min_interval_seconds = 5    # Don't sync more often than this
```

**Pros:** Never lose work, seamless multi-machine handoff
**Cons:** Background process, more resource usage

#### Mode 2: `on-command`

JJ-style. Sync happens automatically on every `jul` command.

```bash
$ jul status      # Syncs draft first, then shows status
$ jul checkpoint  # Syncs draft, then locks it
$ jul ws switch   # Syncs draft, then switches
```

**Implementation:**
- Every `jul` command starts with "sync current draft if dirty"
- No daemon needed
- Sync is implicit but predictable

```toml
[sync]
mode = "on-command"
```

**Pros:** No daemon, predictable, sync happens when you're "at the keyboard"
**Cons:** Stale if you don't run jul commands for a while

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

### 6.4 Continuous Sync Implementation Details

For `continuous` mode, the daemon needs careful implementation:

**Debouncing:**
```
file change → wait 2s → no more changes? → sync
                     → more changes? → reset timer
```

**Ignore rules (beyond .gitignore):**
```
# .jul/syncignore
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

**Multi-machine handoff:**
```bash
# Machine A: daemon running, files syncing continuously

# Machine B: 
$ jul sync --pull    # Fetch latest draft from server
# Working tree updated to match Machine A's state
```

### 6.5 Retention and Fetchability

**Problem:** `gc.pruneExpire=never` keeps objects on disk, but doesn't make them fetchable. Unreachable commits can't be fetched by clients unless:
- (a) They're reachable via refs (keep-refs), or
- (b) Server allows fetching by SHA (`uploadpack.allowAnySHA1InWant`)

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
- Expired keep-refs deleted by server maintenance job
- Objects become unreachable after keep-ref deletion, eventually GC'd

```toml
# Server config
[retention]
checkpoint_keep_days = 90    # Keep-refs for checkpoints
draft_keep_days = 0          # No keep-refs for drafts (ephemeral)
```

**Drafts are intentionally ephemeral:**
- Only the latest draft commit matters
- Previous draft states are overwritten (force-push)
- If you need to recover old draft state, that's what checkpoints are for

**For personal use with full time-travel:**
```toml
[retention]
checkpoint_keep_days = -1    # Never expire (infinite)
```

**Future multi-user consideration:** Don't enable `uploadpack.allowAnySHA1InWant` in multi-tenant scenarios. Keep-refs are the safe path.

### 6.6 Two Classes of Attestations

**Problem:** Rebase/squash changes SHAs. An attestation for checkpoint `abc123` doesn't apply to the rebased commit `xyz789` on main.

**Solution: Separate checkpoint attestations from published attestations.**

| Attestation Type | Attached To | Purpose |
|------------------|-------------|---------|
| **Checkpoint** | Original checkpoint SHA | Pre-integration CI, review |
| **Published** | Post-rebase SHA on target | Final verification on main |

**Storage:**
```
refs/notes/jul/attestations/checkpoint   # Keyed by original SHA
refs/notes/jul/attestations/published    # Keyed by published SHA
```

**Workflow:**
```
checkpoint Iab4f... (sha: abc123)
    │
    ├── CI runs → checkpoint attestation for abc123
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

### 6.7 Notes Fetch Behavior

Git notes are not fetched by default. Jul remotes include explicit refspecs:

```ini
[remote "jul"]
    url = https://jul.example.com/my-project.git
    fetch = +refs/heads/*:refs/remotes/jul/*
    fetch = +refs/jul/workspaces/*:refs/remotes/jul/workspaces/*
    fetch = +refs/notes/jul/*:refs/notes/jul/*
```

**Single-writer rule:**
- `refs/notes/jul/attestations/*` — Server writes only
- `refs/notes/jul/meta` — Server writes only
- Clients only read notes, never push to notes refs

This avoids note-merge conflicts entirely.

### 6.8 Summary: Git Object Model

```
                            refs/heads/main
                                   │
                                   ▼
           ┌─────────── xyz789 (published, rebased) ◄─── published attestation
           │
           │   refs/jul/keep/george/@/Iab4f.../def456
           │                        │
           │                        ▼
           │           def456 (checkpoint, immutable) ◄─── checkpoint attestation
           │               │
           │               │   refs/jul/workspaces/george/@
           │               │              │
           │               │              ▼
           │               └──── ghi789 (draft, mutable)
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
- `refs/jul/workspaces/*` — Current draft per workspace
- `refs/jul/keep/*` — Checkpoint retention anchors
- `refs/jul/suggest/*` — Agent suggestion commits
- `refs/notes/jul/*` — Metadata (attestations, mappings)

---

## 7. Server Architecture

### 7.1 Components

```
┌─────────────────────────────────────────────────────────────────────┐
│                           Jul Server                                │
├─────────────────────────────────────────────────────────────────────┤
│  Git Service     │  API Service    │  CI Runner    │  Review Agent  │
│  (git-http-      │  (REST + SSE)   │  (job queue)  │  (configured   │
│   backend)       │                 │               │   provider)    │
├─────────────────────────────────────────────────────────────────────┤
│                           Core Service                              │
│  • Workspace management                                             │
│  • Checkpoint lifecycle                                             │
│  • Suggestion management                                            │
│  • Event bus                                                        │
├─────────────────────────────────────────────────────────────────────┤
│  Git Storage     │  SQLite         │  Artifacts                     │
│  (bare repos)    │  (indexes)      │  (local/S3)                    │
└─────────────────────────────────────────────────────────────────────┘
```

### 7.2 Retention Configuration

Per Section 6.4, the server manages keep-refs for checkpoint retention:

```toml
# /etc/jul/server.toml
[retention]
checkpoint_keep_days = 90    # Keep-refs expire after 90 days
run_maintenance_hours = 3    # Run cleanup at 3 AM
```

Maintenance job:
1. Find expired keep-refs
2. Delete refs
3. Run `git gc` (objects now unreachable will be pruned per gc config)

---

## 8. CLI Design (`jul`)

### 8.1 Setup Commands

#### `jul configure`

Interactive setup wizard for global configuration.

```bash
$ jul configure
Jul Configuration
─────────────────
Server URL: https://jul.example.com
Username: george

Agent Provider:
  [1] opencode (bundled)
  [2] claude-code
  [3] codex
  [4] custom
Select [1]: 1

Configuration saved to ~/.config/jul/config.toml
```

Creates:
- `~/.config/jul/config.toml` — Server, user defaults
- `~/.config/jul/agents.toml` — Agent provider settings

#### `jul init`

Initialize a repository with Jul.

```bash
# New project (creates local + remote)
$ jul init my-project --create-remote
Created remote: https://jul.example.com/my-project.git
Initialized local repository
Workspace '@' ready

# Existing repo
$ cd existing-project
$ jul init --server https://jul.example.com
Configured Jul remote
Installed hooks
Workspace '@' ready
```

What it does:
1. `git init` (if new)
2. Add remote with Jul refspecs
3. Install hooks (post-commit for auto-sync)
4. Create default workspace `@`
5. Start first draft

### 8.2 Core Workflow Commands

#### `jul sync`

Save current state to draft.

```bash
$ jul sync
Syncing draft...
  3 files changed
Synced to refs/jul/workspaces/george/@
```

Usually runs automatically via hook or daemon. Manual sync is rarely needed.

```bash
# Run as daemon (watches filesystem)
$ jul sync --daemon
Watching for changes...
  15:30:01 synced (2 files)
  15:30:45 synced (1 file)
```

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
- `--no-review` — Skip review
- `--json` — JSON output

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

### 8.3 Workspace Commands

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
Switched to workspace 'feature-auth'
Draft: Ief6a... (clean)
```

No dirty state concerns—current draft is already synced.

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

### 8.4 Suggestion Commands

#### `jul suggestions`

List pending suggestions.

```bash
$ jul suggestions

Pending for Iab4f... "feat: add JWT validation":

  [01HX7Y9A] potential_null_check (92%)
             src/auth.py:42 - Missing null check on token
             
  [01HX7Y9B] test_coverage (78%)
             src/auth.py:67-73 - Uncovered error path

Actions:
  jul show <id>      Show diff
  jul apply <id>     Apply to draft
  jul reject <id>    Reject
```

#### `jul show`

Show suggestion details.

```bash
$ jul show 01HX7Y9A

Suggestion: potential_null_check
Confidence: 92%
Checkpoint: Iab4f... "feat: add JWT validation"

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

#### `jul reject`

Reject a suggestion.

```bash
$ jul reject 01HX7Y9B -m "covered by integration tests"
Rejected.
```

### 8.5 Review Command

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

### 8.6 Resolve Command

#### `jul resolve`

Agent-assisted merge conflict resolution.

```bash
$ git merge feature-branch
CONFLICT: src/auth.py

$ jul resolve
Analyzing conflicts...
  src/auth.py: 3-way merge conflict

Invoking agent (opencode)...

Proposed resolution:
  src/auth.py: +15 -8
  "Both branches add validation. Combined preserving both checks."

Apply? [y/n/edit] y
Resolved src/auth.py

$ git add src/auth.py
$ git commit
```

### 8.7 Query Commands

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

Show workspace history.

```bash
$ jul reflog --limit=10

Icd5e... checkpoint "fix: null check" (2h ago)
Iab4f... checkpoint "feat: add JWT validation" (4h ago)
        └─ draft sync (4h ago)
        └─ draft sync (5h ago)
Ief6a... checkpoint "initial structure" (1d ago)
```

### 8.8 Local Workspaces (Client-Side)

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

## 9. Configuration

### 9.1 Global Config

```toml
# ~/.config/jul/config.toml

[server]
url = "https://jul.example.com"
user = "george"

[workspace]
default_name = "@"

[sync]
mode = "continuous"              # continuous | on-command | explicit
debounce_seconds = 2             # For continuous mode
min_interval_seconds = 5         # For continuous mode
auto_start_daemon = true         # Start daemon on jul init

[checkpoint]
auto_message = true              # Agent generates message

[promote]
default_target = "main"
strategy = "rebase"              # rebase | squash | merge

[ci]
run_on_draft = true              # CI runs on draft syncs
run_on_checkpoint = true

[review]
enabled = true
run_on_checkpoint = true         # Review after checkpoint
run_on_draft = false             # Manual only
min_confidence = 70

[apply]
auto_checkpoint = false          # jul apply also checkpoints
```

### 9.2 Agent Config

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

### 9.3 Repo Config

```toml
# .jul/config.toml (per-repo)

[workspace]
name = "feature-auth"            # Override default

[ci]
# Agent-assisted CI setup (future)
# First checkpoint without config triggers setup wizard
```

### 9.4 Policy Config

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

## 10. Agent Integration

Jul is designed for two types of agents:

1. **External agents** (Codex, Claude Code, etc.) — Build your application, use Jul for feedback
2. **Internal agent** (configured provider) — Reviews your code, generates suggestions

### 9.1 External Agent Integration

External agents use Jul as infrastructure. The key principle: **every command returns structured feedback that agents can act on.**

#### 9.1.1 The Feedback Loop

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

#### 9.1.2 Checkpoint Response

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

#### 9.1.3 Apply Response

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

#### 9.1.4 Full External Agent Loop

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

### 9.2 Internal Agent (Review Agent)

The internal agent is your configured provider (opencode, codex, etc.). It runs review and generates suggestions.

**Key principle**: The internal agent works in an isolated sandbox, never touching your files directly.

#### 9.2.1 Agent Sandbox

```
.jul/
├── agent-workspace/              # Isolated agent sandbox
│   ├── worktree/                 # Git worktree for agent
│   │   └── ... (checked out files)
│   ├── suggestions/
│   │   ├── 01HX7Y9A/            # Each suggestion is a commit
│   │   │   ├── commit           # SHA of suggestion commit
│   │   │   ├── base             # SHA it applies to
│   │   │   └── metadata.json    # Reason, confidence, etc.
│   │   └── 01HX7Y9B/
│   └── logs/
│       └── review-2026-01-19.log
```

#### 9.2.2 How Internal Agent Creates Suggestions

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
     "checkpoint": "Iab4f3c2d...",
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
     "commit": "def456...",
     "base": "abc123...",
     "reason": "potential_null_check",
     "confidence": 0.92
   }
   ```

#### 9.2.3 Applying Suggestions

When user runs `jul apply 01HX7Y9A`:

1. Get the suggestion's commit SHA
2. Cherry-pick or patch into user's draft
   ```bash
   git cherry-pick --no-commit def456
   ```
3. Changes appear in user's working directory
4. User can review, edit, then checkpoint

**The user never sees the agent workspace.** Suggestions appear as "proposed changes" that can be previewed and applied.

#### 9.2.4 Agent Workspace Lifecycle

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

### 9.3 Agent Protocol (v1)

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

### 9.4 Agent Actions

| Action | Triggered by | Agent workspace | Purpose |
|--------|--------------|-----------------|---------|
| `generate_message` | `jul checkpoint` | No | Create commit message |
| `review` | After checkpoint | Yes | Analyze code, create suggestions |
| `resolve_conflict` | `jul resolve` | Yes | 3-way merge resolution |
| `setup_ci` | First checkpoint | No | Auto-configure CI (future) |

### 9.5 Configuration

```toml
# ~/.config/jul/agents.toml

[default]
provider = "opencode"

[providers.opencode]
command = "opencode"
protocol = "jul-agent-v1"
timeout_seconds = 300

[providers.claude-code]
command = "claude"
protocol = "jul-agent-v1"
timeout_seconds = 300

[sandbox]
enable_network = false          # Agent can't make network calls
enable_exec = true              # Agent can run tests
max_iterations = 5              # Max edit-test cycles per review
```

### 9.6 Structured Output

All commands support `--json` for external agent consumption:

```bash
$ jul status --json
$ jul checkpoint --json  
$ jul suggestions --json
$ jul show 01HX... --json
$ jul apply 01HX... --json
$ jul reject 01HX... --json
$ jul review --json
$ jul promote --to main --json
```

Every response includes `next_actions` suggesting what the agent might do next.

---

## 11. Example Workflows

### 10.1 Full Jul Flow

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

### 10.2 Git + Jul Flow

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

### 10.3 Agent-Driven Flow

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

## 12. Deployment

### 11.1 Server Requirements

- Linux (Ubuntu 22.04+)
- Git 2.30+
- Go 1.22+ (for jul binaries)
- nginx (reverse proxy)

### 11.2 Quick Start

```bash
# Install
curl -fsSL https://jul.example.com/install.sh | bash

# Initialize server
jul-server init /var/jul

# Create first repo
jul-server create-repo my-project

# Run
jul-server run
```

---

## 13. Glossary

| Term | Definition |
|------|------------|
| **Attestation** | CI/test/coverage results attached to a checkpoint |
| **Change-Id** | Stable identifier (`Iab4f...`) preserved across amends |
| **Checkpoint** | Locked unit of work with message and Change-Id |
| **Draft** | Current working state, continuously synced, not yet locked |
| **Local Workspace** | Client-side saved state for fast context switching |
| **Promote** | Policy-checked move of checkpoints to a target branch |
| **Suggestion** | Agent-proposed fix with apply/reject lifecycle |
| **Workspace** | Named stream of work (replaces branches for feature work) |

---

## Appendix A: Why Not Just Use X?

| Alternative | Why Jul is different |
|-------------|---------------------|
| **GitHub/GitLab** | No continuous sync, no checkpoint model, CI is bolted on |
| **Gerrit** | Change-centric but complex, not agent-native |
| **JJ alone** | Great local UX but needs server for backup/CI/sharing |
| **Git + hooks** | No queryable attestations, no suggestions, no workspaces |

Jul combines: Continuous sync + Checkpoint model + Workspaces + Agent-native suggestions + Queryable attestations.

---

## Appendix B: Migration from Git Workflow

| Git habit | Jul equivalent |
|-----------|----------------|
| `git checkout -b feature` | `jul ws new feature` |
| `git add . && git commit` | `jul checkpoint` |
| `git commit --amend` | Edit draft, `jul checkpoint` |
| `git push` | Automatic (sync) |
| `git merge` | `jul promote --to main` |
| `git stash` | `jul local save` |

---

## Appendix C: References

- [git-http-backend](https://git-scm.com/docs/git-http-backend) — Smart HTTP server
- [JJ (Jujutsu)](https://github.com/jj-vcs/jj) — Inspiration for working-copy model
- [Gerrit Change-Id](https://gerrit-review.googlesource.com/Documentation/user-changeid.html) — Change identity spec
