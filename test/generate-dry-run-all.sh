#!/bin/bash

set -e

# Base command
BASE_COMMAND="go run ."

# Get the current Git branch name
BRANCH_NAME=$(git rev-parse --abbrev-ref HEAD)
if [ -z "$BRANCH_NAME" ]; then
  echo "Error: Unable to determine the Git branch name."
  exit 1
fi

# Directory to save outputs
OUTPUT_DIR="test/dry-run-output/$BRANCH_NAME"

# Ensure the output directory exists
mkdir -p "$OUTPUT_DIR"

# Targets definition
targets=(
  "path:workloads/inference/LLMs/offline-inference/vllm-batch-single-multinode args:submit --ray --gpus 16 --dry-run --path \$path"
  "path:workloads/inference/LLMs/online-inference/vllm-online-single-multinode args:serve --ray --gpus 16 --dry-run --path \$path"
  "path:workloads/training/LLMs/bert/hf-accelerate-bert args:submit --gpus 8 --dry-run --path \$path"
  "path:workloads/training/LLMs/lora-supervised-finetuning/lora-sft-zero3-single-multinode args:submit --ray --gpus 8 --dry-run --path \$path"
)

# Iterate over each target
for target in "${targets[@]}"; do
  # Extract `path` and `args` from the target string
  path=$(echo "$target" | awk -F 'path:' '{print $2}' | awk -F ' args:' '{print $1}')
  args=$(echo "$target" | awk -F ' args:' '{print $2}')

  # Replace $path in args with the actual path value
  args=${args//\$path/$path}

  # Create the output file path (replace slashes in path with underscores)
  output_file="$OUTPUT_DIR/$(echo "$path" | tr '/' '_').yaml"

  # Construct the full command
  full_command="$BASE_COMMAND $args"
  echo "Executing: $full_command"

  # Run the command and capture only stdout to the output file
  eval "$full_command" > "$output_file"

  # Check exit code and handle errors if necessary
  EXIT_CODE=$?
  if [ $EXIT_CODE -ne 0 ]; then
    echo "Command failed with exit code $EXIT_CODE: $full_command"
    echo "No output written for: $output_file"
    exit $EXIT_CODE # Exit immediately on failure, or comment this line to continue
  fi

  echo "Output saved to: $output_file"
done

echo "All commands executed successfully."
