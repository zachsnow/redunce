# Distribution Guide

This document explains how to package and distribute `redunce` via various channels.

## Homebrew Distribution

### Quick Start

1. **Create a GitHub release** with a source tarball:
   ```bash
   git tag v0.1.0
   git push origin v0.1.0
   ```

2. **Calculate SHA256 of the release tarball**:
   ```bash
   curl -L https://github.com/ZachSnow/redunce/archive/refs/tags/v0.1.0.tar.gz | shasum -a 256
   ```

3. **Calculate SHA256 of sqlite-vector**:
   ```bash
   curl -L https://github.com/sqliteai/sqlite-vector/releases/download/0.9.52/vector-apple-xcframework-0.9.52.zip | shasum -a 256
   ```

4. **Update the formula** (`redunce.rb`) with the correct SHA256 values

5. **Test the formula locally**:
   ```bash
   brew install --build-from-source ./redunce.rb
   ```

6. **Submit to Homebrew**:
   - For a personal tap: Create a repo `homebrew-redunce` and push the formula
   - For main Homebrew: Submit a PR to [homebrew-core](https://github.com/Homebrew/homebrew-core)

### Personal Tap (Recommended for Initial Distribution)

Create your own Homebrew tap:

```bash
# Create a new repo called homebrew-redunce
gh repo create ZachSnow/homebrew-redunce --public

# Clone and add the formula
git clone https://github.com/ZachSnow/homebrew-redunce
cd homebrew-redunce
cp ../redunce/redunce.rb Formula/redunce.rb
git add Formula/redunce.rb
git commit -m "Add redunce formula"
git push
```

Users can then install with:
```bash
brew tap ZachSnow/redunce
brew install redunce
```

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

## Linux Distribution

### Debian/Ubuntu (.deb package)

You can use [nFPM](https://nfpm.goreleaser.com/) to create .deb packages:

```yaml
# nfpm.yaml
name: "redunce"
arch: "amd64"
platform: "linux"
version: "v0.1.0"
maintainer: "Your Name <your@email.com>"
description: "Find potentially redundant code using vector similarity"
license: "MIT"
homepage: "https://github.com/ZachSnow/redunce"

contents:
  - src: ./redunce
    dst: /usr/local/bin/redunce
  - src: ./libvector.so
    dst: /usr/lib/libvector.so

scripts:
  postinstall: |
    echo "redunce installed to /usr/local/bin/redunce"
    echo "sqlite-vector extension installed to /usr/lib/libvector.so"
```

Build with:
```bash
nfpm pkg --packager deb
```

### RPM (Fedora/RHEL)

Similar to above, but use `--packager rpm`:
```bash
nfpm pkg --packager rpm
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
  name_template: 'checksums.txt'

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
