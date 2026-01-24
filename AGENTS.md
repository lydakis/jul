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
- Run smoke tests with `cd apps/server && go test ./integration -run Smoke`.
- Track coverage with `./scripts/coverage.sh` (generates `coverage.out` in `apps/cli` and `apps/server`).

## Jul‑First Development Workflow
- Use Jul commands only for day‑to‑day work in this repo.
- Always start with `jul status` to see draft and CI state.
- Typical loop:
  - `jul trace --prompt "<intent>"` (record intent)
  - `jul sync` (update draft + refs)
  - `jul checkpoint` (lock a snapshot)
  - `jul promote --to main` (publish to Git branch)
- Use `jul ci` for local checks; rely on `jul submit` (CR) only when review state is needed.
- Do not use git commands unless explicitly requested.

## Commit & Pull Request Guidelines
- Commit messages in this repo use imperative, sentence‑case style (e.g., “Add auto-sync hooks”, “Fix sync consistency”).
- PRs should include: a clear summary, tests run, and any relevant screenshots/logs.
- If adding API changes, update `apps/server/README.md` and consider the design spec in `docs/jul-spec.md`.

## Configuration Tips
- CLI uses `JUL_WORKSPACE`. Hooks respect `JUL_HOOK_CMD`, `JUL_NO_SYNC`, and `JUL_HOOK_VERBOSE`.
- Server storage defaults to SQLite; keep paths under `apps/server/data/` for local dev.
