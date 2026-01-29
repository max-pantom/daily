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
  if curl -fL "$url" -o "$tmpdir/$asset"; then
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
    exit 0
  else
    echo "Release binary not found (tried $url). Falling back to go install..."
    if ! command -v go >/dev/null 2>&1; then
      echo "Go is not installed. Please install Go or provide a release binary." >&2
      exit 1
    fi
    ver="$VERSION"
    if [[ "$ver" == "latest" ]]; then
      ver="main"
    fi
    GO111MODULE=on go install github.com/max-pantom/daily/cmd/daily@"$ver"
    dest="$INSTALL_PATH"
    src_bin="$(go env GOPATH)/bin/daily"
    if [[ ! -f "$src_bin" ]]; then
      echo "go install completed but binary not found at $src_bin" >&2
      exit 1
    fi
    sudo_prefix=""
    if [[ ! -w $(dirname "$dest") ]]; then
      sudo_prefix="sudo"
    fi
    $sudo_prefix install -m 755 "$src_bin" "$dest"
    echo "Installed daily $ver to $dest via go install"
    exit 0
  fi
}

main "$@"
