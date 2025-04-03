#!/bin/bash

set -e

if [[ "$1" == "--uninstall" ]]; then
  echo "Uninstalling Kaiwo..."
  if command -v kaiwo >/dev/null 2>&1; then
    sudo rm -f "$(command -v kaiwo)"
    echo "Kaiwo has been uninstalled."
  else
    echo "Kaiwo is not currently installed."
  fi
  exit 0
fi

KAIWO_VERSION="${KAIWO_VERSION:-latest}"

if [ "$KAIWO_VERSION" = "latest" ]; then
  DOWNLOAD_URL="https://github.com/silogen/kaiwo/releases/latest/download/kaiwo_linux_amd64"
else
  DOWNLOAD_URL="https://github.com/silogen/kaiwo/releases/download/${KAIWO_VERSION}/kaiwo_linux_amd64"
fi

echo "Installing Kaiwo version: $KAIWO_VERSION"
wget -q "$DOWNLOAD_URL" -O kaiwo

chmod +x kaiwo
sudo mv kaiwo /usr/local/bin/

echo "Kaiwo installed at: $(which kaiwo)"
kaiwo version
kaiwo help