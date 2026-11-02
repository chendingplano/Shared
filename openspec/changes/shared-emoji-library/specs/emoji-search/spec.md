## ADDED Requirements

### Requirement: True bilingual BM25 text search
The library SHALL expose `GET /shared_api/v1/emojis/search` and rank nonempty textual queries using BM25 over English and Chinese names, mandatory keywords, categories and descriptions. Chinese indexing and query analysis SHALL use a Chinese tokenizer. Text search SHALL work without embeddings or query translation.

#### Scenario: English and Chinese keyword pair
- **WHEN** an author searches `celebration` and then `庆祝`
- **THEN** the party-popper emoji `🎉` is among the top ten BM25 results for each query

#### Scenario: Phrase and category matching
- **WHEN** an author searches a known Chinese description phrase with a category filter
- **THEN** results are BM25-ranked matches from that category with stable tie-breaking

### Requirement: Explicit BM25 unavailability
The application SHALL report emoji search readiness and SHALL NOT silently substitute substring search for BM25. Missing BM25 prerequisites SHALL produce a stable unavailable reason and HTTP 503 for nonempty text searches.

#### Scenario: BM25 index unavailable
- **WHEN** the BM25 extension or emoji index is unavailable
- **THEN** readiness identifies the missing prerequisite and a text search returns 503 instead of successful `ILIKE` results

### Requirement: Browsing and exact sequence lookup
An empty query SHALL browse in stable Unicode order and accept a stable category ID filter, a nonnegative offset defaulting to zero, and continuation metadata identifying the next offset when more entries exist. Every catalogue entry SHALL be reachable through successive browse pages. Exact supported Unicode input SHALL resolve the complete sequence independently of text tokenization. Results SHALL include ID, Unicode value, bilingual metadata and revision but exclude SVG bodies. Result limits SHALL default to 30 and never exceed 200. Nonempty search SHALL start at offset zero.

#### Scenario: Browsing a category
- **WHEN** the query is empty and the category is `food-drink`
- **THEN** results contain only that category in deterministic order

#### Scenario: Browsing beyond the first page
- **WHEN** a category contains more entries than the requested limit and the client follows the returned next offsets
- **THEN** every entry is reachable without knowing its name or keyword, with no duplicate or skipped entries while the catalogue is unchanged

#### Scenario: Exact joined sequence
- **WHEN** the query is a supported multi-codepoint emoji
- **THEN** the exact sequence is returned without splitting it into independent characters

### Requirement: Local metadata and SVG retrieval
The library SHALL expose metadata at `GET /shared_api/v1/emojis/by-id/:unicodeId`, SVG at the corresponding `/svg` path, and categories at `GET /shared_api/v1/emojis/categories`. SVG delivery SHALL use `image/svg+xml`, content-specific cache validators and conditional GET. Unknown IDs SHALL return 404.

#### Scenario: Selected value outside results
- **WHEN** an existing selection is not among the current search results
- **THEN** its metadata can be retrieved separately and its local SVG remains available

#### Scenario: Cached SVG
- **WHEN** a client sends an ETag matching the requested SVG content
- **THEN** the server returns 304 without the SVG body
