#!/bin/bash
set -e

# Install redunce locally on macOS.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BINARY="$SCRIPT_DIR/redunce"
LIBRARY="$SCRIPT_DIR/libvector.dylib"

BIN_DIR="$HOME/.local/bin"
LIB_DIR="/usr/local/lib"

# 1. Check if homebrew is installed
if ! command -v brew &> /dev/null; then
    echo "Error: Homebrew is not installed."
    echo "Install it from https://brew.sh or use 'brew install redunce' after setup."
    exit 1
fi

# 2. Check that the binary has been built
if [[ ! -f "$BINARY" ]]; then
    echo "Error: Binary not found at $BINARY"
    echo "Run ./build.sh first."
    exit 1
fi

if [[ ! -f "$LIBRARY" ]]; then
    echo "Error: Library not found at $LIBRARY"
    echo "See README.md for instructions on obtaining libvector.dylib"
    exit 1
fi

# 3. Install binary to ~/.local/bin
echo "Installing binary to $BIN_DIR..."
mkdir -p "$BIN_DIR"
cp "$BINARY" "$BIN_DIR/redunce"
chmod +x "$BIN_DIR/redunce"

# 4. Install library to /usr/local/lib (requires sudo)
echo "Installing library to $LIB_DIR (may require sudo)..."
if [[ -w "$LIB_DIR" ]]; then
    cp "$LIBRARY" "$LIB_DIR/libvector.dylib"
else
    sudo mkdir -p "$LIB_DIR"
    sudo cp "$LIBRARY" "$LIB_DIR/libvector.dylib"
fi

# 5. Check if ~/.local/bin is in PATH
if [[ ":$PATH:" != *":$BIN_DIR:"* ]]; then
    echo ""
    echo "Warning: $BIN_DIR is not in your PATH."
    echo "Add this to your shell profile (.zshrc, .bashrc, etc.):"
    echo ""
    echo "  export PATH=\"\$HOME/.local/bin:\$PATH\""
    echo ""
fi

echo "Done. Run 'redunce --help' to verify installation."