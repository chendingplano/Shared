-- +goose Up
CREATE SCHEMA IF NOT EXISTS shared;
CREATE TABLE IF NOT EXISTS shared.emoji_library (
 id BIGSERIAL PRIMARY KEY,
 unicode_id TEXT NOT NULL UNIQUE,
 unicode_value TEXT NOT NULL UNIQUE,
 name_en TEXT NOT NULL CHECK (btrim(name_en) <> ''),
 name_zh TEXT NOT NULL CHECK (btrim(name_zh) <> ''),
 category_id TEXT NOT NULL CHECK (btrim(category_id) <> ''),
 category_en TEXT NOT NULL CHECK (btrim(category_en) <> ''),
 category_zh TEXT NOT NULL CHECK (btrim(category_zh) <> ''),
 keywords_en TEXT NOT NULL CHECK (btrim(keywords_en) <> ''),
 keywords_zh TEXT NOT NULL CHECK (btrim(keywords_zh) <> ''),
 description_en TEXT NOT NULL CHECK (btrim(description_en) <> ''),
 description_zh TEXT NOT NULL CHECK (btrim(description_zh) <> ''),
 svg_body TEXT NOT NULL CHECK (btrim(svg_body) <> ''),
 content_hash TEXT NOT NULL,
 sort_order INTEGER NOT NULL,
 source_version TEXT NOT NULL,
 unicode_version TEXT NOT NULL,
 version INTEGER NOT NULL DEFAULT 1,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_emoji_library_category ON shared.emoji_library (category_id, sort_order, unicode_id);
-- A missing required extension is an error, rather than silently installing a
-- catalogue whose keyword search is not BM25.
CREATE EXTENSION IF NOT EXISTS pg_search;
CREATE INDEX IF NOT EXISTS idx_emoji_library_bm25 ON shared.emoji_library
 USING bm25 (id, name_en, name_zh, keywords_en, keywords_zh, category_en, category_zh, description_en, description_zh)
 WITH (key_field='id', text_fields='{
   "name_zh": {"tokenizer": {"type": "jieba"}},
   "keywords_zh": {"tokenizer": {"type": "jieba"}},
   "category_zh": {"tokenizer": {"type": "jieba"}},
   "description_zh": {"tokenizer": {"type": "jieba"}}
 }');

-- +goose Down
DROP TABLE IF EXISTS shared.emoji_library;
