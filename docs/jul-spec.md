# 줄 Jul: AI-First Git Hosting

**Version**: 0.1 (Draft)
**Status**: Design Specification

---

## 0. What Jul Is

Jul is **not** a new VCS or transport protocol. It is:

1. A **regular Git remote** (smart HTTP + SSH), and
2. A **sidecar protocol** (JSON over HTTPS + event stream) that makes the remote "agent-aware"

This is deliberate: implementing Git's pack protocol from scratch is a yak farm. Instead, Jul leans on standard server components like `git-http-backend`, which supports smart HTTP fetch/push and Git protocol v2.

Jul adds:
- **Sync-by-default**: Every commit is backed up to the server immediately
- **Change-centric history**: Stable identity across amend/rebase
- **Attestations**: CI/coverage/lint as first-class queryable metadata
- **Suggestions**: Server/agent-proposed fixes as immutable objects
- **Agent-native queries**: "Last green revision," "interdiff since review," structured context

---

## 1. Goals and Non-Goals

### 1.1 Goals

- **Sync-first**: Every commit gets backed up to the server (optionally workspace snapshots too)
- **Published vs synced**: Separate "I backed it up" from "I'm publishing this as the branch state"
- **Agent-native primitives**:
  - Stable change identity across amend/rewrite
  - Built-in CI/coverage/lint/compile metadata
  - Queryable history (file versions, interdiff, trend charts)
  - Suggestions as first-class objects
