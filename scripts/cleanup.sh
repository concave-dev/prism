#!/usr/bin/env bash

set -euo pipefail

# TODO(prism): Add flags like --keep-bin/--keep-data for selective cleanup.
# TODO(prism): Extend to clean logs, temp volumes, and Firecracker artifacts.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "Cleaning build and data directories..."

delete_dir() {
  local target_dir="$1"
  if [[ -d "$target_dir" ]]; then
    echo "Removing $target_dir"
    rm -rf "$target_dir"
  else
    echo "Already clean: $target_dir"
  fi
}

delete_dir "$PROJECT_ROOT/bin"
delete_dir "$PROJECT_ROOT/data"

echo "Cleanup complete."


