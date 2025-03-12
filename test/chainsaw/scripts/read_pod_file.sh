#!/bin/bash

set -e

# Ensure correct usage
if [ "$#" -ne 3 ]; then
    echo "Usage: $0 <label_key1=value1,label_key2=value2> <namespace> <file_path>"
    exit 1
fi

LABEL_SELECTOR=$1
NAMESPACE=$2
FILE_PATH=$3

# Fetch the first matching pod name
POD_NAME=$(kubectl get pods -n "$NAMESPACE" -l "$LABEL_SELECTOR" -o jsonpath="{.items[0].metadata.name}")

# Check if a POD was found
if [ -z "$POD_NAME" ]; then
    echo "No pod found with label selector: $LABEL_SELECTOR in namespace: $NAMESPACE"
    exit 1
fi

# Read the file inside the pod
kubectl exec -n "$NAMESPACE" "$POD_NAME" -- cat "$FILE_PATH"
