#!/usr/bin/env bash
set -euo pipefail

REPO="max-pantom/daily"
INSTALL_PATH="${INSTALL_PATH:-/usr/local/bin/daily}"
VERSION="${VERSION:-latest}"

detect() {
  os=$(uname -s | tr '[:upper:]' '[:lower:]')
  arch=$(uname -m)
  case "$arch" in
    x86_64|amd64) arch=amd64 ;;
    arm64|aarch64) arch=arm64 ;;
    *) echo "Unsupported arch: $arch" >&2; exit 1 ;;
  esac
  echo "$os" "$arch"
}

latest_tag() {
  curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | sed -n 's/  "tag_name": "\([^"]*\)",/\1/p'
}

main() {
  platform=$(detect)
  os=${platform%% *}
  arch=${platform##* }
  if [[ "$VERSION" == "latest" ]]; then
    VERSION=$(latest_tag)
  fi
  if [[ -z "$VERSION" ]]; then
    echo "Could not determine version" >&2
    exit 1
  fi
  asset="daily_${VERSION}_${os}_${arch}.tar.gz"
  url="https://github.com/$REPO/releases/download/${VERSION}/${asset}"

  tmpdir=$(mktemp -d)
  trap 'rm -rf "$tmpdir"' EXIT
  echo "Downloading $url"
  curl -fL "$url" -o "$tmpdir/$asset"
  tar -xzf "$tmpdir/$asset" -C "$tmpdir"
  bin="$tmpdir/daily"
  if [[ ! -f "$bin" ]]; then
    echo "binary not found in archive" >&2
    exit 1
  fi

  dest="$INSTALL_PATH"
  sudo_prefix=""
  if [[ ! -w $(dirname "$dest") ]]; then
    sudo_prefix="sudo"
  fi
  $sudo_prefix install -m 755 "$bin" "$dest"
  echo "Installed daily $VERSION to $dest"
}

main "$@"
