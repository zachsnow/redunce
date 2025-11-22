#!/bin/bash

# Exit on error, undefined variables, and pipe failures
set -euo pipefail

echo "Building redunce..."

# Build with SQLite extension loading enabled
if ! CGO_CFLAGS="-DSQLITE_ENABLE_LOAD_EXTENSION=1" go build; then
    echo "Error: build failed." >&2
    exit 1
fi

echo "Done."
