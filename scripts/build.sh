#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p dist
for arch in amd64 arm64; do
 CGO_ENABLED=0 GOOS=linux GOARCH=$arch go build -buildvcs=false -trimpath -ldflags='-s -w' -o "dist/phoneops-linux-$arch" ./cmd/phoneops
done
(cd dist && sha256sum phoneops-linux-amd64 phoneops-linux-arm64 > SHA256SUMS)
