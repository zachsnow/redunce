# Distribution Guide

How to release `redunce` via Homebrew.

## Releasing a New Version

1. Update the version in `main.go`

2. Run the release script:

   ```bash
   ./release.sh
   ```

   This will:
   - Tag the current commit with the version from `main.go`
   - Push the tag to GitHub
   - Calculate SHA256 checksums for the release tarball and sqlite-vector
   - Update `Formula/redunce.rb` with the correct SHA256 values
   - Commit and push the updated formula

3. Create a GitHub release at the generated URL

Users install with:

```bash
brew tap ZachSnow/redunce https://github.com/ZachSnow/redunce
brew install redunce
```

## Repository Structure

```
redunce/
├── Formula/
│   └── redunce.rb      # Homebrew formula
├── release.sh          # Automated release script
└── main.go             # Version source of truth
```

The formula is maintained in the same repository as the source code.

## Extension Dependency

The sqlite-vector extension is bundled in the Homebrew formula. The binary searches for it in:

- `./libvector` (current directory)
- `/opt/homebrew/lib/libvector` (Homebrew Apple Silicon)
- `/usr/local/lib/libvector` (Homebrew Intel / Linux)
- `/usr/lib/libvector` (Standard Linux)

Platform-specific files:
- **macOS**: `libvector.dylib`
- **Linux**: `libvector.so`

## Code Signing (macOS)

The Homebrew formula handles code signing automatically. For manual builds:

```bash
codesign --remove-signature libvector.dylib
codesign -s - libvector.dylib
```

## Testing

Before releasing:

1. `brew uninstall redunce` (if installed)
2. `brew install --build-from-source Formula/redunce.rb`
3. `redunce --help` (verify extension loads)
4. `redunce .` (verify basic functionality)
