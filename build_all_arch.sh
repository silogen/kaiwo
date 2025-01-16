#!/bin/bash

# Build version information
BUILD_VERSION="v.0.0.3"
BUILD_COMMIT=$(git rev-parse --short HEAD)
BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)

# List of target platforms and architectures
targets=(
    "linux/amd64"
    "linux/arm64"
    "darwin/amd64"
    "darwin/arm64"
    "windows/amd64"
    "windows/arm64"
)

mkdir -p builds/

# Iterate over targets and build
for target in "${targets[@]}"; do
    # Split the target into OS and ARCH
    IFS="/" read -r os arch <<< "$target"

    # Set output filename
    output="kaiwo_${os}_${arch}"
    if [ "$os" == "windows" ]; then
        output+=".exe"
    fi

    # Build the binary
    echo "Building for $os/$arch..."
    env GOOS=$os GOARCH=$arch go build -ldflags="-X 'main.version=${BUILD_VERSION}' -X 'main.commit=${BUILD_COMMIT}' -X 'main.date=${BUILD_DATE}'" -o builds/"$output" main.go

    if [ $? -eq 0 ]; then
        echo "Successfully built $output"
    else
        echo "Failed to build for $os/$arch"
        exit 1
    fi
done
