# Claude Code Commands for Redunce

This directory contains custom Claude Code slash commands for the redunce tool.

## Available Commands

### `/redunce` - Interactive Mode

Guides you through an iterative code deduplication workflow with approval at each step.

**Use when:**
- You're new to redunce
- You want to review each decision
- You're working on critical code

**Workflow:**
1. Sets up `.redunceignore` with appropriate patterns
2. Runs redunce to find duplication clusters
3. Proposes either ignoring or refactoring each cluster
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
1. Automatically sets up `.redunceignore`
2. Runs redunce iteratively
3. Makes decisions automatically (ignore vs refactor)
4. Applies changes immediately
5. Runs tests to verify changes (reverts if they fail)
6. Reports progress between iterations

## Decision Guidelines

Both commands follow these principles:
- **Be conservative**: Don't force abstractions that hurt readability
- **Prefer size reduction**: Prioritize refactorings that significantly reduce code
- **Context matters**: Similar code isn't always duplication
- **Test safety**: Maintain test coverage, revert if tests fail

## Installation

You can install these commands at two levels:

```bash
# User-level (available in all projects)
redunce --install-claude-commands

# Project-level (available only in this project)
redunce --install-claude-commands .
```

**Which should I use?**
- **User-level** - Install once, use everywhere
- **Project-level** - Customize per project (e.g., modify prompts for project-specific workflows)

## Examples

```bash
# Interactive mode - review each change
/redunce

# Auto-approve mode - let Claude work autonomously
/redunce-approve
```

## Tips

- Start with `/redunce` to understand how it works
- Use `/redunce-approve` once you're comfortable with the tool
- The commands work best on projects with test suites
- Review the git diff after completion to verify changes
