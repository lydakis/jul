# Jul Performance & Scalability Specification

**Project:** Jul (AI-First Git Workflow)  
**Spec Version:** 0.3  
**Status:** Draft  
**Applies To:** Jul Core + Daemon + Sync + Traces + Notes + Review/Agent Sandbox (Performance Only)

---

## 0. Purpose

This document defines **performance goals, budgets, and benchmark scenarios** for Jul so that:

- Day-to-day commands feel “instant” on typical repos.
- Large repos remain usable (progressive degradation, not cliff-falls).
- Background behavior (daemon, draft checks, review agent sandboxing) stays **bounded** in CPU, memory, disk, and network.
- Performance regressions are caught early via a repeatable benchmark suite.

This is a **performance spec**, not a correctness spec. Functional behavior is covered by the integration test specifications.

---

## 1. Scope and Non-Goals

### 1.1 In Scope

- Local command latency and overhead for:
  - `jul init`, `jul status`, `jul sync`, `jul trace`, `jul checkpoint`, `jul ws *`, `jul log`, `jul diff`, `jul blame`, `jul suggestions`, `jul promote`, `jul prune`
- Daemon behavior: debouncing, coalescing, CPU/memory bounds, backoff.
- Git object/ref scalability:
  - Workspaces, per-device sync refs, trace refs, keep-refs, notes refs.
- Notes scalability and merge cost (append-only NDJSON + union merges).
- Agent sandbox (worktree) creation/reuse performance.
- Secret scanning overhead for draft push safety.
- Remote sync performance characteristics (refs count, bytes pushed, pack generation on localhost remote).

### 1.2 Not in Scope

- Speed of the project’s own tests/lints/coverage (Jul may trigger them, but they are not “Jul performance”).
  - We do define **Jul overhead budgets excluding checks**.
- Cloud CI performance variance.
- Real-world WAN variance (we measure deterministic overhead and model transport under controlled conditions).
- Micro-optimizing Git itself.

---

## 2. Measurement Model

### 2.1 Repo Tiers

Performance goals are defined for three repo tiers. Every tier must be tested in at least two “delta modes”: **small delta** and **large delta**.

| Tier | Typical Repo Shape | History/Metadata Shape |
|---|---|---|
| **S (Small)** | 1k–10k files, ≤ 1 GB working tree | ≤ 10k commits, ≤ 1k checkpoints, ≤ 5k traces |
| **M (Medium)** | 10k–80k files, 1–5 GB working tree | ≤ 100k commits, ≤ 5k checkpoints, ≤ 25k traces |
| **L (Large)** | 80k–300k files, 5–25 GB working tree | ≤ 1M commits, ≤ 10k checkpoints, ≤ 100k traces |

**Delta modes (changed paths/blobs in the working tree):**

- **ΔSmall:** ≤ 200 changed paths AND ≤ 50 MB changed blobs
- **ΔLarge:** ≥ 2,000 changed paths OR ≥ 500 MB changed blobs

### 2.2 Machine Tiers

- **Dev Laptop:** 8-core class CPU, SSD/NVMe, 16–32 GB RAM (macOS/Linux).
- **CI Runner:** 2–4 vCPU, networked SSD, 7–16 GB RAM (Linux).

Unless explicitly overridden, budgets are specified for **Dev Laptop**; **CI Runner** is allowed +50% time for S/M and +100% time for L.

### 2.3 Remote Modes

Some commands include Git transport (fetch/push) where cost varies with pack negotiation, pack size, and remote implementation. Benchmarks MUST separate **pure Jul overhead** from **Git transport**.

Remote modes for perf tests:

- **Remote Mode: `none`** — no remote configured; `jul sync` snapshots locally only.
- **Remote Mode: `local`** — local bare repo used as sync/publish remote (same machine). Use a stable protocol (e.g., `file://` or direct path) and record Git version.
- **Remote Mode: `controlled` (optional)** — localhost remote + traffic shaping (latency/bandwidth) for long-term realism. Not required for CI gating.

Budgets in this spec assume `none` or `local` unless stated.

### 2.4 Cache States (“Warm” vs “Cold”)

Benchmarks MUST run in named cache states so results are reproducible.

