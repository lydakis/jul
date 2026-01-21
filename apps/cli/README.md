# Jul CLI

## Requirements
- Go 1.24+
- Git installed (used to read repository state)

## Usage

```bash
# Run locally
cd apps/cli
JUL_BASE_URL=http://localhost:8000 go run ./cmd/jul sync

# Initialize a repo and configure Jul remote
go run ./cmd/jul init --server http://localhost:8000

# Run interactive configuration wizard
go run ./cmd/jul configure
## Configurable defaults
# Set create_remote default in ~/.config/jul/config.toml (via `jul configure`)

# Switch or list workspaces
go run ./cmd/jul ws list
go run ./cmd/jul ws set feature-auth
go run ./cmd/jul ws switch feature-auth
go run ./cmd/jul ws rename auth-feature
go run ./cmd/jul ws delete bugfix-123

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
go run ./cmd/jul ci --cmd "go test ./..." --coverage-line 82.5

# Query recent passing commits with coverage
go run ./cmd/jul query --tests pass --compiles true --coverage-min 80 --limit 5

# Create a checkpoint for the current commit
go run ./cmd/jul checkpoint

# Run review agent (bundled OpenCode by default)
go run ./cmd/jul review

# List suggestions
go run ./cmd/jul suggestions --status pending

# Create a suggestion
go run ./cmd/jul suggest --base HEAD --suggested <sha> --reason fix_tests
```

## Environment

- `JUL_BASE_URL`: Sidecar API base URL (default: `http://localhost:8000`)
- `JUL_WORKSPACE`: Override workspace id (default: `<user>/<hostname>`)
- `JUL_HOOK_CMD`: Command used by git hook (default: `jul`)
- `JUL_NO_SYNC`: Set to disable auto-sync in the hook
- `JUL_HOOK_VERBOSE`: Set to show hook warnings
- `JUL_AGENT_CMD`: Override review agent command (default: bundled OpenCode)
- `JUL_AGENT_MODE`: Review agent mode (`stdin` or `file`)
