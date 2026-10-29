## Context

`shared/` already has an icon manager: table `icons` (id, name, category, file_name, file_path, mime_type, tags JSONB, description, …), files stored on disk under `DATA_HOME_DIR`, handlers at `/shared_api/v1/icons`, Svelte `IconPicker`/`IconGrid`/`IconManager`. It is upload-oriented, searches with `ILIKE` on name and tags, and nothing in ChenWeb uses it. BM25 search (ParadeDB) lives in ChenWeb's `kbsearch`, not in shared; the embedding client is in `shared/go/api/llm`.

## Goals / Non-Goals

**Goals:**
- A catalogue any project can query by keywords or a short description (English or Chinese).
- Extend the existing table and API rather than build a parallel one.
- Vector (SVG) only, scalable, themeable via `currentColor`.

**Non-Goals:**
- Replacing the 114 static `@lucide/svelte` imports in ChenWeb.
- Bitmap icons in the library.
- Moving ChenWeb's `kbsearch` into shared.

## Decisions

1. **New table `shared.icon_library`, not an extension of the legacy `icons` table.** (Revised during implementation: the legacy table is named via config as `resources`, stores uploaded files on disk, allows bitmaps, has NOT NULL file columns and a MIME check, and does not exist in the dev database; nothing uses it. Extending it would have meant relaxing most of its constraints. The legacy upload manager is left untouched.) Columns: Add columns `svg_body TEXT`, `source_set TEXT` (`lucide|tabler|custom`), `tags_zh JSONB`, `category_zh`, `description_en`, `description_zh`, `version INT`, `embedding vector(1024)` (matches the bge-m3 switch). Tags are stored as space-separated TEXT so the BM25 index can tokenise them. Migration: `ChenWeb/shared_migrations/20261103000001_create_shared_icon_library.sql` (shared migrations live in ChenWeb). Pgvector types are schema-qualified (`public.vector`) because the migration connection's search_path excludes `public`.
2. **SVG in the DB, not on disk.** Icons are ~0.5 KB; the DB makes seeding, versioning and serving trivial. The renderer inlines the SVG.
3. **Search: ParadeDB BM25 (`pg_search`) plus pgvector, fused by reciprocal rank.** ChenWeb already runs ParadeDB with the `jieba` tokenizer, so Chinese is tokenised properly at both index and query time. BM25 gives IDF and length normalisation, which `ts_rank` lacks. A `bm25` index covers `name`, `tags_en`, `tags_zh`, `category_en`, `category_zh`, `description_en`, `description_zh` (jieba on the zh fields, default on the English ones), with `key_field = 'id'`. Query embedding comes from the `llm` embedding client. Fallbacks: no embedding service → BM25 only; no `pg_search` extension in a project's database → the migration skips the BM25 index and search uses embeddings only, then `ILIKE`. Caveat: at ~7,000 short documents the quality gain over Postgres full-text is modest; the main reasons are Chinese support and consistency with ChenWeb.
3a. **Translate the metadata.** Besides the descriptions, generate `tags_zh` and `category_zh` (Lucide and Tabler tags and categories are English only) with the same LLM step, and store them as columns beside the English ones. A Chinese query then matches Chinese keywords directly, without relying on embeddings. The generator is given the icon name, English tags, category and description, and returns all Chinese fields in one call; results are reviewable and re-runnable.
4. **One set per name.** Seed Lucide first; add a Tabler icon only if no Lucide icon has that name. Name collisions across sets are resolved by Lucide winning; the rule is applied by the seed tool, and `UNIQUE(name)` enforces it.
5. **Metadata sources.** Lucide tags/categories from the Lucide repo `icons/*.json` (via `lucide-static` or a pinned git checkout), because `@lucide/svelte` ships none. Tabler from `@tabler/icons/icons.json` and `icons/outline/*.svg`. Seed tool is a Go program under `shared/go/cmd/icon-seed`, idempotent, bumps `version` only when content changes.
6. **Descriptions are generated once**, by an LLM, one sentence in each language per icon from name + tags + category, stored in the table and reviewable. Generation is a separate, resumable step so seeding works without LLM access.
7. **Serving.** `GET /icons/search?q=&limit=&category=` returns metadata (no SVG body, to keep responses small); `GET /icons/by-name/:name/svg` returns `image/svg+xml` with long cache headers and an ETag on `version`.
8. **Frontend.** `<Icon name size strokeWidth>` fetches by name, caches in memory, and inlines the SVG; unknown names render a neutral placeholder. A new `IconSearchPicker` (the legacy `IconPicker` is bound to the file-based store and is left alone) calls the search endpoint with debounce and shows results in a grid.
9. **SVG-only for the library.** `POST /icons/library` accepts only SVG, sanitised by an element/attribute allow-list (`iconlib.SanitizeSVG`) (strip `<script>`, event handlers, external refs). Legacy bitmap rows are left alone.

## Risks / Trade-offs

- [Embeddings need a model that is up at seed time] → seeding stores text first; embeddings backfill separately; search degrades to full-text.
- [LLM translations of tags may be poor or inconsistent] → one call per icon with all fields, a fixed glossary for common terms, spot-check sample before backfill.
- [Uploaded SVG is an XSS vector] → sanitise on upload, serve with `Content-Security-Policy: default-src 'none'`, inline only sanitised bodies.
- [Library growth, ~7,000 rows] → fine for a sequential scan plus ANN index; revisit if custom icons grow large.
- [Licences: Lucide ISC, Tabler MIT] → record source, version and licence in the seed tool output and in `README`.

## Migration Plan

1. Goose migration adds the columns, unique index and indexes; `CreateIconsTable` updated to match for fresh installs.
2. Run `icon-seed` (Lucide, then Tabler), then description generation, then embedding backfill.
3. Roll back by dropping the added columns; legacy rows are unaffected.

## Open Questions

- Should the library include Tabler filled variants, or outline only? (Proposed: outline only, to keep one visual style.)
- Where do embeddings come from in projects without the embedding service? (Proposed: BM25-only fallback.)

## Implementation notes

- Query embedding is injected: apps call `iconlib.SetQueryEmbedder`; ChenWeb registers its KB query embedder at startup. No embedder → BM25 only.
- Shared Svelte components have no i18n of their own; `IconSearchPicker` takes `labels` and `locale` props that the host app fills from Paraglide.
- WPI `icon-picker` is currently an alias of `value-picker`; wiring it to `IconSearchPicker` is deferred to the `wpi-value-editor-elements` change, which owns value editors.
