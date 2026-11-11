## MODIFIED Requirements

### Requirement: Key hints and key help fit the window
The footer SHALL show the hints of the focused list or panel with its main key first, and the buttons `Find /`, `Keys ?` and `Quit q` at the right when no dialog and no form is active. Each hint SHALL show its label with a capital first letter in the primary color (the accent `#7D56F4`), then its key in the secondary color (grey), for example `Reply r`. A hint that names one key (one key name or one arrow) SHALL be a button: a chip with a dim background. A click on it SHALL send its key to the focused list, form or dialog, with the same result as the key. A hint with two keys or a word (`Move ↑↓`, `Read level ←→`, `Type`) SHALL be text. The footer SHALL show only the hints that fit the width of the window, in their order, and SHALL NOT cut a hint. When it drops a hint, it SHALL show the button `More…` at the end of the hints. `More…` SHALL open the pick dialog "Actions" with each button hint of the screen, the label and the key; choosing a row SHALL close the pick dialog and run the action on the screen under it. A click on a row of a pick dialog SHALL choose that row. At 80 columns the footer SHALL show `ctrl+r` in the Spaces area and `enter` and `a` in the panel of a space. The key help (`?`) SHALL be a dialog as wide as its longest line, or the window when the window is narrower. It SHALL NOT wrap a line that fits, and it SHALL scroll when the window is too short.

#### Scenario: Spaces footer at 80 columns
- **WHEN** the window is 80 columns wide and a space is selected in the Spaces area
- **THEN** the footer shows `Run now ctrl+r`, and each hint is complete

#### Scenario: Label, then key
- **WHEN** the Feed shows a card
- **THEN** the footer shows `Reply r`: "Reply" in the primary color and `r` in the secondary color

#### Scenario: Click a footer button
- **WHEN** a space is selected in the Spaces area and the user clicks `Add conversations a` in the footer
- **THEN** the add dialog of the space opens, the same as the key `a`

#### Scenario: Click a footer button of a dialog
- **WHEN** the confirmation to delete a space is open and the user clicks `Cancel esc` in the footer
- **THEN** the confirmation closes and the space stays

#### Scenario: More actions
- **WHEN** the Feed shows a card at 130 columns and the user clicks `More…`, then the row "Sort"
- **THEN** the Feed changes its sort, the same as the key `s`

#### Scenario: Key help at 140 columns
- **WHEN** the window is 140 × 34 and the user presses `?`
- **THEN** each line of the key help shows as one line, and the line of the Feed keys starts with "Feed"

### Requirement: Spaces area
The Spaces area SHALL list at the left each space, then Everything else (the Unsorted space), New conversations with their count, Ignored with its count, and "+ New space". A space SHALL show `●` when it sends and `○` when it is paused, the number of its conversations and the number of waiting messages. The panel at the right SHALL be one list. For a space, its first rows SHALL be Name, Status, Agent, Session and When, and a line SHALL show the last send. The row `+ Add conversations` SHALL follow under the heading of the conversations; `enter` or a click on it SHALL open the add dialog of the space, the same as `a`. Its conversation rows SHALL follow, each with the kind mark, the name, the account, the read level ("All messages" or "Only for me"), the other spaces of the conversation ("also in …"), and the time of the last message. The recent sends of the space SHALL follow. `enter` or `→` in the list at the left SHALL move the keyboard to the panel, and `esc` or `←` on a settings row SHALL move it back. In the panel, `enter` on Status SHALL pause or resume the space and save at once; `enter` on Agent SHALL show the agent form of the space in the panel (driver, the Orca warning, CLI, folder, session, model, payload, inline limit, message limit, instruction), which saves with "Save changes" or `ctrl+s` and closes with `esc`; `enter` or `s` on Session SHALL open the sessions dialog of the space; `enter` on When SHALL open the When dialog; `enter` on Name SHALL open the rename dialog. On a conversation row, `←` and `→` SHALL change the read level of the link; `x` SHALL remove the conversation from the space; `enter` SHALL open the spaces dialog; `space` SHALL select the row; `n` SHALL rename the conversation. `x` on a space in the list at the left SHALL delete the space, and its conversations that no other space lists SHALL go to Everything else. `ctrl+r` SHALL run the selected space. Each change SHALL write the configuration file and SHALL show "Saved" with the change in the status line. A new space SHALL get a copy of the agent of the selected space, or of Unsorted, with no instruction.

#### Scenario: Add conversations with the mouse
- **WHEN** the space Review is selected and the user clicks the row `+ Add conversations`
- **THEN** the dialog "Add conversations to Review" opens

