## 1. Pin inputs and confirm deployment prerequisites

- [ ] 1.1 Inspect current shared/ChenWeb instructions and dirty status; identify the latest WPI value-editor registries and keep unrelated work outside task commits.
- [ ] 1.2 Pin compatible stable Twemoji, Unicode emoji inventory and CLDR revisions; record checksums, licenses, Unicode version and source paths in a manifest.
- [ ] 1.3 Verify the target PostgreSQL/ParadeDB version, BM25 index/query API and Chinese `jieba` support; document enablement and readiness policy for each consumer.
- [ ] 1.4 Add emoji-specific deltas to ChenWeb's current property-editor and value-editor element contracts without changing the icon-library follow-ups.

## 2. Build the bilingual catalogue artifact

- [ ] 2.1 Add reusable build logic and a thin project-owned import/build command under shared/go, supporting dry-run and coverage diagnostics.
- [ ] 2.2 Parse fully qualified RGI sequences, groups/subgroups and Twemoji assets into stable Unicode IDs and explicit aliases; test selectors, flags, keycaps, joiners and skin tones.
- [ ] 2.3 Join English/Chinese CLDR base and derived annotations into keywords and short descriptions; translate category labels through a reviewed mapping.
- [ ] 2.4 Add resumable missing-field translation with reviewable overrides, provenance and input hashes; store any model prompt in a versioned prompt file.
- [ ] 2.5 Reject absent/blank keywords in either language, English placeholders in Chinese fields, missing names/categories/descriptions, missing required SVGs and unsafe SVG content. Test complete and failing publication artifacts.
- [ ] 2.6 Include original artwork, code/data licenses and attribution alongside the deterministic build artifact; report complete inventory and metadata counts.

## 3. Store and serve the local library

- [ ] 3.1 Add a new guarded goose migration in ChenWeb/shared_migrations for shared.emoji_library with uniqueness/nonblank constraints and BM25 index; wire shared startup migration/readiness using the centralized pool and a single DDL source.
- [ ] 3.2 Implement atomic idempotent import with content revisions; verify unchanged imports, failed transactions and historical sequence preservation.
- [ ] 3.3 Implement emojilib BM25 text search, paginated category browse, exact Unicode lookup and metadata/category retrieval. Test reachability beyond the first 200 browse entries, real relevance for `celebration`/`庆祝`, descriptions and filters, plus explicit unavailable behavior without BM25.
- [ ] 3.4 Add local read-only search, metadata, category and SVG endpoints with bounded parameters, shared response envelopes, logging, 404 handling and content-based conditional caching.
- [ ] 3.5 Run the complete import against the development database; read back completeness/coverage counts and verify live BM25 behavior without an embedding or translation service.

## 4. Shared frontend components

- [ ] 4.1 Add Go-owned public data types and synchronized TypeScript types/client; export Emoji and EmojiSearchPicker from the shared package.
- [ ] 4.2 Implement local SVG rendering, accessible/decorative modes and Unicode fallback without injecting raw fetched markup.
- [ ] 4.3 Implement controlled Unicode selection, debounced cancellable search, category browsing with localized Load more and page reset, localized metadata/labels and distinct loading/empty/error states; preserve selection outside results.
- [ ] 4.4 Verify keyboard activation, visible focus, no stale-result races, text selection/copy, unknown values, locale changes and offline asset serving in component/browser checks.

## 5. WPI emoji value editor

- [ ] 5.1 Add EmojiPickerEditor using ValueEditorProps and the shared picker; apply only validated Unicode values, unset with undefined, and preserve standard reset, invalid-edit feedback, focus/undo and external-update behavior.
- [ ] 5.2 Register emoji-picker in frontend editor types, resolution/rendering, Class configuration and backend validation; add both Paraglide message sets and any necessary option validation.
- [ ] 5.3 Add a separate goose migration seeding the active emoji-picker class over the existing value-editor module. Verify standalone palette visibility, bindings, read-only mode and clear-to-null behavior.
- [ ] 5.4 Browser-test English and Chinese property selection, a standalone bound picker, invalid-value rejection, keyboard/Escape focus restoration, undo/reset, immediate preview and save/reopen.

## 6. Documentation, verification and commits

- [ ] 6.1 Update WPI devdoc 2026102701's value-editor table and code map plus the existing value-editor code-module document and affected WPI contracts; leave icon-library issues out of this change.
- [ ] 6.2 Document/catalogue backend, shared frontend and WPI editor reusable modules through create-reusable-module, keeping database rows and module docs in agreement.
- [ ] 6.3 Write the implementation devdoc with license notices, local-serving architecture, metadata/translation updates, strict BM25 prerequisites, fresh-install and production migration/seed instructions; identify deliberately undocumented internals.
- [ ] 6.4 Run shared Go tests, go work sync, shared Svelte build/check and appropriate ChenWeb tests/checks plus dependent application builds; record pre-existing failures separately.
- [ ] 6.5 Verify runtime with external network disabled and database BM25 enabled, as well as the unavailable state without BM25; confirm deployed third-party notices exist.
- [ ] 6.6 Commit only task-owned changes through jj in separate shared, ChenWeb and KnowledgeStore commits; verify expected linear histories and preserve unrelated working-copy changes.
