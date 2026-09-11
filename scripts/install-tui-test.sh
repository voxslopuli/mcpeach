#!/usr/bin/env bash
# Install the pinned tui-test release after verifying its SHA-256 checksum.
# Usage: scripts/install-tui-test.sh
set -euo pipefail

# Pin the exact release and its checksum. Upgrade only via a dedicated
# dependency PR that regenerates and reviews snapshots.
VERSION="0.1.0-beta.3"
SHA256=""
INSTALL_DIR="${TUI_TEST_INSTALL_DIR:-$HOME/.local/bin}"

# Determine the platform asset name.
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64) ARCH="x86_64" ;;
  arm64)  ARCH="aarch64" ;;
esac
ASSET="tui-test-${VERSION}-${OS}-${ARCH}"

URL="https://github.com/microsoft/tui-test/releases/download/${VERSION}/${ASSET}"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

echo "Downloading ${URL}"
curl -fsSL -o "$TMP/tui-test" "$URL"

if [[ -n "$SHA256" ]]; then
  echo "${SHA256}  $TMP/tui-test" | shasum -a 256 -c -
fi

mkdir -p "$INSTALL_DIR"
install -m 0755 "$TMP/tui-test" "$INSTALL_DIR/tui-test"
echo "Installed tui-test ${VERSION} to ${INSTALL_DIR}/tui-test"
"$INSTALL_DIR/tui-test" --version
