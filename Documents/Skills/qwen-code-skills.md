# Qwen Code Skills Guide

## Overview

Skills extend Qwen Code's capabilities with specialized knowledge, workflows, and tool integrations. They are stored in a hierarchical structure with global (shared) and project-specific skills.

## Where Skills Are Stored

### Central Repository (Source of Truth)

**Path:** `~/Workspace/.agents/skills/`

This is a **Git repository** that contains all global skills. Each skill is a directory with:
- `SKILL.md` - Required skill definition file with metadata and instructions
- Supporting files (scripts, templates, examples)
- Optional `README.md` for complex skills

### Global Skills

**Path:** `~/.qwen/skills/`

Global skills are **symlinks** pointing to the central repository:

```
~/.qwen/skills/
├── pdf -> /Users/cding/Workspace/.agents/skills/pdf
├── docx -> /Users/cding/Workspace/.agents/skills/docx
├── xlsx -> /Users/cding/Workspace/.agents/skills/xlsx
├── frontend-design -> /Users/cding/Workspace/.agents/skills/frontend-design
└── ... (other skills)
```

**Scope:** Available to all projects in Qwen Code.

### Project-Specific Skills

**Path:** `<project>/.qwen/skills/`

Example: `~/Workspace/tax/.qwen/skills/`

Project-specific skills are stored directly in the project directory and are only available when working on that specific project.

**Scope:** Only available to the specific project.

## How Skills Work (Under the Hood)

### What Happens When You Invoke a Skill

```
user: skill: "content-pipeline"
         ↓
Qwen Code reads SKILL.md file content
         ↓
Content added to conversation context
         ↓
LLM processes the text using its training
         ↓
LLM understands patterns, formats, workflows
         ↓
LLM applies knowledge to user requests
```

### What Files Are Loaded

| File | Loaded Automatically? | When Is It Read? |
|------|----------------------|------------------|
| `SKILL.md` | ✅ **Yes, entire file** | On skill invocation |
| `local/.env` | ❌ No | Only if Qwen Code explicitly reads it |
| `local/SKILL.local.md` | ❌ No | Only if Qwen Code explicitly reads it |
| `references/*.md` | ❌ No | Only if SKILL.md says "read X" and Qwen Code follows through |
| `scripts/*.py` | ❌ No | Only if Qwen Code explicitly reads them |

**Key Point:** Only `SKILL.md` is automatically loaded. Other files must be explicitly referenced AND Qwen Code must decide to read them.

### What Drives "Learning"?

**Nothing explicit drives it.** The LLM:

1. **Reads text** (the `SKILL.md` content as plain text)
2. **Recognizes patterns** (from its training on similar documents)
3. **Applies understanding** (generates responses following the patterns)

**Example:**

When `SKILL.md` contains:
```markdown
### current.json 格式
```json
{
  "topic": "主题（可选）",
  "materials": [
    {
      "time": "2026-01-30 14:30",
      "content": "素材内容",
      "type": "搞笑时刻",
      "auto": true
    }
  ]
}
```
```

The LLM:
- Sees the JSON structure → understands the format
- Reads field descriptions → knows what each field means
- Infers usage → can create/modify `current.json` files

**No code executes this** - it's pure LLM pattern matching from training + context.

### Why This Works

LLMs are trained on:
- Technical documentation → understands format specifications
- JSON schemas → recognizes structure patterns
- Workflow descriptions → follows step-by-step instructions
- Classification tables → applies categorization rules

When `SKILL.md` contains these patterns, the LLM **automatically applies them**.

## Skill Structure

### Required Files

Every skill must have a `SKILL.md` file with YAML frontmatter:

```markdown
---
name: skill-name
description: Clear description of when and why to use this skill
license: Optional license information
---

# Skill Documentation

Detailed instructions, examples, and guidance for using this skill.
```

**Key Fields:**

| Field | Purpose | Example |
|-------|---------|---------|
| `name` | **Unique identifier** - used to invoke the skill | `content-pipeline` |
| `description` | **Auto-trigger hint** - helps Qwen Code know when to suggest the skill | "内容生产和分发统一管线..." |

**The `name` field is the authoritative source** - even if the directory is named differently, Qwen Code uses the `name:` value to identify the skill.

### Typical Structure

```
skill-name/
├── SKILL.md           # Required: Skill definition and main guide
├── README.md          # Optional: Additional documentation
├── scripts/           # Optional: Helper scripts
├── templates/         # Optional: Code/document templates
└── examples/          # Optional: Usage examples
```

## How to Add Skills

### Adding a Global Skill

**Step 1:** Create the skill in the central repository:

