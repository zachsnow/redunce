# Distribution Guide

How to release `redunce` via Homebrew.

## Repository Structure

Redunce uses two repositories:

- **[zachsnow/redunce](https://github.com/zachsnow/redunce)** - Source code, releases
- **[zachsnow/homebrew-redunce](https://github.com/zachsnow/homebrew-redunce)** - Homebrew tap (formula only)

## Setup

Clone both repositories as siblings:

```
projects/
├── redunce/           # This repo
└── homebrew-redunce/  # Tap repo (git clone git@github.com:zachsnow/homebrew-redunce.git)
```

## Releasing a New Version

1. Update the version in `main.go` (the `Version` constant)

2. Commit and ensure working directory is clean

3. Run the release script:

   ```bash
   ./release.sh
   ```

   This will:
   - Tag the current commit with the version from `main.go`
   - Push the tag to GitHub
   - Download the release tarball and calculate its SHA256
   - Calculate SHA256 for the sqlite-vector dependency
   - Update the formula in `../homebrew-redunce/`
   - Commit and push the formula update
   - Optionally test the formula

4. Create a GitHub release at the generated URL

## Version Constants

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
2. `brew install --build-from-source ../homebrew-redunce/Formula/redunce.rb`
3. `redunce --help` (verify extension loads)
4. `redunce .` (verify basic functionality)
