# Release & Packaging

## Local build

```bash
go build -o ./bin/jul ./apps/cli/cmd/jul
```

The bundled agent binary is expected at `libexec/jul/opencode` relative to `jul`.

## GoReleaser (snapshot)

```bash
# Fetch the pinned OpenCode assets into build/opencode/*
./scripts/fetch-opencode.sh

# Build archives without publishing
goreleaser release --snapshot --clean
```

## Publishing releases + Homebrew

1. Create a tap repo (e.g. `lydakis/homebrew-jul`).
2. Create a GitHub Actions secret `GORELEASER_TOKEN` with repo access to:
   - `lydakis/jul`
   - `lydakis/homebrew-jul`
3. Tag and push (GitHub Actions runs GoReleaser on tags):

```bash
git tag v0.0.1
git push --tags
```

GoReleaser writes the brew formula into the tap and installs the bundled agent
under `libexec/jul/opencode`.

Install:

```bash
brew tap lydakis/jul
brew install jul
```

## OpenCode version pin

`scripts/fetch-opencode.sh` reads the pinned version from `scripts/opencode-version.txt`.
Override with:

```bash
OPENCODE_VERSION=1.1.29 ./scripts/fetch-opencode.sh
```
