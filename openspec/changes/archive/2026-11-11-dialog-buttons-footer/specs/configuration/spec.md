## MODIFIED Requirements

### Requirement: Key hints and key help fit the window
The footer SHALL show the hints of the focused list or panel with its main key first, and `Keys ? · Quit q` at the right. Each hint SHALL show its label with a capital first letter in the primary color (the accent `#7D56F4`), then its key in the secondary color (grey), for example `Reply r`. It SHALL show only the hints that fit the width of the window, in their order, and SHALL NOT cut a hint. At 80 columns the footer SHALL show `ctrl+r` in the Spaces area and `enter` and `a` in the panel of a space. The key help (`?`) SHALL be a dialog as wide as its longest line, or the window when the window is narrower. It SHALL NOT wrap a line that fits, and it SHALL scroll when the window is too short.

#### Scenario: Spaces footer at 80 columns
- **WHEN** the window is 80 columns wide and a space is selected in the Spaces area
- **THEN** the footer shows `Run now ctrl+r`, and each hint is complete

#### Scenario: Label, then key
- **WHEN** the Feed shows a card
- **THEN** the footer shows `Reply r`: "Reply" in the primary color and `r` in the secondary color

#### Scenario: Key help at 140 columns
- **WHEN** the window is 140 × 34 and the user presses `?`
- **THEN** each line of the key help shows as one line, and the line of the Feed keys starts with "Feed"

### Requirement: Search of conversations
`/` SHALL open a search of the conversations of all accounts from each area, except in a form. The rows SHALL show the kind mark, the name, the account and the spaces, with the newest activity first. Typing SHALL filter the rows by the name, the label, the account or a space; the search SHALL ignore case. `enter` SHALL open the spaces dialog of the row. `a` in the Spaces area SHALL open the add dialog of the selected space, with the title "Add conversations to <space>": each row SHALL show a check mark when the space has it, `space` SHALL add or remove it, and `←`/`→` SHALL change its level in that space. The add dialog SHALL keep these changes until the user selects OK, mark each changed row with `*`, and show the number of changes that are not saved. It SHALL have the buttons OK and Cancel under the list. `tab` SHALL move from the list to OK, then to Cancel; `enter` in the list and `ctrl+s` SHALL run OK; `esc` SHALL run Cancel; a mouse click on a button SHALL run it. OK SHALL write all changes in one save and close the dialog. Cancel SHALL close the dialog with no change to the file.

#### Scenario: Find and add
- **WHEN** the user presses `a` in Review, types `rel`, moves to `# release`, presses `space` and selects OK
- **THEN** the file lists `# release` in Review with `all`

#### Scenario: No save before OK
- **WHEN** the user presses `a` in Review, marks `# release` with `space`, and presses `esc`
- **THEN** the file does not list `# release` in Review

#### Scenario: Click on OK
- **WHEN** the user marks a row in the add dialog and clicks OK
- **THEN** the file has the change and the dialog closes
