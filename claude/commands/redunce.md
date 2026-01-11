# Redunce Interactive Mode

You are going to help the user reduce code duplication using the `redunce` tool.

## Key Concepts

There are two distinct mechanisms:

1. **Ignoring files** (`.redunceignore`) - Excludes entire files/directories from analysis
2. **Skipping clusters** (`--skip`) - Marks a specific duplication cluster as acceptable

## Overview

`redunce` is a code duplication detection tool that clusters similar code blocks. Your job is to iteratively:

1. Configure `.redunceignore` to exclude files that shouldn't be analyzed
2. Find duplication clusters
3. Propose refactorings or skipping clusters
4. Apply approved changes
5. Repeat until no further improvements are possible

## Initial Setup

First, check if a `.redunceignore` file exists. If not, create one with sensible defaults based on the
project structure.

Evaluate whether any file patterns should be added to `.redunceignore`. Consider:

- Test files that intentionally have duplicated setup/teardown code
- Generated code
- Vendor/third-party code
- Configuration files with repeated patterns
- Mock/fixture files

If you identify patterns that should be ignored, propose them to the user and add them to `.redunceignore` after approval.

## Iterative Deduplication Loop

Repeat the following steps 3-5 times, or until no further progress is possible:

### 1. Run Redunce

Run: `redunce . --limit 10`

This will output clusters of similar code. Each cluster represents potential duplication.

### 2. Analyze Clusters

For each cluster, evaluate:

- **Should it be skipped?** Some duplication is intentional (e.g., similar but semantically different code)
- **Can it be refactored?** Look for opportunities to extract common patterns
- **Impact on maintainability**: Prefer refactorings that make code smaller and more maintainable

### 3. Propose Changes

For each cluster, propose ONE of:

a) **Update `.redunceignore`**: If the duplication is an entire file and is intentional/acceptable

- Add the file path to the project-level `.redunceignore`
- Explain why this duplication should be preserved

b) **Skip the cluster**: If the duplication is intentional/acceptable

- Run `redunce --skip <cluster index or SHA>`
- Explain why this duplication should be preserved

c) **Refactor**: If the code can be deduplicated

- Provide a concrete refactoring plan
- Show a sketch of the proposed abstraction
- Explain how it improves maintainability
- Estimate lines saved
- Prioritize refactorings that significantly reduce code size

d) **Defer**: If unsure

### 4. Apply Approved Changes

Wait for user approval, then apply the changes:

- Skip clusters using `redunce --skip` if approved
- Implement refactorings if approved
- Run tests if they exist

### 5. Check Progress

After applying changes, re-run `redunce` to see the new state.

## Completion

After 3-5 iterations, check in with the user:

- Summarize what was accomplished
- Show metrics (total lines reduced, clusters addressed, clusters skipped)
- Ask if they want to continue for another round

## Guidelines

- **Be conservative**: Don't force abstractions that hurt readability
- **Context matters**: Similar code isn't always duplicated - consider semantic differences
- **Small wins**: Even reducing 20-30 lines across a few files is valuable
- **Test preservation**: Always maintain test coverage
- **One cluster at a time**: Don't try to fix everything at once

Start by examining the project structure and setting up `.redunceignore`.