#### Scenario: Change the read level of a link
- **WHEN** `# release` is in the space Review with "All messages", and the user selects its row in Review and presses `→`
- **THEN** the file has `read = "mentions"` for `# release` in Review only, and the row shows "Only for me"

#### Scenario: Remove the last space
- **WHEN** `# release` is only in Review, and the user presses `x` on its row in Review
- **THEN** the file lists `# release` in no space, and the row shows in Everything else

#### Scenario: Pause a space
- **WHEN** the user presses `enter` on the row Status of Review
- **THEN** the file has `paused = true` for Review, the row says "Paused", and the list at the left shows `○` for Review

#### Scenario: Edit the agent of a space
- **WHEN** the user presses `enter` on the row Agent of Review, sets the folder to `~/review` and the instruction to "Review the pull requests", and presses `ctrl+s`
- **THEN** the file has `folder = "~/review"` and `instruction = "Review the pull requests"` for Review only, and the panel shows the rows again

#### Scenario: Session row
- **WHEN** the space Review has the stored terminal `term_a` and the Claude session `04ebe19c-…`
- **THEN** the row Session shows `term_a` and `04ebe19c`, and `enter` opens the sessions dialog of Review

### Requirement: Spaces dialog of a conversation
The dialog SHALL list each space with a check mark and the read level of the conversation in that space, then "Ignore this conversation" and "+ New space". `space` or a click SHALL check or uncheck a row; `←` and `→` SHALL change the level of a checked space; `enter` SHALL save and close; `esc` SHALL close with no change. With no space checked and no Ignore, the conversation SHALL be in Everything else. Ignore SHALL uncheck all spaces. For a selection of many conversations, the dialog SHALL start with the spaces of the first one and SHALL save the same spaces for all of them. When the user checks a space that the conversation was not in, the dialog SHALL show the option "Also send the messages of the last <lookback> to the new spaces", off by default.

#### Scenario: A conversation in two spaces
- **WHEN** `# release` is in Review, and the user opens its dialog, checks Team, sets "Only for me" for Team and presses enter
- **THEN** the file lists `# release` in Review with `all` and in Team with `mentions`

#### Scenario: Check a space with the mouse
- **WHEN** the spaces dialog of `# release` is open and the user clicks the row Team, then presses enter
- **THEN** the file lists `# release` in Team

#### Scenario: Move
- **WHEN** `# release` is in Review, and the user unchecks Review, checks Team and presses enter
- **THEN** the file lists `# release` only in Team

#### Scenario: Send the old messages
- **WHEN** a message of `# release` from today was sent to triage, and the user adds Review with the option on
- **THEN** the next send of Review contains the message, and triage does not get it again

### Requirement: Search of conversations
`/` SHALL open a search of the conversations of all accounts from each area, except in a form. The rows SHALL show the kind mark, the name, the account and the spaces, with the newest activity first. Typing SHALL filter the rows by the name, the label, the account or a space; the search SHALL ignore case. `enter` SHALL open the spaces dialog of the row. A click on a row SHALL select it, and a click on the selected row SHALL open its spaces dialog. `a` in the Spaces area SHALL open the add dialog of the selected space, with the title "Add conversations to <space>": each row SHALL show a check mark when the space has it, `space` or a click on the row SHALL add or remove it, and `←`/`→` SHALL change its level in that space. The add dialog SHALL keep these changes until the user selects OK, mark each changed row with `*`, and show the number of changes that are not saved. It SHALL have the buttons OK and Cancel under the list. `tab` SHALL move from the list to OK, then to Cancel; `enter` in the list and `ctrl+s` SHALL run OK; `esc` SHALL run Cancel; a mouse click on a button SHALL run it. OK SHALL write all changes in one save and close the dialog. Cancel SHALL close the dialog with no change to the file.

#### Scenario: Find and add
- **WHEN** the user presses `a` in Review, types `rel`, moves to `# release`, presses `space` and selects OK
- **THEN** the file lists `# release` in Review with `all`

#### Scenario: No save before OK
- **WHEN** the user presses `a` in Review, marks `# release` with `space`, and presses `esc`
- **THEN** the file does not list `# release` in Review

#### Scenario: Add with the mouse only
- **WHEN** the user clicks `+ Add conversations` in Review, types `rel`, clicks the row `# release` and clicks OK
- **THEN** the file lists `# release` in Review with `all`

#### Scenario: Click on OK
- **WHEN** the user marks a row in the add dialog and clicks OK
- **THEN** the file has the change and the dialog closes
