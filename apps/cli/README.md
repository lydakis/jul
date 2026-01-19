# Jul CLI

## Requirements
- Go 1.24+
- Git installed (used to read repository state)

## Usage

```bash
# Run locally
cd apps/cli
JUL_BASE_URL=http://localhost:8000 go run ./cmd/jul sync

# Status
JUL_BASE_URL=http://localhost:8000 go run ./cmd/jul status

# Promote
JUL_BASE_URL=http://localhost:8000 go run ./cmd/jul promote --to main

# Install auto-sync hook
go run ./cmd/jul hooks install

# Reflog (workspace history)
go run ./cmd/jul reflog

# JSON output
go run ./cmd/jul sync --json
go run ./cmd/jul changes --json

# Run local CI and record attestation
go run ./cmd/jul ci run --cmd "go test ./..."

# Query recent passing commits
go run ./cmd/jul query --tests pass --limit 5
```

## Environment

- `JUL_BASE_URL`: Sidecar API base URL (default: `http://localhost:8000`)
- `JUL_WORKSPACE`: Override workspace id (default: `<user>/<hostname>`)
- `JUL_HOOK_CMD`: Command used by git hook (default: `jul`)
- `JUL_NO_SYNC`: Set to disable auto-sync in the hook
- `JUL_HOOK_VERBOSE`: Set to show hook warnings
