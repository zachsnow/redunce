# REDUNCE REDUNCE REDUNCE

`redunce` is a golang CLI tool for identifying potentially-redundant text in a file or collection of files.
The intention is that an agent like Codex or Claude will use this tool to refactor its work after making
large-scale changes to a codebase.

# Usage

To use `redunce` simply pass it list of files and/or directories. It operates recursively over directories.
If you don't pass any files or directories, it prints usage information.

```bash
$ redunce <file-or-directory ...>
```

Other options:

```
  --threshold <float> : set the similarity threshold (0 - 1)
  --embed <local|openai> : use a local embedding vs. sending to OpenAI; eventually other options
  --format <json|md> : emit JSON or Markdown; eventually other options
```

Specific options for particular components:

```
  --local-min-lines : minimum lines per chunk for local
  --local-max-lines : maximum lines per chunk for local embedding
  --local-step-lines : how many lines to step by
  --treesitter-min : minimum node-size
  --treesitter-max : maximum node-size
  --openai-api-key : API key for OpenAI; also checks the `OPENAI_API_KEY` environment variable
  --db <file> : the location of the database file; defaults to redunce.db in the working directory
```

## Claude Code Integration

`redunce` includes built-in slash commands for [Claude Code](https://claude.ai/claude-code), making it easy to systematically eliminate code duplication using an AI-assisted workflow.

### Installation

After installing `redunce`, run:

```bash
# User-level installation (available in all projects)
redunce --install-claude-commands

# Project-level installation (available only in current project)
redunce --install-claude-commands .
```

This installs two slash commands:
- `/redunce` - Interactive mode with approval at each step
- `/redunce-approve` - Auto-approve mode for autonomous refactoring

**User-level vs Project-level:**
- **User-level** (`~/.claude/commands/`) - Commands available in any project where you use Claude Code
- **Project-level** (`./.claude/commands/`) - Commands only available when working in this specific project

Use user-level for general availability, or project-level if you want project-specific customization.

### Using the Commands

In any project, simply type `/redunce` or `/redunce-approve` in Claude Code, and Claude will:

1. **Set up `.redunceignore`** - Automatically identify and add patterns for test files, generated code, etc.
2. **Run iterative analysis** - Execute `redunce . -limit 10` to find duplication clusters
3. **Evaluate clusters** - For each cluster, decide whether to:
   - Add it to `.redunceignore` (intentional duplication)
   - Refactor it to eliminate duplication (preferring changes that reduce code size)
4. **Apply changes** - In approve mode, changes are automatic; in interactive mode, Claude asks for approval
5. **Repeat** - Continue for 3-5 iterations or until no progress is made
6. **Check in** - Ask if you want to continue

### Interactive vs Auto-Approve

- **`/redunce`** - Best for learning the tool or when you want full control over each decision
- **`/redunce-approve`** - Best for trusted codebases where you want Claude to work autonomously (it will still run tests and revert if they fail)

For more details, see `claude/README.md` in this repository.

## Ignore Patterns

### Default Patterns

`redunce` includes embedded default ignore patterns for common files that shouldn't be analyzed:
- Binary files (`.exe`, `.dll`, `.so`, etc.)
- Media files (images, videos, audio)
- Package manager lock files
- Build artifacts and dependencies
- Documentation files (`.md`, `.txt`, etc.)
- Version control directories (`.git/`, `.svn/`, etc.)
- Hidden files (`.*`)

### Custom Ignore Files

You can override or extend the defaults by creating ignore files:

**User-level** (applies to all projects):
- `~/.config/redunce/ignore` (recommended, XDG-compliant)
- `~/.redunceignore` (alternative, simpler)

**Project-level**:
- `.redunceignore` in your project root
- `.gitignore` is also respected

### Pattern Precedence

Patterns are applied in order: **embedded defaults → user global → .gitignore → .redunceignore**

Use `!pattern` in later files to un-ignore patterns from earlier sources.

### Skipping Defaults

Use `--no-default-ignore` to skip embedded defaults entirely, forcing complete specification via user/project ignore files.

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

## Scanning

First, `redunce` collects all files and nested files, so that it has a single large list of absolute filenames.

The scanner automatically filters out:

- Binary files (executables, images, archives, etc.)
- Vendored code (detected via [enry](https://github.com/go-enry/go-enry))
- Generated files (auto-generated code)
- Configuration files (`go.mod`, `package.json`, etc.)
- Documentation files (README, `.md`, `.txt`, etc.)
- Files matching `.gitignore` patterns
- Files matching `.redunceignore` patterns

## Chunking

It then iterates over this list, chunking the files into smaller and smaller parts. It prefers to use a
syntax-aware chunker (via [treesitter](github.com/tree-sitter/go-tree-sitter)), but falls back to a
line-based chunker if it doesn't understand a particular format. Each chunk has the following form:

```sql
language: swift
path: Sources/Foo/Bar.swift
lines: 42-87

func foo(bar: Baz) -> Qux {
  // ...
}
```

That is, it includes the source language, the path, the line numbers, and the actual code. Chunks can "contain"
other chunks in the sense that we have a chunk for a class, and then chunks for each function in the class.

The syntax aware chunker iterates over the set of nodes in the AST and works from the bottom up,
starting with nodes of the minimum size and rendering larger and larger trees until reaching the maximum chunk size.
"Size" is basically node depth, but only for "meaningful" nodes.

The line-based chunker starts with the smallest number of lines and generates those chunks, then increments
the number of lines by the step count (defaults to 1).

## Embedding

Next we embed the chunks as vectors and enter them into a SQLite-Vector database. For embedding we use either
an [OpenAI](github.com/openai/openai-go) embedding via text-embedding-3-small (1536-dim), batching in
64–128 chunks per request; for local we will eventually load a small ONNX model from a file embedded with go:embed,
using [onnxruntime](https://github.com/yalue/onnxruntime_go).

## Store

We use [SQLite-Vector](https://github.com/sqliteai/sqlite-vector) and [SQLite](https://github.com/mattn/go-sqlite3)
to store embeddings generated by the embedding step. The database file lives in the working directory where
`redunce` was run (or can be set as an option). Our table looks like:

```sql
--- chunks --
CREATE TABLE chunks (
  id INTEGER PRIMARY KEY,
  path TEXT NOT NULL,
  modifiedAt INTEGER NOT NULL, -- modification time of file, in seconds
  language TEXT NOT NULL,
  start_line INTEGER NOT NULL,
  end_line INTEGER NOT NULL,
  code TEXT NOT NULL,
  embedding VECTOR(1536) NOT NULL -- or whatever dim your model uses
);

CREATE INDEX chunks_embedding_idx ON chunks(embedding);

CREATE TABLE IF NOT EXISTS clusters (
  id INTEGER PRIMARY KEY,
  created_at TEXT DEFAULT CURRENT_TIMESTAMP,

  canonical_chunk_id INTEGER,
  avg_similarity REAL,
  max_similarity REAL,
  size INTEGER,

  FOREIGN KEY (canonical_chunk_id) REFERENCES chunks(id)
);

CREATE TABLE IF NOT EXISTS cluster_chunks (
  cluster_id INTEGER NOT NULL,
  chunk_id INTEGER NOT NULL,

  similarity REAL NOT NULL, -- similarity to canonical, 0–1

  PRIMARY KEY (cluster_id, chunk_id),

  FOREIGN KEY (cluster_id) REFERENCES clusters(id),
  FOREIGN KEY (chunk_id) REFERENCES chunks(id)
);

```

## Clustering

Once our database is populated we simply iterate over the chunks in order of length of code, and process them
as follows, ignoring already-marked clusters (see below):

First, we find similar chunks:

```sql
WITH q AS (
  SELECT embedding FROM chunks WHERE id = :id
)
SELECT
c.id,
c.path,
c.language,
c.start_line,
c.end_line,
vector_cosine_distance(c.embedding, q.embedding) AS dist
FROM chunks c, q
WHERE c.id != :id
ORDER BY dist ASC
LIMIT 10;
```

Next we record the most-similar chunks in a cluster (assuming that the similarity threshold is met).
We mark all entered chunks so we don't reprocess them later. We start with the canonical chunk being the
first chunk found for the cluster.

At the end we have a bunch of clusters that have a canonical chunk that we can suggest is used to
refactor all the related/similar chunks. But, we could consider reprocessing each chunk to determine which
is most similar to the average of the chunk, too.

## Output

Once we have a set of clusters, we simply iterate over them and emit them in the format requested.
Something like:

```md
# chunk 1:

language: swift

path: path1
lines: ...
<code>

path: path2
lines: ...
<code>

# chunk 2:

language: typescript

path: path1
lines: ...
<code>

path: path2lines: ...
<code>

path: path3
lines: ...
<code>
...
```

## Updates

Once a codebase has been analyzed and inserted into the database, we can reanalyze it by scanning the codebase,
getting the modified time of each file, and deleting and reanalyzing a file's chunks only when the file has
changed since the chunks were created. Then we delete all clusters and re-cluster the chunks.

By default `redunce` _always_ updates the current database; you can pass `--reset` if you want to reset the
database first.

## Search

Once you have an up-to-date chunk database you can also ask it questions about code that matches some query:

```bash
$ redunce -q "some query"
```
