#!/bin/bash
set -e

# Release script for redunce
# This script:
# 1. Tags the current commit and pushes to origin
# 2. Calculates SHA256 of the release tarball
# 3. Calculates SHA256 of sqlite-vector
# 4. Updates the Homebrew formula in the tap repo
# 5. Commits and pushes the updated formula

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TAP_REPO="${SCRIPT_DIR}/../homebrew-redunce"
FORMULA_PATH="${TAP_REPO}/Formula/redunce.rb"

# State tracking for cleanup
TAG_CREATED=false
TAG_PUSHED=false
FORMULA_MODIFIED=false

# Cleanup function
cleanup() {
    local exit_code=$?

    if [ $exit_code -ne 0 ]; then
        echo ""
        echo -e "${YELLOW}Cleaning up after error...${NC}"

        # Remove local tag if created but release not completed
        if [ "$TAG_CREATED" = true ]; then
            echo "Removing local tag ${TAG}"
            git tag -d "$TAG" 2>/dev/null || true

            if [ "$TAG_PUSHED" = true ]; then
                echo -e "${YELLOW}Warning: Tag ${TAG} was pushed to origin${NC}"
                echo "You may want to delete the remote tag:"
                echo "  git push origin :refs/tags/${TAG}"
            fi
        fi

        # Restore formula changes in tap repo
        if [ "$FORMULA_MODIFIED" = true ]; then
            echo "Restoring formula in tap repo"
            (cd "$TAP_REPO" && git checkout -- Formula/redunce.rb 2>/dev/null || true)
        fi

        echo -e "${RED}Release failed - cleanup complete${NC}"
    fi
}

trap cleanup EXIT INT TERM

# Verify tap repo exists
if [ ! -d "$TAP_REPO" ]; then
    echo -e "${RED}Error: Tap repo not found at ${TAP_REPO}${NC}"
    echo "Clone it with: git clone git@github.com:zachsnow/homebrew-redunce.git ../homebrew-redunce"
    exit 1
fi

if [ ! -f "$FORMULA_PATH" ]; then
    echo -e "${RED}Error: Formula not found at ${FORMULA_PATH}${NC}"
    exit 1
fi

# Extract versions from main.go
VERSION=$(grep 'Version = ' main.go | head -1 | sed 's/.*Version = "\(.*\)"/\1/')
SQLITE_VECTOR_VERSION=$(grep 'SQLiteVectorVersion = ' main.go | sed 's/.*SQLiteVectorVersion = "\(.*\)"/\1/')

if [ -z "$VERSION" ]; then
    echo -e "${RED}Error: Could not extract Version from main.go${NC}"
    exit 1
fi

if [ -z "$SQLITE_VECTOR_VERSION" ]; then
    echo -e "${RED}Error: Could not extract SQLiteVectorVersion from main.go${NC}"
    exit 1
fi

TAG="v${VERSION}"

echo -e "${GREEN}=== Redunce Release ===${NC}"
echo -e "Version: ${YELLOW}${VERSION}${NC}"
echo -e "Tag: ${YELLOW}${TAG}${NC}"
echo -e "SQLite-Vector: ${YELLOW}${SQLITE_VECTOR_VERSION}${NC}"
echo -e "Tap repo: ${YELLOW}${TAP_REPO}${NC}"
echo ""

# Check if working directory is clean
if [ -n "$(git status --porcelain)" ]; then
    echo -e "${YELLOW}Warning: Working directory has uncommitted changes${NC}"
    echo "Please commit or stash your changes before releasing"
    git status --short
    exit 1
fi

# Check if tap repo is clean
if [ -n "$(cd "$TAP_REPO" && git status --porcelain)" ]; then
    echo -e "${YELLOW}Warning: Tap repo has uncommitted changes${NC}"
    (cd "$TAP_REPO" && git status --short)
    exit 1
fi

# Check if tag already exists
if git rev-parse "$TAG" >/dev/null 2>&1; then
    echo -e "${RED}Error: Tag ${TAG} already exists${NC}"
    echo ""
    echo "To re-release, delete the tag first:"
    echo "  git tag -d ${TAG}"
    echo "  git push origin :refs/tags/${TAG}"
    echo ""
    echo "Or update Version in main.go"
    exit 1
fi

# Step 1: Create and push tag
echo -e "${GREEN}Step 1: Creating and pushing tag${NC}"
git tag "$TAG"
TAG_CREATED=true
git push origin "$TAG"
TAG_PUSHED=true
echo ""

# Wait for GitHub to process the tag
echo -e "${YELLOW}Waiting 10 seconds for GitHub to process the release...${NC}"
sleep 10
echo ""

# Step 2: Calculate SHA256 of release tarball
echo -e "${GREEN}Step 2: Calculating SHA256 of release tarball${NC}"
TARBALL_URL="https://github.com/zachsnow/redunce/archive/${TAG}.tar.gz"
echo "URL: ${TARBALL_URL}"
REDUNCE_SHA256=$(curl -sL "$TARBALL_URL" | shasum -a 256 | awk '{print $1}')

