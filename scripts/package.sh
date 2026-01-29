#!/usr/bin/env bash
set -euo pipefail

# Build release tarballs and checksums for macOS/Linux (amd64, arm64).
# Usage:
#   VERSION=v0.1.3 scripts/package.sh
# Outputs tarballs named daily_${VERSION}_${os}_${arch}.tar.gz and a checksums.txt.

VERSION=${VERSION:-}
if [[ -z "$VERSION" ]]; then
  echo "VERSION env var required (e.g. VERSION=v0.1.3)" >&2
  exit 1
fi

if [[ "$VERSION" != v* ]]; then
  echo "VERSION should look like v0.1.3" >&2
  exit 1
fi

ROOT=$(cd "$(dirname "$0")/.." && pwd)
OUT="$ROOT/dist"
mkdir -p "$OUT"
pushd "$ROOT" >/dev/null

targets=("darwin arm64" "darwin amd64" "linux arm64" "linux amd64")

for target in "${targets[@]}"; do
  read -r os arch <<<"$target"
  echo "Building $os/$arch..."
  BIN="daily"
  TAR="daily_${VERSION}_${os}_${arch}.tar.gz"
  GOOS=$os GOARCH=$arch CGO_ENABLED=0 go build -o "$OUT/$BIN" ./cmd/daily
  tar -C "$OUT" -czf "$OUT/$TAR" "$BIN"
  rm -f "$OUT/$BIN"
done

pushd "$OUT" >/dev/null
shasum -a 256 daily_${VERSION}_*.tar.gz > checksums.txt
popd >/dev/null

echo "Artifacts in $OUT"