- **Warm:**
  - `.jul/` present
  - shadow index present
  - same repo directory
  - command is run at least once immediately before measurement

- **Jul-cache-cold:**
  - `.jul/` removed (or at least shadow index + caches removed)
  - `.git/` untouched
  - repo directory is the same (OS file cache may still be warm)

- **Clone-cold:**
  - fresh clone into a new directory
  - no `.jul/`
  - `jul init` executed (or implicitly triggered) before the measured command

- **Disk-cold (optional):**
  - best-effort attempt to clear OS disk cache (Linux only, privileged)
  - not required for CI gating

**When this spec says “cold” without qualifier**, it refers to **Clone-cold**.

### 2.5 Metrics

For each command and test scenario record:

- **Latency:** P50 and P95 wall-clock time
- **CPU:** CPU-seconds, peak CPU%
- **Memory:** peak RSS
- **Disk I/O:** optional (bytes read/written) but recommended for Tier L
- **Sync/remote:** refs updated count, notes updated count, pack bytes pushed/fetched

### 2.6 Reporting Requirements

All agent-usable commands MUST support `--json` and include:

```json
{
  "timings_ms": {
    "total": 1234,
    "phase": {
      "sync_fetch": 120,
      "draft_snapshot": 340,
      "secret_scan": 40,
      "notes_merge": 120,
      "push": 200
    }
  },
  "resources": {
    "rss_peak_bytes": 123456789,
    "cpu_seconds": 0.42
  },
  "io": {
    "bytes_read": 0,
    "bytes_written": 0
  },
  "remote": {
    "mode": "local",
    "pack_bytes_pushed": 1048576,
    "pack_bytes_fetched": 0,
    "refs_updated": 5,
    "notes_objects_written": 12
  },
  "degraded": false,
  "degradation": null
}
```

### 2.7 P50/P95 Ratio Sanity Checks

P95 is your “tail pain.” P50 is your “what does this feel like all day.” Both matter.

Expected variance behavior:

- **“Instant read-only” commands** (`status`, `log`, `suggestions`):
  - P50 can be extremely small; OS scheduler jitter and occasional cache misses can inflate P95.
  - **Ratio expectation:** P95/P50 up to ~4× is acceptable if absolute P95 remains within budget.

- **“Work proportional to delta” commands** (`sync`, `checkpoint` overhead, small `diff`):
  - Variance should mostly track the delta and filesystem behavior.
  - **Ratio expectation:** P95/P50 typically ≤ 3×; if > 3×, investigate cache invalidation, ref enumeration, or contention.

- **“Clamped / degraded” commands** (`blame`, huge `diff`, `prune`):
  - Tail variance should be **suppressed by design** because Jul should clamp the work and degrade.
  - **Ratio expectation:** P95/P50 can be relatively tight (≤ 2×) if degradation triggers reliably.
  - If ratios widen significantly, it’s often a sign that degradation stopped triggering.

---

## 3. Performance Principles

1. **Separate Jul Overhead From User Tooling Time.**  
   For commands that run checks or agents, report timing splits:
   - core Jul work (Git plumbing, metadata, sync)
   - checks runtime (external tools)
   - agent runtime (LLM + local execution)

2. **Scale With the Delta, Not the Repo.**  
   In steady-state, draft snapshotting must be proportional to changed paths/blobs.

3. **Bound Background Work.**  
   The daemon must coalesce bursts, avoid spin loops, and cap concurrency.

4. **No 30-Second Surprise Stalls.**  
   If an operation might take “a long time,” it must:
   - show progress quickly
   - be cancellable
   - degrade to a cheaper answer when reasonable

5. **One Fetch + One Push Per User Action (When Possible).**  
   Sync should batch ref and notes updates.

---

## 4. Command Budgets (Latency)

**Unless otherwise noted:**
- Budgets are **Jul overhead only** (exclude project tests/lints/coverage and agent runtime).
- Budgets assume **Remote Mode: `none`** for pure local overhead.
- For **Remote Mode: `local`**, see the transport addenda in the relevant command sections.

### 4.1 `jul init`

**Goal:** “Does not feel heavy; safe to run in CI.”

