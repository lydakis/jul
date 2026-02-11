# Jul (줄)

Jul is an AI-first Git workflow tool. It keeps Git as the source of truth, then layers on:

- checkpoint-centric workflows for agents and humans
- local CI/review metadata attached to commits
- suggestions/apply loops for iterative coding
- sync/promotion flows designed for multi-device and stacked workspaces

## Project Status

Jul is actively evolving and currently spec-driven. Expect fast iteration and occasional breaking changes while core behavior is being finalized.

- Spec: `docs/jul-spec.md`
- Integration test spec: `docs/jul-integration-tests.md`
- Performance spec: `docs/jul-performance-spec.md`
- Tracker: `docs/tracker.md`

## Quick Start

Requirements:

- Go 1.24+
- Git

From this repo root:

```bash
# Explore the CLI
go run ./apps/cli/cmd/jul --help

# Build a local binary
go build -o ./bin/jul ./apps/cli/cmd/jul
```

Typical workflow inside a Git repo:

```bash
jul init demo
jul sync --json
jul checkpoint -m "feat: improve auth flow" --json
jul review --suggest --json
jul suggestions --json
jul apply <suggestion-id> --json
jul promote --to main --json
```

## Monorepo Layout

```text
apps/
  cli/                 Go CLI (`jul`)
  server/              Go server (API + storage)
  web/                 Next.js frontend
infra/
  cloudformation/      AWS infrastructure templates
docs/                  Specs, smoke/perf docs, roadmap/tracker
```

## Development

Run tests:

```bash
go test ./apps/cli/...
go test ./apps/server/...
```

Run integration spec tests:

```bash
go test ./apps/cli/integration/spec -tags jul_integ_spec
```

Run performance smoke tests:

```bash
JUL_PERF_SMOKE=1 go test ./apps/cli/integration -run Perf -count=1 -v
```

Track coverage:

```bash
./scripts/coverage.sh
```

## Additional Docs

- CLI details: `apps/cli/README.md`
- Server details: `apps/server/README.md`
- Web details: `apps/web/README.md`
- Smoke test guide: `docs/smoke-tests.md`
- Release process: `docs/release.md`

## Licensing

Licensing in this monorepo is currently scoped per directory.

- `apps/cli/` is licensed under Apache-2.0.
- Other directories are not licensed for reuse at this time.

See `LICENSES.md` for the canonical statement.
