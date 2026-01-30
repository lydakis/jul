.PHONY: dev-bundle build dev test

dev-bundle:
	OPENCODE_DEST_ROOT=./libexec/jul ./scripts/fetch-opencode.sh

build:
	go build -o ./bin/jul ./apps/cli/cmd/jul

dev: dev-bundle build

test:
	go test ./apps/cli/...
