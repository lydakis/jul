#!/bin/sh
set -eu

root_dir=$(cd "$(dirname "$0")/.." && pwd)

echo "Server coverage:"
(cd "$root_dir/apps/server" && go test ./... -coverprofile=coverage.out)
(cd "$root_dir/apps/server" && go tool cover -func=coverage.out | tail -n 1)

echo ""
echo "CLI coverage:"
(cd "$root_dir/apps/cli" && go test ./... -coverprofile=coverage.out)
(cd "$root_dir/apps/cli" && go tool cover -func=coverage.out | tail -n 1)
