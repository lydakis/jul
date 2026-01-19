# Jul Server

## Run

```bash
cd apps/server

# start server with sqlite

go run ./cmd/jul-server --addr :8000 --db ./data/jul.db
```

## API (current)

- `POST /api/v1/sync` — record a sync payload
- `GET /api/v1/workspaces` — list workspaces
- `GET /api/v1/workspaces/{id}` — workspace details
- `POST /api/v1/workspaces/{id}/promote` — promote request (stub)
- `GET /api/v1/changes` — list changes
- `GET /api/v1/changes/{id}` — change details
- `GET /api/v1/changes/{id}/revisions` — list revisions
- `GET /api/v1/commits/{sha}` — commit metadata
- `GET /api/v1/commits/{sha}/attestation` — latest attestation
- `GET/POST /api/v1/attestations` — list/create attestations
- `GET /events/stream` — SSE stream
