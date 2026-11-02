## Context

The request remains a proposal for an emoji library, now with mandatory BM25, bilingual metadata and a WPI value editor. Icon selection is implemented and is not part of this change. Read with `KnowledgeStore/doc-repo/devdocs/202610/2026102701-devdoc-wpi-editor.md` §1.3.9 and `KnowledgeStore/doc-repo/hand-offs/202611/2026110202-hand-offs-icon-library-open-issues.md`.

Current code provides useful precedents:

- `shared/go/api/iconlib` uses the centralized shared database and ParadeDB BM25 with `jieba` on Chinese fields. Its substring fallback and optional embeddings are not requirements for emojis.
- `ChenWeb/shared_migrations/20261103000001_create_shared_icon_library.sql` establishes the migration location and index syntax.
- ChenWeb's uncommitted `wpi-value-editor-elements` work already includes `IconPickerEditor.svelte`, common `ValueEditorProps`, editor ID resolution, backend validation, input-doc bindings, and a palette section. The historical handoff says this wiring is pending; inspect current code rather than treating that historical status as authoritative.
- Shared Svelte takes translated labels and locale from its host; ChenWeb uses Paraglide. Do not introduce a second localization system.

## Goals / Non-Goals

**Goals:** locally served consistent emoji artwork; mandatory English and Simplified Chinese metadata; true BM25; accessible reusable rendering/picking; integrated WPI editing with Unicode values; independently deployable consumers; reproducible imports and notices.

**Non-Goals:** icon work, uploads/custom emojis, animated artwork, embeddings or semantic search, replacing every native emoji in every page, and an emoji-specific database/API for each application.

## Decisions

### 1. Twemoji artwork, Unicode identity

Use one pinned stable Twemoji release and pinned compatible Unicode emoji/CLDR data. Record exact tags/commits, checksums, Unicode version, import-tool version and licenses in a source manifest. Fetch upstream only as an explicit build/update operation; application startup and runtime never download it.

Twemoji supplies SVGs under CC BY 4.0 and code under MIT. Include its original notices, a third-party attribution entry linking source and license, and modification information when applicable. CLDR annotation data carries Unicode-3.0 notices. Preserve the data notice separately from artwork notices.

Alternatives: Fluent offers MIT artwork but would require a fresh coverage audit; OpenMoji's artwork adds ShareAlike conditions. Twemoji is the proposed choice for standard emoji coverage; the wrapper keeps provider filenames out of saved data.

Persist a complete Unicode emoji sequence, including joiners, selectors and modifiers, as the value. Give each fully qualified sequence a stable codepoint ID, such as `1f389`, and record explicit input aliases in the manifest. Match Unicode sequences to upstream SVG paths during the build; do not assume removing every FE0F gives a valid unique filename. Do not deduplicate different skin tones, genders or flags. CLDR annotation lookup has its own documented selector normalization and must not change the stored Unicode string.

### 2. Build complete bilingual metadata before importing

For each RGI sequence in the pinned Unicode inventory:

1. Resolve the Twemoji asset and Unicode group/subgroup.
2. Join CLDR `common/annotations/{en,zh}.xml` and `common/annotationsDerived/{en,zh}.xml` by sequence, resolving aliases with explicit rules.
3. Extract keyword annotations and short names. Use the short name as the initial short description. Translate category labels with a reviewed English/Chinese group dictionary; category IDs stay language-independent.
4. Translate any remaining English-only keywords, names, categories or descriptions during the build. Store missing-data translations as reviewable overrides keyed by Unicode ID and input hash. Use the workspace's configured translation model if needed; save any prompt in a versioned prompt file, not a code literal. No translation occurs in a user's search request.
5. Trim and deduplicate keywords without collapsing meaningful variants. Require nonempty English and Chinese keyword lists and bilingual names, categories and descriptions. Chinese fields must contain real Chinese translations rather than copied English placeholders.
6. Fail the build with IDs and reasons for missing assets or metadata. Do not import a partial catalogue as the default success path. Validate the whole artifact before changing the active data; import atomically. Keep the previous valid catalogue on failure.

This uses existing translations first and reserves generated translations for gaps. A resumable translation cache makes interrupted builds inexpensive to resume. Rebuilding identical inputs and reviewed overrides must produce identical output.

### 3. Catalogue and local asset delivery

Use `shared.emoji_library` in the existing shared database. Proposed fields:

