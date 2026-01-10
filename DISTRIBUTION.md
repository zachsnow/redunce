# Distribution Guide

How to release `redunce` via Homebrew.

## Releasing a New Version

1. Update the version in `main.go` (the `Version` constant is the source of truth)

2. Commit and ensure working directory is clean

3. Run the release script:

   ```bash
   ./release.sh
   ```

   This will:
   - Tag the current commit with the version from `main.go`
   - Push the tag to GitHub
   - Wait for GitHub to process the tag
   - Download the release tarball and calculate its SHA256
   - Calculate SHA256 for the sqlite-vector dependency
   - Update `Formula/redunce.rb` with the new version and SHA256 values
   - Commit and push the updated formula
   - Optionally test the formula by building from source

4. Create a GitHub release at the generated URL

The formula always contains the values from the most recent release. The release
script uses regex to find and replace version numbers and SHA256 hashes.

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
└── main.go             # Version constants (source of truth)
```

The formula is maintained in the same repository as the source code.

Version constants in `main.go`:
- `Version` - The redunce version (e.g., "0.1.1")
- `SQLiteVectorVersion` - The sqlite-vector dependency version (e.g., "0.9.52")

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
