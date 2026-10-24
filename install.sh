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

BASE="${PORTUCCA_BASE_URL:-https://github.com/gomezv/portucca/releases}"
if [ "$VERSION" = "latest" ]; then
  URL="$BASE/latest/download"
else
  URL="$BASE/download/$VERSION"
fi
ASSET="portucca_${OS}_${ARCH}.tar.gz"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
curl -fsSL "$URL/$ASSET" -o "$TMP/$ASSET"
curl -fsSL "$URL/checksums.txt" -o "$TMP/checksums.txt"

if command -v sha256sum >/dev/null 2>&1; then
  SHA=sha256sum
else
  SHA="shasum -a 256"
fi
( cd "$TMP" && grep "  $ASSET$" checksums.txt | $SHA -c - )

DEST="${PORTUCCA_INSTALL_DIR:-$HOME/.local/bin}"
mkdir -p "$DEST"
tar -xzf "$TMP/$ASSET" -C "$TMP"
install -m 0755 "$TMP/portucca" "$DEST/portucca"
echo "installed $DEST/portucca"
case ":$PATH:" in
  *":$DEST:"*) ;;
  *) echo "add $DEST to PATH" ;;
esac
