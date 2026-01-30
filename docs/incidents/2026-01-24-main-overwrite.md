# Incident Report — Jul Drafts Overwrote `main` (Jan 24, 2026)

## Summary
A sequence of Jul commands caused the Git branch `main` to be moved onto Jul draft/checkpoint commits, creating an incorrect history (`main_bak`) and overwriting real commits. The incident happened because Jul used Git commands that **move `HEAD` and branch refs** during workspace operations, and because Jul **lacked pinned base tracking + promote safety invariants**, allowing drafts to drift from the real target history and then be promoted onto `main`.

## Impact
- `main` was force-updated to a chain of Jul checkpoint commits (`main_bak`).
- The repository temporarily lost real commits until `main` was restored from reflog.
- Jul refs (`refs/jul/workspaces/...`, `refs/jul/sync/...`) remained attached to the wrong base.

## Timeline (approximate)
1. **Workspace checkout / promote used `git reset --hard`**  
   This moved `HEAD` and the `main` branch to the current draft commit.
2. **Draft base drift**  
   Because `main` was now pointing at the draft, subsequent `jul sync` and `jul checkpoint` chained new checkpoints on top of the *wrong base*.
3. **Promote moved `main` again**  
   `jul promote` updated `refs/heads/main` to checkpoint commits. Since `main` was already polluted, fast-forward checks were meaningless. A forced promote then pushed the broken branch history.
4. **Main restored manually**  
   `main` was restored using reflog, but `main_bak` still holds the bad checkpoint-only chain.

## Root Causes

### 1) Jul workspace operations moved Git branches
- `jul ws checkout` used `git reset --hard`
- `jul promote` used `git reset --hard` as part of `startNewDraftAfterPromote`

These commands **move `HEAD`** and **update the currently checked out branch**, which in this repo was `main`. That is how `main` got pointed to drafts.

### 2) No pinned base + unsafe promote to target
Jul did not persist a pinned base (`base_ref` + `base_sha`) per workspace, so it could not reliably detect that a draft/checkpoint chain was anchored to an outdated target tip. `jul promote` then updated `refs/heads/main` without first ensuring the publish target was a fast‑forward of the *current* target tip.

### 3) Divergence checks only compared workspace vs sync refs
The divergence detection never compared `draft base` vs `target tip`, so it never detected that the workspace was based on an outdated branch.

## Contributing Factors
- `jul ws checkout` also cleaned the repo in a way that removed `.jul` state in some cases.
- `jul promote --force` was used while the branch was already in a bad state.
- No automatic detection of "workspace base behind target branch".

## Current State (after incident)
- `main` has been restored to correct history.
- `main_bak` contains the broken checkpoint stack.
- Jul refs (`refs/jul/workspaces/...`) still point to the old checkpoint chain.

## Fixes Already Applied
1. **`jul ws checkout` no longer moves `HEAD`**  
   It now uses `git read-tree --reset -u` + `git clean -fd --exclude=.jul`.
2. **`jul promote` no longer moves `HEAD`**  
   Same change for the "start new draft" step.
3. **Smoke test added**  
   Ensures workspace checkout does not move `HEAD`.
4. **Pinned base tracking (`base_ref` + `base_sha`)**  
   Stored per-workspace; diffs/divergence use the pinned base.
5. **`jul ws restack` implemented**  
   Restacks checkpoint chains onto the pinned base; updates `base_sha`.
6. **Promote safety invariant**  
   Promote fetches target tip and only fast‑forwards by default; `--force-target` is explicit.
7. **Stacked promote auto‑land**  
   Promote lands stacked workspaces bottom‑up.
8. **Lease validation + tests**  
   Corrupted lease is detected and requires explicit recovery.

## Follow‑ups
- Keep the new safeguards covered by smoke/integration tests as we refactor sync and promote.
- Re‑verify lease validation when adding repo‑meta and user namespace resolution.

## Lessons Learned
- Jul must never change Git branch pointers during workspace operations.  
- Workspaces must have a **pinned base**; base updates must be explicit (`jul ws restack`) or part of promote.  
- `jul promote` must be safe by construction (fetch + fast‑forward only).
