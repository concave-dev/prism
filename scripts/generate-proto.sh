#!/bin/bash

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Ensure we have the required tools
if ! command -v protoc &> /dev/null; then
    echo "protoc not found. Install with: brew install protobuf"
    exit 1
fi

# Add Go protobuf generators to PATH
export PATH="$PATH:$(go env GOPATH)/bin"

if ! command -v protoc-gen-go &> /dev/null; then
    go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
fi

if ! command -v protoc-gen-go-grpc &> /dev/null; then
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
fi

cd "$PROJECT_ROOT"

protoc \
    --go_out=. \
    --go-grpc_out=. \
    --go_opt=paths=source_relative \
    --go-grpc_opt=paths=source_relative \
    internal/grpc/proto/node_service.proto
