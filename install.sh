#!/bin/bash

# TLC Installation Script
# This script builds the tlc binary from source and installs it to a local bin directory.

set -e

# Detect OS and architecture
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

# Colors for output
BLUE='\033[0;34m'
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${BLUE}🚀 Starting TLC installation...${NC}"

# Check for Go
if ! command -v go &> /dev/null; then
    echo -e "${RED}❌ Error: Go is not installed. Please install Go 1.21 or later.${NC}"
    exit 1
fi

# Create a temporary directory for building if not running from inside the repo
if [ ! -f "go.mod" ]; then
    TMP_DIR=$(mktemp -d)
    echo -e "${BLUE}📦 Cloning TLC repository to $TMP_DIR...${NC}"
    git clone https://github.com/IdeaCraftersLabs/oss-tlc-cli.git "$TMP_DIR" --quiet
    cd "$TMP_DIR"
fi

# Build the binary
echo -e "${BLUE}🛠 Building TLC binary...${NC}"
mkdir -p bin
go build -o bin/tlc cmd/tlc/main.go

# Determine installation directory
INSTALL_DIR="/usr/local/bin"
if [ ! -w "$INSTALL_DIR" ]; then
    INSTALL_DIR="$HOME/.local/bin"
fi

mkdir -p "$INSTALL_DIR"

# Install the binary
echo -e "${BLUE}🚚 Installing to $INSTALL_DIR/tlc...${NC}"
cp bin/tlc "$INSTALL_DIR/tlc"
chmod +x "$INSTALL_DIR/tlc"

echo -e "${GREEN}✅ TLC successfully installed!${NC}"
echo -e "You can now run: ${BLUE}tlc --help${NC}"

# Remind to add to PATH if necessary
if [[ ":$PATH:" != ":$INSTALL_DIR:"* ]]; then
    echo -e "${RED}⚠ Warning: $INSTALL_DIR is not in your PATH.${NC}"
    echo -e "Add this to your shell config (.bashrc or .zshrc):"
    echo -e "${BLUE}export PATH=\"
PATH:$INSTALL_DIR\"${NC}"
fi

# Cleanup
if [ -n "$TMP_DIR" ]; then
    rm -rf "$TMP_DIR"
fi
