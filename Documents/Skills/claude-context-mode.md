# claude-context-mode

**Repository:** https://github.com/mksglu/claude-context-mode
**Version:** 0.8.1
**Author:** Mert Koseoğlu

## What It Does

claude-context-mode is a Claude Code plugin that keeps large command outputs out of the context window by routing them through an in-process sandbox. Instead of flooding the context with raw log files, API responses, or test output, data is processed in an isolated subprocess and only the summary enters context.

Claimed savings: up to 98% context reduction on data-heavy sessions.

## How It Works

The plugin has two layers:

1. **PreToolUse hook** — intercepts `Bash`, `WebFetch`, `Read`, `Grep`, and `Task` tool calls and nudges toward sandbox execution instead.
2. **MCP server** (`mcp__plugin_context-mode_context-mode__*`) — provides sandbox execution tools that run code in-process without returning raw output to context.

## Installation

The plugin is installed via the Claude Code plugin marketplace:

```bash
# Add the marketplace (one-time)
claude /plugin marketplace add mksglu/claude-context-mode

# Install the plugin (opens interactive TUI — arrow keys + Space to select)
claude /plugin install context-mode@claude-context-mode
```

After installation, **restart VS Code** (or run `Developer: Restart Extension Host`) for the plugin to be picked up by the VS Code extension.

### Scope

The plugin was installed at **project scope** for `~/Workspace`. This means it is active when Claude Code is opened in that workspace. The `enabledPlugins` entry lives in `~/Workspace/.claude/settings.json`.

## Slash Commands

| Command | Description |
|---|---|
| `/context-mode:stats` | Show token savings for the current session |
| `/context-mode:doctor` | Run diagnostics (runtimes, hooks, FTS5, versions) |
| `/context-mode:upgrade` | Pull latest from GitHub and reinstall |
| `/context-mode:context-mode` | Load the routing skill (instructs Claude when to use sandbox tools) |

## MCP Tools

| Tool | Description |
|---|---|
| `execute` | Run code in a sandboxed subprocess (JS, TS, Python, Shell, Ruby, Go, Rust, Perl, Elixir) |
| `execute_file` | Process a file in the sandbox — file loaded as `FILE_CONTENT`, never enters context |
| `index` | Index markdown/text into an FTS5 BM25 knowledge base for searchable retrieval |
| `search` | Query the knowledge base with ranked results |
| `fetch_and_index` | Fetch a URL, convert to markdown, index it, return a ~3KB preview |
| `batch_execute` | Run multiple commands in one call, auto-index all output, search results |
| `stats` | Return session token savings metrics |

## When to Use Sandbox Tools vs Bash

**Use `execute` / `execute_file` for:**
- Any command that reads, queries, fetches, lists, logs, or tests
- CLI tools: `gh`, `aws`, `kubectl`, `docker`, `npm test`, `go test`, `git log`, `git diff`
- API calls, log analysis, data file parsing, build output

**Use Bash directly for:**
- File mutations: `mkdir`, `mv`, `cp`, `rm`, `touch`, `chmod`
- Git writes: `git add`, `git commit`, `git push`, `git checkout`
- Navigation: `cd`, `pwd`, `which`
- Process control: `kill`, `pkill`
- Package installs: `npm install`, `pip install`
- Simple output: `echo`, `printf`

## Playwright Integration

Playwright snapshots can be 10K–135K tokens. Always use the `filename` parameter:

```
# Wrong — dumps 135K tokens into context
browser_snapshot()

# Correct — saves to file, then process in sandbox
browser_snapshot(filename: "/tmp/snap.md")
→ index(path: "/tmp/snap.md") → search(...)
→ OR execute_file(path: "/tmp/snap.md", ...)
```

## Doctor Output (as of 2026-03-01)

```
Runtimes: 9/11 — JS, TS, Python, Shell, Ruby, Go, Rust, Perl, Elixir (Bun 1.3.9 ⚡)
Server test: PASS
Hooks: PASS (PreToolUse configured)
FTS5 / better-sqlite3: PASS
npm (MCP): v0.8.1
Marketplace: v0.8.1
WARN: "Plugin enabled" check fails — expected, plugin is project-scoped not user-scoped
```

## Troubleshooting

**"Unknown skill: context-mode:stats" in terminal / nothing happens in VS Code**

The plugin was installed after VS Code was already open. Restart the extension host:
- `Cmd+Shift+P` → `Developer: Restart Extension Host`

**Doctor warns "context-mode not in enabledPlugins"**

This is expected when installed at project scope. The doctor only checks the global `~/.claude/settings.json`, but the plugin entry is in `~/Workspace/.claude/settings.json`.

**MCP server not connecting**

The MCP server is launched automatically by the plugin system via `start.sh`. It requires Node.js. Run `/context-mode:doctor` to verify the server test passes.
