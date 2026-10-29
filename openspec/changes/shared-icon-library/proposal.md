## Why

Projects need to choose icons at runtime (e.g. the WPI `icon-picker` has no catalogue, so every instance supplies its own list). `shared/` already has an upload-based icon manager, but it is empty of useful content, searches only by `ILIKE` on name and tags, and is not used by ChenWeb. Users must be able to find an icon by describing it, so every icon needs keywords and a description, and search must understand short descriptions in English and Chinese.

## What Changes

- Seed the existing `icons` table with a vector icon library: Lucide as the base set, Tabler only for names Lucide lacks (one set per icon name). Lucide tags come from the Lucide repo / `lucide-static` (`@lucide/svelte` ships no metadata); Tabler tags/category come from `@tabler/icons/icons.json`.
- Store the SVG body in the table (new column), plus `source_set`, `version`, and LLM-generated `description_en` / `description_zh`.
- Replace `ILIKE` search with hybrid search (BM25 over name/tags/descriptions + embedding similarity), accepting keywords or a short description.
- Add `GET /shared_api/v1/icons/search` and `GET /shared_api/v1/icons/:name/svg`.
- Add a shared `<Icon name>` component that fetches and caches the SVG, and rework the existing shared `IconPicker` to use search.
- Wire the WPI `icon-picker` value editor to the shared picker (follow-up task in ChenWeb).
- Restrict the library to SVG: new uploads of bitmap types are rejected for the library; existing bitmap rows are untouched. Custom SVG upload stays, with `source_set='custom'`.
- The 114 static `@lucide/svelte` imports in ChenWeb stay as they are.

## Capabilities

### New Capabilities
- `icon-library`: seeded, versioned vector icon catalogue stored in the `icons` table, with tags, category and bilingual descriptions.
- `icon-search`: hybrid keyword/description search over the catalogue.
- `icon-picker`: shared Svelte `<Icon>` renderer and `IconPicker` that other projects use to pick and show icons.

### Modified Capabilities
<!-- none: no existing icon spec in openspec/specs -->

## Impact

- `shared/go/api/sysdatastores/table-icons.go` (columns, indexes), `shared/go/api/goose` migration, `shared/go/api/RequestHandlers/icons_handlers.go`, `shared/go/api/icons/`.
- `shared/svelte/src/lib/components/icons/*`, `types/IconTypes.ts` (kept in sync with `ApiTypes/icons_types.go`).
- New seed/import tool (Go) reading Lucide and Tabler metadata; one-off LLM description generation.
- Reuses the existing embedding and BM25 infrastructure; needs a decision on where it lives in shared (see design).
- ChenWeb: WPI `icon-picker` wiring only, committed separately.
