#!/usr/bin/env bash
set -euo pipefail

# Build release tarballs and checksums for the host platform by default.
# Cross-compiling the tray (systray) often needs platform SDKs/CGO; run this on each target OS/arch or set TARGETS manually if your toolchain supports it.
# Usage:
#   VERSION=v0.1.3 scripts/package.sh
#   TARGETS="darwin arm64" VERSION=v0.1.3 scripts/package.sh   # override targets (requires proper SDK/CGO)
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

if [[ -n "${TARGETS:-}" ]]; then
  read -ra targets <<<"$TARGETS"
else
  # default to host platform only
  targets=("$(go env GOOS) $(go env GOARCH)")
fi

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
