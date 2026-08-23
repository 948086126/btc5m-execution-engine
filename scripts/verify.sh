#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
echo "== Go unit/integration tests =="
go test -count=1 ./...
echo "== Core fail-closed verification =="
go run ./cmd/verify
echo "== Local mock-live integration =="
rm -rf artifacts/local-mock-live-verify
go run ./cmd/mock_live --duration=6s --out artifacts/local-mock-live-verify