- Tier S: P50 ≤ 250 ms, P95 ≤ 800 ms (warm repo), Clone-cold P95 ≤ 2.0 s
- Tier M: P50 ≤ 500 ms, P95 ≤ 1.5 s (warm repo), Clone-cold P95 ≤ 4.0 s
- Tier L: P50 ≤ 1.0 s, P95 ≤ 3.0 s (warm repo), Clone-cold P95 ≤ 8.0 s

Notes:
- If remote probing occurs (`jul doctor`-like behavior), report that separately in timings.

### 4.2 `jul status`

**Goal:** “Instant status.”

**Dev Laptop budgets:**

| Tier | Warm P50 | Warm P95 | Jul-cache-cold P95 | Clone-cold P95 |
|---|---:|---:|---:|---:|
| S | 25 ms | 80 ms | 150 ms | 250 ms |
| M | 50 ms | 150 ms | 250 ms | 400 ms |
| L | 120 ms | 300 ms | 600 ms | 900 ms |

Requirements:
- Must not fetch publish remote.
- Must not trigger a sync if `sync.mode = explicit`.

### 4.3 `jul sync` (Draft Snapshot + Optional Remote Push)

Measure and budget **two separate totals**:

1. **Local snapshot only** (Remote Mode: `none`).
2. **Snapshot + local remote transport** (Remote Mode: `local`).

#### 4.3.1 Local Snapshot Only (Remote Mode: `none`)

**Incremental sync (ΔSmall):**

| Tier | Warm P50 | Warm P95 | Jul-cache-cold P95 | Clone-cold P95 |
|---|---:|---:|---:|---:|
| S | 300 ms | 1.0 s | 2.0 s | 3.0 s |
| M | 800 ms | 2.5 s | 5.0 s | 8.0 s |
| L | 1.8 s | 5.0 s | 15 s | 25 s |

**Full rebuild sync** (shadow index rebuild or equivalent):

| Tier | Warm P50 | Warm P95 |
|---|---:|---:|
| S | 1.5 s | 5 s |
| M | 6 s | 20 s |
| L | 20 s | 60 s |

Full rebuild requirements:
- Must show progress (phase + spinner) within 250 ms.
- Must be cancellable without corrupting `.jul/` state.

#### 4.3.2 Transport Addendum (Remote Mode: `local`)

Git transport cost is still variable, but on a local bare remote it is stable enough to budget by **pack size**.

For incremental sync (ΔSmall), add:

- pack ≤ 1 MB: P95 +150 ms
- pack ≤ 20 MB: P95 +500 ms
- pack ≤ 200 MB: P95 +2.0 s (and must show “Pushing…” progress)

If pack > 200 MB, Jul must:
- warn “Large push (N MB)”
- show progress
- remain cancellable

#### 4.3.3 Secret Scan Budget

Secret scanning must not become a hidden cliff:

- For ΔSmall with mostly text files: add ≤ 150 ms P95
- If changed blobs include large binaries, Jul may take longer **but must surface the scan** (“Scanning changed blobs for secrets…”) and block draft push unless scan completes or the user explicitly overrides.

### 4.4 `jul trace`

- Tier S/M: Warm P50 ≤ 80 ms, Warm P95 ≤ 250 ms
- Tier L: Warm P50 ≤ 150 ms, Warm P95 ≤ 500 ms

Requirements:
- Trace creation must be O(changed paths).
- Must not create one ref per trace (single tip ref per workspace).

### 4.5 `jul checkpoint` (Overhead Only)

“Locking should feel quick; checks can take time.”

| Tier | Warm P50 | Warm P95 | Clone-cold P95 |
|---|---:|---:|---:|
| S | 250 ms | 800 ms | 2.0 s |
| M | 500 ms | 1.5 s | 4.0 s |
| L | 1.0 s | 3.0 s | 8.0 s |

Includes:
- flush final trace (if needed)
- write checkpoint commit
- update refs/notes/keep-refs
- start new draft metadata

Excludes:
- running checks and review agent runtime

### 4.6 `jul ws checkout` / `jul ws switch` / `jul ws restack`

These commands touch the working tree; budgets are primarily “Git checkout + Jul metadata.”

**Checkout / Switch (clean working tree):**

