#!/usr/bin/env bash
set -euo pipefail

# Automate building and uploading release artifacts via GitHub CLI.
# Requirements: gh CLI authenticated, VERSION env var (e.g. v0.1.4).
# Usage: VERSION=v0.1.4 scripts/release.sh

VERSION=${VERSION:-}
if [[ -z "$VERSION" ]]; then
  echo "VERSION env var required (e.g. VERSION=v0.1.4)" >&2
  exit 1
fi

ROOT=$(cd "$(dirname "$0")/.." && pwd)

echo "Building artifacts for $VERSION ..."
VERSION=$VERSION "$ROOT/scripts/package.sh"

echo "Creating release $VERSION ..."
gh release create "$VERSION" "$ROOT"/dist/daily_${VERSION}_*.tar.gz "$ROOT"/dist/checksums.txt \
  --title "$VERSION" \
  --notes "Automated release $VERSION"

echo "Done."
