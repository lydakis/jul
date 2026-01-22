# Smoke Tests

These integration tests build the CLI, create temporary git repos, and exercise Jul’s local-first flows end-to-end. Local smoke tests live under `apps/cli`, while remote smoke tests live under `apps/server`.

Covered flows:
- **Local-only**: `jul init`, `jul sync`, `jul checkpoint`, `jul ci`, `jul status`, `jul log`, `jul show`, `jul diff`,
  `jul review` (stub agent), `jul suggestions`, `jul apply`, `jul reflog` with no remotes configured.
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
- `jul ci` defaults to the current draft; use `--target <rev>` or `--change <id>` to pin a specific checkpoint.
- Real agent smoke test is opt-in:
  - Set `JUL_REAL_AGENT=1`
  - Ensure OpenCode is configured (e.g., `~/.config/opencode` or env vars) so it can run headless.
  - If the bundled binary is missing, the test downloads it into `dist/opencode/` before running.