| Field | Role |
|---|---|
| `id` | Internal numeric BM25 key |
| `unicode_id`, `unicode_value` | Unique stable ID and complete emoji string |
| `name_en`, `name_zh` | Localized names |
| `category_id`, `category_en`, `category_zh` | Stable filter and localized labels |
| `keywords_en`, `keywords_zh` | Required nonblank search text, generated from keyword arrays in the build artifact |
| `description_en`, `description_zh` | Required short descriptions |
| `source_set`, `source_version`, `unicode_version` | Artwork/data provenance |
| `svg_body`, `content_hash`, `version` | Local artwork, cache identity and revision |
| `created_at`, `updated_at` | Audit fields |

Keep metadata/provenance/aliases in the versioned build artifact; enforce unique sequence identity and nonblank required fields in PostgreSQL as well as in the importer. A content change bumps the row version; unchanged imports do not churn timestamps or versions. Never remove old Unicode IDs automatically during an update, because saved pages may still reference them.

Store SVG in the shared DB and serve it locally, following the established icon library pattern. This is simpler than adding a second asset-copy deployment pipeline and lets independent apps serve the same reference catalogue. Use the centralized shared pool; runtime library packages do not open their own connections. Project-owned Go build/import commands follow `icon-seed`'s location, with reusable logic outside `main`.

Endpoints:

- `GET /shared_api/v1/emojis/search?q=&category=&limit=&offset=`: bilingual metadata, Unicode string, stable ID, version and score, without SVG bodies; blank query browses by stable Unicode order. For browsing, `offset` defaults to zero and the response includes continuation metadata (`next_offset` when more entries exist). Reset the offset on query/category changes. Nonempty search starts at offset zero; search result pagination is outside this initial scope.
- `GET /shared_api/v1/emojis/by-id/:unicodeId`: retrieve metadata for an existing selection outside the current search results.
- `GET /shared_api/v1/emojis/by-id/:unicodeId/svg`: SVG, `image/svg+xml`, content-based ETag, conditional GET and bounded caching.
- `GET /shared_api/v1/emojis/categories`: stable category IDs and bilingual labels.

Use shared response envelopes and existing route conventions. Read-only reference endpoints follow icon endpoints' access policy; no public write endpoint. Reject unsafe SVG content before publication and render via an image URL rather than injecting fetched markup. Unknown IDs return 404. Record import counts, missing metadata, versions and startup/search failures with the shared logger and unique locations; avoid logging credentials or individual keystroke queries.

### 4. BM25 is a deployment requirement

Create a ParadeDB `pg_search` BM25 index on names, English/Chinese keywords, category labels and descriptions, keyed by numeric `id`; use the installed version's `jieba` tokenizer for Chinese fields. Use parameterized plain-text queries and category filters with deterministic tie-breaking, bounded query length and result limit (default 30, maximum 200). Exact Unicode input resolves through the sequence manifest/database identity before textual search, because a text tokenizer need not preserve emoji glyphs. Optional exact-name/keyword boosts must preserve BM25 as the text ranking engine and be verified against the installed ParadeDB version.

Use the same technology as icon search without changing iconlib or adding vector columns. Do not copy its process-global index-availability cache or optional `ILIKE` fallback. If BM25 prerequisites are absent, startup/readiness reports that emoji search is unavailable and nonempty text-search requests return 503 with a stable machine-readable reason. Existing deployments without the new feature may still start; they must not report emoji search as healthy. SVG retrieval, exact Unicode lookup and blank-query browsing may still work with the catalogue present.

Shared table creation must be wired through `sysdatastores.CreateSysTables(...)` with goose migration ownership preserved. Extension installation follows deployment policy; when enabled for the target deployment, migration/index setup fails visibly if required BM25 cannot be provided. Do not introduce a second hand-maintained DDL definition. Verify live Chinese tokenization and relevance, not just a SQL string or an extension-exists check.

### 5. Shared components and WPI adapter

Shared exports:

- `Emoji`: Unicode `value`, size and optional accessible label; local SVG rendering; Unicode fallback for unknown/unavailable assets; decorative mode for adjacent labels.
- `EmojiSearchPicker`: controlled Unicode `value`, `onSelect`, locale, translated labels, limit and category filter. Debounce requests, cancel obsolete ones, and prevent old responses replacing newer results. Display localized names/descriptions, categories and keywords. Provide localized Load more for blank-query browsing so every entry in a category can be reached; reset paging when query/category changes. Keep an existing selection visible even when it is not in the current result list. Loading, no results and service errors are distinct.

