# Jul CLI

## Requirements
- Go 1.22+
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
```

## Environment

- `JUL_BASE_URL`: Sidecar API base URL (default: `http://localhost:8000`)
- `JUL_WORKSPACE`: Override workspace id (default: `<user>/<hostname>`)
