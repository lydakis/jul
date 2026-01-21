# Jul Build Tracker

## Current Focus (v0.3 Pivot: Local-First)
- [x] Local sync engine: shadow-index draft commits + `refs/jul/sync/<user>/<device>/<ws>`
- [x] Device ID + `workspace_base` tracking per workspace
- [x] Remote selection rules (origin fallback) + `jul remote set/show`
- [x] Update `jul init` and config to local-first defaults (`[remote]`, `[user]`)

## Next Up
- [x] Checkpoint semantics: new checkpoint commit + new draft + keep-ref
- [x] Workspace ref lease + auto-merge flow (`jul sync`, `jul merge`)
- [ ] Local metadata in notes (attestations, suggestions, review, prompts)
- [ ] Agent sandbox + review pipeline (internal agent)
- [ ] `jul ws checkout` + local workspace save/restore integration
- [ ] Query/log/diff/show over local metadata
- [x] Update smoke tests: local-only, Git remote, Jul-remote (optional)

## Completed (pre-pivot groundwork)
- [x] Monorepo scaffold (CLI/server/web/infra)
- [x] Go server skeleton + storage + SSE
- [x] Go CLI skeleton + server calls
- [x] Basic tests (storage + server + CLI parsing)
- [x] Git hooks for auto-sync (post-commit)
- [x] Keep-refs handling on server side (workspace reflog)
- [x] Smoke tests (server + CLI + git repo)
- [x] Promotion policy checks + ref updates
- [x] Attestation ingestion from CI runner
- [x] JSON output modes to CLI commands (`changes`, `sync`)
- [x] CI trigger endpoint + query API
- [x] Mirror attestations into git notes
- [x] Suggestions API + refs
- [x] Query endpoints (advanced filters)
- [x] CLI config wizard (agent/provider selection, default server, workspace)
- [x] Align CLI config format with v0.2 spec (server/workspace/init sections)
- [x] Workspace commands (`jul ws` list/set/switch/rename/delete)