- **Git compatibility**: Any Git client can clone/fetch/push published branches
- **JJ friendliness**: JJ users work normally (JJ's Git backend produces regular commits)

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
| **Workspace** | A named stream of work from one device/context: `george-mbp`, `desktop`, `ci-bot` |
| **Change** | A stable "line of work" that can have multiple rewritten revisions. Change-Id survives amend/rebase. |
| **Revision** | A specific commit SHA representing one version of a change |
| **Attestation** | Results attached to a commit: build/test/lint/coverage, plus logs and artifacts |
| **Suggestion** | A server/agent-proposed patch (new commits) targeting a change/revision |

### 2.2 Change Identity

We need a stable ID that survives commit SHA churn (amend/rebase). There's ecosystem convergence around "Change-Id":

- **Gerrit**: `Change-Id: Iabc...` trailer in commit message
- **JJ**: `change-id` commit header (can be lost in some Git operations)
- **GitButler**: Similar header approach

**Jul's approach** (maximally interoperable):

1. Accept Change-Id from **either**:
   - `Change-Id:` trailer in commit message (preferred)
   - `change-id:` commit header
   - Server-generated ID if neither present (stored in Jul metadata)

2. Canonical format: `I` + 40 hex characters (Gerrit-compatible)

3. The `jul` CLI writes `Change-Id:` trailers because they survive all Git operations

**Generation algorithm**:
```python
import hashlib
import time
import os

def generate_change_id(tree_sha: str, author: str, message: str) -> str:
    """Generate a stable Change-Id for a new change."""
    data = f"{tree_sha}\n{author}\n{message}\n{time.time_ns()}\n{os.urandom(8).hex()}"
    return f"I{hashlib.sha1(data.encode()).hexdigest()}"
```

---

## 3. Git Layer: Ref Namespaces

Jul uses Git refs to separate *sync* from *publish*. All Jul-specific refs live under `refs/jul/` to avoid collisions.

### 3.1 Published Refs (Standard Git)

```
refs/heads/<branch>     # Normal branches (main, dev, etc.)
refs/tags/<tag>         # Normal tags
```

**Important**: Published branches are normal `refs/heads/*`. Jul treats "promotion" as a policy-controlled update of these standard refs. We do NOT create a parallel `refs/publish/*` namespace—that would be a consistency tax.

Any Git client can interact with these refs normally.

### 3.2 Workspace Refs (Jul-Specific)

```
refs/jul/workspaces/<user>/<workspace>
```

- Updated frequently (every commit, debounced)
- Never treated as "reviewed/published," just "backed up + shareable"
- Force-push is normal and expected

Examples:
```
refs/jul/workspaces/george/macbook-main
refs/jul/workspaces/george/desktop-feature-auth
```

### 3.3 Keep Refs (Object Retention)

**Critical**: Without retention, old commits become unreachable after force-push and `git gc` will prune them. Your SQLite reflog would point to nonexistent SHAs.

```
refs/jul/keep/<workspace>/<timestamp>
```

When a workspace ref is updated:
1. If old SHA differs from new SHA, create a keep ref pointing to old SHA
2. Keep refs expire after N days (configurable, default 30)
3. A background job prunes expired keep refs

This ensures "time travel across rewrites" actually works.

### 3.4 Suggestion Refs

```
refs/jul/suggest/<change_id>/<suggestion_id>
```

- Points to suggested commit head(s)
- Immutable once created (new suggestion_id for new versions)
- Can be fetched, inspected, cherry-picked by client

### 3.5 Notes Namespaces

Git notes attach metadata without changing commit SHAs. Jul uses dedicated namespaces:

```
refs/notes/jul/attestations    # CI/test/coverage results per commit
refs/notes/jul/review          # Comments, approvals
refs/notes/jul/meta            # Change-Id mapping, workspace metadata
```

**Important**: Only fetch `refs/notes/jul/*`, not all notes. This avoids stomping other note namespaces.

```bash
# Correct refspec (in .git/config)
fetch = +refs/notes/jul/*:refs/notes/jul/*

# CLI reads notes explicitly
git notes --ref=refs/notes/jul/attestations show <commit>
```

### 3.6 Complete Ref Layout

```
refs/
├── heads/                           # Standard Git branches (published)
│   ├── main
│   └── feature-auth
├── tags/                            # Standard Git tags
├── jul/
│   ├── workspaces/
│   │   └── <user>/
│   │       └── <workspace>          # Sync targets
│   ├── keep/
│   │   └── <workspace>/
│   │       └── <timestamp>          # Retention refs
│   └── suggest/
│       └── <change_id>/
│           └── <suggestion_id>      # Suggested commits
└── notes/
    └── jul/
        ├── attestations             # CI results
        ├── review                   # Comments
        └── meta                     # Metadata
```

---

## 4. Sidecar Protocol: Jul API v1

### 4.1 Transport

- **Base URL**: `https://<host>/<repo>.jul/api/v1/...`
- **Event stream**: Server-Sent Events (SSE) at `/events/stream`

The `.jul` suffix keeps API endpoints separate from Git endpoints.

### 4.2 Authentication

```
Authorization: Bearer <token>
```

Tokens have scopes:
- `git:read`, `git:write` — Git operations
- `jul:read`, `jul:write` — Jul API operations  
- `ci:trigger`, `ci:read` — CI operations
- `ai:suggest` — Server-side agent (if enabled)

Git auth uses standard mechanisms (SSH keys, HTTP basic/token).

### 4.3 Canonical IDs

| ID Type | Format | Example |
|---------|--------|---------|
| `commit_sha` | 40 hex (SHA-1/SHA-256) | `abc123def456...` |
| `change_id` | `I` + 40 hex | `Iab4f3c2d1e5f...` |
| `workspace_id` | `<user>/<workspace>` | `george/macbook-main` |
| `attestation_id` | ULID | `01HX7Y8Z...` |
| `suggestion_id` | ULID | `01HX7Y9A...` |
| `event_id` | ULID (monotonic) | `01HX7Y9B...` |

### 4.4 API Objects

#### Change

```json
{
  "change_id": "Iab4f3c2d1e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b",
  "title": "feat: add user authentication",
  "author": "george",
  "created_at": "2026-01-19T15:30:00Z",
  "latest_revision": {
    "rev_index": 7,
    "commit_sha": "abc123..."
  },
  "revisions": [
    {"rev_index": 1, "commit_sha": "111aaa...", "created_at": "..."},
    {"rev_index": 2, "commit_sha": "222bbb...", "created_at": "..."}
  ],
  "status": "draft"
}
```

Status values: `draft` | `ready` | `published` | `abandoned`

#### Attestation

```json
{
  "attestation_id": "01HX7Y8Z...",
  "commit_sha": "abc123...",
  "change_id": "Iab4f3c2d...",
  "type": "ci",
  "status": "fail",
  "started_at": "2026-01-19T15:30:00Z",
  "finished_at": "2026-01-19T15:31:45Z",
  "signals": {
    "format": {
      "status": "warn",
      "message": "2 files need formatting",
      "files": ["src/auth.py", "src/utils.py"]
    },
    "lint": {
      "status": "pass",
      "warnings": 2,
      "errors": 0
    },
    "compile": {
      "status": "pass",
      "duration_ms": 8500
    },
    "test": {
      "status": "fail",
      "passed": 47,
      "failed": 2,
      "skipped": 1,
      "failures": [
        {
          "name": "test_refresh_token_expiry",
          "file": "tests/test_auth.py",
          "line": 42,
          "message": "AssertionError: expected True, got False",
          "stack_trace": "..."
        }
      ]
    },
    "coverage": {
      "status": "complete",
      "line_pct": 82.5,
      "branch_pct": 71.2,
      "diff_line_pct": 91.7,
      "uncovered_lines": {
        "src/auth.py": [45, 46, 78, 79, 80]
      }
    }
  },
  "artifacts": [
    {"name": "junit.xml", "uri": "artifact://01HX7Y8Z.../junit.xml"},
    {"name": "coverage.json", "uri": "artifact://01HX7Y8Z.../coverage.json"}
  ],
  "log_excerpt": "FAILED tests/test_auth.py::test_refresh_token_expiry..."
}
```

**Important**: The `format` signal is **check-only** on the server. If files need formatting, the server reports it but does NOT rewrite commits. Formatting fixes come via:
1. Client-side pre-commit hooks (auto-fix locally)
2. Server-generated suggestion (a new commit the client can accept)

#### Suggestion

```json
{
  "suggestion_id": "01HX7Y9A...",
  "change_id": "Iab4f3c2d...",
  "base_commit_sha": "abc123...",
  "suggested_commit_sha": "def456...",
  "created_by": "server-agent",
  "created_at": "2026-01-19T15:32:00Z",
  "reason": "fix_failing_tests",
  "description": "The test expects old behavior. Updated test to match new auto-refresh logic.",
  "confidence": 0.85,
  "status": "open",
  "diffstat": {
    "files_changed": 1,
    "additions": 5,
    "deletions": 3
  }
}
```

Status values: `open` | `accepted` | `rejected` | `superseded`

### 4.5 API Endpoints

#### Discovery

```
GET /api/v1/capabilities
```

Returns supported features, ref namespaces, CI runners, AI modes.

#### Workspaces

```
GET  /api/v1/workspaces
GET  /api/v1/workspaces/<workspace_id>
POST /api/v1/workspaces/<workspace_id>/promote
     Body: {"target_branch": "main", "commit_sha": "optional"}
```

#### Changes

```
GET /api/v1/changes
    Query: ?status=draft&author=george&limit=20
GET /api/v1/changes/<change_id>
GET /api/v1/changes/<change_id>/revisions
GET /api/v1/changes/<change_id>/interdiff
    Query: ?from_rev=5&to_rev=7
```

#### Commits & Attestations

```
GET /api/v1/commits/<sha>
GET /api/v1/commits/<sha>/attestation
GET /api/v1/attestations
    Query: ?commit_sha=...&change_id=...&status=fail
POST /api/v1/ci/trigger
     Body: {"commit_sha": "...", "profile": "unit|full|lint"}
```

#### Suggestions

```
GET  /api/v1/suggestions
     Query: ?change_id=...&status=open
GET  /api/v1/suggestions/<suggestion_id>
POST /api/v1/suggestions
     Body: {"change_id": "...", "reason": "fix_tests"}
     (Requests server agent to propose a fix)
POST /api/v1/suggestions/<suggestion_id>/accept
POST /api/v1/suggestions/<suggestion_id>/reject
```

#### Files & History

```
GET /api/v1/files/<path:path>/history
    Query: ?ref=main&limit=20
GET /api/v1/files/<path:path>/content
    Query: ?ref=HEAD
```

#### Query

```
GET /api/v1/query
    Query params:
      tests: pass|fail
      compiles: true|false
      coverage_min: float
      coverage_max: float  
      change_id: string
      author: string
      since: ISO datetime
      until: ISO datetime
      limit: int (default 20)
```

#### Artifacts

```
GET /api/v1/artifacts/<artifact_id>
    Returns signed download URL or streams content
```

#### Events

```
GET /api/v1/events/stream
    Query: ?since=<event_id>
    Returns: Server-Sent Events
```

Event types:
- `ref.updated` — Workspace or published ref changed
- `ci.started`, `ci.finished` — Pipeline lifecycle
- `attestation.added` — New attestation available
- `suggestion.created` — New suggestion available
- `policy.violation` — Attempted publish without required checks

Example event:
```json
{
  "event_id": "01HX7Y9B...",
  "type": "ci.finished",
  "repo": "my-project",
  "ref": "refs/jul/workspaces/george/macbook-main",
  "commit_sha": "abc123...",
  "change_id": "Iab4f3c2d...",
  "summary": "fail: 2 tests failed",
  "attestation_id": "01HX7Y8Z..."
}
```

**Design rule**: Server pushes events; clients pull data. The server never reaches into your working tree.

---

## 5. Policy Model (Promotion Gate)

### 5.1 Policy Configuration

Per-repo and per-branch policies:

```toml
# .jul/policy.toml or server config

[policy.default]
required_checks = ["compile", "test"]
min_coverage_pct = 80
no_coverage_regression = true
require_format_clean = true
require_lint_clean = false  # Warnings OK, errors not OK

[policy.branches.main]
# Stricter for main
required_checks = ["compile", "test", "lint"]
require_lint_clean = true
```

### 5.2 Promotion Workflow

Promotion moves a workspace head to a published branch:

```
POST /api/v1/workspaces/<workspace_id>/promote
Body: {
  "target_branch": "main",
  "commit_sha": "abc123..."  // Optional, defaults to workspace head
}
```

Server validates:
1. Commit exists and is reachable from workspace
2. Required attestations exist and pass
3. Coverage thresholds met
4. (Optional) Change status is `ready`

If valid:
1. Fast-forward `refs/heads/<target_branch>` (or reject if not FF)
2. Update change status to `published`
3. Emit `ref.updated` event

If invalid:
```json
{
  "error": "policy_violation",
  "violations": [
    {"check": "test", "status": "fail", "message": "2 tests failing"},
    {"check": "coverage", "status": "fail", "message": "78% < 80% required"}
  ]
}
```

---

## 6. Server Architecture

### 6.1 Component Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│                           Jul Server                                │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌─────────┐ │
│  │ Git Service  │  │ API Service  │  │ CI Runner    │  │ Web UI  │ │
│  │              │  │              │  │              │  │         │ │
│  │ smart HTTP   │  │ FastAPI      │  │ Job queue    │  │ React   │ │
│  │ post-receive │  │ REST + SSE   │  │ Sandbox exec │  │ SPA     │ │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘  └────┬────┘ │
│         │                 │                 │               │      │
│         └─────────────────┴─────────────────┴───────────────┘      │
│                                    │                                │
│                           ┌────────▼────────┐                       │
│                           │  Core Service   │                       │
│                           │                 │                       │
│                           │ • Hook handler  │                       │
│                           │ • Change index  │                       │
│                           │ • Attestations  │                       │
│                           │ • Suggestions   │                       │
│                           │ • Event bus     │                       │
│                           │ • Retention mgr │                       │
│                           └────────┬────────┘                       │
│                                    │                                │
│              ┌─────────────────────┼─────────────────────┐          │
│              │                     │                     │          │
│       ┌──────▼──────┐       ┌──────▼──────┐       ┌──────▼──────┐  │
│       │ Git Storage │       │   SQLite    │       │  Artifacts  │  │
│       │             │       │             │       │             │  │
│       │ Bare repos  │       │ Indexes     │       │ Local FS    │  │
│       │ Notes refs  │       │ Attestations│       │ (S3 later)  │  │
│       │ Keep refs   │       │ Events      │       │             │  │
│       └─────────────┘       └─────────────┘       └─────────────┘  │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

### 6.2 Git Service

Uses `git-http-backend` (standard Git smart HTTP):

```nginx
# nginx config
location ~ ^/(?<repo>[^/]+)\.git(?<path>/.*)?$ {
    auth_basic "Jul";
    auth_basic_user_file /etc/jul/htpasswd;
    
    include fastcgi_params;
    fastcgi_pass unix:/var/run/fcgiwrap.socket;
    fastcgi_param SCRIPT_FILENAME /usr/lib/git-core/git-http-backend;
    fastcgi_param GIT_PROJECT_ROOT /var/jul/repos;
    fastcgi_param GIT_HTTP_EXPORT_ALL "";
    fastcgi_param PATH_INFO $path;
    fastcgi_param REMOTE_USER $remote_user;
}

# Jul API (separate path)
location ~ ^/(?<repo>[^/]+)\.jul/ {
    proxy_pass http://127.0.0.1:8000;
}
```

### 6.3 Hook Handler (Async Ingestion)

**Critical**: Don't block `post-receive` on API calls. Use async ingestion:

```bash
#!/bin/bash
# /var/jul/repos/<repo>.git/hooks/post-receive

# Append to spool file (fast, non-blocking)
while read old_sha new_sha ref; do
    echo "{\"repo\":\"$GIT_DIR\",\"ref\":\"$ref\",\"old\":\"$old_sha\",\"new\":\"$new_sha\",\"user\":\"$REMOTE_USER\",\"time\":\"$(date -Iseconds)\"}" >> /var/jul/spool/events.jsonl
done

# Signal the ingestion service (non-blocking)
curl -s -X POST "http://localhost:8000/internal/hooks/notify" &
```

The core service watches the spool and processes events asynchronously.

### 6.4 Core Service

```python
# Simplified structure

class JulService:
    def __init__(self, config: Config):
        self.db = Database(config.db_path)
        self.repos = RepoManager(config.repos_dir)
        self.events = EventBus()
        self.ci = CIRunner(config.ci)
        self.retention = RetentionManager(config.retention)
    
    async def process_push(self, push: PushEvent):
        """Process a push event from the spool."""
        
        # 1. Create keep ref if needed (object retention)
        if push.old_sha != ZERO_SHA and push.old_sha != push.new_sha:
            if push.ref.startswith("refs/jul/workspaces/"):
                await self.retention.create_keep_ref(
                    repo=push.repo,
                    workspace=extract_workspace(push.ref),
                    old_sha=push.old_sha
                )
        
        # 2. Log ref movement
        await self.db.log_ref_movement(push)
        
        # 3. Index new commits
        new_commits = await self.repos.get_new_commits(
            push.repo, push.old_sha, push.new_sha
        )
        for commit in new_commits:
            change_id = self.extract_change_id(commit)
            await self.db.index_commit(commit, change_id)
        
        # 4. Queue CI job
        await self.ci.queue_job(
            repo=push.repo,
            commit_sha=push.new_sha,
            ref=push.ref
        )
        
        # 5. Emit event
        await self.events.emit(RefUpdatedEvent(
            repo=push.repo,
            ref=push.ref,
            old_sha=push.old_sha,
            new_sha=push.new_sha
        ))
    
    def extract_change_id(self, commit: Commit) -> str:
        """Extract Change-Id from trailer or header, or generate one."""
        # Try trailer first (most interoperable)
        for line in reversed(commit.message.split('\n')):
            if line.startswith('Change-Id: '):
                return line[11:].strip()
        
        # Try header
        if 'change-id' in commit.headers:
            return commit.headers['change-id']
        
        # Generate and store (won't be in commit, only in DB)
        return generate_change_id(commit.tree, commit.author, commit.message)
```

### 6.5 CI Runner

```python
class CIRunner:
    async def run_job(self, job: CIJob):
        """Run CI pipeline for a commit."""
        work_dir = self.create_work_dir(job.id)
        
        try:
            # Checkout
            await self.checkout(job.repo, job.commit_sha, work_dir)
            
            # Detect project type
            project = await self.detect_project(work_dir)
            
            # Run stages
            attestation = Attestation(
                commit_sha=job.commit_sha,
                change_id=job.change_id
            )
            
            # Format: CHECK ONLY (no rewriting)
            format_result = await self.run_format_check(work_dir, project)
            attestation.signals['format'] = format_result
            
            # Lint
            lint_result = await self.run_lint(work_dir, project)
            attestation.signals['lint'] = lint_result
            
            # Compile
            compile_result = await self.run_compile(work_dir, project)
            attestation.signals['compile'] = compile_result
            if compile_result.status == 'fail':
                attestation.status = 'fail'
                return await self.store_attestation(attestation)
            
            # Test
            test_result = await self.run_tests(work_dir, project)
            attestation.signals['test'] = test_result
            
            # Coverage
            coverage_result = await self.run_coverage(work_dir, project)
            attestation.signals['coverage'] = coverage_result
            
            # Determine overall status
            attestation.status = self.compute_status(attestation.signals)
            
            # Store attestation (DB + optionally git notes)
            await self.store_attestation(attestation)
            
            # If tests failed and AI suggestions enabled, queue suggestion
            if test_result.status == 'fail' and self.config.ai_suggestions:
                await self.queue_suggestion_request(job, attestation)
            
        finally:
            self.cleanup_work_dir(work_dir)
```

### 6.6 Retention Manager

```python
class RetentionManager:
    def __init__(self, config: RetentionConfig):
        self.keep_days = config.keep_days  # Default: 30
    
    async def create_keep_ref(self, repo: str, workspace: str, old_sha: str):
        """Create a keep ref to prevent GC of old commits."""
        timestamp = datetime.now().strftime("%Y%m%d%H%M%S")
        keep_ref = f"refs/jul/keep/{workspace}/{timestamp}"
        
        await run_git(repo, ["update-ref", keep_ref, old_sha])
        await self.db.record_keep_ref(repo, keep_ref, old_sha, workspace)
    
    async def prune_expired_keep_refs(self):
        """Background job: remove keep refs older than retention period."""
        cutoff = datetime.now() - timedelta(days=self.keep_days)
        expired = await self.db.get_expired_keep_refs(cutoff)
        
        for ref in expired:
            await run_git(ref.repo, ["update-ref", "-d", ref.ref_name])
            await self.db.delete_keep_ref(ref.id)
```

### 6.7 Database Schema

```sql
-- Repositories
CREATE TABLE repos (
    id INTEGER PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    path TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Changes (logical units of work)
CREATE TABLE changes (
    id INTEGER PRIMARY KEY,
    repo_id INTEGER REFERENCES repos(id),
    change_id TEXT NOT NULL,  -- Iab4f3c2d...
    title TEXT,
    author TEXT,
    status TEXT DEFAULT 'draft',  -- draft, ready, published, abandoned
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    UNIQUE(repo_id, change_id)
);

-- Revisions (commits belonging to a change)
CREATE TABLE revisions (
    id INTEGER PRIMARY KEY,
    change_id INTEGER REFERENCES changes(id),
    rev_index INTEGER NOT NULL,  -- 1, 2, 3, ...
    commit_sha TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    UNIQUE(change_id, rev_index),
    UNIQUE(change_id, commit_sha)
);

-- Commits (denormalized for fast queries)
CREATE TABLE commits (
    id INTEGER PRIMARY KEY,
    repo_id INTEGER REFERENCES repos(id),
    commit_sha TEXT NOT NULL,
    change_id TEXT,  -- May be NULL if not extracted
    tree_sha TEXT,
    author TEXT,
    message TEXT,
    created_at TIMESTAMP,
    indexed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    UNIQUE(repo_id, commit_sha)
);

CREATE INDEX idx_commits_change_id ON commits(repo_id, change_id);

-- Attestations
CREATE TABLE attestations (
    id INTEGER PRIMARY KEY,
    attestation_id TEXT UNIQUE NOT NULL,  -- ULID
    repo_id INTEGER REFERENCES repos(id),
    commit_sha TEXT NOT NULL,
    change_id TEXT,
    type TEXT DEFAULT 'ci',
    status TEXT,  -- running, pass, fail, error
    
    -- Denormalized signals for fast filtering
    format_status TEXT,
    lint_status TEXT,
    lint_errors INTEGER,
    compile_status TEXT,
    test_status TEXT,
    test_passed INTEGER,
    test_failed INTEGER,
    coverage_line_pct REAL,
    coverage_branch_pct REAL,
    
    -- Full data
    signals_json TEXT,  -- Full signals object
    artifacts_json TEXT,  -- Artifact references
    log_excerpt TEXT,
    
    started_at TIMESTAMP,
    finished_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_attestations_commit ON attestations(repo_id, commit_sha);
CREATE INDEX idx_attestations_change ON attestations(repo_id, change_id);
CREATE INDEX idx_attestations_status ON attestations(repo_id, test_status);

-- Suggestions
CREATE TABLE suggestions (
    id INTEGER PRIMARY KEY,
    suggestion_id TEXT UNIQUE NOT NULL,  -- ULID
    repo_id INTEGER REFERENCES repos(id),
    change_id TEXT NOT NULL,
    base_commit_sha TEXT NOT NULL,
    suggested_commit_sha TEXT NOT NULL,
    created_by TEXT,  -- server-agent, george-cli, etc.
    reason TEXT,
    description TEXT,
    confidence REAL,
    status TEXT DEFAULT 'open',  -- open, accepted, rejected, superseded
    diffstat_json TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    resolved_at TIMESTAMP
);

CREATE INDEX idx_suggestions_change ON suggestions(repo_id, change_id);
CREATE INDEX idx_suggestions_status ON suggestions(repo_id, status);

-- Ref movement log (server-side reflog)
CREATE TABLE ref_log (
    id INTEGER PRIMARY KEY,
    repo_id INTEGER REFERENCES repos(id),
    ref TEXT NOT NULL,
    old_sha TEXT,
    new_sha TEXT NOT NULL,
    actor TEXT,
    operation TEXT,  -- push, promote, delete
    is_force BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_ref_log_ref ON ref_log(repo_id, ref, created_at DESC);

-- Keep refs (for object retention)
CREATE TABLE keep_refs (
    id INTEGER PRIMARY KEY,
    repo_id INTEGER REFERENCES repos(id),
    ref_name TEXT NOT NULL,
    commit_sha TEXT NOT NULL,
    workspace TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NOT NULL
);

CREATE INDEX idx_keep_refs_expires ON keep_refs(expires_at);

-- Events (for SSE replay)
CREATE TABLE events (
    id INTEGER PRIMARY KEY,
    event_id TEXT UNIQUE NOT NULL,  -- ULID
    repo_id INTEGER REFERENCES repos(id),
    type TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_events_repo ON events(repo_id, event_id);

-- File index (for file history queries)
CREATE TABLE file_commits (
    id INTEGER PRIMARY KEY,
    repo_id INTEGER REFERENCES repos(id),
    file_path TEXT NOT NULL,
    commit_sha TEXT NOT NULL,
    blob_sha TEXT NOT NULL,
    change_type TEXT,  -- add, modify, delete, rename
    
    UNIQUE(repo_id, file_path, commit_sha)
);

CREATE INDEX idx_file_commits_path ON file_commits(repo_id, file_path);
```

---

## 7. CLI Design (`jul`)

### 7.1 Responsibilities

The `jul` CLI makes sync-by-default feel invisible:

- Guarantee workspace refs are updated on every commit
- Fetch notes and Jul refs automatically
- Provide agent-friendly structured output
- Never silently mutate history

### 7.2 Installation & Configuration

```bash
# Initialize new repo
$ jul init my-project --server https://jul.example.com
Initialized Git repository
Configured Jul remote: https://jul.example.com/my-project.git
Workspace: george/macbook-main

# Configure existing repo
$ cd existing-project
$ jul init --server https://jul.example.com
```

**What `jul init` does**:

1. `git init` (if new)
2. Add remote with correct refspecs:
   ```ini
   [remote "jul"]
       url = https://jul.example.com/my-project.git
       fetch = +refs/heads/*:refs/remotes/jul/*
       fetch = +refs/jul/workspaces/*:refs/jul/workspaces/*
       fetch = +refs/jul/suggest/*:refs/jul/suggest/*
       fetch = +refs/notes/jul/*:refs/notes/jul/*
   ```
3. Install hooks (optional):
   - `commit-msg`: Ensure Change-Id exists
   - `post-commit`: Trigger sync (debounced)
4. Create config:
   ```ini
   # .git/jul.toml
   [workspace]
   user = "george"
   name = "macbook-main"
   
   [sync]
   auto = true
   debounce_ms = 1000
   ```

**Planned: `jul configure` wizard**
- Interactive setup to choose default server, workspace name, and agent provider.
- Writes `~/.config/jul/config.toml` (server + workspace defaults) and `~/.config/jul/agents.toml`.
- Allows selecting a preferred provider (e.g., opencode, codex) and storing credentials.

**Planned: Repo bootstrap**
- `jul init` can optionally create the remote repo on the Jul server (dual-backend flow).
- Requires a simple `POST /api/v1/repos` endpoint with `{name}` and returns repo URL.

### 7.3 Core Commands

#### `jul sync`

Push current HEAD to workspace ref.

```bash
$ jul sync
Pushing to refs/jul/workspaces/george/macbook-main... done
```

**Implementation** (explicit push, not magic refspec):
```bash
git push jul HEAD:refs/jul/workspaces/george/macbook-main
```

With daemon mode:
```bash
$ jul sync --daemon
Watching for commits...
  15:30:01 abc123 synced
  15:30:45 def456 synced
```

#### `jul status`

Show current state with attestation info.

```bash
$ jul status

Workspace: george/macbook-main
Branch: main
Change-Id: Iab4f3c2d1e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b (rev 3)

Sync: ✓ up to date
Published: 2 commits ahead of jul/main

Latest attestation (abc123):
  format:   ⚠ 2 files need formatting
  lint:     ✓ pass (2 warnings)
  compile:  ✓ pass
  test:     ✗ fail (47 passed, 2 failed)
  coverage: 82.5% lines, 71.2% branches

Promotion: blocked
  - test: 2 tests failing
  - format: files need formatting

Suggestions available: 1
  [01HX7Y9A] fix_failing_tests (85% confidence)
```

#### `jul commit`

Wrapper around `git commit` that ensures Change-Id.

```bash
$ jul commit -m "feat: add authentication"

[main abc123] feat: add authentication
Change-Id: Iab4f3c2d1e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b

Syncing... done
CI queued.
```

Flags:
- `-m <message>`: Commit message
- `--amend`: Amend (preserves Change-Id)
- `--no-sync`: Skip auto-sync

#### `jul promote`

Promote workspace to published branch.

```bash
$ jul promote --to main

Checking policy for main...
  ✓ compile: pass
  ✗ test: fail (2 tests failing)
  ✗ format: 2 files need formatting

Promotion blocked. Fix issues or use --force.

$ jul promote --to main --force
⚠ Forcing promotion despite policy violations

Promoted abc123 to main
```

#### `jul changes`

List changes.

```bash
$ jul changes

Draft:
  Iab4f3c2d feat: add authentication (rev 3, 2h ago)
            ✗ test failing

  Icd5e6f7a refactor: extract utils (rev 1, 1d ago)  
            ✓ all passing

Published:
  Ief8a9b0c fix: handle edge case (rev 2, 3d ago)
```

#### `jul interdiff`

Show diff between revisions of a change.

```bash
$ jul interdiff Iab4f3c2d --from 1 --to 3

Interdiff for Iab4f3c2d (rev 1 → rev 3):

src/auth.py:
  @@ -42,6 +42,10 @@
  + def refresh_token(token: str) -> str:
  +     """Auto-refresh expired tokens."""
  +     ...
```

#### `jul query`

Query commits by criteria.

```bash
$ jul query --tests=pass --coverage-min=80 --limit=5

def456 (1d ago) refactor: extract utils
       ✓ tests, 84.1% coverage
       
789abc (3d ago) fix: handle edge case
       ✓ tests, 81.2% coverage
```

#### `jul suggestions`

List and manage suggestions.

```bash
$ jul suggestions

Open suggestions for Iab4f3c2d:

  [01HX7Y9A] fix_failing_tests
             Created by: server-agent
             Confidence: 85%
             Files: +5 -3 in tests/test_auth.py
             
             "Updated test to match new auto-refresh behavior"

  Actions: jul apply 01HX7Y9A | jul diff 01HX7Y9A | jul reject 01HX7Y9A
```

#### `jul apply`

Apply a suggestion.

```bash
$ jul apply 01HX7Y9A

Fetching suggestion...
Showing diff:

tests/test_auth.py:
  @@ -42,8 +42,10 @@
  - def test_validate_token_expired():
  -     with pytest.raises(TokenExpired):
  -         validate_token(expired_token)
  + def test_validate_token_expired_refreshes():
  +     result = validate_token(expired_token)
  +     assert result.refreshed == True

Apply this suggestion? [y/n/edit] y

Cherry-picked suggestion onto workspace.
New commit: ghi789

Syncing... done
CI queued.
```

#### `jul watch`

Watch for events.

```bash
$ jul watch

Watching jul.example.com/my-project...

15:30:01 ref.updated george/macbook-main → abc123
15:30:02 ci.started abc123
15:30:05 ci.finished abc123 (fail: 2 tests)
15:30:06 suggestion.created 01HX7Y9A for Iab4f3c2d

^C
```

### 7.4 Structured Output for Agents

All commands support `--json` for agent consumption:

```bash
$ jul status --json
{
  "workspace": "george/macbook-main",
  "branch": "main",
  "change_id": "Iab4f3c2d...",
  "revision": 3,
  "commit_sha": "abc123...",
  "sync_status": "synced",
  "attestation": {
    "status": "fail",
    "signals": {
      "test": {"status": "fail", "passed": 47, "failed": 2}
    }
  },
  "promotion": {
    "eligible": false,
    "blockers": ["test.status != pass", "format.status != pass"]
  },
  "suggestions": [
    {"suggestion_id": "01HX7Y9A", "reason": "fix_failing_tests"}
  ]
}
```

### 7.5 Agent Integration

The CLI provides a stable interface for coding agents:

```bash
# Get structured context
$ jul context --json
{
  "change_id": "Iab4f3c2d...",
  "diff": "...",
  "failing_tests": [
    {"name": "test_validate_token_expired", "trace": "..."}
  ],
  "lint_warnings": [...],
  "coverage_gaps": [...]
}

# Request suggestion from server
$ jul suggest --reason fix_tests --json
{
  "suggestion_id": "01HX7Y9A...",
  "status": "created"
}

# Wait for and apply
$ jul apply 01HX7Y9A --auto
Applied suggestion 01HX7Y9A
New commit: ghi789
```

---

## 8. Agent Hooks

### 8.1 Overview

Jul supports hooks that can invoke AI agents at key points. The design:

- Server provides context
- Agent (local or server-side) provides intelligence  
- Human (or calling agent) decides what to accept

### 8.2 Hook Configuration

```toml
# .jul/hooks.toml

[hooks]
pre_commit = ["format-fix", "lint-fix"]
post_sync = ["notify"]
on_test_fail = ["diagnose"]
on_conflict = ["resolve"]

[hooks.format-fix]
type = "builtin"
auto_apply = true

[hooks.lint-fix]
type = "agent"
provider = "opencode"
prompt = "Fix lint errors. Only modify code."
auto_apply = true
# Guardrails
max_files = 5
allowed_extensions = [".py", ".ts", ".js"]

[hooks.diagnose]
type = "agent"
provider = "opencode"
prompt = "Analyze test failures and suggest fixes."
auto_apply = false  # Human reviews

[hooks.resolve]
type = "agent"
provider = "opencode"
prompt = "Resolve merge conflict preserving both intents."
auto_apply = false  # NEVER auto-apply conflict resolution
```

### 8.3 Agent Provider Configuration

```toml
# ~/.config/jul/agents.toml

[agents.opencode]
command = "opencode"
protocol = "jul-agent-v1"

[agents.claude-code]
command = "claude"
protocol = "jul-agent-v1"

[agents.custom]
command = "/path/to/my-agent"
protocol = "jul-agent-v1"
```

### 8.4 Agent Protocol (v1)

**Request** (JSON to stdin):
```json
{
  "version": 1,
  "action": "diagnose",
  "context": {
    "repo": "/path/to/repo",
    "commit_sha": "abc123...",
    "change_id": "Iab4f3c2d...",
    "diff": "...",
    "files": [
      {"path": "src/auth.py", "content": "...", "language": "python"}
    ],
    "test_failures": [
      {
        "name": "test_validate_token_expired",
        "file": "tests/test_auth.py",
        "line": 42,
        "message": "AssertionError",
        "trace": "..."
      }
    ],
    "prompt": "Analyze test failures and suggest fixes."
  }
}
```

**Response** (JSON from stdout):
```json
{
  "version": 1,
  "status": "success",
  "analysis": "The test expects TokenExpired but code now auto-refreshes.",
  "patches": [
    {
      "file": "tests/test_auth.py",
      "hunks": [
        {
          "start_line": 42,
          "end_line": 45,
          "content": "def test_validate_token_expired_refreshes():\n    result = validate_token(expired_token)\n    assert result.refreshed == True"
        }
      ]
    }
  ],
  "confidence": 0.85
}
```

### 8.5 Guardrails

Even for personal use, agent hooks have guardrails:

```toml
[hooks.guardrails]
# Limit scope
max_files_per_hook = 10
max_lines_changed = 500
allowed_file_extensions = [".py", ".ts", ".js", ".go", ".rs"]

# Safety
never_auto_apply = ["resolve"]  # Conflicts always need human
require_clean_state = true      # Don't run on dirty workdir
require_passing = ["format", "lint"]  # Must pass before auto-apply
```

---

## 9. Example Workflows

### 9.1 Agent Development Loop

```bash
# Agent starts coding
$ claude-code "implement user authentication"

# Agent makes commits (auto-synced via post-commit hook)
# Each gets a Change-Id, pushed to workspace ref
# Server runs CI on each push

# Human checks in
$ jul status
  ✗ test (2 failed)
  
$ jul suggestions
  [01HX7Y9A] fix_failing_tests (85% confidence)

# Review and apply suggestion
$ jul diff 01HX7Y9A
$ jul apply 01HX7Y9A

# CI runs again, passes
$ jul status
  ✓ all passing
  Promotion: eligible

# Promote
$ jul promote --to main
Promoted to main
```

### 9.2 Multi-Device Workflow

```bash
# On laptop
$ jul commit -m "start feature"
Synced to george/laptop-feature

# On desktop (later)  
$ git fetch jul
$ jul checkout george/laptop-feature
$ jul commit -m "continue feature"
Synced to george/desktop-feature

# Merge when ready
$ git checkout main
$ git merge george/desktop-feature
$ jul promote --to main
```

### 9.3 Time Travel (Finding Last Green)

```bash
# Tests broken, need last good state
$ jul query --tests=pass --limit=1
  def456 (1d ago) ✓ tests

# See what changed since then
$ jul interdiff Iab4f3c2d --from def456

# Or use the reflog
$ jul reflog george/macbook-main
  15:30 abc123 (current, tests failing)
  14:45 def456 (tests passing)
  14:30 999aaa (tests passing)

# Checkout old state
$ git checkout def456
```

---

## 10. Minimal Viable Cut (v1)

The smallest slice that feels useful:

### Must Have

1. **Git hosting** via smart HTTP (`git-http-backend`)
2. **Workspace refs** (`refs/jul/workspaces/...`) + CLI auto-sync
3. **Keep refs** for object retention (or gc will eat your history)
4. **CI runner** that writes attestations to SQLite
5. **SSE events** for `ci.finished` + `ref.updated`
6. **`jul status`** showing sync state + attestation
7. **`jul promote`** with basic policy checks

### Nice to Have (v1.1)

8. Suggestions as refs + API
9. Git notes mirroring of attestations
10. `jul interdiff` 
11. Web UI for browsing

### Later (v2+)

12. Server-side AI agent for auto-suggestions
13. Full hook system with agent integration
14. Workspace snapshots (Dropbox mode)
15. Multi-user / teams

---

## 11. Deployment

### 11.1 Directory Structure

```
/var/jul/
├── repos/                    # Bare Git repositories
│   └── my-project.git/
│       └── hooks/
│           └── post-receive
├── data/
│   └── jul.db               # SQLite
├── artifacts/               # CI artifacts
├── spool/                   # Hook event spool
│   └── events.jsonl
├── config/
│   └── jul.toml
└── logs/
```

### 11.2 Configuration

```toml
# /var/jul/config/jul.toml

[server]
host = "0.0.0.0"
port = 8000
base_url = "https://jul.example.com"

[storage]
repos_dir = "/var/jul/repos"
database = "/var/jul/data/jul.db"
artifacts_dir = "/var/jul/artifacts"
spool_dir = "/var/jul/spool"

[retention]
keep_days = 30

[ci]
workers = 2
timeout_seconds = 300
sandbox = "container"  # or "none" for personal use

[ci.stages]
format = {enabled = true, check_only = true}
lint = {enabled = true}
compile = {enabled = true}
test = {enabled = true}
coverage = {enabled = true}

[policy.default]
required_checks = ["compile", "test"]
min_coverage_pct = 0  # No minimum for personal use

[auth]
htpasswd_file = "/etc/jul/htpasswd"
```

### 11.3 Quick Start

```bash
# Install
pip install jul-server

# Initialize
jul-server init /var/jul

# Create first repo
jul-server create-repo my-project

# Run
jul-server run
```

---

## 12. Glossary

| Term | Definition |
|------|------------|
| **Attestation** | Structured CI/test/coverage results attached to a commit |
| **Change** | A logical unit of work with stable identity across rewrites |
| **Change-Id** | Stable identifier (`Iab4f...`) preserved across amend/rebase |
| **Interdiff** | Diff between two revisions of the same change |
| **Keep ref** | Ref that prevents GC of old commits after force-push |
| **Promotion** | Policy-controlled update of a published branch |
| **Revision** | A specific commit SHA within a change's history |
| **Suggestion** | Agent-proposed patch stored as immutable ref |
| **Workspace** | Named sync target for a device/context |

---

## Appendix A: Why Not Just Use X?

| Alternative | Why Jul is different |
|-------------|---------------------|
| **GitHub/GitLab** | No sync-by-default, no change-centric history, CI is bolted on |
| **Gerrit** | Change-centric but complex, not agent-native, no sync refs |
| **JJ alone** | Great VCS but needs a server for backup/CI/sharing |
| **Dropbox + Git** | No merge safety, no CI, no change identity |
| **Plain Git + hooks** | No queryable attestations, no change tracking, no suggestions |

Jul combines: Git compatibility + sync-by-default + change identity + first-class attestations + agent-native queries.

---

## Appendix B: References

- [git-http-backend](https://git-scm.com/docs/git-http-backend) — Smart HTTP server
- [git-notes](https://git-scm.com/docs/git-notes) — Attach metadata without changing commits
- [JJ (Jujutsu)](https://github.com/jj-vcs/jj) — Git-compatible VCS with change identity
- [Gerrit Change-Id](https://gerrit-review.googlesource.com/Documentation/user-changeid.html) — Stable change identity spec
- [JJ + Gerrit integration](https://docs.jj-vcs.dev/latest/gerrit/) — How JJ handles Change-Id
