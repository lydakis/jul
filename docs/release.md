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
2. Configure GitHub Actions secrets in `lydakis/jul`:
   - `GORELEASER_TOKEN` with repo access to:
     - `lydakis/jul`
     - `lydakis/homebrew-jul`
   - `APPLE_DEVELOPER_ID_CERTIFICATE_P12_BASE64`: base64-encoded Developer ID Application `.p12`
   - `APPLE_DEVELOPER_ID_CERTIFICATE_PASSWORD`: `.p12` export password
   - `APPLE_DEVELOPER_ID_APPLICATION`: exact identity string (for consistency with other repos)
   - `APP_STORE_CONNECT_API_KEY_P8`: App Store Connect API key content (`.p8`)
   - `APP_STORE_CONNECT_KEY_ID`: App Store Connect key id
   - `APP_STORE_CONNECT_ISSUER_ID`: App Store Connect issuer id
3. Tag and push (GitHub Actions runs GoReleaser on tags):

```bash
git tag v0.0.1
git push --tags
```

GoReleaser signs and notarizes macOS binaries, then writes the brew formula into
the tap and installs the bundled agent under `libexec/jul/opencode`.

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
