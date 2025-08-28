#!/bin/bash

set -e  # Exit on any error

# Step 1: Create directory for certificates
mkdir -p certs

# Step 2: Export CAROOT environment variable for mkcert
export CAROOT=$(pwd)/certs

# Step 3: Install mkcert CA (Creates rootCA.pem and rootCA-key.pem)
mkcert -install

# Step 4: Generate TLS certificates for localhost and Docker bridge IP
mkcert -cert-file=$CAROOT/tls.crt -key-file=$CAROOT/tls.key host.docker.internal 172.17.0.1

# Step 5: Extract and encode rootCA.pem in base64
if [[ "$OSTYPE" == "darwin"* ]]; then
    # macOS
    CA_BUNDLE=$(cat certs/rootCA.pem | base64)
else
    # Linux
    CA_BUNDLE=$(cat certs/rootCA.pem | base64 -w 0)
fi

# Step 6: Update webhook configuration file with the new caBundle
WEBHOOK_CONFIG="config/webhook_local_dev/webhooks.yaml"

if [[ -f "$WEBHOOK_CONFIG" ]]; then
    # Replace existing caBundle values in webhook config
    sed -i "s|caBundle: .*|caBundle: ${CA_BUNDLE}|" "$WEBHOOK_CONFIG"
    echo "Updated caBundle in ${WEBHOOK_CONFIG}"
else
    echo "Webhook configuration file not found at ${WEBHOOK_CONFIG}"
    exit 1
fi

echo "Certificate generation and webhook update completed successfully."
