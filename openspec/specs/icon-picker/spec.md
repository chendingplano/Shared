# icon-picker Specification

## Purpose
TBD - created by archiving change shared-icon-library. Update Purpose after archive.
## Requirements
### Requirement: Icon renderer
The shared Svelte library SHALL export an `Icon` component that renders a library icon by name, sized and coloured by its container.

#### Scenario: Unknown name
- **WHEN** the name does not exist
- **THEN** a neutral placeholder is rendered and no error is thrown

### Requirement: Icon picker
The shared Svelte library SHALL export an `IconPicker` that searches the catalogue as the user types and emits the chosen icon name. All interface text SHALL be available in English and Chinese, and the picker SHALL allow text selection.

#### Scenario: Pick by description
- **WHEN** the user types a short description and selects a result
- **THEN** the picker emits that icon's name

