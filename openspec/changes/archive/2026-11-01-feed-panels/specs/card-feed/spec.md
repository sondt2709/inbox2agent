## MODIFIED Requirements

### Requirement: Feed tab
The dashboard SHALL have a Feed tab as its first tab. The Feed SHALL show the `new` and `read` cards in a list and the selected card at the right: the space (with its color), the category, the urgency, the group, the title, the body, the suggested reply, and the references with account, conversation, author, time and text of each message, or "message deleted". Each card row SHALL start at the left edge of the list, with no bar and no mark column. Its first line SHALL be the category as plain text, the urgency as bars (`▰▰▰` for `high` in red, `▰▰▱` for `normal`, and `▰▱▱` for `low` in the faint color), `● ` for a `new` card, and the title up to the width of the list. Its second line SHALL be the space, the group in bold, and the time. The detail panel SHALL show the bars and the word of the urgency. The space of a card SHALL be the space of its newest ref message that has one. The Feed SHALL read the store again every 2 seconds. When no store exists, it SHALL show "No cards yet".

#### Scenario: New card arrives while the Feed is open
- **WHEN** the Feed is open and an agent adds a card
- **THEN** the card shows in the list within 3 seconds, with `●` before its title and its space

#### Scenario: Card with deleted messages
- **WHEN** retention deleted the messages that a card refers to
- **THEN** the detail panel shows "message deleted" for each reference, and the space shows as "unknown"

#### Scenario: Urgency bars
- **WHEN** the Feed has a `high` card and a `low` card
- **THEN** the row of the high card shows `▰▰▰`, the row of the low card shows `▰▱▱`, and the detail panel of the high card shows `▰▰▰ high`

#### Scenario: Card row
- **WHEN** a `new` action card of the space Review has the group `release` and the title "Check the notes"
- **THEN** its first line starts with `ACTION` in column 1 of the list and ends with `● Check the notes`, and its second line starts with `Review · release`

### Requirement: Feed sort and filter
The key `s` SHALL switch the sort between `urgency` (high first, then the newest update) and `newest` (the newest update first). When the window is 100 columns or wider, the Feed SHALL show at the left a list with "All" and each space, Everything else last, each with the number of its cards. When that list has the keyboard, `↑` and `↓` SHALL select the previous and the next row, and the Feed SHALL show the cards of the selected space. The key `F` SHALL open a dialog with the same rows. The filter SHALL keep the ID of the space; when that space does not exist any more, the filter SHALL be all spaces. The title of the card list SHALL show the number of cards and the sort. The key `a` SHALL show or hide the `done` cards. The Feed SHALL have no category filter.

#### Scenario: Sort by urgency
- **WHEN** the Feed has a `low` info card from 17:57 and a `high` action card from 17:55, and the sort is `urgency`
- **THEN** the action card is first

#### Scenario: Filter a space with the list
- **WHEN** the spaces are Acme and Personal, the window is 120 columns, the user presses `←` to give the keyboard to the space list, and presses `↓` two times
- **THEN** the list shows only cards of Personal, and the space list marks Personal

#### Scenario: Card counts in the space list
- **WHEN** Everything else has 2 cards and Acme has 1 card
- **THEN** the space list shows "All" with 3, "Acme" with 1 and "Everything else" with 2

#### Scenario: The space of the filter is removed
- **WHEN** the filter is Acme and another process removes the space Acme from `config.toml`
- **THEN** after the reload the Feed shows the cards of all spaces

### Requirement: Read and send now from the dashboard
The header SHALL always show `ctrl+r run now` and the next send of the spaces. The key `ctrl+r` SHALL read all accounts and send the waiting messages of each space that has an agent, except in the Spaces area, where it SHALL send the selected space. `R` SHALL NOT start a run. When the daemon runs, the dashboard SHALL ask the daemon (`POST /v1/run`); when no daemon runs, the dashboard SHALL run the same work in its own process and SHALL hold the daemon lock while it runs. While a run is busy, a second `ctrl+r` SHALL show that a run is busy and SHALL NOT start a run. The line under the header SHALL show the result of the last run until the next run: the time, the number of new messages and each batch with its space and status, or "nothing waits". While the daemon runs, the status line SHALL show the `send`, `reload` and `error` events of the daemon.

#### Scenario: Run in the Feed with the daemon
- **WHEN** the daemon runs, a new message waits, and the user presses `ctrl+r` in the Feed
- **THEN** the daemon sends the batch, and the line under the header names the space and the status of the batch

#### Scenario: Run in the Spaces area without a daemon
- **WHEN** no daemon runs, the selected space is Acme with a waiting message, and the user presses `ctrl+r` in the Spaces area
- **THEN** the dashboard sends one batch of Acme, and the batches of other spaces stay pending

#### Scenario: Event of the daemon
- **WHEN** the dashboard is open and the daemon sends a batch at its schedule
- **THEN** the status line shows the space and the status of that batch within 3 seconds

## ADDED Requirements

### Requirement: Panels of the Feed
The Feed SHALL have three panels: Spaces (the space list), Cards and Card. One panel SHALL have the keyboard, and it SHALL have the accent border; at the start it SHALL be Cards. `←` and `→` SHALL give the keyboard to the panel at the left or at the right; below 100 columns the Spaces panel is hidden and SHALL be skipped. `↑` and `↓` SHALL select a space in Spaces, select a card in Cards, and scroll the text in Card. `pgup` and `pgdn` SHALL scroll the text of the card from each panel. The text of a card SHALL show from its first line when another card is selected. The title of the Card panel SHALL show `↓ more` when text is below the visible lines. The mouse wheel and a two-finger scroll on a trackpad SHALL act on the panel under the pointer, as `↑` and `↓` do there, and SHALL give that panel the keyboard. In the Spaces area, the wheel SHALL move the cursor of the list or of the panel under the pointer.

#### Scenario: Scroll a long card
- **WHEN** the selected card has 80 lines of text in a window of 40 rows, and the user presses `→` and then `↓` five times
- **THEN** the Card panel has the accent border, its text starts at line 6, and its title shows `↓ more`

#### Scenario: Wheel on a panel
- **WHEN** the Cards panel has the keyboard and the user scrolls down with the pointer over the Card panel
- **THEN** the card text scrolls, the selected card stays the same, and the Card panel has the accent border

#### Scenario: Another card
- **WHEN** the card text is scrolled and the user selects the next card
- **THEN** the text of the next card shows from its first line
