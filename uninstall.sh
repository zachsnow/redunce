#!/bin/bash
set -e

# Uninstall redunce from macOS.

BIN_DIR="$HOME/.local/bin"
LIB_DIR="/usr/local/lib"

BINARY="$BIN_DIR/redunce"
LIBRARY="$LIB_DIR/libvector.dylib"

# 1. Remove binary
if [[ -f "$BINARY" ]]; then
    echo "Removing binary from $BINARY..."
    rm "$BINARY"
else
    echo "Binary not found at $BINARY (already removed?)"
fi

# 2. Remove library (may require sudo)
if [[ -f "$LIBRARY" ]]; then
    echo "Removing library from $LIBRARY (may require sudo)..."
    if [[ -w "$LIBRARY" ]]; then
        rm "$LIBRARY"
    else
        sudo rm "$LIBRARY"
    fi
else
    echo "Library not found at $LIBRARY (already removed?)"
fi

echo "Done. redunce has been uninstalled."
