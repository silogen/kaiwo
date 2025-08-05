#!/bin/bash

set -e

echo "Setting up development debugging environment..."

# Generate kubeconfig
echo "Generating kubeconfig..."
kind get kubeconfig -n "$TEST_NAME" > kaiwo_test_kubeconfig.yaml

# Generate certificates
echo "Generating certificates..."
./test/generate_certs.sh

# Setup environment variables
WEBHOOK_CERT_DIRECTORY=$(pwd)/certs
KUBECONFIG=$(pwd)/kaiwo_test_kubeconfig.yaml
ENV_FILE=".env"

update_env_var() {
    local var_name="$1"
    local var_value="$2"

    if grep -q "^${var_name}=" "$ENV_FILE"; then
        sed -i "s|^${var_name}=.*|${var_name}=${var_value}|" "$ENV_FILE"
    else
        echo "${var_name}=${var_value}" >> "$ENV_FILE"
    fi
}

touch "$ENV_FILE"
update_env_var "WEBHOOK_CERT_DIRECTORY" "$WEBHOOK_CERT_DIRECTORY"
update_env_var "KUBECONFIG" "$KUBECONFIG"

# Generate and install CRDs
echo "Generating manifests and installing CRDs..."
make generate
make manifests
make install

# Apply static configurations
echo "Applying static configurations..."
find config/static -name '*.yaml' -print0 | xargs -0 -n1 kubectl apply -f

# Apply webhook configuration
echo "Applying webhook configuration..."
kubectl apply -f config/webhook_local_dev/webhooks.yaml

# Apply test configuration
echo "Applying test kaiwoconfig..."
kubectl apply -f test/kaiwoconfig.yaml

echo "Development debugging environment is ready!"
echo "You can now run debugger in your IDE"