if [ -z "$REDUNCE_SHA256" ] || [ "$REDUNCE_SHA256" = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" ]; then
    echo -e "${RED}Error: Failed to download release tarball${NC}"
    echo "The tag may not be available on GitHub yet."
    exit 1
fi

echo -e "SHA256: ${YELLOW}${REDUNCE_SHA256}${NC}"
echo ""

# Step 3: Calculate SHA256 of sqlite-vector
echo -e "${GREEN}Step 3: Calculating SHA256 of sqlite-vector${NC}"
SQLITE_VECTOR_URL="https://github.com/sqliteai/sqlite-vector/releases/download/${SQLITE_VECTOR_VERSION}/vector-apple-xcframework-${SQLITE_VECTOR_VERSION}.zip"
echo "URL: ${SQLITE_VECTOR_URL}"
SQLITE_VECTOR_SHA256=$(curl -sL "$SQLITE_VECTOR_URL" | shasum -a 256 | awk '{print $1}')

if [ -z "$SQLITE_VECTOR_SHA256" ]; then
    echo -e "${RED}Error: Failed to calculate SHA256 for sqlite-vector${NC}"
    exit 1
fi

echo -e "SHA256: ${YELLOW}${SQLITE_VECTOR_SHA256}${NC}"
echo ""

# Step 4: Update the formula in tap repo
echo -e "${GREEN}Step 4: Updating Homebrew formula${NC}"

# Update version in URL (e.g., /archive/v0.1.0.tar.gz -> /archive/v0.1.1.tar.gz)
sed -i '' -E "s|/archive/v[0-9]+\.[0-9]+\.[0-9]+\.tar\.gz|/archive/${TAG}.tar.gz|" "$FORMULA_PATH"

# Update redunce tarball sha256 (first sha256 after url line)
sed -i '' -E "0,/sha256 \"[a-f0-9]{64}\"/s//sha256 \"${REDUNCE_SHA256}\"/" "$FORMULA_PATH"

# Update sqlite-vector version in URL
sed -i '' -E "s|sqlite-vector/releases/download/[0-9]+\.[0-9]+\.[0-9]+/vector-apple-xcframework-[0-9]+\.[0-9]+\.[0-9]+\.zip|sqlite-vector/releases/download/${SQLITE_VECTOR_VERSION}/vector-apple-xcframework-${SQLITE_VECTOR_VERSION}.zip|" "$FORMULA_PATH"

# Update sqlite-vector sha256 (inside the resource block)
awk -v new_sha="${SQLITE_VECTOR_SHA256}" '
    /sqlite-vector/ { in_resource = 1 }
    in_resource && /sha256/ {
        sub(/sha256 "[a-f0-9]+"/, "sha256 \"" new_sha "\"")
        in_resource = 0
    }
    { print }
' "$FORMULA_PATH" > "${FORMULA_PATH}.tmp" && mv "${FORMULA_PATH}.tmp" "$FORMULA_PATH"

FORMULA_MODIFIED=true

# Verify updates
if ! grep -q "refs/tags/${TAG}" "$FORMULA_PATH"; then
    echo -e "${RED}Error: Failed to update version in formula${NC}"
    exit 1
fi

if ! grep -q "sha256 \"${REDUNCE_SHA256}\"" "$FORMULA_PATH"; then
    echo -e "${RED}Error: Failed to update redunce SHA256 in formula${NC}"
    exit 1
fi

echo "Updated formula:"
echo "  - Version: ${TAG}"
echo "  - Redunce SHA256: ${REDUNCE_SHA256:0:16}..."
echo "  - SQLite-Vector SHA256: ${SQLITE_VECTOR_SHA256:0:16}..."
echo ""

# Step 5: Commit and push formula
echo -e "${GREEN}Step 5: Committing formula to tap repo${NC}"
(
    cd "$TAP_REPO"
    git add Formula/redunce.rb
    git commit -m "Update redunce to ${TAG}"
    git push origin main
)
echo ""

# Step 6: Test the formula (optional, pass --test flag)
if [[ "$1" == "--test" ]]; then
    echo -e "${GREEN}Step 6: Testing formula${NC}"
    brew install --build-from-source "$FORMULA_PATH"
    echo ""
    echo -e "${GREEN}Installation successful!${NC}"
    redunce --version
else
    echo -e "${YELLOW}Skipping installation test (pass --test to enable)${NC}"
fi

echo ""
echo -e "${GREEN}=== Release complete! ===${NC}"
echo ""
echo "Next steps:"
echo "  1. Create GitHub release: https://github.com/zachsnow/redunce/releases/new?tag=${TAG}"
echo ""
echo "Users install with:"
echo "  brew tap zachsnow/redunce"
echo "  brew install redunce"
