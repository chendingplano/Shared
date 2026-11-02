## Why

Applications need a free, commercially usable, locally hosted emoji library with actual BM25 search in English and Chinese. The WPI Editor also needs an emoji value editor that follows its existing property-editing and value-editor element contracts.

## What Changes

- Adopt pinned Twemoji SVG artwork and Unicode CLDR English/Simplified Chinese metadata. Preserve artwork attribution and all applicable code/data licenses in deployments.
- Add an emoji catalogue in `shared` with a stable Unicode sequence, source/version, mandatory nonempty English and Chinese keywords, bilingual categories and short descriptions, and a locally stored SVG. Translate remaining English-only metadata during catalogue construction; incomplete metadata must fail publication.
- Add BM25 search over both languages using the existing PostgreSQL/ParadeDB infrastructure, with Chinese tokenization. No embedding service is required and substring search is not a substitute for BM25.
- Add shared `Emoji` and `EmojiSearchPicker` Svelte components with localized labels supplied by the host.
- Add WPI `emoji-picker` as a real value editor in both Properties/Class configuration and the existing `value-editor` element module. Store the Unicode string, not a URL or provider name.
- Include a guarded goose migration, startup wiring, repeatable seed/build workflow, validation, tests, and reusable-module documentation.

## Capabilities

### New Capabilities

- `emoji-library`: licensed local SVG catalogue, mandatory bilingual metadata, sequence identity, and repeatable validated imports.
- `emoji-search`: required BM25 ranking in both languages, category browsing, and local SVG delivery.
- `emoji-value-editor`: shared rendering/picking plus the WPI property and standalone value-editor integration.

### Modified Capabilities

None in shared. The implementation must add corresponding deltas to ChenWeb's current `wpi-property-editors` and `wpi-value-editor-elements` contracts when wiring the host application; those contracts currently reside in pending changes. Existing icon-library contracts and open issues are out of scope.

## Impact

- Shared Go: new `api/emojilib`, request handlers and route registration, startup integration using the centralized shared pool; project-owned build/import command under `go/cmd`, following the existing icon-seed pattern.
- Shared Svelte: `Emoji`, `EmojiSearchPicker`, client/types and public exports.
- Database: `shared.emoji_library` and its BM25 index; migrations live in ChenWeb's `shared_migrations`, matching existing deployment ownership. No vector columns or extensions are needed.
- ChenWeb: `EmojiPickerEditor`, editor ID/validation registries, `emoji-picker` class seed migration, both Paraglide message files, and browser tests. Build on the current uncommitted value-editor work without committing or rewriting it as part of this change.
- KnowledgeStore: catalogue backend/frontend/editor modules; update WPI Editor devdoc 2026102701 and the value-editor code-module document. Leave the icon handoff's issues unchanged.
- Deployment: pinned upstream source manifest, validated complete bilingual import, BM25 readiness checks, licenses and attribution. No third-party network calls at runtime.
