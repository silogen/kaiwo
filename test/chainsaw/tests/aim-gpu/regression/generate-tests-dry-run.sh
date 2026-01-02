#!/bin/bash

set -euo pipefail

# Script to show what would be generated without actually running tests
# This is a dry-run version of generate-tests.sh

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MODELS_FILE="${SCRIPT_DIR}/models.csv"

# Check if models.csv exists
if [[ ! -f "$MODELS_FILE" ]]; then
  echo "Error: models.csv not found at $MODELS_FILE"
  exit 1
fi

echo "DRY RUN: Showing what would be generated..."
echo ""

# Parse models.csv and show what would be generated (skip header line)
tail -n +2 "$MODELS_FILE" | while IFS=, read -r skip registry name version unoptimized hfToken private timeout concurrent; do
  # Trim whitespace
  skip=$(echo "$skip" | xargs)
  registry=$(echo "$registry" | xargs)
  name=$(echo "$name" | xargs)
  version=$(echo "$version" | xargs)

  # Skip empty lines
  [[ -z "$registry" ]] && [[ -z "$name" ]] && continue

  # Skip if skip column is 'yes'
  if [[ "$skip" == "yes" ]]; then
    echo "Skipping: $registry/$name:$version"
    echo ""
    continue
  fi

  # Combine registry, name, and version to create full image path
  image="${registry}/${name}:${version}"

  unoptimized=$(echo "$unoptimized" | xargs)
  hfToken=$(echo "$hfToken" | xargs)
  private=$(echo "$private" | xargs)
  timeout=$(echo "$timeout" | xargs)
  concurrent=$(echo "$concurrent" | xargs)

  # Apply defaults
  unoptimized=${unoptimized:-no}
  hfToken=${hfToken:-no}
  private=${private:-no}
  timeout=${timeout:-10m}
  concurrent=${concurrent:-yes}

  # Sanitize image name to create folder name
  # Convert to lowercase, replace : / . with -, and ensure RFC 1123 compliance
  folder_name="aim-$(echo "$image" | tr '[:upper:]' '[:lower:]' | sed 's|[/:.]|-|g')"

  echo "Test: $folder_name"
  echo "  Image: $image"
  echo "  Unoptimized: $unoptimized"
  echo "  Private: $private"
  echo "  Requires HF Token: $hfToken"
  echo "  Timeout: $timeout"
  echo "  Concurrent: $concurrent"
  echo "  Files that would be created:"
  echo "    - ${folder_name}/service.yaml"
  echo "    - ${folder_name}/chainsaw-test.yaml"
  if [[ "$private" == "yes" ]]; then
    echo "  Would reference: ../secret-imagepull.yaml"
  fi
  if [[ "$hfToken" == "yes" ]]; then
    echo "  Would reference: ../secret-hftoken.yaml"
  fi
  echo ""
done

echo "To actually generate and run tests, run: ./generate-tests.sh"
