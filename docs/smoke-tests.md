# Smoke Tests

These integration tests build the CLI, create temporary git repos, and exercise Jul’s local-first flows end-to-end. Local smoke tests live under `apps/cli`, while remote smoke tests live under `apps/server`.

Covered flows:
- **Local-only**: `jul init`, `jul sync`, `jul checkpoint`, `jul ci run`, `jul status`, `jul log`, `jul show`, `jul diff`,
  `jul blame`, `jul review` (stub agent), `jul suggestions`, `jul apply`, `jul reflog` with no remotes configured.
- **Draft CI on sync**: when `ci.run_on_draft = true`, `jul sync` triggers CI (blocking or background) and updates `.jul/ci/results.json`.
- **CI config**: if `.jul/ci.toml` exists, CI uses its `[commands]` list instead of the default `go test ./...`.
- **CI config command**: `jul ci config --set name=cmd` writes `.jul/ci.toml` and is used in smoke tests.
- **CI config show**: `jul ci config --show` prints the resolved command list (file or inferred).
- **CI list**: `jul ci list` shows recent CI runs.
- **Status + inferred CI**: when commands are inferred (go.mod/go.work), stale draft CI results remain visible in `jul status`.
- **Draft reuse**: repeated `jul sync` with no working tree changes reuses the same draft SHA and keeps draft files changed at 0.
- **Adopt git commits**: when `checkpoint.adopt_on_commit = true`, a git commit triggers `jul checkpoint --adopt` via post-commit hook.
- **Sync CI output**: `jul sync` prints when draft CI is triggered and points to `jul ci status` for background runs.
- **Go workspaces**: when a repo has `go.work` (no root `go.mod`), default CI runs `go test ./...` inside each `use` module.
- **Traces**: `jul trace --prompt` stores metadata in `refs/notes/jul/traces` and local prompt text in `.jul/traces/`.
- **Git remote**: bare repo as `origin`, `jul sync` pushes sync/workspace refs, `jul checkpoint` pushes keep refs.
- **Jul remote config**: `jul init --server <path> --create-remote` sets a remote and runs the same flow.
- **Review agent**: `jul review` runs against a stub agent and creates suggestions.
- **Real OpenCode agent** (opt-in): run `jul review` with the bundled OpenCode binary and real model config.

## Run

```bash
# Local-only smoke tests
cd apps/cli
go test ./integration -run Smoke

# Remote-only smoke tests
cd apps/server
go test ./integration -run Smoke
```

## Notes
- Requires `git` and `go` on PATH.
- Tests build the CLI binary from `apps/cli`.
- `jul ci run` defaults to the current draft; use `--target <rev>` or `--change <id>` to pin a specific checkpoint.
- Real agent smoke test is opt-in:
  - Set `JUL_REAL_AGENT=1`
  - Ensure OpenCode is configured (e.g., `~/.config/opencode` or env vars) so it can run headless.
  - If the bundled binary is missing, the test downloads it into `build/opencode/` before running.
