## ADDED Requirements

### Requirement: Shared Unicode renderer and search picker
Shared SHALL export `Emoji` and `EmojiSearchPicker`. The renderer SHALL use local artwork with a Unicode fallback. The controlled picker SHALL select a complete Unicode string and present localized metadata, categories, loading, empty and error states using host-supplied translated labels. Blank-query browsing SHALL offer a localized Load more action when a next page exists and reset paging when category or query changes. Obsolete search responses SHALL NOT replace current results.

#### Scenario: English and Chinese selection
- **WHEN** an author finds `🎉` using either supported interface language and selects it
- **THEN** the component emits the Unicode string `🎉` and shows the selected emoji with the appropriate localized metadata

#### Scenario: Obsolete response
- **WHEN** an earlier search completes after a later search
- **THEN** the picker retains the later query's results

#### Scenario: Pick from a later browse page
- **WHEN** an author loads more entries from a category and selects an emoji beyond the initial page
- **THEN** the picker emits that entry's Unicode value and preserves it as the selection

#### Scenario: Unknown saved value
- **WHEN** artwork for the current Unicode value is unavailable
- **THEN** the renderer displays the original Unicode string and preserves the stored value

### Requirement: WPI emoji value editor
ChenWeb SHALL provide `emoji-picker` through its common editor registries and `ValueEditorProps` contract. Selection SHALL apply the Unicode string only after the property's validation succeeds. Clearing SHALL call `onChange(undefined)` and reset SHALL use the existing remove-own-value behavior. External changes SHALL immediately update the control; a focus session SHALL follow the existing single-undo-step behavior.

#### Scenario: Pick save and reopen
- **WHEN** an author picks `🎉` for an emoji property, saves the page and reopens it
- **THEN** the stored value is `🎉` and the editor shows the same emoji

#### Scenario: Property validation rejects a selection
- **WHEN** a chosen value fails the property's `check` rule
- **THEN** it is not applied and the existing invalid-edit feedback and recovery controls are shown

#### Scenario: Clear reset and undo
- **WHEN** an author clears or resets a value and then undoes the property edit
- **THEN** unset/default semantics match other value editors and undo restores the previous value

### Requirement: Standalone WPI value-editor element
The system SHALL register an active `emoji-picker` class using the existing `value-editor` code module. Properties/Class configuration and standalone rendering SHALL resolve the same editor ID. Runtime input-doc binding and read-only behavior SHALL follow the existing value-editor element contract.

#### Scenario: Bound standalone picker
- **WHEN** an author places `emoji-picker` on a page, binds it to an input-doc field and selects an emoji in runtime preview
- **THEN** that field receives the full Unicode string, persists through normal save/reopen, and remains unchanged when the element is read-only

### Requirement: Accessible localized interaction
The picker and WPI adapter SHALL support keyboard activation, visible focus, accessible localized names, Escape to close the popup with focus restoration, and selectable text. ChenWeb SHALL supply all UI labels through both Paraglide message files and update displayed copy when the locale changes.

#### Scenario: Keyboard selection
- **WHEN** an author opens the picker and activates a result using the keyboard
- **THEN** the value is selected with the same behavior as pointer activation

#### Scenario: Escape and text selection
- **WHEN** an author closes the popup with Escape and selects nearby text in ChenWeb's shell
- **THEN** focus returns to the trigger and text remains selectable
