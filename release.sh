#!/bin/bash
set -e

# Release script: tags, calculates SHAs, updates Homebrew formula

TAP_REPO="$(dirname "$0")/../homebrew-redunce"
FORMULA="$TAP_REPO/Formula/redunce.rb"

VERSION=$(grep 'Version = ' main.go | head -1 | sed 's/.*Version = "\(.*\)"/\1/')
SQLITE_VERSION=$(grep 'SQLiteVectorVersion = ' main.go | sed 's/.*SQLiteVectorVersion = "\(.*\)"/\1/')
TAG="v$VERSION"

# On failure: delete local tag, restore formula
cleanup() {
    local code=$?
    [ $code -ne 0 ] || return 0
    git tag -d "$TAG" 2>/dev/null
    (cd "$TAP_REPO" && git checkout -- Formula/redunce.rb 2>/dev/null)
}
trap cleanup EXIT

echo "Releasing $TAG (sqlite-vector $SQLITE_VERSION)..."

# Preflight checks
[ -z "$VERSION" ] && echo "Error: no Version in main.go" && exit 1
[ -z "$SQLITE_VERSION" ] && echo "Error: no SQLiteVectorVersion in main.go" && exit 1
[ ! -f "$FORMULA" ] && echo "Error: formula not found at $FORMULA" && exit 1
[ -n "$(git status --porcelain)" ] && echo "Error: uncommitted changes" && exit 1

# Handle --force: delete existing tag
if [[ "$1" == "--force" ]] && git rev-parse "$TAG" >/dev/null 2>&1; then
    echo "Deleting existing tag $TAG..."
    git tag -d "$TAG"
    git push origin ":refs/tags/$TAG"
fi

git rev-parse "$TAG" >/dev/null 2>&1 && echo "Error: tag $TAG exists (use --force)" && exit 1

# Create and push tag
git tag "$TAG" && git push origin "$TAG"
echo "Waiting for GitHub..."
sleep 10

# Calculate SHAs
REDUNCE_SHA=$(curl -sL "https://github.com/zachsnow/redunce/archive/$TAG.tar.gz" | shasum -a 256 | cut -d' ' -f1)
VECTOR_SHA=$(curl -sL "https://github.com/sqliteai/sqlite-vector/releases/download/$SQLITE_VERSION/vector-apple-xcframework-$SQLITE_VERSION.zip" | shasum -a 256 | cut -d' ' -f1)

[ "$REDUNCE_SHA" = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" ] && echo "Error: empty tarball" && exit 1

echo "Redunce SHA: $REDUNCE_SHA"
echo "Vector SHA: $VECTOR_SHA"

# Update formula
sed -i '' -E "s|/archive/v[0-9.]+\.tar\.gz|/archive/$TAG.tar.gz|" "$FORMULA"
sed -i '' -E "s|vector-apple-xcframework-[0-9.]+\.zip|vector-apple-xcframework-$SQLITE_VERSION.zip|" "$FORMULA"
sed -i '' -E "s|download/[0-9.]+/vector|download/$SQLITE_VERSION/vector|" "$FORMULA"

# Update SHAs with awk (first sha256 = redunce, second in resource block = vector)
awk -v r="$REDUNCE_SHA" -v v="$VECTOR_SHA" '
    /resource.*sqlite-vector/ { in_res = 1 }
    /sha256 "[a-f0-9]+"/ {
        if (in_res) { sub(/sha256 "[a-f0-9]+"/, "sha256 \"" v "\""); in_res = 0 }
        else if (!did_r) { sub(/sha256 "[a-f0-9]+"/, "sha256 \"" r "\""); did_r = 1 }
    }
    { print }
' "$FORMULA" > "$FORMULA.tmp" && mv "$FORMULA.tmp" "$FORMULA"

# Commit and push formula
(cd "$TAP_REPO" && git add Formula/redunce.rb && git commit -m "Update redunce to $TAG" && git push origin main)

echo ""
echo "Done!"
echo "  - Homebrew tap updated: $TAP_REPO"
echo "  - Optionally create a release: https://github.com/zachsnow/redunce/releases/new?tag=$TAG"
echo ""
