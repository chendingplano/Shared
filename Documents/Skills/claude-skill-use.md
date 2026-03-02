# Claude Code Skill Use

> A practical reference for how Claude Code skills work, based on hands-on experience with the `content-pipeline` skill.

---

## What Is a Skill?

A **skill** is a directory containing a `SKILL.md` file that encodes a specialized workflow, prompt instructions, tool usage patterns, and trigger conditions. Claude Code loads skill metadata into the system prompt at the start of every conversation, making skills discoverable.

**Skill directory locations:**

```
~/.claude/skills/<skill-name>/SKILL.md          # global, user-level
<project>/.claude/skills/<skill-name>/SKILL.md  # project-level
```

In this workspace, skills are centrally managed and symlinked:

```
~/.claude/skills/<skill>  →  ~/Workspace/.agents/skills/<skill>
~/Workspace/.claude/skills/<skill>  →  ../../.agents/skills/<skill>
```

---

## How Skills Are Loaded

All installed skills have their **names and trigger descriptions** injected into the system prompt at the start of every conversation — regardless of whether they are used. This means:

- Every skill costs tokens in every conversation
- Keep trigger descriptions in `SKILL.md` concise to minimize overhead
- There is no lazy-loading mechanism for skills (unlike deferred MCP tools, which use `ToolSearch`)

---

## Invoking a Skill

### Explicit Invocation (Recommended)

```
/skill-name
```

For example: `/content-pipeline`

This expands the full `SKILL.md` into the conversation context. From that point, the skill's complete workflow — file paths, schemas, trigger words, reference files — is available to Claude for the rest of the session.

### Implicit Invocation (Unreliable for Complex Skills)

You can say a trigger word (e.g., `记一笔`) without invoking the skill first. Claude may recognize the intent from the system prompt summary, but:

- Only the trigger summary is available, not the full `SKILL.md`
- Detailed conventions (file paths, JSON schemas, output directories) will be missing
- Results may look correct but not follow the skill's actual spec

**Rule of thumb:**
- Simple skills (e.g., generating a commit message) → implicit may work
- Complex skills with file I/O, multi-step flows, specific schemas → always invoke explicitly first

---

## Skill Lifecycle Within a Session

| Event | Behavior |
|-------|----------|
| `/skill-name` typed | Full `SKILL.md` loaded into context |
| Trigger words used | Skill workflows execute correctly |
| Session ends | Skill context is gone |
| New session starts | Must invoke `/skill-name` again |
| "Deselect" a skill | No mechanism — tell Claude explicitly to ignore it |

Skills do **not** persist across sessions. Each conversation starts fresh.

---

## Example: `content-pipeline` Session Walk-Through

This session demonstrated the full `content-pipeline` skill lifecycle.

### 1. Invoke the Skill

```
/content-pipeline
```

Claude loaded the full `SKILL.md` from `~/.claude/skills/content-pipeline/`. From this point, trigger words like `记一笔`, `出稿`, `清空素材` were all active.

### 2. Collect Materials (`记一笔`)

Materials were added to `/tmp/drafts/current.json` — the skill's persistent scratchpad. Each entry records:

```json
{
  "time": "2026-03-06 00:00",
  "content": "Pi: Your Own Coding Assistant: https://mp.weixin.qq.com/s/...",
  "type": "意外发现",
  "context": "公众号文章链接，介绍 Pi 个人编程助手工具",
  "auto": false
}
```

Materials added in this session:
- WeChat article: *Pi: Your Own Coding Assistant*
- Toutiao video: *Article from Claude Code Author*
- Personal diary notes: `diary-20260304.typ § 1 Pi.dev` — individual analysis with original observations on MCP design philosophy, tool discovery, and LLM-initiated tool requests

### 3. Add Reference from Local File

```
Add a reference material: Section 1. Pi.dev in local file: /Users/cding/OneDrive/Diary/diary-20260304.typ
```

Claude read the `.typ` file, extracted the relevant section, and summarized it (including personal commentary) as a material entry in `current.json`. This pattern works for any local file — `.md`, `.typ`, `.txt`, etc.

### 4. Draft the Article (`出稿`)

```
出稿
```

Claude:
1. Fetched the WeChat article via `scripts/fetch_wechat_article.py`
2. Determined content type → **tutorial** (六段式框架, 2000–4000 words)
3. Wrote the article in Markdown, integrating personal diary observations as distinct opinion blocks
4. Generated styled HTML preview (01fish theme)
5. Generated cover image HTML (900×383 downloadable PNG)
6. Generated manifest.json for `/distribute`
7. Produced WeChat Moments copy

### 5. Outputs

| File | Purpose |
|------|---------|
| `/tmp/Pi-属于你自己的编程助手.md` | Article Markdown |
| `/tmp/Pi-属于你自己的编程助手_preview.html` | Styled HTML preview (01fish theme) |
| `/tmp/Pi-属于你自己的编程助手_cover.html` | Cover image HTML (download PNG in browser) |
| `/tmp/manifest.json` | Distribution manifest for `/distribute` |

---

## Key Concepts

### Skills vs. Deferred Tools

| | Skills | Deferred MCP Tools |
|--|--------|-------------------|
| Loaded | Always, every session | On demand via `ToolSearch` |
| Token cost | All skills, every conversation | Only loaded tools |
| Invocation | `/skill-name` | `ToolSearch` → direct call |
| Persists across sessions | No | No |

### Skill Count and Token Overhead

There is no hard limit on the number of skills, but each skill's trigger description adds to the system prompt. Practical limits:

- Dozens of skills: fine
- Hundreds: degraded performance and higher cost
- Mitigation: keep `SKILL.md` trigger descriptions short; put detailed instructions in `references/` files that are loaded on demand

### Skill vs. AGENTS.md / CLAUDE.md

| | Skills | AGENTS.md / CLAUDE.md |
|--|--------|----------------------|
| Purpose | Specialized workflows | General project context |
| Loaded | On invocation | Always, automatically |
| Scope | Single workflow domain | Entire codebase / project |

---

## Adding a New Skill

```bash
# 1. Create the skill directory with a SKILL.md
mkdir -p ~/Workspace/.agents/skills/<skill-name>
# write SKILL.md

# 2. Create symlinks
ln -s ~/Workspace/.agents/skills/<skill-name> ~/.claude/skills/<skill-name>
ln -s ../../.agents/skills/<skill-name> ~/Workspace/.claude/skills/<skill-name>

# 3. Commit to the .agents/skills repo
cd ~/Workspace/.agents/skills
git add <skill-name>
git commit -m "add <skill-name> skill"
```

See `CLAUDE.md` for the full skill management reference.

---

## Tips

- **Always invoke complex skills explicitly** before using their trigger words.
- **Output directory**: set a default in `local/SKILL.local.md` so you don't have to specify it every time.
- **Reference files load on demand**: `content-pipeline` only reads `references/writing-style.md`, `references/cover-template.md`, etc. when needed — this is the right pattern for keeping skills lean.
- **Personal overrides**: `local/SKILL.local.md` is gitignored and is where you put your WeChat ID, author name, brand color paths, and other personal settings.
- **Re-invocation**: if a session runs long and context is compressed, re-invoke `/skill-name` to ensure the full workflow is still in context.