- Tier S: Warm P50 ≤ 600 ms, Warm P95 ≤ 2.0 s
- Tier M: Warm P50 ≤ 2.0 s, Warm P95 ≤ 6.0 s
- Tier L: Warm P50 ≤ 6.0 s, Warm P95 ≤ 20 s (progress required)

**Restack (no conflicts, ≤ 10 checkpoints ahead):**

- Tier S: Warm P50 ≤ 600 ms, Warm P95 ≤ 2.0 s
- Tier M: Warm P50 ≤ 2.0 s, Warm P95 ≤ 6.0 s
- Tier L: Warm P50 ≤ 6.0 s, Warm P95 ≤ 20 s

If conflicts occur, strict latency budgets do not apply, but Jul MUST:
- detect and report conflicts within 1.0 s after the conflict is first observed
- leave state consistent and recoverable

### 4.7 `jul log`

Default view (last 50 checkpoints):

- Tier S/M: Warm P50 ≤ 80 ms, Warm P95 ≤ 250 ms
- Tier L: Warm P50 ≤ 250 ms, Warm P95 ≤ 800 ms

Requirements:
- Pagination must be the default for large histories.

### 4.8 `jul diff`

Draft vs base, or Change-Id net diff where diff touches ≤ 200 files:

- Tier S: Warm P50 ≤ 300 ms, Warm P95 ≤ 1.0 s
- Tier M: Warm P50 ≤ 800 ms, Warm P95 ≤ 2.5 s
- Tier L: Warm P50 ≤ 2.0 s, Warm P95 ≤ 6.0 s

For massive diffs:
- must stream output progressively
- must offer a fast `--stat` path
  - Tier S/M: Warm P95 ≤ 500 ms
  - Tier L: Warm P95 ≤ 1.5 s

### 4.9 `jul blame` (Trace-Aware)

Blame is inherently variable; the goal is **bounded work + useful fallbacks**, not heroics.

**Without trace-index:**

- Tier S/M, file ≤ 1,000 LOC, trace chain ≤ 200 commits:
  - Warm P50 ≤ 1.4 s, Warm P95 ≤ 2.5 s
- Tier L:
  - Warm P50 ≤ 3.0 s, Warm P95 ≤ 6.0 s

**With trace-index:**

- Tier S/M:
  - Warm P50 ≤ 500 ms, Warm P95 ≤ 1.0 s
- Tier L:
  - Warm P50 ≤ 1.2 s, Warm P95 ≤ 2.5 s

If expected work exceeds thresholds:
- show a “Collecting blame (phase X/Y)…” indicator within 200 ms
- degrade to `--no-trace` (checkpoint-level blame; no trace attribution)

Fallback goals:
- Tier S/M: Warm P95 ≤ 1.0 s
- Tier L: Warm P95 ≤ 2.5 s

Note: This fallback can be faster than trace-aware blame because it avoids trace attribution work.

### 4.10 `jul suggestions`

List pending suggestions for the current change/checkpoint.

- Tier S/M: Warm P50 ≤ 50 ms, Warm P95 ≤ 200 ms
- Tier L: Warm P50 ≤ 150 ms, Warm P95 ≤ 500 ms

Requirements:
- Must not load or render all suggestions unbounded; default limit and pagination required.

### 4.11 `jul promote` (Overhead Only)

Promotion includes: fetch publish tip, create published commits (rebase/squash/merge), update mappings/notes, push published branch, create workspace base marker.

| Tier | Warm P50 | Warm P95 | Clone-cold P95 |
|---|---:|---:|---:|
| S | 1.5 s | 5 s | 8 s |
| M | 3 s | 10 s | 18 s |
| L | 7 s | 25 s | 45 s |

Excludes:
- project tests (if re-run at promote time)
- agent review runtime
- WAN variance (use Remote Mode: `local` or controlled)

Transport addendum (Remote Mode: `local`):
- pack ≤ 20 MB: add ≤ 1.0 s P95
- pack ≤ 200 MB: add ≤ 4.0 s P95 (progress required)

### 4.12 `jul prune`

`jul prune` may touch many refs/notes. It MUST be safe at scale.

- Tier S/M: Warm P50 ≤ 500 ms, Warm P95 ≤ 3.0 s
- Tier L: Warm P50 ≤ 1.5 s, Warm P95 ≤ 8.0 s

