#!/bin/bash

set -e

TEST_NAME=${TEST_NAME:-"kaiwo-test"}

./test/setup_kind.sh

kind get kubeconfig -n "$TEST_NAME" > kaiwo_test_kubeconfig.yaml 

./test/generate_certs.sh

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

make generate
make manifests
make install

find config/static -name '*.yaml' -print0 | xargs -0 -n1 kubectl apply -f

kubectl apply -f config/webhook_local_dev/webhooks.yaml

kubectl apply -f test/kaiwoconfig.yaml

echo "You can now run debugger in your IDE"
