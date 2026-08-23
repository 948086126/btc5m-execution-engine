#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p bin/windows-amd64 bin/linux-amd64
for c in verify replay mock_live mock_source mock_collector live_source; do
  out="${c//_/-}"
  GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o "bin/windows-amd64/btc5m-${out}.exe" "./cmd/${c}"
  GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o "bin/linux-amd64/btc5m-${out}" "./cmd/${c}"
done
sha256sum bin/windows-amd64/* bin/linux-amd64/* | tee artifacts/BINARY_SHA256.txt
