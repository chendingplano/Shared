# icon-search Specification

## Purpose
TBD - created by archiving change shared-icon-library. Update Purpose after archive.
## Requirements
### Requirement: Search by keywords or description
The system SHALL provide `GET /shared_api/v1/icons/search` that returns ranked icons for a free-text query in English or Chinese.

#### Scenario: Keyword match
- **WHEN** the query is `reload`
- **THEN** results include `refresh-cw`

#### Scenario: Descriptive query
- **WHEN** the query is `两个箭头循环`
- **THEN** results include `refresh-cw` among the top results

#### Scenario: Chinese keyword
- **WHEN** the query is `刷新`
- **THEN** results include `refresh-cw` from the Chinese tags, without needing embeddings

#### Scenario: Embedding unavailable
- **WHEN** the embedding service cannot be reached
- **THEN** the endpoint returns BM25 results and does not fail

### Requirement: SVG delivery
The system SHALL serve an icon's SVG by name with a cache validator derived from its version.

#### Scenario: Unknown name
- **WHEN** an unknown icon name is requested
- **THEN** the response is 404

