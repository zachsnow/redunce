#!/bin/bash
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Test directory
TEST_DIR="/tmp/redunce-ignore-test"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REDUNCE="$SCRIPT_DIR/redunce"

# Check if redunce binary exists
if [ ! -f "$REDUNCE" ]; then
    echo "Error: redunce binary not found at $REDUNCE"
    echo "Please run 'go build' first"
    exit 1
fi

echo "Creating test directory structure..."
rm -rf "$TEST_DIR"
mkdir -p "$TEST_DIR"

# Create directory structure
# Root level
cd "$TEST_DIR"
mkdir -p level1/level2 level1/other different

# Root level files
cat > .gitignore << 'EOF'
# Root gitignore
*_test.go
*.backup
EOF

cat > .redunceignore << 'EOF'
# Root redunceignore - overrides gitignore
!important_test.go
secrets.go
EOF

touch root.go
touch debug_test.go       # Should be ignored by .gitignore
touch important_test.go   # Should be INCLUDED (.redunceignore overrides .gitignore with negation)
touch secrets.go          # Should be ignored by .redunceignore
touch data.backup         # Should be ignored by .gitignore

# Level 1 - has .gitignore only
cd level1
cat > .gitignore << 'EOF'
# Level1 gitignore
*_generated.go
EOF

touch level1.go
touch app_generated.go    # Should be ignored by level1/.gitignore
touch helper_test.go      # Should be ignored by root .gitignore
touch utils.go

# Level 2 - has .redunceignore only
cd level2
cat > .redunceignore << 'EOF'
# Level2 redunceignore
*_draft.go
EOF

touch level2.go
touch design_draft.go     # Should be ignored by level2/.redunceignore
touch api_generated.go    # Should be ignored by level1/.gitignore
touch types.go

# Go back to level1/other - has both files
cd "$TEST_DIR/level1/other"
cat > .gitignore << 'EOF'
# Other gitignore
mock_*.go
EOF

cat > .redunceignore << 'EOF'
# Other redunceignore - takes precedence
!mock_important.go
vendor/
EOF

touch other.go
touch mock_user.go        # Should be ignored by .gitignore
touch mock_important.go   # Should be INCLUDED (.redunceignore overrides .gitignore)
mkdir -p vendor
touch vendor/lib.go       # Should be ignored by .redunceignore

# Different branch - no ignore files, but tests enry bypass
cd "$TEST_DIR/different"
cat > .gitignore << 'EOF'
# Different gitignore
*.log
*.json
EOF

cat > .redunceignore << 'EOF'
# Different redunceignore - explicitly include non-code files
!important.log
!config.json
EOF

touch different.go
touch another_test.go     # Should be ignored by root .gitignore
touch config.go
touch debug.log           # Should be ignored by .gitignore
touch important.log       # Should be INCLUDED (explicit !pattern bypasses enry)
touch data.json           # Should be ignored by .gitignore
touch config.json         # Should be INCLUDED (explicit !pattern bypasses enry)

echo -e "${YELLOW}Test directory structure created at: $TEST_DIR${NC}"
echo ""

# Expected files (in sorted order)
# Note: Files with negations (!) in .redunceignore are explicitly included
# and bypass go-enry filtering (e.g., important.log, config.json)
EXPECTED=(
    "$TEST_DIR/different/config.go"
    "$TEST_DIR/different/config.json"
    "$TEST_DIR/different/different.go"
    "$TEST_DIR/different/important.log"
    "$TEST_DIR/important_test.go"
    "$TEST_DIR/level1/level1.go"
    "$TEST_DIR/level1/level2/level2.go"
    "$TEST_DIR/level1/level2/types.go"
    "$TEST_DIR/level1/other/mock_important.go"
    "$TEST_DIR/level1/other/other.go"
    "$TEST_DIR/level1/utils.go"
    "$TEST_DIR/root.go"
)

echo "Running: $REDUNCE --print-files $TEST_DIR"
echo ""

# Run redunce and capture output (run from script directory)
cd "$SCRIPT_DIR"
ACTUAL_OUTPUT=$("$REDUNCE" --print-files "$TEST_DIR" 2>&1)

# Extract file paths from output (skip header lines)
ACTUAL_FILES=($(echo "$ACTUAL_OUTPUT" | grep "^/" | sort))

echo "Expected ${#EXPECTED[@]} files:"
printf '%s\n' "${EXPECTED[@]}"
echo ""

echo "Got ${#ACTUAL_FILES[@]} files:"
printf '%s\n' "${ACTUAL_FILES[@]}"
echo ""

# Compare results
FAILED=0
MISSING=()
EXTRA=()

# Check for missing files
for expected in "${EXPECTED[@]}"; do
    found=0
    for actual in "${ACTUAL_FILES[@]}"; do
        if [ "$expected" = "$actual" ]; then
            found=1
            break
        fi
    done
    if [ $found -eq 0 ]; then
        MISSING+=("$expected")
        FAILED=1
    fi
done

# Check for extra files
for actual in "${ACTUAL_FILES[@]}"; do
    found=0
    for expected in "${EXPECTED[@]}"; do
        if [ "$expected" = "$actual" ]; then
            found=1
            break
        fi
    done
    if [ $found -eq 0 ]; then
        EXTRA+=("$actual")
        FAILED=1
    fi
done

# Print results
if [ $FAILED -eq 0 ]; then
    echo -e "${GREEN}✓ All tests passed!${NC}"
    echo ""
    echo "Verified behaviors:"
    echo "  ✓ .gitignore patterns work (*_test.go, *_generated.go, mock_*.go, *.log ignored)"
    echo "  ✓ .redunceignore patterns work (secrets.go, *_draft.go, vendor/ ignored)"
    echo "  ✓ .redunceignore takes precedence over .gitignore at each directory level"
    echo "  ✓ Negation patterns (!) in .redunceignore successfully override .gitignore"
    echo "  ✓ Explicit includes (!) bypass go-enry filter (important.log, config.json included)"
    echo "  ✓ Hierarchical ignore files work (parent patterns inherited)"
    echo "  ✓ Per-directory ignore files work independently"
    exit 0
else
    echo -e "${RED}✗ Tests failed!${NC}"

    if [ ${#MISSING[@]} -gt 0 ]; then
        echo -e "${RED}Missing files (should be included):${NC}"
        printf '  %s\n' "${MISSING[@]}"
    fi

    if [ ${#EXTRA[@]} -gt 0 ]; then
        echo -e "${RED}Extra files (should be ignored):${NC}"
        printf '  %s\n' "${EXTRA[@]}"
    fi

    echo ""
    echo "Test directory preserved at: $TEST_DIR"
    echo "You can inspect it manually."
    exit 1
fi
