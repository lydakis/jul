# Smoke Tests

These integration tests build the CLI, create temporary git repos, and exercise Jul’s local-first flows end-to-end. They focus on draft/checkpoint behavior and remote ref updates without requiring server APIs.

Covered flows:
- **Local-only**: `jul init`, `jul sync`, `jul checkpoint`, `jul ci` with no remotes configured.
- **Git remote**: bare repo as `origin`, `jul sync` pushes sync/workspace refs, `jul checkpoint` pushes keep refs.
- **Jul remote config**: `jul init --server <path> --create-remote` sets a remote and runs the same flow.
- **Review agent**: `jul review` runs against a stub agent and creates suggestions.

## Run

```bash
cd apps/server

go test ./integration -run Smoke

# or run the full server suite including integration
go test ./...
```

## Notes
- Requires `git` and `go` on PATH.
- Tests build the CLI binary from `apps/cli`.
- `jul ci` defaults to the current draft; use `--target <rev>` or `--change <id>` to pin a specific checkpoint.