Use standard buttons with keyboard activation and visible focus; search and result labels are accessible, Escape closes the WPI popup and restores trigger focus. Do not declare an ARIA grid without its complete keyboard model. Use existing styles/theme variables; allow text selection, including in ChenWeb's `select-none` shell.

Add `EmojiPickerEditor.svelte` in ChenWeb following `ValueEditorProps`: show current emoji/name, open the shared search picker, apply a selected Unicode string, and clear with `onChange(undefined)`. Call `check` before `onChange`; rejected values never update the descriptor and use the existing invalid-edit recovery behavior. Keep current value and validation feedback in sync with external changes/undo. Reset remains the property's standard remove-own-value action. Clearing a standalone element follows the host's existing unset-to-null binding rule.

Register `emoji-picker` in the frontend `ValueEditorId`, editor-ID resolution, `valueEditors`, Class editor catalogue and backend allowed IDs; add localized labels/help and options where actually needed. Seed an active `emoji-picker` descriptor using the existing `value-editor` code module, so the same editor works as a property control and a standalone WPI element with normal bindings/read-only behavior. Use a new goose migration; do not edit the already existing value-editor seed migration. All host text goes through both Paraglide files and follows locale switches reactively.

Tests must exercise one-focus-session undo, immediate canvas updates, reset/clear, validation rejection, runtime binding, save/reopen, and both languages. This addition must not alter icon selection.

## Risks / Trade-offs

- [CLDR and artwork releases cover different sequences] → pin compatible inputs, use explicit aliases and reviewed gap translations, fail coverage validation rather than silently dropping entries.
- [Generated keywords are poor translations] → prefer CLDR; retain provenance, review overrides and test representative English/Chinese relevance pairs.
- [Required BM25 is absent in another consumer's database] → documented prerequisite and explicit unavailable state; independent deployments opt into the required migration/setup.
- [Value-editor restructuring is concurrently changing] → integrate against the current registries and add an isolated migration; do not sweep pre-existing work into emoji commits.
- [Image substitution affects copying] → store Unicode and verify copying an emoji/text selection yields the expected Unicode; provide an explicit copy action if the browser cannot preserve it.

## Migration Plan

1. Add catalogue schema/index migration and startup readiness integration; verify against the target database's actual ParadeDB version, including Chinese tokenization.
2. Build the complete pinned bilingual artifact, retain source licenses and attribution, dry-run, then atomically import it. Read back completeness counts and versions.
3. Add local endpoints and shared components. Build shared packages and verify dependents remain buildable.
4. Integrate the WPI adapter and new `emoji-picker` descriptor with a separate project migration; test property and standalone modes.
5. Update devdoc 2026102701, the value-editor module document, reusable module catalogue docs/rows, and the relevant WPI contracts. Record BM25 prerequisites, import/update and production setup.
6. Commit only task-owned paths/hunks through jj in separate shared, application and KnowledgeStore commits; verify linear history. Run shared Go tests, `go work sync`, dependent builds, Svelte checks and browser tests appropriate to the implementation.

Rollback: disable the new picker/class and routes while leaving catalogue data available for saved values; remove only this migration's owned class record. Avoid destructive deletion of reference rows needed by saved pages. Existing icon data and APIs remain untouched.

## Open Questions

No product decision blocks this proposed design. Implementation must determine exact compatible upstream pins and confirm the installed ParadeDB API/tokenizer support. Those values belong in the reviewed manifest and verification evidence.

## Sources

- Twemoji artwork/code licenses and supported Unicode inventory: https://github.com/jdecked/twemoji/blob/main/README.md
- CLDR English annotations: https://github.com/unicode-org/cldr/blob/main/common/annotations/en.xml
- CLDR Chinese annotations: https://github.com/unicode-org/cldr/blob/main/common/annotations/zh.xml
- CLDR derived Chinese annotations: https://github.com/unicode-org/cldr/blob/main/common/annotationsDerived/zh.xml
- CLDR data license (distinct from LDML publication terms): https://github.com/unicode-org/cldr/blob/main/LICENSE

These were inspected on 2026-11-02; the implementation must use pinned revisions rather than these moving URLs.
