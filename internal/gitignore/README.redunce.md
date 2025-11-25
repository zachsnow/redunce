# Vendored go-gitignore Library

This directory contains a modified version of [`denormal/go-gitignore`](https://github.com/denormal/go-gitignore) (MIT License).

## Modifications

We've modified the library to support checking multiple ignore filenames at each directory level with priority ordering:

### Key Changes:

1. **Multiple filename support**: Added `_files []string` field to `repository` struct
2. **New constructor**: `NewRepositoryWithFiles(base, files, cache, errors)`
3. **Priority-based matching**: At each directory level, files are checked in the order provided

### Usage:

```go
// Check .redunceignore first, then .gitignore at each directory level
files := []string{".redunceignore", ".gitignore"}
matcher := gitignore.NewRepositoryWithFiles(baseDir, files, nil, nil)
```

### Behavior:

- Walks from leaf to root (git-like semantics)
- At each directory level, checks files in priority order
- First match wins (allows `.redunceignore` to override `.gitignore`)
- Supports negation patterns (`!`) for un-ignoring
- Handles nested ignore files automatically

## Original Library

- **Source**: https://github.com/denormal/go-gitignore
- **License**: MIT
- **Version**: Vendored from commit (see git history)

## Why Vendor?

We needed to support checking multiple ignore filenames (`.redunceignore` and `.gitignore`) at each directory level with `.redunceignore` taking precedence. The upstream library only supports a single filename per repository.

If these changes prove valuable, we may contribute them back upstream. It seems like a lot of developer tools
try to respect `.gitignore` and add their own ignore file on top.