If there is more work than can be completed within the budget, `jul prune` MUST:
- show progress
- time-slice (do “some” work, then stop cleanly)
- be resumable (running it again continues)

---

## 5. Per-Command Memory Budgets

Budgets below are **peak RSS** for Jul + Git subprocesses invoked directly by Jul.

| Command | Soft Cap | Hard Cap | Notes |
|---|---:|---:|---|
| `jul status` | 50 MB | 150 MB | Must not allocate proportional to repo size |
| `jul log` | 80 MB | 250 MB | Pagination required |
| `jul suggestions` | 80 MB | 250 MB | Pagination required |
| `jul trace` | 80 MB | 250 MB | O(delta) |
| `jul sync` | 200 MB | 800 MB | Shadow index + write-tree are main costs |
| `jul checkpoint` (overhead) | 200 MB | 800 MB | Includes notes/keep-refs updates |
| `jul diff` | 350 MB | 1.2 GB | Must stream for huge diffs |
| `jul blame` | 500 MB | 1.5 GB | Must clamp/degrade without index |
| `jul promote` (overhead) | 350 MB | 1.2 GB | Rebase/squash work scales with stack |
| `jul prune` | 300 MB | 1.0 GB | Must time-slice large deletions |
| Daemon steady state | 60 MB | 120 MB | Must settle after bursts |

**Hard cap rule:** Jul must not rely on the OS to OOM-kill the process. See Section 6.6 for required behavior when approaching hard caps.

---

## 6. Degradation and Fallback Policy

### 6.1 Principles

1. **Never silently drop safety.**
   - Secret scanning, promote policies, and conflict safety may not be skipped without explicit user intent.

2. **Degradation must be explicit and inspectable.**
   - Human output must say what was degraded and why.
   - `--json` must set `degraded=true` and include a machine-parseable `degradation` object.

3. **Prefer “cheaper but correct” over “fancy but maybe wrong.”**
   - Especially for provenance (`blame`).

### 6.2 Enforced Default Thresholds

These are defaults (configurable), but **must be enforced** in default builds.

- `blame.max_trace_scan_commits = 500`
- `blame.max_candidate_traces_per_line = 20`
- `diff.max_files_full_patch = 2_000`
- `diff.max_patch_bytes = 50_MB` (beyond this, default to `--stat` unless `--full`)
- `sync.full_rebuild_warn_after_ms = 5_000`
- `sync.secret_scan_max_file_bytes = 5_MB` (above: treat as “unscannable” and block draft push by default)
- `prune.max_wall_time_ms = 10_000` (time-slice)
- `prune.max_deletes_per_run = 5_000`

### 6.3 Required Degraded Behaviors

#### 6.3.1 `jul blame`

If trace scan would exceed thresholds:
- return checkpoint/commit-level blame (equivalent to `--no-trace`)
- include a message:
  - “Trace attribution degraded: scanned last N traces (limit M). Use `--slow` to attempt deeper scan or enable trace-index.”
- JSON must include:

```json
{
  "degraded": true,
  "degradation": {
    "kind": "blame_trace_clamp",
    "max_trace_scan_commits": 500,
    "scanned": 500,
    "fallback": "no-trace"
  }
}
```

#### 6.3.2 `jul diff`

If diff would exceed `diff.max_patch_bytes` or `diff.max_files_full_patch`:
- default output becomes `--stat` (or `--name-only`) with a clear warning
- user can force full patch with `jul diff --full` (explicit intent)

#### 6.3.3 `jul sync`

If sync becomes a full rebuild or crosses the warning threshold:
- show progress within 250 ms (phase names are acceptable)
- emit a warning after `sync.full_rebuild_warn_after_ms`

Secret scan safety:
- If changed files include blobs larger than `sync.secret_scan_max_file_bytes`, Jul must:
  - treat the draft push as **blocked** by default
  - report which paths triggered the block
  - recommend remediation: add to `.jul/syncignore`, remove the blob, or use an explicit override (`--allow-secrets`)

#### 6.3.4 `jul promote`

If promote involves a large pack push or a deep stack:
- show progress for:
  - fetching
  - rebasing/squashing
  - pushing
- must not silently skip promote policy checks (unless user passes `--no-policy`)

