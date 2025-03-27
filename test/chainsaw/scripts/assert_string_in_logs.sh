#!/bin/bash

set -e

if [ "$#" -lt 6 ]; then
    echo "Usage: $0 <mode: --all | --any> <positive | negative> <label_key1=value1,label_key2=value2> <namespace> <max_attempts> <search_string1> [<search_string2> ...]"
    exit 1
fi

MODE=$1  # Either "--all" or "--any"
EXPECTATION=$2  # Either "positive" or "negative"
LABEL_SELECTOR=$3
NAMESPACE=$4
MAX_ATTEMPTS=$5
shift 5  # Move past the first 5 arguments
SEARCH_STRINGS=("$@")
SLEEP_INTERVAL=3  

if [[ "$MODE" != "--all" && "$MODE" != "--any" ]]; then
    echo "Error: Mode must be either --all or --any"
    exit 1
fi

if [[ "$EXPECTATION" != "positive" && "$EXPECTATION" != "negative" ]]; then
    echo "Error: Expectation must be either 'positive' or 'negative'"
    exit 1
fi

echo "Searching for '${SEARCH_STRINGS[*]}' in logs of pods with label '${LABEL_SELECTOR}' in namespace '${NAMESPACE}' (Mode: $MODE, Expectation: $EXPECTATION)..."

for ((i=1; i<=MAX_ATTEMPTS; i++)); do
    LOGS=$(kubectl logs -n "$NAMESPACE" --selector "$LABEL_SELECTOR" 2>/dev/null || true)

    if [[ "$MODE" == "--all" ]]; then
        FOUND_ALL=true
        for SEARCH_STRING in "${SEARCH_STRINGS[@]}"; do
            if ! echo "$LOGS" | grep -q "$SEARCH_STRING"; then
                FOUND_ALL=false
                break
            fi
        done
        if [ "$FOUND_ALL" = true ]; then
            if [[ "$EXPECTATION" == "positive" ]]; then
                echo "✅ All search strings found as expected!"
                exit 0
            fi
        elif [[ "$EXPECTATION" == "negative" ]]; then
            echo "✅ All search strings NOT found as expected!"
            exit 0
        fi
    else
        FOUND_ANY=false
        for SEARCH_STRING in "${SEARCH_STRINGS[@]}"; do
            if echo "$LOGS" | grep -q "$SEARCH_STRING"; then
                FOUND_ANY=true
                break
            fi
        done
        if [ "$FOUND_ANY" = true ]; then
            if [[ "$EXPECTATION" == "positive" ]]; then
                echo "✅ Found at least one matching string as expected!"
                exit 0
            fi
        elif [[ "$EXPECTATION" == "negative" && "$FOUND_ANY" = false ]]; then
            echo "✅ No search strings found as expected!"
            exit 0
        fi
    fi

    echo "Attempt $i/$MAX_ATTEMPTS: Condition not met yet, retrying in ${SLEEP_INTERVAL}s..."
    sleep "$SLEEP_INTERVAL"
done

if [[ "$EXPECTATION" == "positive" ]]; then
    echo "❌ Failed to find required search strings in logs after $MAX_ATTEMPTS attempts."
else
    echo "❌ Search strings were found when they should not be present."
fi
exit 1

