## 1. Schema

- [x] 1.1 Goose migration: add `svg_body`, `source_set`, `description_en`, `description_zh`, `version`, `embedding vector(1024)`, `tags_zh`, `category_zh`, unique index on `name`, ParadeDB `bm25` index (jieba on zh fields; skipped if `pg_search` is absent)
- [x] 1.2 (Superseded: new table `shared.icon_library`; no changes to the legacy table or its types)

## 2. Seeding

- [x] 2.1 `icon-seed` tool: import Lucide (repo metadata), then Tabler outline (gap-filler), idempotent, version bump on change
- [ ] 2.2 Resumable generator for Chinese tags, category and descriptions (and English descriptions) and embedding backfill
- [ ] 2.3 Report missing tags/descriptions/embeddings; record sources and licences

## 3. API

- [x] 3.1 `GET /icons/search` (BM25 + vector, RRF, fallbacks)
- [x] 3.2 `GET /icons/by-name/:name/svg` with ETag on version and CSP header
- [x] 3.3 SVG-only upload with sanitiser; tests for script/handler stripping
- [ ] 3.4 Go tests; `cd shared/go && go test ./...`, `go work sync`, build ChenWeb and tax

## 4. Frontend

- [x] 4.1 `Icon` component with cache and placeholder
- [x] 4.2 Rework `IconPicker` to use search; en/zh text; text selectable
- [ ] 4.3 Wire the WPI `icon-picker` value editor (deferred to `wpi-value-editor-elements`); ChenWeb registers the query embedder (done)

## 5. Docs

- [ ] 5.1 Update `shared/go/api/goose` docs if needed, add devdoc, module catalogue entry
- [ ] 5.2 Answer the CLAUDE.md checklist: knowledge changed, docs affected, stale docs
