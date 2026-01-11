# Claude Code Commands for Redunce

This directory contains custom Claude Code slash commands for the `redunce` tool.

## Available Commands

### `/redunce` - Interactive Mode

Guides you through an iterative code deduplication workflow with approval at each step.

**Use when:**

- You're new to `redunce`
- You want to review each decision
- You're working on critical code

**Workflow:**

1. Sets up `.redunceignore` with appropriate file patterns
2. Runs `redunce` to find duplication clusters
3. Proposes either skipping or refactoring each cluster
4. Waits for your approval before applying changes
5. Repeats 3-5 times or until no progress
6. Checks in before continuing

### `/redunce-approve` - Auto-Approve Mode

Automatically makes decisions and applies changes without asking for approval.

**Use when:**

- You trust Claude to make good decisions
- You want to quickly clean up a codebase
- You have good test coverage to catch issues

**Workflow:**

1. Automatically sets up `.redunceignore` for files to exclude
2. Runs `redunce` iteratively
3. Makes decisions automatically (ignore vs skip vs refactor)
4. Applies changes immediately
5. Runs tests to verify changes (reverts if they fail)
6. Reports progress between iterations

## Decision Guidelines

Both commands follow these principles:

- **Be conservative**: Don't force abstractions that hurt readability
- **Prefer size reduction**: Prioritize refactorings that significantly reduce code
- **Context matters**: Similar code isn't always duplication
- **Test safety**: Maintain test coverage, revert if tests fail

## Tools Reference

| Action | Command | Purpose |
|--------|---------|---------|
| Exclude files from analysis | Add pattern to `.redunceignore` | Entire files/directories won't be scanned |
| Mark cluster as acceptable | `redunce --skip <index or SHA>` | Cluster won't be reported again |
| Adjust skip matching | `--skip-threshold 0.95` | Higher = stricter matching for skipped clusters |
