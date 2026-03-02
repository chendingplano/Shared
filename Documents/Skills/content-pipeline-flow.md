# Content Pipeline Skill Flow

## Example: Using "记一笔" to Add Reference Material to current.json

This document shows the complete flow from skill invocation to adding material to `current.json`.

---

## Step-by-Step Flow

### Step 1: User Invokes the Skill

```
user: skill: "content-pipeline"
```

**What Qwen Code does:**

1. **Reads `SKILL.md` file content** (entire file, as plain text)
2. **Adds content to conversation context** (becomes part of the prompt)
3. **LLM processes the text** using its training on:
   - Technical documentation (understands format specs)
   - JSON schemas (recognizes structure patterns)
   - Workflow descriptions (follows step-by-step instructions)
   - Classification tables (applies categorization rules)

**What files are NOT automatically loaded:**

| File | Loaded? |
|------|---------|
| `local/.env` | ❌ No |
| `local/SKILL.local.md` | ❌ No |
| `references/*.md` | ❌ No (unless SKILL.md explicitly says "read X") |
| `scripts/*.py` | ❌ No |

**Key Point:** Only `SKILL.md` is automatically loaded. The LLM understands patterns from the text using its training - no explicit "learning code" runs.

---

### Step 2: User Triggers "记一笔"

```
user: 记一笔：今天发现 Qwen Code 的技能系统很好用，可以通过触发词快速调用内容管线
```

**What Qwen Code does:**

1. **Recognizes trigger** - "记一笔" matches the trigger word from `SKILL.md`
2. **Understands intent** - User wants to add material to the content pipeline
3. **Extracts content** - "今天发现 Qwen Code 的技能系统很好用..."

---

### Step 3: Qwen Code Processes the Material

Based on the skill's `SKILL.md`, Qwen Code:

1. **Identifies material type** by analyzing content:
   - Content: "今天发现 Qwen Code 的技能系统很好用..."
   - Type: "意外发现" (unexpected discovery) or "方法沉淀" (method insight)

2. **Formats the material** according to `current.json` schema:

```json
{
  "time": "2026-03-06 15:30",
  "content": "今天发现 Qwen Code 的技能系统很好用，可以通过触发词快速调用内容管线",
  "type": "意外发现",
  "context": "在使用 content-pipeline skill 时的心得",
  "auto": false
}
```

---

### Step 4: Qwen Code Updates current.json

**Determine output directory:**

1. Check if user specified a directory
2. Check `local/SKILL.local.md` for configured output directory
3. Default to `/tmp/` if neither exists

**File path:** `[output-dir]/drafts/current.json`

**If file doesn't exist**, create it:

```json
{
  "topic": "",
  "materials": [],
  "created": "2026-03-06"
}
```

**If file exists**, read and append:

```json
{
  "topic": "",
  "materials": [
    {
      "time": "2026-03-06 14:20",
      "content": "之前遇到的那个问题原来是配置问题",
      "type": "踩坑翻车",
      "auto": true
    },
    {
      "time": "2026-03-06 15:30",
      "content": "今天发现 Qwen Code 的技能系统很好用，可以通过触发词快速调用内容管线",
      "type": "意外发现",
      "context": "在使用 content-pipeline skill 时的心得",
      "auto": false
    }
  ],
  "created": "2026-03-06"
}
```

---

### Step 5: Qwen Code Confirms

```
assistant: ✓ 素材已记录

**类型**: 意外发现
**内容**: 今天发现 Qwen Code 的技能系统很好用，可以通过触发词快速调用内容管线

当前素材数：2 条
说"出稿"可以生成文章，或继续"记一笔"添加更多素材
```

---

## Complete Session Example

```
user: skill: "content-pipeline"

[Skill loaded. Qwen Code now knows:]
- Trigger words: 记一笔，转小红书，出稿，etc.
- Output directory: /tmp/ (default) or from SKILL.local.md
- File format: drafts/current.json

---

user: 记一笔：尝试了三种不同的方案来解决 API 认证问题，最后发现是 token 过期了

assistant: ✓ 素材已记录（✓ 素材 +1）

**类型**: 踩坑翻车
**内容**: 尝试了三种不同的方案来解决 API 认证问题，最后发现是 token 过期了

当前素材数：1 条

---

user: 记一笔：原来技能系统会自动读取 SKILL.md 来理解触发词，这个设计很巧妙

assistant: ✓ 素材已记录（✓ 素材 +1）

**类型**: 意外发现
**内容**: 原来技能系统会自动读取 SKILL.md 来理解触发词，这个设计很巧妙

当前素材数：2 条

---

user: 出稿

assistant: [Reads drafts/current.json]
[Analyzes 2 materials to extract theme]
[Selects writing framework based on content type]
[Generates article with proper structure]
[Creates formatted HTML preview]
[Generates cover image HTML]
[Creates social media copy]
[Generates manifest.json]

文章已完成！

📄 文章：[output-dir]/Qwen-Code-技能系统解析.md
🎨 排版预览：[output-dir]/Qwen-Code-技能系统解析_preview.html
🖼️  封面图：[output-dir]/Qwen-Code-技能系统解析_cover.html
📱 朋友圈文案：[output-dir]/moments-copy.txt
📋 Manifest: [output-dir]/manifest.json

要清空素材吗？
```

