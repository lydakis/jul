# Jul Monorepo

This repository hosts the Jul CLI, server, web UI, and infrastructure-as-code.

## Layout

```
apps/
  cli/                 # Go CLI (jul)
  server/              # Server (API + git sidecar)
  web/                 # Frontend UI
infra/
  cloudformation/      # AWS CloudFormation templates

docs/                  # Design docs (specs, notes)
```

## Getting started

- CLI: `cd apps/cli` then `go run ./cmd/jul --help`
- Server: `cd apps/server` then `go run ./cmd/jul-server`

Note: Module paths in `go.mod` may need to be updated once the repo remote is set.
