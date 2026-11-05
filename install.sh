#!/usr/bin/env bash
set -euo pipefail

# Usage: bash install.sh [client|server] [installation-directory]
ROLE="${1:-client}"
INSTALL_DIR="${2:-$HOME/.local/bin}"
REPO="mberrishdev/wormhole"
case "$ROLE" in client|server) ;; *) echo "Usage: $0 [client|server] [installation-directory]" >&2; exit 2 ;; esac
case "$(uname -s)" in Darwin) PLATFORM=darwin ;; Linux) PLATFORM=linux ;; *) echo "Download the Windows executable from GitHub Releases." >&2; exit 1 ;; esac
case "$(uname -m)" in arm64|aarch64) ARCH=arm64 ;; x86_64|amd64) ARCH=amd64 ;; *) echo "Unsupported architecture" >&2; exit 1 ;; esac
FILE="wormhole-${ROLE}-${PLATFORM}-${ARCH}"
BASE="https://github.com/${REPO}/releases/latest/download"
TASK_TMP=$(mktemp -d)
trap 'rm -rf "$TASK_TMP"' EXIT
curl -fLsS "$BASE/$FILE" -o "$TASK_TMP/$FILE"
curl -fLsS "$BASE/SHA256SUMS" -o "$TASK_TMP/SHA256SUMS"
EXPECTED=$(awk -v name="$FILE" '$2 == name { print $1 }' "$TASK_TMP/SHA256SUMS")
if [ -z "$EXPECTED" ]; then echo "No checksum for $FILE; installation stopped." >&2; exit 1; fi
if command -v sha256sum >/dev/null 2>&1; then
  ACTUAL=$(sha256sum "$TASK_TMP/$FILE" | awk '{print $1}')
else
  ACTUAL=$(shasum -a 256 "$TASK_TMP/$FILE" | awk '{print $1}')
fi
if [ "$ACTUAL" != "$EXPECTED" ]; then echo "Checksum mismatch; installation stopped." >&2; exit 1; fi
mkdir -p "$INSTALL_DIR"
install -m 755 "$TASK_TMP/$FILE" "$INSTALL_DIR/wormhole-$ROLE"
echo "Installed $INSTALL_DIR/wormhole-$ROLE"
echo "Ensure $INSTALL_DIR is on PATH."
