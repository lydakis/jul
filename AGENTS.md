# Repository Guidelines

## Project Structure & Module Organization
- `apps/cli/` – Go CLI (`jul`) and related packages. Tests live alongside code (e.g., `apps/cli/internal/*/*_test.go`).
- `apps/server/` – Go server (API + storage). Unit tests in `internal/*`, integration smoke tests in `apps/server/integration/`.
- `apps/web/` – Placeholder frontend (not currently active).
- `infra/` – CloudFormation templates.
- `docs/` – Design spec and operational docs (e.g., `docs/jul-spec.md`, `docs/smoke-tests.md`, `docs/tracker.md`).

## Build, Test, and Development Commands
- CLI run: `cd apps/cli && go run ./cmd/jul --help`
- Server run: `cd apps/server && go run ./cmd/jul-server --addr :8000 --db ./data/jul.db`
- Unit tests (CLI): `cd apps/cli && go test ./...`
- Unit tests (Server): `cd apps/server && go test ./...`
- Smoke tests (local-only): `cd apps/cli && go test ./integration -run Smoke`
- Smoke tests (remote): `cd apps/server && go test ./integration -run Smoke`
- Local build (release): `go build -o ./bin/jul ./apps/cli/cmd/jul`
- GoReleaser snapshot: `./scripts/fetch-opencode.sh && goreleaser release --snapshot --clean`
- Format: `gofmt -w $(rg --files -g '*.go' apps/cli apps/server)`

## Coding Style & Naming Conventions
- Go formatting via `gofmt` is required before commit.
- Prefer small, focused packages under `apps/*/internal/`.
- Use descriptive file names and keep tests close to their implementation (`*_test.go`).
- CLI commands follow `jul <noun|verb>` (e.g., `jul sync`, `jul reflog`).

## Testing Guidelines
- Use Go’s `testing` package; no external test framework required.
- Unit tests should cover core logic and error paths.
- Integration tests live in `apps/server/integration/` and exercise real CLI local/remote flows; server API tests are separate.
- New features should include unit tests and, when relevant, a smoke/integration scenario.
- Bug fixes should add a unit test or a spec integration test that reproduces the issue; if it’s a new spec scenario, update `docs/jul-integration-tests.md`.
- When tests create remotes (especially bare remotes), never assume default branch names. Push/fetch explicit refs (for example `refs/heads/main`) and set remote `HEAD` explicitly when branch identity matters.
- Run smoke tests with `cd apps/server && go test ./integration -run Smoke`.
- Track coverage with `./scripts/coverage.sh` (generates `coverage.out` in `apps/cli` and `apps/server`).

## Commit & Pull Request Guidelines
- Commit messages in this repo use imperative, sentence‑case style (e.g., “Add auto-sync hooks”, “Fix sync consistency”).
- PRs should include: a clear summary, tests run, and any relevant screenshots/logs.
- If adding API changes, update `apps/server/README.md` and consider the design spec in `docs/jul-spec.md`.

## Configuration Tips
- CLI uses `JUL_WORKSPACE`. Hooks respect `JUL_HOOK_CMD`, `JUL_NO_SYNC`, and `JUL_HOOK_VERBOSE`.
- Server storage defaults to SQLite; keep paths under `apps/server/data/` for local dev.
