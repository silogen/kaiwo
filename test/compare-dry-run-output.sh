#!/bin/bash

# Directories for the two branches
BRANCH1="main"
BRANCH2="typed-templates"

# Base directory where the outputs are stored
OUTPUT_BASE_DIR="test/dry-run-output"

# Full paths to the branch directories
DIR1="$OUTPUT_BASE_DIR/$BRANCH1"
DIR2="$OUTPUT_BASE_DIR/$BRANCH2"

# Log file to store differences
LOG_FILE="comparison_log.txt"

# Ensure both directories exist
if [ ! -d "$DIR1" ]; then
  echo "Error: Directory for branch1 does not exist: $DIR1"
  exit 1
fi

if [ ! -d "$DIR2" ]; then
  echo "Error: Directory for branch2 does not exist: $DIR2"
  exit 1
fi

# Clear or create the log file
> "$LOG_FILE"

echo "Comparing outputs between branches '$BRANCH1' and '$BRANCH2'..."
echo "Differences will be logged to: $LOG_FILE"

# Iterate through the files in branch1's directory
for file1 in "$DIR1"/*.yaml; do
  # Extract the filename (without path)
  filename=$(basename "$file1")

  # Corresponding file in branch2
  file2="$DIR2/$filename"

  # Check if the file exists in both directories
  if [ ! -f "$file2" ]; then
    echo "Missing file in branch2: $filename" >> "$LOG_FILE"
    continue
  fi

  # Compare the files using dyff
  diff_output=$(dyff between "$file1" "$file2" 2>&1)

  echo "$diff_output" >> "$LOG_FILE"
  echo "----" >> "$LOG_FILE"
done

echo "Comparison complete. Check the log file for details: $LOG_FILE"