#### 6.3.5 `jul prune`

If prune work exceeds time or delete thresholds:
- prune performs a bounded chunk of work
- exits successfully with a “partial completion” status
- next run continues

JSON example:

```json
{
  "degraded": true,
  "degradation": {
    "kind": "prune_timeslice",
    "deleted_refs": 5000,
    "remaining_estimate": 95000,
    "next_action": "jul prune --continue"
  }
}
```

### 6.4 “First Output” Requirements

For operations that can take seconds or more (`sync` rebuild, big `diff`, `blame`, large `promote`, large `prune`):
- user must see an initial phase/progress indicator within **200 ms**

### 6.5 Cancellation Safety

All long operations must be cancellable (SIGINT / Ctrl-C) with these guarantees:
- No corrupted `.jul/` state (shadow index writes must be atomic)
- No partially-written ref updates (use atomic ref updates / `--force-with-lease` semantics)

### 6.6 Hard Memory Cap Consequences

If Jul detects (or predicts) it is approaching a hard cap:

1. **Attempt a cheaper plan first** (streaming, smaller batch size, clamped scan).
2. If still likely to exceed hard cap:
   - abort the operation **before** OOM
   - return a structured error
   - leave state consistent

Minimum requirements:
- exit code must be non-zero and stable (`JUL_E_RESOURCE_LIMIT` or equivalent)
- JSON must include:

```json
{
  "error": {
    "code": "resource_limit",
    "message": "Memory hard cap exceeded while computing diff.",
    "soft_cap_bytes": 367001600,
    "hard_cap_bytes": 1288490188,
    "suggested_flags": ["--stat", "--full --no-limits"]
  }
}
```

---

## 7. Daemon Performance Goals

### 7.1 Idle/Steady State

- CPU: ≤ 0.5% average while idle (no changes), with occasional wakeups acceptable.
- Memory: ≤ 60 MB RSS steady state.
- Disk writes: no periodic writes when idle (except minimal logs).

### 7.2 Burst Handling (File Change Storms)

With 10,000 file events over ~10 seconds (e.g., dependency install):

- No more than **1 sync per debounce window**.
- No more than **1 sync in flight**.
- Must not sync `.jul/**` (avoid infinite loops).

**“Settle quickly” definition:**
- Within **10 seconds** after the last non-ignored file event:
  - daemon CPU usage must fall below **1% average**
  - and there must be **no additional sync attempts** unless new file events occur

### 7.3 Remote Failure Backoff

Under remote unavailability or push rejection:

- Exponential backoff with jitter
- Minimum retry interval: 2 seconds
- Maximum retry interval: 5 minutes
- Must not hammer remote under repeated failures

### 7.4 Observability

Daemon must log:
- sync attempts (phase timings)
- errors with actionable codes
- backoff state

---

## 8. Refs and Notes Scalability Goals

### 8.1 Refs

Goals:
- No ref explosion from traces (single tip ref per workspace).
- Keep-refs only at checkpoint boundaries.
- Sync refs: one per device per workspace.

Target upper bounds (Tier L stress):
- Workspaces: 1–200
- Checkpoints per Change-Id: 1–200
- Total checkpoints: up to 10,000
- Total trace commits: up to 100,000

`jul sync` and other commands must not enumerate all refs in a way that is linear in total ref count.

### 8.2 Notes

Notes payload caps (synced notes):
- ≤ 16 KB per note entry

Notes merge:
- append-only NDJSON notes must merge in time proportional to newly appended lines

Targets:
- merging notes refs with 100k events should complete within:
  - Tier M: ≤ 3 seconds
  - Tier L: ≤ 10 seconds

**Term: trace-index**
- A per-trace metadata note (e.g., `refs/notes/jul/trace-index`) containing small hints like changed paths/hunk hashes.
- Used to speed up `jul blame` by avoiding full-tree diffs.

---

## 9. Agent Sandbox (Worktree) Performance

Internal review and agentic merge resolution use an isolated Git worktree.

Goals:

- First-time creation of agent worktree (Clone-cold):
  - Tier S: P95 ≤ 5 seconds
  - Tier M: P95 ≤ 15 seconds
  - Tier L: P95 ≤ 60 seconds (progress required)

