#!/bin/bash

set -e

# Ensure correct usage
if [ "$#" -ne 2 ]; then
    echo "Usage: $0 <label_key1=value1,label_key2=value2> <namespace>"
    exit 1
fi

LABEL_SELECTOR=$1
NAMESPACE=$2

# Read the file inside the pod
kubectl logs -n "$NAMESPACE" --selector "$LABEL_SELECTOR"
