# Jul Server

## Licensing status

`apps/server/` is currently source-visible but not licensed for reuse.
See `LICENSES.md` in the repository root for the canonical licensing statement.

## Requirements
- Go 1.24+

## Run

```bash
cd apps/server

# start server with sqlite + local repos dir

go run ./cmd/jul-server --addr :8000 --db ./data/jul.db --repos ./repos
```

## API (current)

- `POST /api/v1/sync` — record a sync payload
- `GET /api/v1/workspaces` — list workspaces
- `GET /api/v1/workspaces/{id}` — workspace details
- `POST /api/v1/workspaces/{id}/checkpoint` — record a checkpoint (sync alias)
- `POST /api/v1/workspaces/{id}/promote` — promote request (fast-forward required unless `force=true`)
- `GET /api/v1/workspaces/{id}/reflog` — workspace history (keep refs)
- `DELETE /api/v1/workspaces/{id}` — delete a workspace
- `GET /api/v1/changes` — list changes
- `GET /api/v1/changes/{id}` — change details
- `GET /api/v1/changes/{id}/revisions` — list revisions
- `GET /api/v1/commits/{sha}` — commit metadata
- `GET /api/v1/commits/{sha}/attestation` — latest attestation
- `GET/POST /api/v1/attestations` — list/create attestations
- `POST /api/v1/ci/trigger` — run CI profile for commit
- `GET/POST /api/v1/suggestions` — list/create suggestions
- `GET /api/v1/suggestions/{id}` — suggestion details
- `POST /api/v1/suggestions/{id}/accept` — mark suggestion applied
- `POST /api/v1/suggestions/{id}/reject` — mark suggestion rejected
- `POST /api/v1/repos` — create or fetch a bare repo
- `GET /api/v1/query` — query commits by filters (`tests`, `compiles`, `coverage_min`, `coverage_max`, `author`, `change_id`, `since`, `until`, `limit`)
- `GET /events/stream` — SSE stream

Notes:
- Attestations are mirrored into git notes at `refs/notes/jul/attestations` when a repo is available.