- Subsequent reuse (Warm):
  - updating to a new checkpoint: ≤ 2 seconds (S/M), ≤ 8 seconds (L)

Rules:
- must reuse worktree when possible
- must never leak `.jul/agent-workspace/**` into draft snapshot

---

## 10. Benchmark Suite (Perf Tests)

Performance tests are split into:
- **Microbenchmarks:** one operation in isolation (e.g., notes merge).
- **Scenario benchmarks:** realistic workflows (edit → sync → checkpoint → promote).
- **Stress tests:** extreme but bounded (ref counts, trace chain length, file storms).

### 10.1 Naming

Use IDs like: `PT-SYNC-001`, `PT-CHECKPOINT-002`, `PT-DAEMON-003`.

### 10.2 Core Benchmarks

#### PT-STATUS-001 — Warm Status
- Repo: Tier S/M/L
- State: Warm
- Run: `jul status` 50x
- Assert: P50/P95 within budgets

#### PT-STATUS-002 — Clone-Cold Status
- Repo: Tier S/M/L
- State: Clone-cold (fresh clone + `jul init`)
- Run: `jul status`
- Assert: within clone-cold budgets

#### PT-SYNC-001 — Warm Incremental Sync (Remote Mode: none)
- Repo: Tier S/M/L
- Delta: ΔSmall
- Run: `jul sync` 20x
- Assert: P50/P95 within local snapshot budgets

#### PT-SYNC-002 — Full Rebuild Sync (Remote Mode: none)
- Invalidate caches (remove shadow index / `.jul/` parts)
- Run: `jul sync`
- Assert: within full rebuild budgets + progress visible

#### PT-SYNC-003 — Incremental Sync + Local Transport (Remote Mode: local)
- Repo: Tier S/M/L
- Delta: ΔSmall
- Run: `jul sync` 20x with a local bare remote
- Assert: transport addendum budgets (by pack size) + phase timings reported

#### PT-SYNC-004 — First Sync After Clone (Remote Mode: local)
- Repo: Tier S/M/L
- State: Clone-cold
- Run: `jul init` then `jul sync`
- Assert: within clone-cold budgets; no pathological full rebuild loops

#### PT-CHECKPOINT-001 — Warm Checkpoint Overhead
- Checks/review disabled or mocked
- Delta: ΔSmall
- Run: `jul checkpoint -m "msg"`
- Assert: within budgets; no extra background work spawned when checks/review are disabled (for example `--no-ci --no-review`)

#### PT-CHECKPOINT-002 — Clone-Cold Checkpoint Overhead
- State: Clone-cold
- Run: `jul init`, make small edit, `jul checkpoint -m "msg"`
- Assert: within clone-cold budgets

#### PT-PROMOTE-001 — Warm Promote Overhead (Remote Mode: local)
- 3 checkpoints, rebase strategy
- Local publish remote (bare repo)
- Assert: budgets met; timings split includes fetch/rewrite/push

#### PT-PROMOTE-002 — Clone-Cold Promote Overhead (Remote Mode: local)
- State: Clone-cold
- Create minimal checkpoints and promote
- Assert: clone-cold promote budgets

#### PT-DIFF-001 — Warm Small Diff
- Delta: ≤ 200 files
- Run: `jul diff`
- Assert: within budgets

#### PT-DIFF-003 — Diff Degradation Triggers
- Delta: ≥ `diff.max_patch_bytes` OR ≥ `diff.max_files_full_patch`
- Run: `jul diff`
- Assert:
  - output defaults to stat/name-only
  - JSON sets `degraded=true` with correct `degradation.kind`

#### PT-BLAME-001 — Blame Without Trace Index
- Vary trace chain length: 200 / 2k / 10k
- Assert: bounded scan; fallback is available

#### PT-BLAME-002 — Blame With Trace Index
- Same as PT-BLAME-001, but with trace-index populated
- Assert: improved budgets

#### PT-BLAME-003 — Blame Degradation Triggers
- Configure or construct trace chain > `blame.max_trace_scan_commits`
- Run: `jul blame <file>`
- Assert:
  - clamp occurs
  - JSON includes `degraded=true` and clamp metadata
  - `--no-trace` meets ≤ 1.0 s P95

