#!/usr/bin/env bash
# Usage: ./parse_failures.sh input.json > output.json
# Requires: jq

set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "Usage: $0 <chainsaw-report.json>" >&2
  exit 1
fi

jq '{
  tests: (
    .tests
    | map({
        basePath,
        name,
        steps: (
          .steps
          # keep only operations that have a failure, and drop timestamps
          | map({
              name,
              operations: (
                .operations
                | map(select(has("failure")) | {name, type, failure})
              )
            })
          # drop steps that ended up with zero failing operations
          | map(select((.operations | length) > 0))
        )
      })
    # (optional) drop tests that ended up with zero steps after filtering
    | map(select((.steps | length) > 0))
  )
}' "$1"