#!/bin/bash
set -e

# Release script for redunce
# This script:
# 1. Tags the current commit and pushes to origin
# 2. Calculates SHA256 of the release tarball
# 3. Calculates SHA256 of sqlite-vector
# 4. Updates the Homebrew formula
# 5. Commits the updated formula
# 6. Tests the formula by building from source

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# State tracking for cleanup
FORMULA_PATH="Formula/redunce.rb"
BACKUP_PATH="${FORMULA_PATH}.bak"
TAG_CREATED=false
TAG_PUSHED=false
FORMULA_MODIFIED=false
FORMULA_COMMITTED=false

# Cleanup function
cleanup() {
    local exit_code=$?

    if [ $exit_code -ne 0 ]; then
        echo ""
        echo -e "${YELLOW}Cleaning up after error...${NC}"

        # Restore formula backup if it exists
        if [ -f "$BACKUP_PATH" ]; then
            echo "Restoring ${FORMULA_PATH} from backup"
            mv "$BACKUP_PATH" "$FORMULA_PATH"
        fi

        # Remove local tag if created but not fully committed
        if [ "$TAG_CREATED" = true ] && [ "$FORMULA_COMMITTED" = false ]; then
            echo "Removing local tag ${TAG}"
            git tag -d "$TAG" 2>/dev/null || true

            # If we pushed the tag but didn't finish, warn the user
            if [ "$TAG_PUSHED" = true ]; then
                echo -e "${YELLOW}Warning: Tag ${TAG} was pushed to origin but release was not completed${NC}"
                echo "You may want to delete the remote tag:"
                echo "  git push origin :refs/tags/${TAG}"
            fi
        fi

        # Restore any git changes to the formula
        if [ "$FORMULA_MODIFIED" = true ] && [ "$FORMULA_COMMITTED" = false ]; then
            echo "Restoring ${FORMULA_PATH} in git"
            git checkout -- "$FORMULA_PATH" 2>/dev/null || true
        fi

        echo -e "${RED}Release failed - cleanup complete${NC}"
    else
        # Success - just remove backup if it exists
        rm -f "$BACKUP_PATH" 2>/dev/null || true
    fi
}

# Set up trap to call cleanup on exit, error, or interrupt
trap cleanup EXIT INT TERM

# Extract version from main.go
VERSION=$(grep 'Version = ' main.go | sed 's/.*Version = "\(.*\)"/\1/')

if [ -z "$VERSION" ]; then
    echo -e "${RED}Error: Could not extract version from main.go${NC}"
    exit 1
fi

TAG="v${VERSION}"

echo -e "${GREEN}=== Redunce Release ===${NC}"
echo -e "Version: ${YELLOW}${VERSION}${NC}"
echo -e "Tag: ${YELLOW}${TAG}${NC}"
echo ""

# Check if working directory is clean
if [ -n "$(git status --porcelain)" ]; then
    echo -e "${YELLOW}Warning: Working directory has uncommitted changes${NC}"
    echo "Please commit or stash your changes before releasing"
    git status --short
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
TARBALL_URL="https://github.com/ZachSnow/redunce/archive/refs/tags/${TAG}.tar.gz"
echo "URL: ${TARBALL_URL}"
REDUNCE_SHA256=$(curl -L "$TARBALL_URL" 2>/dev/null | shasum -a 256 | awk '{print $1}')

if [ -z "$REDUNCE_SHA256" ]; then
    echo -e "${RED}Error: Failed to calculate SHA256 for release tarball${NC}"
    echo "The tag may not be available on GitHub yet. Try running this script again in a few moments."
    exit 1
fi

echo -e "SHA256: ${YELLOW}${REDUNCE_SHA256}${NC}"
echo ""

# Step 3: Calculate SHA256 of sqlite-vector
echo -e "${GREEN}Step 3: Calculating SHA256 of sqlite-vector${NC}"
SQLITE_VECTOR_URL="https://github.com/sqliteai/sqlite-vector/releases/download/0.9.52/vector-apple-xcframework-0.9.52.zip"
echo "URL: ${SQLITE_VECTOR_URL}"
SQLITE_VECTOR_SHA256=$(curl -L "$SQLITE_VECTOR_URL" 2>/dev/null | shasum -a 256 | awk '{print $1}')

if [ -z "$SQLITE_VECTOR_SHA256" ]; then
    echo -e "${RED}Error: Failed to calculate SHA256 for sqlite-vector${NC}"
    exit 1
fi

echo -e "SHA256: ${YELLOW}${SQLITE_VECTOR_SHA256}${NC}"
echo ""

# Step 4: Update the formula
echo -e "${GREEN}Step 4: Updating Homebrew formula${NC}"
if [ ! -f "$FORMULA_PATH" ]; then
    echo -e "${RED}Error: ${FORMULA_PATH} not found${NC}"
    exit 1
fi

# Create a backup
cp "$FORMULA_PATH" "${BACKUP_PATH}"

# Replace the placeholders
sed -i '' "s/\${VERSION}/${TAG}/" "$FORMULA_PATH"
sed -i '' "s/\${REDUNCE_SHA256}/${REDUNCE_SHA256}/" "$FORMULA_PATH"
sed -i '' "s/\${SQLITE_VECTOR_SHA256}/${SQLITE_VECTOR_SHA256}/" "$FORMULA_PATH"
FORMULA_MODIFIED=true

# Verify replacements
if grep -q '\${VERSION}' "$FORMULA_PATH" || grep -q '\${REDUNCE_SHA256}' "$FORMULA_PATH" || grep -q '\${SQLITE_VECTOR_SHA256}' "$FORMULA_PATH"; then
    echo -e "${RED}Error: Failed to replace placeholders in ${FORMULA_PATH}${NC}"
    mv "${BACKUP_PATH}" "$FORMULA_PATH"
    FORMULA_MODIFIED=false
    exit 1
fi

echo "Updated ${FORMULA_PATH} with version and SHA256 values"
echo ""

# Step 5: Commit the updated formula
echo -e "${GREEN}Step 5: Committing updated formula${NC}"
git add Formula/redunce.rb
git commit -m "Update Homebrew formula for ${TAG}"
git push origin main
FORMULA_COMMITTED=true
echo ""

# Step 6: Test the formula
echo -e "${GREEN}Step 6: Testing formula by building from source${NC}"
echo -e "${YELLOW}This will install redunce using Homebrew. Continue? (y/N)${NC}"
read -r response
if [[ "$response" =~ ^([yY][eE][sS]|[yY])$ ]]; then
    brew install --build-from-source ./Formula/redunce.rb
    echo ""
    echo -e "${GREEN}=== Installation successful! ===${NC}"
    echo "Testing installed binary:"
    redunce --version
else
    echo "Skipping installation test"
fi

echo ""
echo -e "${GREEN}=== Release complete! ===${NC}"
echo "Next steps:"
echo "  1. Create a GitHub release at: https://github.com/zachsnow/redunce/releases/new?tag=${TAG}"
echo ""
echo "Users can now install redunce with:"
echo "  brew tap zachsnow/redunce https://github.com/zachsnow/redunce"
echo "  brew install redunce"