#### PT-NOTES-001 — Notes Merge Smoke (10k events)
- Preload notes with 10k NDJSON events
- Sync notes with +1k events
- Assert: merge time within Tier S/M budgets

#### PT-NOTES-002 — Notes Merge Scale (100k events)
- Preload 100k
- Sync +1k
- Assert: merge time within Tier M/L targets

#### PT-PRUNE-001 — Prune Small
- 1k expired keep-refs
- Run: `jul prune`
- Assert: within budgets

#### PT-PRUNE-002 — Prune Large (100k orphans)
- 100k expired refs/notes entries
- Run: `jul prune`
- Assert:
  - time-slicing triggers
  - progress is shown
  - operation is resumable

#### PT-DAEMON-001 — Idle Daemon
- Start daemon, no file changes for 10 minutes
- Assert: CPU/memory budgets; no periodic disk churn

#### PT-DAEMON-002 — File Storm Coalescing + Settle
- Generate 10k file events
- Assert:
  - at most 1 sync per debounce window
  - settles within 10 seconds (as defined in Section 7.2)

#### PT-SUGGESTIONS-001 — Suggestions Listing
- Populate repo with 1k suggestions (notes)
- Run: `jul suggestions`
- Assert: pagination, budgets met

### 10.3 Degradation Coverage Requirement

Every degradation rule in Section 6 MUST have at least one perf test that:
- forces the threshold
- asserts the fallback behavior
- asserts structured reporting (`degraded=true`)

---

## 11. Regression Gates

For Tier S and M benchmarks (CI gating):

- Fail if P95 exceeds absolute budget by > 10%
- Fail if P50 regresses by > 25% vs baseline
- Fail if P95 regresses by > 20% vs baseline

For Tier L benchmarks:
- Warn on regressions > 25%
- Fail only for catastrophic regressions (> 50% or hard timeouts)

Memory regression gates:
- Warn if soft cap exceeded by > 25%
- Fail if hard cap is exceeded

Baselines must be versioned by:
- OS + Git version + Jul build flags
- repo fixture hash

---

## 12. Instrumentation and Profiling

### 12.1 `--json` Timing Fields

Commands used by agents MUST include `timings_ms.total` and per-phase breakdown.

### 12.2 Debug Timings

- `--debug-timings` prints per-phase timings in human output

### 12.3 Profiling Toggles

Optional profiling toggles:
- `JUL_PROFILE=cpu`
- `JUL_PROFILE=mem`

Profiling must be off by default and must not leak secrets.

---

## 13. Provisional Decisions (v0.3)

These are explicit stances to prevent the spec from becoming a TODO list.

1. **v1 allows full rebuild sync**, but it must be bounded (≤ 60s Tier L) and visible (progress + cancel). Incremental shadow-index optimizations may ship later.

2. **`trace-index` is optional in v1.**
   - If present, `jul blame` must use it.
   - If absent, `jul blame` must clamp and degrade (never unbounded scans).

3. **Daemon does not attempt “smart incremental indexing” beyond debouncing in v1.**
   - Persisted “changed paths” caches are a later optimization.

4. **Git transport is modeled with a local bare remote for budgeting.**
   - WAN is out-of-scope for gating, but the tooling should allow controlled-mode tests.

---

## Appendix A: Reference Workload Shapes

**Workload A (Typical edit loop):**
- modify 5–15 files
- `jul sync` every 30–120 seconds (daemon or on-command)
- `jul checkpoint` every 10–30 minutes

**Workload B (Refactor):**
- modify 200–2,000 files
- big diffs, renames, formatters
- needs progress + cancel + stable memory

**Workload C (Generated-file storm):**
- `npm install` or codegen
- thousands of file events in seconds
- daemon must debounce and remain bounded

---

## Appendix B: Minimum “Perf Smoke Suite” (Run on Every PR)

- PT-STATUS-001 (Tier S)
- PT-SYNC-001 (Tier S, Remote Mode: none)
- PT-CHECKPOINT-001 (Tier S)
- PT-NOTES-001 (10k events)
- PT-PROMOTE-001 (Tier S, Remote Mode: local)
- PT-DAEMON-002 (scaled down: 1k events)

Everything else can run nightly/weekly.
