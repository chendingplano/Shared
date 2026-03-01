# Jujutsu (jj) Quick Reference

Jujutsu is a version control system that provides a more flexible workflow than traditional Git, with powerful conflict resolution and commit manipulation capabilities.

## Quick Reference

### Status and Inspection

| Command | Description |
|---------|-------------|
| `jj status`, `jj st` | Show working copy status |
| `jj log` | View commit history |
| `jj log -r main..@` | View commits between main and working copy |
| `jj diff` | Show changes in working copy |
| `jj diff -r @-` | Show changes relative to parent commit |
| `jj show <revision>` | Show changes in a specific commit |

### Making Changes

| Command | Description |
|---------|-------------|
| `jj commit` | Commit working copy changes (interactive) |
| `jj commit -m "msg"` | Commit with message |
| `jj new` | Create a new empty commit on top of current |
| `jj amend` | Amend working copy changes into parent commit |
| `jj squash` | Squash current commit into parent |

### Bookmarks (Branches)

| Command | Description |
|---------|-------------|
| `jj bookmark list`, `jj bl` | List all bookmarks |
| `jj bookmark create <name>` | Create a new bookmark |
| `jj bookmark track <name> --remote=origin` | Track a remote bookmark |
| `jj bookmark set <name> -r <revision>` | Move bookmark to a revision |
| `jj bookmark rename <old> <new>` | Rename a bookmark |
| `jj bookmark delete <name>` | Delete a bookmark |

### Remote Operations

| Command | Description |
|---------|-------------|
| `jj git fetch` | Fetch from remote |
| `jj git fetch origin` | Fetch from origin remote |
| `jj git push` | Push to remote |
| `jj git push --bookmark <name>` | Push specific bookmark |
| `jj git push --all` | Push all tracked bookmarks |
| `jj git clone <url> <directory>` | Clone a Git repo with jj |

### Revision Selection (Revsets)

| Command | Description |
|---------|-------------|
| `jj log -r @` | Current working copy |
| `jj log -r @-` | Parent of working copy |
| `jj log -r main` | Main branch |
| `jj log -r 'main::'` | Descendants of main |
| `jj log -r '::@'` | Ancestors of working copy |
| `jj log -r 'main..@'` | Range from main to @ (inclusive) |
| `jj log -r 'merges()'` | Merge commits |
| `jj log -r 'conflicts()'` | Commits with conflicts |

### Rebasing

| Command | Description |
|---------|-------------|
| `jj rebase -s <source> -d <destination>` | Rebase a commit onto another |
| `jj rebase -s <start> -b <end> -d <destination>` | Rebase multiple commits |
| `jj rebase -r @ -d <destination>` | Rebase working copy |

### Conflicts

| Command | Description |
|---------|-------------|
| `jj resolve --list` | Show conflicts |
| `jj resolve <file>` | Mark conflict as resolved |
| `jj abandon -r <revision>` | Abort merge/rebase |

### Undo and Recovery

| Command | Description |
|---------|-------------|
| `jj undo` | Undo last operation |
| `jj op log` | View operation log |
| `jj op restore <operation-id>` | Restore to previous operation |
| `jj debug watchman-query` | Show reflog-like history |

### Abandoning and Restoring Changes

| Command | Description |
|---------|-------------|
| `jj abandon -r @` | Abandon working copy changes |
| `jj abandon -r <revision>` | Abandon a specific commit |
| `jj restore -r <revision>` | Restore an abandoned commit |

### Common Workflows

| Command | Description |
|---------|-------------|
| `jj bookmark create feature-name -r main` | Create new bookmark from main |
| `jj merge <revision>` | Start a merge |
| `jj describe -r <revision> -m "New message"` | Edit a commit message |
| `jj squash -s <start> -b <end>` | Squash a range of commits |
| `jj rebase -s <commit> -i <insert-after>` | Reorder commits |

### Configuration

| Command | Description |
|---------|-------------|
| `jj help` | General help |
| `jj help <command>` | Command-specific help |
| `jj <command> --help` | Show command help |

### Migration from Git

| Command | Description |
|---------|-------------|
| `jj git init` | Initialize jj in existing Git repo |
| `jj git clone <url> <directory>` | Clone Git repo with jj |
| `jj git push --all` | Push to Git remote |

## Installation

Jujutsu is available via Homebrew or direct download:

```bash
brew install jujutsu
```

Or check installation at: https://github.com/martinvonz/jj

## Core Concepts

### Working Copy vs. Commits

