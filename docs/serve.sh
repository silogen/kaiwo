#!/bin/bash

# This script sets up the environment and servers the site locally

set -e

# Define virtual environment directory
VENV_DIR=".venv"

# Check if the virtual environment exists, if not, create one
if [ ! -d "$VENV_DIR" ]; then
    echo "Creating virtual environment..."
    python3 -m venv "$VENV_DIR"
fi

# Activate the virtual environment
echo "Activating virtual environment..."
source "$VENV_DIR/bin/activate"

# Install dependencies
echo "Installing dependencies from requirements.txt..."
pip install -r requirements.txt

# Serve the MkDocs site
echo "Starting MkDocs server..."
mkdocs serve