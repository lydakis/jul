# Jul Server

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
- `POST /api/v1/workspaces/{id}/promote` — promote request (updates git ref)
- `GET /api/v1/workspaces/{id}/reflog` — workspace history (keep refs)
- `GET /api/v1/changes` — list changes
- `GET /api/v1/changes/{id}` — change details
- `GET /api/v1/changes/{id}/revisions` — list revisions
- `GET /api/v1/commits/{sha}` — commit metadata
- `GET /api/v1/commits/{sha}/attestation` — latest attestation
- `GET/POST /api/v1/attestations` — list/create attestations
- `GET /events/stream` — SSE stream
