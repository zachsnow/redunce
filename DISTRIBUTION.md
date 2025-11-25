# Distribution Guide

This document explains how to package and distribute `redunce` via various channels.

## Homebrew Distribution

### Quick Start

This repository is configured as a Homebrew tap. To release a new version:

1. **Run the release script**:

   ```bash
   ./release.sh
   ```

   This will:
   - Tag the current commit with the version from `main.go`
   - Push the tag to GitHub
   - Calculate SHA256 checksums for the release tarball and sqlite-vector
   - Update `Formula/redunce.rb` with the correct SHA256 values
   - Commit and push the updated formula
   - Optionally test the installation locally

2. **Create a GitHub release** at the generated URL

Users can then install with:

```bash
brew tap ZachSnow/redunce https://github.com/ZachSnow/redunce
brew install redunce
```

### Repository Structure

This repository follows the Homebrew tap structure:

```
redunce/
├── Formula/
│   └── redunce.rb      # Homebrew formula
├── release.sh          # Automated release script
├── main.go             # Version source of truth
└── ...
```

The formula is maintained in the same repository as the source code, eliminating the need for a separate `homebrew-redunce` repository.

### Submit to Homebrew Core

Once your formula is stable and tested:

1. Fork [homebrew-core](https://github.com/Homebrew/homebrew-core)
2. Add your formula to `Formula/r/redunce.rb`
3. Run `brew audit --strict redunce`
4. Submit a PR

Users will then install with just:

```bash
brew install redunce
```

## GoReleaser (All Platforms)

[GoReleaser](https://goreleaser.com/) can automate releases for multiple platforms:

```yaml
# .goreleaser.yml
before:
  hooks:
    - go mod tidy

builds:
  - env:
      - CGO_ENABLED=1
      - CGO_CFLAGS=-DSQLITE_ENABLE_LOAD_EXTENSION=1
    goos:
      - linux
      - darwin
    goarch:
      - amd64
      - arm64

archives:
  - format: tar.gz
    name_template: >-
      {{ .ProjectName }}_
      {{- title .Os }}_
      {{- if eq .Arch "amd64" }}x86_64
      {{- else if eq .Arch "386" }}i386
      {{- else }}{{ .Arch }}{{ end }}

brews:
  - tap:
      owner: ZachSnow
      name: homebrew-redunce
    homepage: "https://github.com/ZachSnow/redunce"
    description: "Find potentially redundant code using vector similarity"
    install: |
      bin.install "redunce"
    test: |
      system "#{bin}/redunce", "--help"

checksum:
  name_template: "checksums.txt"

changelog:
  sort: asc
```

Note: You'll still need to handle the sqlite-vector extension separately for each platform.

## Docker Distribution

Create a multi-stage Dockerfile:

```dockerfile
FROM golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache gcc musl-dev sqlite-dev

WORKDIR /build

# Copy source
COPY . .

# Download sqlite-vector extension
RUN wget https://github.com/sqliteai/sqlite-vector/releases/download/0.9.52/vector-linux-x64-0.9.52.tar.gz
RUN tar -xzf vector-linux-x64-0.9.52.tar.gz

# Build
RUN CGO_ENABLED=1 CGO_CFLAGS="-DSQLITE_ENABLE_LOAD_EXTENSION=1" go build -o redunce

FROM alpine:latest

RUN apk add --no-cache sqlite-libs

COPY --from=builder /build/redunce /usr/local/bin/
COPY --from=builder /build/libvector.so /usr/lib/

ENTRYPOINT ["redunce"]
```

Build and publish:

```bash
docker build -t zachsnow/redunce:latest .
docker push zachsnow/redunce:latest
```

## Key Considerations

### Embedded Assets

The `redunce` binary includes embedded assets:

1. **Default ignore patterns** (`internal/scanner/redunceignore.default`) - Embedded in the binary, used by default
2. **Claude Code commands** (`claude/commands/*.md`) - Embedded in the binary, can be installed via `--install-claude-commands`

These assets are embedded using Go's `embed` package, so they're part of the binary and don't require separate distribution.

### Extension Dependency

The main complexity in distributing redunce is the sqlite-vector extension dependency:

1. **Bundled approach** (recommended): Include the extension in the package/formula
2. **Separate package**: Create a separate package for sqlite-vector and depend on it
3. **Build from source**: Have users build both from source

The current implementation searches for the extension in:

- `./libvector` (current directory)
- `libvector` (system library paths)
- `/opt/homebrew/lib/libvector` (Homebrew Apple Silicon)
- `/usr/local/lib/libvector` (Homebrew Intel / Linux)
- `/usr/lib/libvector` (Standard Linux)

### Platform-Specific Extension Files

- **macOS**: `libvector.dylib` (must be code-signed)
- **Linux**: `libvector.so`
- **Windows**: `libvector.dll` (if supported in future)

### Code Signing (macOS)

macOS binaries and libraries should be code-signed. The Homebrew formula handles this automatically, but for manual distribution:

```bash
codesign --remove-signature libvector.dylib
codesign -s - libvector.dylib
```

## Testing Your Distribution

Before releasing:

1. Test installation in a clean environment (VM or Docker container)
2. Verify the extension loads correctly: `redunce --help` should not error
3. Test basic functionality: `redunce ./testdata`
4. Check that the extension is found in the expected location

## GitHub Releases

For direct binary distribution:

1. Create a release on GitHub
2. Attach pre-built binaries for each platform
3. Include the sqlite-vector extension in the archive
4. Provide clear installation instructions

Example release structure:

```
redunce-v0.1.0-darwin-arm64.tar.gz
  ├── redunce
  └── libvector.dylib

redunce-v0.1.0-linux-amd64.tar.gz
  ├── redunce
  └── libvector.so
```
