# Smoke Tests

These integration tests spin up a real Jul server, build the CLI, create a git repo, and exercise sync + reflog + CI attestation + query (including coverage filters) + suggestions refs + git notes.

Two variants:
- Mixed Git + HTTP: validates raw API responses and ref updates.
- Jul CLI flow: uses `jul` commands for sync/query/suggestions; still uses `git` for commits/push because Git transport isn’t implemented yet.

## Run

```bash
cd apps/server

go test ./integration -run Smoke

# or run the full server suite including integration
go test ./...
```

## Notes
- Requires `git` and `go` available on PATH.
- The test builds the CLI binary from `apps/cli`.
