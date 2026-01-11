# Redunce Auto-Approve Mode

You are going to help the user reduce code duplication using the `redunce` tool in **auto-approve** mode.

## Overview

`redunce` is a code duplication detection tool that clusters similar code blocks.

This is the automatic mode of `redunce`. You will make decisions about skipping or refactoring
duplication clusters WITHOUT asking for approval at each step. Only check in with the user
between major iterations. Your job is to iteratively:

1. Configure `.redunceignore` to exclude files that shouldn't be analyzed
2. Find duplication clusters
3. Automatically decide whether to skip or refactor
4. Apply changes automatically
5. Repeat until no further improvements are possible

## Initial Setup

First, check if a `.redunceignore` file exists. If not, create one with sensible defaults based on the project
structure.

Then evaluate whether any additional file patterns should be added to `.redunceignore`. Consider:

- Test files that intentionally have duplicated setup/teardown code
- Generated code
- Vendor/third-party code
- Configuration files with repeated patterns
- Mock/fixture files

**Automatically add these patterns** to `.redunceignore` if they make sense for the project.

## Auto-Approve Iteration Loop

Repeat the following steps 3-5 times, or until no further progress is possible:

### 1. Run `redunce`

Run: `redunce . --limit 10`

This will output clusters of similar code. Each cluster represents potential duplication.

### 2. Analyze and Decide

For each cluster, **automatically decide**:

a) **Add to `.redunceignore`**: If the duplication is an entire file, and is intentional/acceptable
   - Common patterns: test files, similar but semantically different code, acceptable boilerplate
   - Add pattern to `.redunceignore` immediately
   - Log: "Ignoring cluster X: [reason]"

b) **Skip the cluster**: If the duplication is intentional/acceptable
   - Similar but semantically different code, acceptable boilerplate
   - Run `redunce --skip <index>` immediately
   - Log: "Skipped cluster X: [reason]"

c) **Refactor**: If the code can be deduplicated
   - Extract common patterns into functions/methods
   - Prioritize refactorings that make code significantly smaller and more maintainable
   - Implement the refactoring immediately
   - Log: "Refactored cluster X: [description] - saved ~N lines"

c) **Defer**: If uncertain
   - Log: "Deferred cluster X: [reason]"

### 3. Apply Changes Automatically

For each decision:
- Add files to `.redunceignore`, skip using `redunce --skip`, implement refactorings for deduplicated code
- Check types and fix type errors (if possible)
- Run tests after each significant change (if tests exist)
- If tests fail, revert the change and log the failure

### 4. Re-run and Continue

After processing clusters from one run, re-run `redunce` to see the new state and continue.

## Decision Guidelines

Be aggressive but smart:
- **Safe refactorings**: Extract common patterns that are clearly identical
- **Preserve semantics**: Don't merge code that looks similar but does different things
- **Readability first**: If an abstraction would hurt clarity, skip it
- **Test-driven**: Run tests frequently, revert if anything breaks
- **Meaningful size reduction**: Prioritize refactorings that save 15+ lines

## Iteration Check-in

After each complete iteration (processing all clusters once), briefly report:
- Files added to `.redunceignore`
- Clusters skipped (count)
- Clusters refactored (count + lines saved)
- Clusters deferred
- Tests status

Then continue to the next iteration automatically.

## Completion

After 3-5 iterations, or when no more significant clusters appear, provide a summary:
- Total clusters addressed
- Total lines saved
- Clusters skipped
- Test results
- Ask: "Continue for another round?"

## Critical Rules

- **Auto-apply everything**: Don't ask for permission on individual changes
- **Test safety**: Always run tests after refactorings, revert if they fail
- **Log decisions**: Explain each decision briefly as you go
- **Be bold but safe**: Make changes confidently, but preserve correctness

Start by examining the project structure and setting up `.redunceignore`, then begin the first iteration.
