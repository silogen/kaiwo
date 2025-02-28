#!/bin/bash

set -e

kubectl delete crd kaiwoqueueconfigs.kaiwo.silogen.ai
kubectl delete crd kaiwojobs.kaiwo.silogen.ai
kubectl delete crd kaiwoservices.kaiwo.silogen.ai

kubectl delete -f config/webhook_local_dev/webhooks.yaml

echo "CRDs and webhooks for debugging were cleaned from the cluster"
echo "You should be able to run make test-e2e now"
