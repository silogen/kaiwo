#!/bin/bash

NAMESPACE=$1
WORKLOAD=$2
STRING=$3
MAX_ATTEMPTS=10
SLEEP_INTERVAL=3

if [ -z "$NAMESPACE" ]; then
  echo "Usage: $0 <namespace>"
  exit 1
fi


for ((i=1; i<=MAX_ATTEMPTS; i++)); do
  echo "Attempt $i of $MAX_ATTEMPTS..."
  
  if kubectl get events -n "$NAMESPACE" | grep "$WORKLOAD" | grep -q "$STRING"; then
    exit 0
  fi

  sleep $SLEEP_INTERVAL
done

exit 1
