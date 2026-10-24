#!/usr/bin/env bash
set -euo pipefail

VERSION="${PORTUCCA_VERSION:-latest}"
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$OS" in
  linux|darwin) ;;
  *) echo "unsupported OS: $OS" >&2; exit 1 ;;
esac
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *) echo "unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

require() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "portucca install: '$1' is required but was not found on PATH" >&2
    echo "install the missing tool and run this script again" >&2
    exit 1
  fi
}
require curl
require tar
require install
if command -v sha256sum >/dev/null 2>&1; then
  SHA=sha256sum
elif command -v shasum >/dev/null 2>&1; then
  SHA="shasum -a 256"
else
  echo "portucca install: need sha256sum or shasum to verify the download" >&2
  echo "install one of them and run this script again" >&2
  exit 1
fi

BASE="${PORTUCCA_BASE_URL:-https://github.com/gomezv/portucca/releases}"
if [ "$VERSION" = "latest" ]; then
  URL="$BASE/latest/download"
else
  URL="$BASE/download/$VERSION"
fi
ASSET="portucca_${OS}_${ARCH}.tar.gz"
DEST="${PORTUCCA_INSTALL_DIR:-$HOME/.local/bin}"
TMP="$(mktemp -d)"
DEST_TMP=""
cleanup() {
  rm -rf "$TMP"
  if [ -n "$DEST_TMP" ]; then
    rm -f "$DEST_TMP"
  fi
}
trap cleanup EXIT

curl -fsSL "$URL/$ASSET" -o "$TMP/$ASSET"
curl -fsSL "$URL/checksums.txt" -o "$TMP/checksums.txt"
( cd "$TMP" && grep "  $ASSET$" checksums.txt | $SHA -c - )

mkdir -p "$DEST"
tar -xzf "$TMP/$ASSET" -C "$TMP"
DEST_TMP="$DEST/.portucca.tmp.$$"
install -m 0755 "$TMP/portucca" "$DEST_TMP"
mv -f "$DEST_TMP" "$DEST/portucca"
DEST_TMP=""
echo "installed $DEST/portucca"
case ":$PATH:" in
  *":$DEST:"*) ;;
  *) echo "add $DEST to PATH" ;;
esac
