## ADDED Requirements

### Requirement: Complete local licensed catalogue
The library SHALL publish locally stored Twemoji SVG artwork from pinned upstream inputs, preserving source versions, licenses and attribution. Runtime rendering and search SHALL require no external network service.

#### Scenario: Runtime without internet
- **WHEN** a seeded application renders or searches emojis while external network access is blocked
- **THEN** assets and search results are served locally without CDN or translation requests

#### Scenario: Reproducible provenance
- **WHEN** an operator inspects the published catalogue artifact
- **THEN** it identifies exact upstream revisions, checksums, Unicode version, build version and applicable notices

### Requirement: Mandatory bilingual searchable metadata
Every published emoji SHALL have nonblank English and Simplified Chinese keywords, localized categories and short descriptions. Keyword lists SHALL contain at least one meaningful entry per language. The build SHALL use available CLDR translations and translate remaining English-only metadata before publication. English text copied into Chinese fields SHALL NOT count as translation.

#### Scenario: English-only source metadata
- **WHEN** an imported emoji has English keywords and no Chinese annotations
- **THEN** the build generates or applies reviewed Chinese translations and includes them in the published artifact before import

#### Scenario: Incomplete translation
- **WHEN** any required keyword list, category or description remains absent or untranslated
- **THEN** validation fails with the affected Unicode IDs and the previous valid active catalogue is retained

### Requirement: Sequence identity and coverage
The catalogue SHALL store each fully qualified RGI emoji sequence in the pinned inventory with its complete Unicode string and a unique stable codepoint ID. The build SHALL resolve explicit aliases to artwork and annotations without collapsing distinct variants.

#### Scenario: Complex sequences
- **WHEN** the inventory contains skin-tone, gender, joined-family, flag or keycap sequences
- **THEN** each published sequence retains its joiners, modifiers and selectors and resolves to the corresponding local asset

#### Scenario: Missing artwork
- **WHEN** a required inventory sequence cannot be mapped to an SVG
- **THEN** the complete-publication build fails with that sequence's ID rather than silently omitting it

### Requirement: Safe repeatable atomic import
Import SHALL validate the full artifact before publication, reject unsafe SVG, and update the catalogue atomically. Unchanged inputs SHALL leave row versions unchanged. Updates SHALL preserve historical IDs needed by existing stored values.

#### Scenario: Idempotent import
- **WHEN** the same validated artifact is imported twice
- **THEN** the second import changes no content version or update timestamp

#### Scenario: Invalid asset or interrupted import
- **WHEN** an SVG is unsafe or an import fails partway through
- **THEN** no partially updated active catalogue is published and a diagnostic is logged
