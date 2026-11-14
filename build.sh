#!/bin/sh
set -eu

script_dir=$(CDPATH= cd "$(dirname "$0")" && pwd)
cd "$script_dir"

if ! command -v go >/dev/null 2>&1; then
    printf "%s\n" "Error: Go is not available in PATH." >&2
    exit 1
fi

output_dir="$script_dir/NexFatal_Storage/dist"
mkdir -p "$output_dir"

printf "%s\n" "[1/2] Building Linux/amd64..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$output_dir/NexFatal_linux_amd64" ./cmd/cli

printf "%s\n" "[2/2] Building Windows/amd64..."
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o "$output_dir/NexFatal_windows_amd64.exe" ./cmd/cli

printf "%s\n" "Build complete. Output: $output_dir"
