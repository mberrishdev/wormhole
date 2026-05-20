#!/usr/bin/env bash

REPO="mberrishdev/wormhole"
ARCH=$(uname -m)

if [ "$ARCH" = "arm64" ]; then
  FILE="wormhole-client-darwin-arm64"
else
  FILE="wormhole-client-darwin-amd64"
fi

URL="https://github.com/$REPO/releases/latest/download/$FILE"

if [ -f /usr/local/bin/wormhole ]; then
  echo "Removing existing installation..."
  sudo rm /usr/local/bin/wormhole
fi

echo "Downloading $URL"

curl -fsSL $URL -o /tmp/wormhole
chmod +x /tmp/wormhole
sudo mv /tmp/wormhole /usr/local/bin/wormhole

echo "Installed wormhole"