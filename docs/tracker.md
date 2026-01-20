# Jul Build Tracker

## Current Focus
- [x] Monorepo scaffold (CLI/server/web/infra)
- [x] Go server skeleton + storage + SSE
- [x] Go CLI skeleton + server calls
- [x] Basic tests (storage + server + CLI parsing)

## Next Up
- [x] Add git hooks for auto-sync (post-commit)
- [x] Add keep-refs handling on server side (workspace reflog)
- [x] Add smoke tests (server + CLI + git repo)
- [x] Add promotion policy checks + actual ref updates
- [x] Add attestation ingestion from CI runner
- [x] Add JSON output modes to CLI commands (`changes`, `sync`)
- [x] Add CI trigger endpoint + query API
- [x] Mirror attestations into git notes

## Later
- [x] Suggestions API + refs
- [x] Query endpoints (advanced filters)
- [ ] CLI-only bootstrap (`jul init` done, `jul clone` pending)
- [x] CLI config wizard (agent/provider selection, default server, workspace)
- [ ] Agent-backed suggestions/review flow (auto-suggest on sync/CI) - config stub in place
- [x] Align CLI config format with v0.2 spec (server/workspace/init sections)
- [ ] Implement draft → checkpoint → promote flow (v0.2)
- [ ] Workspace commands (jul ws, default @) - list/set done, switch/rename/delete pending
- [ ] Checkpoint APIs + queries (v0.2)
- [ ] Publish prompts for outstanding suggestions
- [ ] Notes namespaces
- [ ] Web UI
