# Smoke Tests

These integration tests spin up a real Jul server, build the CLI, create a git repo, and exercise sync + reflog.

## Run

```bash
cd apps/server

go test ./integration -run Smoke
```

## Notes
- Requires `git` and `go` available on PATH.
- The test builds the CLI binary from `apps/cli`.
