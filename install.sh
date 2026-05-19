#!/usr/bin/env bash

REPO="mberrishdev/wormhole"
OS="darwin"
ARCH=$(uname -m)

if [ "$ARCH" = "arm64" ]; then
  FILE="wormhole-client-darwin-arm64"
else
  FILE="wormhole-client-darwin-amd64"
fi

URL="https://github.com/$REPO/releases/latest/download/$FILE"

echo "Downloading $URL"

curl -L $URL -o wormhole
chmod +x wormhole
sudo mv wormhole /usr/local/bin/wormhole

echo "Installed wormhole"