```bash
cd ~/Workspace/.agents/skills/
mkdir -p new-skill
# Add SKILL.md and skill files
```

**Step 2:** Create symlink in global location:

```bash
ln -s ~/Workspace/.agents/skills/new-skill ~/.qwen/skills/new-skill
```

**Step 3:** Commit to the central repository:

```bash
cd ~/Workspace/.agents/skills/
git add new-skill
git commit -m "Add new-skill"
```

### Adding a Project-Specific Skill

**Step 1:** Create the skill directly in the project:

```bash
mkdir -p ~/Workspace/tax/.qwen/skills/tax-specific-skill
# Add SKILL.md and skill files
```

**Step 2:** The skill is immediately available when working on that project.

**Step 3:** Commit to the project's repository:

```bash
cd ~/Workspace/tax/
git add .qwen/skills/tax-specific-skill
git commit -m "Add tax-specific skill"
```

### Adding External Skills (Third-Party)

For skills from external sources (like the content-pipeline example):

**Option A: Symlink to external directory**

```bash
ln -s /path/to/external/skill ~/.qwen/skills/skill-name
```

**Option B: Copy to central repository**

```bash
cp -r /path/to/external/skill ~/Workspace/.agents/skills/skill-name
ln -s ~/Workspace/.agents/skills/skill-name ~/.qwen/skills/skill-name
```

## How to Remove Skills

### Removing a Global Skill

```bash
# Remove symlink
rm ~/.qwen/skills/skill-name

# Optionally remove from central repository
cd ~/Workspace/.agents/skills/
git rm -r skill-name
git commit -m "Remove skill-name"
```

### Removing a Project-Specific Skill

```bash
rm -rf ~/Workspace/project/.qwen/skills/skill-name
```

## How to Use Skills

### Skill Types and Invocation Patterns

Different skill types have different invocation requirements:

| Skill Type | Examples | Invocation Pattern |
|------------|----------|-------------------|
| **File-type skills** | `pdf`, `xlsx`, `docx`, `pptx` | Auto-triggered by file extension |
| **Workflow skills** | `content-pipeline`, `doc-coauthoring` | Explicit load + trigger words |
| **Creative skills** | `frontend-design`, `canvas-design`, `algorithmic-art` | Explicit or context-based |
| **Knowledge skills** | `brand-guidelines`, `internal-comms` | Context-based or explicit |

### Invoking a Skill

**Explicit invocation (always works for all skill types):**

```
skill: "pdf"
skill: "docx"
skill: "frontend-design"
skill: "content-pipeline"
```

### When Skills Are Auto-Triggered

Qwen Code may automatically consider skills when:

| Trigger Type | Example | Reliability |
|--------------|---------|-------------|
| **File type** | Attach `report.pdf` → PDF skill | High |
| **Task matches description** | "Create a spreadsheet" → xlsx skill | Medium |
| **Skill loaded in session** | After `skill: "content-pipeline"`, triggers work | High |

### Trigger Words (Workflow Skills)

Some skills define **trigger words** in their `SKILL.md`:

```markdown
## 触发词
| 触发词 | 说明 |
|--------|------|
| "记一笔：xxx" | 手动添加素材 |
| "转小红书" + 链接 | 微信文章转小红书 |
```

**For workflow skills with triggers:**

1. **First use in session:** Explicitly load the skill
   ```
   skill: "content-pipeline"
   ```

2. **Then use triggers directly:**
   ```
   记一笔：今天学习了 Qwen Code 技能
   转小红书：https://mp.weixin.qq.com/...
   ```

3. **Why?** Trigger words are documentation for you and the LLM, but explicit invocation ensures the skill's full context is loaded.

### File-Type Skills (No Explicit Invocation Needed)

For skills like `pdf`, `xlsx`, `docx`, `pptx`:
- Just mention or attach the file
- Qwen Code auto-loads based on file extension
- No need for `skill: "pdf"` first

```
# These work without explicit skill invocation:
"Extract text from report.pdf"
"Create a spreadsheet from this data"
"Merge these PDF files"
```

### Skill Capabilities

Skills can provide:
- **Specialized knowledge** - Domain-specific expertise
- **Workflow guidance** - Step-by-step processes
- **Tool integrations** - Access to external services
- **Code generation** - Templates and patterns
- **Best practices** - Established conventions

## Available Global Skills

