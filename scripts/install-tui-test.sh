#!/usr/bin/env bash
# Install the pinned tui-test release after verifying its SHA-256 checksum.
# Usage: scripts/install-tui-test.sh
set -euo pipefail

# Pin the exact release. Upgrade only via a dedicated dependency PR that
# regenerates and reviews snapshots.
VERSION="0.1.0-beta.3"
# SHA-256 of the linux x86_64 asset (the CI platform). macOS users should
# update this to the darwin asset's checksum.
SHA256="35d0cfff1b3d6cfeba3abb0eeae7537c5722c36414106ca28b142e5e48235a37"
INSTALL_DIR="${TUI_TEST_INSTALL_DIR:-$HOME/.local/bin}"

# Determine the platform asset name.
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64) ASSET_ARCH="x86_64" ;;
  arm64)  ASSET_ARCH="aarch64" ;;
esac

case "$OS" in
  darwin) TARGET="${ASSET_ARCH}-apple-darwin" ;;
  linux)  TARGET="${ASSET_ARCH}-unknown-linux-gnu" ;;
  *) echo "unsupported OS: $OS" >&2; exit 1 ;;
esac

ASSET="tui-test-${TARGET}.tar.gz"
URL="https://github.com/microsoft/tui-test/releases/download/${VERSION}/${ASSET}"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

echo "Downloading ${URL}"
# NOSONAR S6506: pinned HTTPS release URL from the official repo
curl -fsSL -o "$TMP/tui-test.tar.gz" "$URL"

if [[ -n "$SHA256" ]]; then
  echo "${SHA256}  $TMP/tui-test.tar.gz" | shasum -a 256 -c -
fi

tar -xzf "$TMP/tui-test.tar.gz" -C "$TMP"
mkdir -p "$INSTALL_DIR"
install -m 0755 "$TMP/tui-test" "$INSTALL_DIR/tui-test"
echo "Installed tui-test ${VERSION} to ${INSTALL_DIR}/tui-test"
"$INSTALL_DIR/tui-test" --version
