#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p dist
for arch in amd64 arm64; do
 CGO_ENABLED=0 GOOS=linux GOARCH=$arch go build -buildvcs=false -trimpath -ldflags='-s -w' -o "dist/cpo-linux-$arch" ./cmd/cpo
done
(cd dist && sha256sum cpo-linux-amd64 cpo-linux-arm64 > SHA256SUMS)
