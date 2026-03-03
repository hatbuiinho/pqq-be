#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

cd "$ROOT_DIR"

if ! command -v sqlc >/dev/null 2>&1; then
	echo "sqlc is not installed or not in PATH." >&2
	exit 1
fi

echo "==> Generating sqlc code"
sqlc generate

if compgen -G "internal/postgres/db/*.go" >/dev/null; then
	echo "==> Formatting generated queries"
	gofmt -w internal/postgres/db/*.go
fi

echo "==> Verifying backend compile"
GOCACHE="${ROOT_DIR}/.gocache" go test ./...

echo "==> Done"
