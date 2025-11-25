# REDUNCE REDUNCE REDUNCE

> Stupid redundancy detector.

`redunce` is a golang CLI tool for identifying potentially-redundant text in a file or collection of files.
The intention is that an agent like Codex or Claude will use this tool to refactor its work after making
large-scale changes to a codebase.

# Usage

To use `redunce` simply pass it list of files and/or directories. It operates recursively over directories.
If you don't pass any files or directories, it prints usage information.

```bash
$ redunce <file-or-directory ...>
```

## Ignore Patterns

By default `redunce` ignores common files that shouldn't be analyzed, including those specified in `.gitignore`.
You can override or extend this behavior by creating `.redunceignore` files, which share a syntax and semantics
with `.gitignore`, and have a higher precedence.

**User-level** (applies to all projects):

- `~/.config/redunce/ignore`
- `~/.redunceignore`

**Project-level**:

- `.redunceignore` in project root or nested directories

Use `--no-default-ignore` to skip default ignore patterns.

## Claude Code Integration

`redunce` includes built-in slash commands for [Claude Code](https://claude.ai/claude-code), making it easy to systematically eliminate code duplication using an AI-assisted workflow.

After installing `redunce`, run:

```bash
# User-level installation (available in all projects)
redunce --install-claude-commands

# Project-level installation (available only in current project)
redunce --install-claude-commands <directory>
```

This installs two slash commands:

- `/redunce` - Interactive mode with approval at each step
- `/redunce-approve` - Auto-approve mode for autonomous refactoring

In any project, simply type `/redunce` or `/redunce-approve` in Claude Code, and Claude will:

1. **Set up `.redunceignore`** - Automatically identify and add patterns for test files, generated code, etc.
2. **Run iterative analysis** - Execute `redunce . -limit 10` to find duplication clusters
3. **Evaluate clusters** - For each cluster, decide whether to:
   - Add it to `.redunceignore` (intentional duplication)
   - Refactor it to eliminate duplication (preferring changes that reduce code size)
4. **Apply changes** - In approve mode, changes are automatic; in interactive mode, Claude asks for approval
5. **Repeat** - Continue for 3-5 iterations or until no progress is made
6. **Check in** - Ask if you want to continue

For more details, see `claude/README.md` in this repository.

# Building

## Prerequisites

- Go 1.25.4 or later
- [sqlite-vector](https://github.com/sqliteai/sqlite-vector) extension (required for vector similarity search)

## Setup sqlite-vector extension

`redunce` requires the sqlite-vector extension to be available at runtime. You can download the pre-built binary:

1. Download the latest release for your platform from [sqlite-vector releases](https://github.com/sqliteai/sqlite-vector/releases)

For macOS (Apple Silicon or Intel):

```bash
   curl -L https://github.com/sqliteai/sqlite-vector/releases/download/0.9.52/vector-apple-xcframework-0.9.52.zip -o vector.zip
   unzip vector.zip
   cp vector.xcframework/macos-arm64_x86_64/vector.framework/vector libvector.dylib
```

2. Code-sign the extension (macOS only):

```bash
codesign --remove-signature libvector.dylib
codesign -s - libvector.dylib
```

3. Place `libvector.dylib` (or `libvector.so` on Linux) in the project root directory

## Build the binary

Once the extension is in place, build `redunce`:

```bash
$ ./build.sh
```

This will create the `redunce` binary in the current directory. You can then move it to your `$PATH`:

```bash
mv redunce /usr/local/bin/
```

The `libvector` extension must remain accessible to the binary at runtime (either in the same directory as the binary, or in a system library path).

## Cleaning

Clean build artifacts with:

```bash
$ ./clean.sh
```

# Algorithm

See [algorithm](ALGORITHM.md) for information about the stupid algorithm.
