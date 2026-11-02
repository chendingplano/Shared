# icon-library Specification

## Purpose
TBD - created by archiving change shared-icon-library. Update Purpose after archive.
## Requirements
### Requirement: Seeded vector catalogue
The system SHALL store a catalogue of vector icons in the `icons` table, each with a unique name, SVG body, source set (`lucide`, `tabler` or `custom`), category, tags, and a version.

#### Scenario: One set per name
- **WHEN** the seed tool imports Lucide and then Tabler
- **THEN** a Tabler icon whose name already exists from Lucide is skipped

#### Scenario: Idempotent re-seed
- **WHEN** the seed tool runs twice with unchanged sources
- **THEN** no row is changed and no version is bumped

### Requirement: Searchable metadata
Every catalogue icon SHALL have at least one tag and Chinese tags and category, and an English and a Chinese one-sentence description once description generation has run.

#### Scenario: Missing descriptions are reported
- **WHEN** the seed tool finishes
- **THEN** it reports the number of icons lacking either description

### Requirement: SVG only
The library SHALL accept only SVG uploads for new icons and SHALL sanitise them.

#### Scenario: Bitmap upload rejected
- **WHEN** a user uploads a PNG as a library icon
- **THEN** the request fails with a validation error

#### Scenario: Script stripped
- **WHEN** an uploaded SVG contains a `<script>` element or `on*` attribute
- **THEN** those are removed before storage

