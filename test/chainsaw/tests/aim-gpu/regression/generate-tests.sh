#!/bin/bash

set -euo pipefail

# Script to dynamically generate Chainsaw regression tests from models.csv
# Usage: ./generate-tests.sh [--skip-tests]

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MODELS_FILE="${SCRIPT_DIR}/models.csv"
VALUES_FILE="${SCRIPT_DIR}/values.yaml"
TEMPLATES_DIR="${SCRIPT_DIR}/templates"

# Check if models.csv exists
if [[ ! -f "$MODELS_FILE" ]]; then
  echo "Error: models.csv not found at $MODELS_FILE"
  exit 1
fi

# Check if values.yaml exists
if [[ ! -f "$VALUES_FILE" ]]; then
  echo "Error: values.yaml not found at $VALUES_FILE"
  echo "Please create values.yaml with imagePullSecret and hfToken values"
  exit 1
fi

echo "Cleaning up existing test folders..."
# Remove all folders starting with 'aim-'
find "$SCRIPT_DIR" -maxdepth 1 -type d -name "aim-*" -exec rm -rf {} +

echo "Generating test folders from models.csv..."

# Parse models.csv and generate tests (skip header line)
tail -n +2 "$MODELS_FILE" | while IFS=, read -r skip registry name version unoptimized hfToken private timeout concurrent; do
  # Trim whitespace
  skip=$(echo "$skip" | xargs)
  registry=$(echo "$registry" | xargs)
  name=$(echo "$name" | xargs)
  version=$(echo "$version" | xargs)

  # Skip empty lines
  [[ -z "$registry" ]] && [[ -z "$name" ]] && continue

  # Skip if skip column is 'yes'
  [[ "$skip" == "yes" ]] && continue

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

  # Convert yes/no to true/false for compatibility with existing logic
  isUnoptimized=$([ "$unoptimized" = "yes" ] && echo "true" || echo "false")
  isPrivate=$([ "$private" = "yes" ] && echo "true" || echo "false")
  requiresHfToken=$([ "$hfToken" = "yes" ] && echo "true" || echo "false")
  concurrent_bool=$([ "$concurrent" = "yes" ] && echo "true" || echo "false")

  # Sanitize image name to create folder name
  # Convert to lowercase, replace : / . with -, and ensure RFC 1123 compliance
  folder_name="aim-$(echo "$image" | tr '[:upper:]' '[:lower:]' | sed 's|[/:.]|-|g')"
  test_dir="${SCRIPT_DIR}/${folder_name}"

  echo "  Creating test: $folder_name"
  mkdir -p "$test_dir"

  # Determine test name from folder
  test_name="$folder_name"

  # Export variables for envsubst
  export TEST_NAME="$test_name"
  export IMAGE="$image"
  export TIMEOUT="$timeout"

  # Create service.yaml from templates
  cat "$TEMPLATES_DIR/service-base.yaml" | envsubst > "${test_dir}/service.yaml"

  # Add optional sections
  if [[ "$isUnoptimized" == "true" ]]; then
    cat "$TEMPLATES_DIR/service-unoptimized.yaml" >> "${test_dir}/service.yaml"
  fi

  if [[ "$isPrivate" == "true" ]]; then
    cat "$TEMPLATES_DIR/service-imagepull.yaml" >> "${test_dir}/service.yaml"
  fi

  if [[ "$requiresHfToken" == "true" ]]; then
    cat "$TEMPLATES_DIR/service-hftoken.yaml" >> "${test_dir}/service.yaml"
  fi

  # Create chainsaw-test.yaml from templates
  cat "$TEMPLATES_DIR/chainsaw-test-header.yaml" | envsubst > "${test_dir}/chainsaw-test.yaml"

  # Add concurrent: false if needed (for models that require many GPUs)
  if [[ "$concurrent_bool" == "false" ]]; then
    sed -i "/^spec:/a\\  concurrent: false" "${test_dir}/chainsaw-test.yaml"
  fi

  # Add secret creation steps as needed
  if [[ "$isPrivate" == "true" ]]; then
    echo "  - name: Create image pull secret" >> "${test_dir}/chainsaw-test.yaml"
    echo "    try:" >> "${test_dir}/chainsaw-test.yaml"
    echo "    - apply:" >> "${test_dir}/chainsaw-test.yaml"
    echo "        file: ../secret-imagepull.yaml" >> "${test_dir}/chainsaw-test.yaml"
  fi

  if [[ "$requiresHfToken" == "true" ]]; then
    echo "  - name: Create HF token secret" >> "${test_dir}/chainsaw-test.yaml"
    echo "    try:" >> "${test_dir}/chainsaw-test.yaml"
    echo "    - apply:" >> "${test_dir}/chainsaw-test.yaml"
    echo "        file: ../secret-hftoken.yaml" >> "${test_dir}/chainsaw-test.yaml"
  fi

  # Add error checks for Degraded/Failed states
  sed "s/\${TEST_NAME}/${test_name}/g" "$TEMPLATES_DIR/chainsaw-test-error-check.yaml" >> "${test_dir}/chainsaw-test.yaml"

  # Add readiness assertion
  sed "s/\${TEST_NAME}/${test_name}/g; s/\${TIMEOUT}/${timeout}/g" "$TEMPLATES_DIR/chainsaw-test-readiness-assert.yaml" >> "${test_dir}/chainsaw-test.yaml"

done

echo "Test generation complete!"
echo ""

# Check if --skip-tests flag is provided
if [[ "${1:-}" == "--skip-tests" ]]; then
  echo "Skipping test execution (--skip-tests flag provided)"
  exit 0
fi

echo "Running Chainsaw tests..."
chainsaw test "$SCRIPT_DIR" --values "$VALUES_FILE" --parallel 4