| Skill | Description |
|-------|-------------|
| `algorithmic-art` | Create generative art using p5.js with seeded randomness |
| `allium` | LLM-native language for intent alongside implementation |
| `brand-guidelines` | Apply Anthropic brand colors, typography, and style |
| `canvas-design` | Create visual art, posters, and designs as .png/.pdf |
| `doc-coauthoring` | Structured workflow for co-authoring docs and specs |
| `docx` | Create, read, edit Word documents |
| `frontend-design` | Create distinctive, production-grade frontend interfaces |
| `intent-layer` | Set up hierarchical AGENTS.md files for codebases |
| `internal-comms` | Write internal communications: status reports, newsletters |
| `mcp-builder` | Guide for creating MCP servers |
| `pdf` | Read, merge, split, create, OCR PDF files |
| `pptx` | Create, read, edit PowerPoint presentations |
| `skill-creator` | Guide for creating and updating Claude skills |
| `slack-gif-creator` | Create animated GIFs optimized for Slack |
| `superpowers` | Suite of 14 coding workflow skills (TDD, git worktrees, code review) |
| `theme-factory` | Apply professional font/color themes to artifacts |
| `ui-ux-pro-max` | UI/UX design intelligence (styles, palettes, stacks) |
| `web-artifacts-builder` | Build multi-component React/Tailwind/shadcn artifacts |
| `webapp-testing` | Test local web apps using Playwright |
| `xlsx` | Create, read, edit spreadsheets |

## Managing Skills with mise

Some skills include `mise.toml` for setup and management:

```bash
# Check skill status
cd ~/Workspace/Skills/skill-name
mise run status

# Install/setup skill
mise run install

# Get help
mise run help
```

## Troubleshooting

### Skill Not Found

1. Verify symlink exists: `ls -la ~/.qwen/skills/`
2. Check symlink target is valid: `readlink ~/.qwen/skills/skill-name`
3. Ensure `SKILL.md` exists in the skill directory

### Skill Not Loading

1. Check Qwen Code is opened in the project directory
2. Verify skill directory permissions
3. Restart Qwen Code to reload skills

### Conflicting Skills

If global and project skills have the same name:
- Project-specific skills take precedence
- Rename one to avoid conflicts

## Best Practices

### For Skill Creators

1. **Clear naming** - Use descriptive, unique skill names
2. **Comprehensive SKILL.md** - Include when-to-use guidance
3. **Test thoroughly** - Verify skill works in real scenarios
4. **Document dependencies** - List required tools/libraries
5. **Version control** - Commit skills to appropriate repositories

### For Skill Users

1. **Review skill docs** - Understand capabilities and limitations
2. **Configure properly** - Set up API keys and environment variables
3. **Provide context** - Give skills enough information to work effectively
4. **Verify outputs** - Always review skill-generated content

## Example: Adding the content-pipeline Skill

```bash
# The skill is at: ~/Workspace/Skills/content-pipeline/content-pipeline/

# Create symlink to global skills
ln -s /Users/cding/Workspace/Skills/content-pipeline/content-pipeline \
      ~/.qwen/skills/content-pipeline

# Verify
ls -la ~/.qwen/skills/content-pipeline

# Set up the skill (if it has mise.toml)
cd ~/Workspace/Skills/content-pipeline
mise run install

# Use in Qwen Code
skill: "content-pipeline"
```

## Related Files

- `~/Workspace/.agents/skills/` - Central skill repository
- `~/.qwen/skills/` - Global skills (symlinks)
- `~/Workspace/tax/.qwen/skills/` - Tax project skills
- `~/Workspace/ChenWeb/.qwen/skills/` - ChenWeb project skills
- `~/Workspace/.qwen/QWEN.md` - Workspace documentation

## FAQ

### Q: Do I need to type `skill: "content-pipeline"` before using trigger words like "记一笔"?

**A:** For workflow skills, **yes** - explicitly load first for reliable triggering:

```
skill: "content-pipeline"   # Load once at session start
记一笔：今天学习了 Qwen Code 技能  # Then triggers work
```

Without explicit loading, trigger words may or may not auto-discover the skill.

### Q: Do I need to type `skill: "pdf"` before working with PDFs?

**A:** **No** - file-type skills auto-trigger:

```
"Extract text from report.pdf"  # Works without explicit skill invocation
```

### Q: How is a skill identified?

**A:** By the `name:` field in `SKILL.md` YAML frontmatter:

```yaml
---
name: content-pipeline    # ← This is the skill identifier
description: ...
---
```

The directory name should match, but `name:` is authoritative.

### Q: Can I use a skill without installing it globally?

**A:** Yes - project-specific skills in `<project>/.qwen/skills/` work without global installation.

### Q: How do I know if a skill has trigger words?

**A:** Check the skill's `SKILL.md` for a "Trigger Words" or "触发词" section.