---

## Key Files and Paths

### Skill Files

```
~/.qwen/skills/content-pipeline/
├── SKILL.md                    # Main skill definition (triggers, workflows, formats)
├── README.md                   # Additional documentation
├── local/
│   ├── .env                    # API keys and tool paths
│   └── SKILL.local.md          # User overrides (output dir, personal settings)
├── scripts/
│   └── fetch_wechat_article.py # WeChat article fetcher
└── references/
    ├── tutorial-framework.md   # Tutorial writing template
    ├── writing-style.md        # Long-form writing template
    ├── xiaohongshu-format.md   # Xiaohongshu card format
    └── platform-copy.md        # Social media copy templates
```

### Output Files (Generated)

```
[output-dir]/
├── drafts/
│   └── current.json            # Current materials (updated by 记一笔)
├── [article].md                # Generated article
├── [article]_preview.html      # Formatted HTML preview
├── [article]_cover.html        # Cover image HTML
├── moments-copy.txt            # WeChat moments copy
└── manifest.json               # Distribution manifest
```

---

## Material Type Classification

Qwen Code classifies materials based on content signals from `SKILL.md`:

| Type | Signals | Example |
|------|---------|---------|
| **踩坑翻车** | 预期≠结果、报错、折腾半天 | "试了三种方案都不行" |
| **意外发现** | "没想到"、"原来可以"、意外有效 | "居然这样就解决了" |
| **迭代打磨** | 改了多版、从复杂到简洁 | "200 行改成 20 行还能跑" |
| **搞笑时刻** | 对话金句、AI 抽风、神奇 bug | "它认真地给我写了一堆错的" |
| **突破时刻** | 卡了很久终于通 | "困扰一周的 bug 终于找到了" |
| **方法沉淀** | 可复用的技巧、心得 | "以后遇到这种情况就这么办" |

---

## current.json Schema

```json
{
  "topic": "string (optional, auto-extracted when generating article)",
  "materials": [
    {
      "time": "ISO 8601 datetime",
      "content": "string (the material content)",
      "type": "one of the 6 material types",
      "context": "string (optional context notes)",
      "auto": "boolean (true if auto-recorded, false if manual 记一笔)"
    }
  ],
  "created": "YYYY-MM-DD"
}
```

---

## How Qwen Code Uses SKILL.md

### During Skill Invocation

1. **Read entire SKILL.md as text** - File content added to conversation context

2. **LLM parses YAML frontmatter:**
   ```yaml
   name: content-pipeline
   description: 内容生产和分发统一管线...
   ```
   - `name` → Used for skill identification
   - `description` → Helps LLM understand when to suggest skill

3. **LLM recognizes trigger words** from the "触发词" tables:
   - "记一笔" → Manual material recording
   - "出稿" → Generate article from materials
   - "转小红书" → Convert WeChat article to Xiaohongshu format

4. **LLM understands data formats from examples:**
   ```markdown
   ### current.json 格式
   ```json
   {
     "topic": "...",
     "materials": [...]
   }
   ```
   ```
   - LLM's training on JSON schemas → understands structure
   - No explicit parser needed

5. **LLM learns workflows from descriptions:**
   - Path A: materials → article (steps described in text)
   - Path B: WeChat link → multi-platform (steps described in text)

### During Trigger Execution

1. **Match trigger** → LLM recognizes pattern from SKILL.md
2. **Recall relevant section** → LLM retrieves from context
3. **Follow prescribed workflow** → LLM generates steps from description
4. **Use specified formats** → LLM applies patterns from examples

**Nothing is "executed"** - the LLM simply:
- Reads text
- Recognizes patterns (from training)
- Generates responses following those patterns

---

## Automatic vs Manual Recording

### Automatic Recording (cc collaboration)

When collaborating with cc (AI assistant), materials are auto-recorded:

```
user: 这个报错信息好奇怪，试了 Google 前三页都没找到答案
cc: [Analyzes error, identifies it's a version compatibility issue]
cc: [Provides solution]
[Auto-recorded: ✓ 素材 +1]
```

**Signals detected:**
- "好奇怪" → surprise
- "试了...都没找到" → struggle
- Type: "踩坑翻车"

### Manual Recording (记一笔)

User explicitly triggers recording:

```
user: 记一笔：分享一个调试技巧 - 先用最小复现代定位问题，再逐步添加代码
```

**Benefits:**
- User controls what's recorded
- Can add context and insights
- Marks important moments intentionally

---

## Best Practices

### For Users

1. **Start session with skill invocation:**
   ```
   skill: "content-pipeline"
   ```

2. **Use triggers naturally:**
   ```
   记一笔：xxx
   出稿
   转小红书：链接
   ```

3. **Review materials before generating article:**
   ```
   看看素材
   ```

4. **Configure output directory** in `local/SKILL.local.md` for consistent paths

### For Skill Creators

1. **Define clear trigger words** in `SKILL.md`
2. **Document data formats** (JSON schemas)
3. **Provide file path conventions**
4. **Include examples** of expected inputs/outputs
5. **Specify classification rules** for auto-detection