- **Working Copy (`@`)**: Your current working state (like Git's index + working directory)
- **Parent Commit (`@-`)**: The commit your working copy is based on
- **Bookmarks**: Like Git branches, but more flexible

### Key Differences from Git

| Git | Jujutsu |
|-----|---------|
| Index/Staging area | Working copy directly |
| Branches | Bookmarks |
| HEAD | `@` (at-sign) |
| `git add` | Changes automatically tracked |
| `git commit -a` | `jj commit` |

## Basic Commands

### Status and Inspection

```bash
# Show working copy status
jj status
jj st

# View commit history
jj log
jj log -r main..@

# Show changes in working copy
jj diff
jj diff -r @-

# Show changes in a specific commit
jj show <revision>
```

### Making Changes

```bash
# Commit working copy changes (interactive)
jj commit

# Commit with message
jj commit -m "Your commit message"

# Create a new empty commit on top of current
jj new

# Amend working copy changes into parent commit
jj amend

# Squash current commit into parent
jj squash
```

### Bookmarks (Branches)

```bash
# List all bookmarks
jj bookmark list
jj bl

# Create a new bookmark
jj bookmark create <name>

# Track a remote bookmark
jj bookmark track <name> --remote=origin

# Move bookmark to a revision
jj bookmark set <name> -r <revision>

# Rename a bookmark
jj bookmark rename <old> <new>

# Delete a bookmark
jj bookmark delete <name>
```

### Remote Operations

```bash
# Fetch from remote
jj git fetch
jj git fetch origin

# Push to remote
jj git push
jj git push --bookmark <name>

# Push all tracked bookmarks
jj git push --all

# Clone with jj
jj git clone <url> <directory>
```

### Revision Selection

Jujutsu uses powerful revset syntax:

```bash
# Current working copy
jj log -r @

# Parent of working copy
jj log -r @-

# Main branch
jj log -r main

# Descendants of main
jj log -r 'main::'

# Ancestors of working copy
jj log -r '::@'

# Range (main to @, inclusive)
jj log -r 'main..@'

# Merge commits
jj log -r 'merges()'

# Commits with conflicts
jj log -r 'conflicts()'
```

## Advanced Operations

### Rebasing

```bash
# Rebase a commit onto another
jj rebase -s <source> -d <destination>

# Rebase multiple commits
jj rebase -s <start> -b <end> -d <destination>

# Rebase working copy
jj rebase -r @ -d <destination>
```

### Conflicts

```bash
# Show conflicts
jj resolve --list

# Mark conflict as resolved
jj resolve <file>

# Abort merge/rebase
jj abandon -r <revision>
```

### Undo and Recovery

```bash
# Undo last operation
jj undo

# View operation log
jj op log

# Restore to previous operation
jj op restore <operation-id>

# Show reflog-like history
jj debug watchman-query
```

### Abandoning Changes

```bash
# Abandon working copy changes
jj abandon -r @

# Abandon a specific commit
jj abandon -r <revision>

# Restore an abandoned commit
jj restore -r <revision>
```

## Common Workflows

### Starting a New Feature

```bash
# Create new bookmark from main
jj bookmark create feature-name -r main

# Make changes and commit
jj commit -m "First commit"

# Create another commit on top
jj new
jj commit -m "Second commit"
```

### Syncing with Remote

```bash
# Fetch latest changes
jj git fetch

# Rebase your work on top of updated main
jj rebase -r feature -d main@origin

# Push your changes
jj git push --bookmark feature
```

### Cleaning Up History

```bash
# Squash multiple commits
jj squash    # Squash @ into @-
jj squash -s <start> -b <end>  # Squash range

# Reorder commits (using rebase)
jj rebase -s <commit> -i <insert-after>

# Edit a commit message
jj describe -r <revision> -m "New message"
```

### Handling Conflicts

```bash
# Start a merge
jj merge <revision>

# If conflicts occur, resolve them
jj resolve --list          # List conflicted files
# Edit files to resolve conflicts
jj resolve <file>          # Mark as resolved

# Complete the merge
jj commit -m "Merge commit"
```

## Configuration

### User Configuration

Edit `~/.config/jj/config.toml`:

```toml
[user]
name = "Your Name"
email = "your.email@example.com"

[ui]
color = "auto"
pager = "less -FRX"

[git]
push-bookmark-prefix = "push-"
```

### Aliases

Add to `~/.config/jj/config.toml`:

```toml
[aliases]
s = "status"
l = "log"
d = "diff"
c = "commit"
```

## Migration from Git

### Initialize jj in Existing Git Repo

```bash
cd <git-repo>
jj git init
```

### Clone Git Repo with jj

```bash
jj git clone <url> <directory>
```

### Push to Git Remote

```bash
jj git push --all
```

## Tips and Best Practices

1. **Frequent commits**: Jujutsu makes it easy to commit often and reorganize later
2. **Use revsets**: Learn revision selection syntax for powerful operations
3. **Bookmark tracking**: Always track remote bookmarks for easy sync
4. **Operation log**: Use `jj op log` to understand what happened and undo if needed
5. **Conflicts are first-class**: Jujutsu handles conflicts better than Git

## Getting Help

```bash
# General help
jj help

# Command-specific help
jj help <command>
jj <command> --help

# Online documentation
# https://github.com/martinvonz/jj/blob/main/docs/
```

## Common Issues

### "Working copy is stale"

Run `jj status` to refresh. If needed, rebase onto latest main:

```bash
jj git fetch
jj rebase -r @ -d main@origin
```

### "Bookmark is already tracked"

The bookmark is already tracking a remote. Use `jj bookmark list` to see details.

### "Conflict in working copy"

Resolve conflicts with `jj resolve`, then commit:

```bash
jj resolve --list
# Edit files
jj resolve <file>
jj commit -m "Resolved conflicts"
```

## Resources

- **Official Documentation**: https://github.com/martinvonz/jj/tree/main/docs
- **GitHub Repository**: https://github.com/martinvonz/jj
- **Discord Community**: https://discord.gg/6dVYDakDqm
- **Tutorial**: https://github.com/martinvonz/jj/blob/main/docs/tutorial.md
