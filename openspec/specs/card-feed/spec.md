# card-feed Specification

## Purpose
This capability is the return path from the agent to the user. The agent reports its results as cards with `i2a card add`. i2a stores the cards, shows notifications by urgency, and shows the cards in the Feed tab, where the user copies a reply, opens the source thread and closes each card.
## Requirements
### Requirement: Add cards from an agent
The system SHALL provide `i2a card add`. It SHALL read a JSON array of card objects, or one card object, from stdin. A card object SHALL have the fields `category` (`action`, `question` or `info`, required), `urgency` (`high`, `normal` or `low`, default `normal`), `title` (required), `group`, `body`, `reply`, `refs` (an array of message IDs) and `key`. The command SHALL reject unknown fields. When one card is not valid, the command SHALL save no card, print the card number and the error on stderr, and exit with code 1. The command SHALL save all cards of one call in one transaction. On success, it SHALL print one line with the number of added, updated and unchanged cards.

#### Scenario: Three cards in one call
- **WHEN** an agent runs `i2a card add` with an array of one `action`, one `question` and one `info` card
- **THEN** the store has 3 new cards with the state `new`, and the output says "3 cards saved: 3 added"

#### Scenario: Unknown category
- **WHEN** the input has two cards and the second has the category `todo`
- **THEN** the command exits with code 1, stderr names card 2 and the category, and the store has no new card

#### Scenario: Misspelled field
- **WHEN** a card has the field `titel`
- **THEN** the command exits with code 1 and the error names the field `titel`

### Requirement: Card text limits
The system SHALL remove hidden characters from all card text with the rule of message text. It SHALL replace newlines in `title`, `group` and `key` with spaces. It SHALL cut `title` to 200, `group` to 100, `key` to 200, `body` to 4,000 and `reply` to 2,000 characters, and print a warning on stderr for each cut. One call SHALL accept at most 50 cards, 50 references for each card, and 1 MiB of input. The command SHALL drop a reference to a message ID that is not in the store and print a warning.

#### Scenario: Long body
- **WHEN** a card has a body of 6,000 characters
- **THEN** the stored body has 4,000 characters, stderr has a warning, and the exit code is 0

#### Scenario: Escape sequence in a title
- **WHEN** a card title contains `ESC [ 2 J` and U+202E
- **THEN** the stored title contains no ESC character and no U+202E

#### Scenario: Unknown reference
- **WHEN** a card has `refs` `[812, 99999]` and the store has no message 99999
- **THEN** the stored card refers to 812 only, and stderr has a warning about 99999

### Requirement: Update a card by key
A card with a `key` that the store already has SHALL update that card: all fields, the update time, and the state `new`. When all fields are the same as the stored card, the card SHALL be `unchanged` and keep its state. A card without a key SHALL always be a new card.

#### Scenario: Follow-up in the same thread
- **WHEN** the store has a `read` card with the key `mm:pr-57` and the urgency `normal`, and an agent adds a card with the key `mm:pr-57` and the urgency `high`
- **THEN** the store has one card with that key, with the urgency `high` and the state `new`

#### Scenario: Same card again
- **WHEN** an agent adds a card with a known key and the same fields as the stored `read` card
- **THEN** the card stays `read`, and the output counts it as unchanged

### Requirement: Close and list cards
The system SHALL provide `i2a card done --key <key>` and `i2a card done --id <id>`, which set the state `done`. When no card matches, the command SHALL exit with code 1. The system SHALL provide `i2a card list`, which prints the `new` and `read` cards, and with `--all` also the `done` cards. With `--json`, it SHALL print one JSON array with the fields `id`, `key`, `category`, `urgency`, `group`, `title`, `body`, `reply`, `refs`, `state`, `created_at` and `updated_at`.

#### Scenario: Agent closes a card
- **WHEN** an agent runs `i2a card done --key mm:lena-demo-login`
- **THEN** that card has the state `done`, and `i2a card list --json` does not contain it

#### Scenario: Unknown key
- **WHEN** a user runs `i2a card done --key missing`
- **THEN** the command exits with code 1 and says that no card has that key

### Requirement: Card notifications
On macOS, `i2a card add` SHALL show desktop notifications for the cards of the call. A card SHALL count when it was added, or updated to a higher urgency, or updated from the state `done`. Cards with an urgency lower than `[notify] min_urgency` SHALL NOT count. Each counted `high` card SHALL get one notification, for at most 3 cards in one call. All other counted cards SHALL get one grouped notification with the title `i2a: <n> new cards` and the counts by category. When the group has one card, that card SHALL get its own notification instead. A single notification SHALL have the title `i2a · <CATEGORY> · <urgency>` and the text `<group>: <title>`, cut to 200 characters. `[notify] enabled = false` SHALL turn the card notifications off.

#### Scenario: One high and two normal cards
- **WHEN** a call adds one `high` action and two `normal` questions, and `min_urgency` is `normal`
- **THEN** macOS shows 2 notifications: "i2a · ACTION · high" with the action title, and "i2a: 2 new cards" with "2 question"

#### Scenario: Low card
- **WHEN** a call adds one `low` info card, and `min_urgency` is `normal`
- **THEN** no notification shows, and the card is in the Feed

#### Scenario: Many urgent cards
- **WHEN** a call adds 10 `high` cards
- **THEN** macOS shows 3 single notifications and one grouped notification for the other 7 cards

#### Scenario: Unchanged card
- **WHEN** a call contains only cards that are unchanged
- **THEN** no notification shows

### Requirement: Card section in the primer
The system SHALL append a fixed card section to the primer of each batch for a driver other than `dry-run`. The section SHALL name the absolute path of the `i2a` executable (with `I2A_HOME=<path>` before it when the variable is set), show the JSON shape of a card, explain each field, and tell the agent to use the `id` values of the messages in `refs` and the same `key` for the same topic. The section SHALL say that the body can use Markdown (headings, lists, bold, code, tables) and that links show as text and cannot be opened. A custom primer template SHALL NOT remove the section.

#### Scenario: Custom primer
- **WHEN** a custom primer in Vietnamese is set
- **THEN** the primer that the agent receives is the custom primer, followed by the card section with the path of the `i2a` executable

#### Scenario: Markdown line
- **WHEN** the system builds the primer for an `orca` driver
- **THEN** the card section contains "You can use Markdown"

### Requirement: Feed tab
The dashboard SHALL have a Feed tab as its first tab. The Feed SHALL show the `new` and `read` cards in a list at the left and the selected card at the right: the space (with its color), the category, the urgency, the group, the title, the body, the suggested reply, and the references with account, conversation, author, time and text of each message, or "message deleted". The list SHALL show the urgency as bars: `▰▰▰` for `high` in red, `▰▰▱` for `normal`, and `▰▱▱` for `low` in the faint color. The detail panel SHALL show the bars and the word of the urgency. The space of a card SHALL be the space of its newest ref message that has one. A `new` card SHALL have an unread mark. The Feed SHALL read the store again every 2 seconds. When no store exists, it SHALL show "No cards yet".

#### Scenario: New card arrives while the Feed is open
- **WHEN** the Feed is open and an agent adds a card
- **THEN** the card shows in the list within 3 seconds, with the unread mark and its space

#### Scenario: Card with deleted messages
- **WHEN** retention deleted the messages that a card refers to
- **THEN** the detail panel shows "message deleted" for each reference, and the space shows as "unknown"

#### Scenario: Urgency bars
- **WHEN** the Feed has a `high` card and a `low` card
- **THEN** the row of the high card shows `▰▰▰`, the row of the low card shows `▰▱▱`, and the detail panel of the high card shows `▰▰▰ high`

### Requirement: Feed sort and filter
The key `s` SHALL switch the sort between `urgency` (high first, then the newest update) and `newest` (the newest update first). The key `f` SHALL change the category filter in the order all, action, question, info. The key `F` SHALL open a dialog with "All spaces" and each space in the order of the configuration, Unsorted first, each with the number of its cards that the other filters show. Enter SHALL set the space filter to the selected row. The filter SHALL keep the ID of the space; when that space does not exist any more, the filter SHALL be all spaces. The filter bar SHALL show the name of the space of the filter. The header SHALL show the number of `new` cards, of actions and of questions. The key `a` SHALL show or hide the `done` cards.

#### Scenario: Sort by urgency
- **WHEN** the Feed has a `low` info card from 17:57 and a `high` action card from 17:55, and the sort is `urgency`
- **THEN** the action card is first

#### Scenario: Filter questions
- **WHEN** the user presses `f` two times
- **THEN** the list shows only `question` cards and the filter bar marks "Question"

#### Scenario: Filter a space
- **WHEN** the spaces are Acme and Personal, and the user presses `F`, moves to "Personal" and presses enter
- **THEN** the list shows only cards of Personal and the filter bar shows "space: Personal"

#### Scenario: Card counts in the space dialog
- **WHEN** Unsorted has 2 cards and Acme has 1 card, and the user presses `F`
- **THEN** the dialog shows "All spaces" with 3, "Unsorted" with 2 and "Acme" with 1

#### Scenario: The space of the filter is removed
- **WHEN** the filter is Acme and another process removes the space Acme from `config.toml`
- **THEN** after the reload the Feed shows the cards of all spaces

### Requirement: Act on a card
A card SHALL become `read` when the user moves the selection to it or presses `enter`, `c`, `o`, `r` or `t` on it. The key `c` SHALL copy the suggested reply to the clipboard (`pbcopy` on macOS, OSC 52 on other systems). The key `o` SHALL open the URL of the first reference message that still exists, and only when the URL starts with `http://` or `https://`. The system SHALL NOT open a URL from the card text. The key `r` SHALL open the reply form of the `reply-send` capability. The key `t` SHALL open the ask form of the `agent-ask` capability. The key `d` SHALL set the state `done` and remove the card from the list. The key `u` SHALL set the state `new`.

#### Scenario: Copy and open
- **WHEN** the user selects an action card that refers to message 812 and presses `c` and then `o`
- **THEN** the clipboard has the suggested reply, the browser opens the URL of message 812, and the card is `read`

#### Scenario: Link in the card body
- **WHEN** a card body contains `https://evil.example` and the card has no reference
- **THEN** `o` opens nothing and the status line says that the card has no message link

#### Scenario: Done
- **WHEN** the user presses `d` on a card
- **THEN** the card leaves the list, and `i2a card list --json` does not contain it

#### Scenario: Reply key
- **WHEN** the user presses `r` on a `new` card that refers to message 812
- **THEN** the card is `read` and the reply form shows the target message 812

#### Scenario: Ask key
- **WHEN** the user presses `t` on a `new` card
- **THEN** the card is `read` and the ask form shows the agent that gets the ask

### Requirement: Terminal of the card
`i2a card add` SHALL record the value of the environment variable `ORCA_TERMINAL_HANDLE` as the terminal of each card of the call, when the value matches `term_` followed by 1 to 64 letters, digits or hyphens. It SHALL ignore another value and print a warning on stderr. Card JSON SHALL NOT set the terminal. An update by key SHALL replace the terminal when the call has one. A call that changes only the terminal SHALL count the card as unchanged and keep its state.

#### Scenario: Agent in an Orca terminal
- **WHEN** an agent in the Orca terminal `term_b9f22da8-13d6-49bc-a1ab-53eba56c9578` runs `i2a card add`
- **THEN** each card of the call has that terminal

#### Scenario: Bad value
- **WHEN** `ORCA_TERMINAL_HANDLE` is `term_x; rm -rf /`
- **THEN** the cards have no terminal, and stderr has a warning

### Requirement: Card body in Markdown
The Feed SHALL render the body of the selected card as Markdown (CommonMark and GitHub tables), wrapped to the width of the detail panel. Links and images SHALL show as text; the output SHALL contain no terminal hyperlink sequence and no other OSC sequence from card text. Raw HTML SHALL NOT show. The title and the suggested reply SHALL stay plain text. When the render fails, the Feed SHALL show the plain body. The Feed SHALL NOT render a body again when the card, its update time and the panel width are the same.

#### Scenario: Formatted body
- **WHEN** a card body is `**Blocked**: QA needs sign-off\n\n- item one\n- item two`
- **THEN** the detail panel shows "Blocked" in bold, no `**` marks, and two list items with bullets

#### Scenario: Link in a body
- **WHEN** a card body contains `[click](https://evil.example/x)`
- **THEN** the detail panel shows the text and the URL, and the rendered text contains no `ESC ] 8` sequence

#### Scenario: Raw HTML
- **WHEN** a card body contains `<script>alert(1)</script>`
- **THEN** the detail panel does not show the tag or `alert(1)`

#### Scenario: Reply stays plain
- **WHEN** the suggested reply is `**yes**, ship it`
- **THEN** the reply panel shows `**yes**, ship it`, and `c` copies the same text

### Requirement: Space filter in Activity
The Activity tab SHALL use the space filter of the Feed. The key `F` in Activity SHALL open the same dialog, with the number of sends of each space. When the filter is a space, the Activity tab SHALL show only the sends of that space and SHALL show the name of the space in its title.

#### Scenario: Filter the sends of a space
- **WHEN** Activity has sends of Acme and of Unsorted, and the user selects Acme in the `F` dialog
- **THEN** the table shows only the sends of Acme and the title shows "space Acme"

#### Scenario: One filter for both tabs
- **WHEN** the user selects Acme with `F` in the Feed and then opens Activity
- **THEN** Activity shows only the sends of Acme

### Requirement: Read and send now from the dashboard
The key `R` in the Feed tab SHALL read all accounts and send the waiting messages of each space that has an agent. The key `R` in the Spaces tab SHALL read all accounts and send the selected space. When the daemon runs, the dashboard SHALL ask the daemon (`POST /v1/run`); when no daemon runs, the dashboard SHALL run the same work in its own process and SHALL hold the daemon lock while it runs. While a run is busy, a second `R` SHALL show that a run is busy and SHALL NOT start a run. The status line SHALL show the result: the number of new messages and each batch with its space and status. While the daemon runs, the status line SHALL show the `send`, `reload` and `error` events of the daemon.

#### Scenario: R in the Feed with the daemon
- **WHEN** the daemon runs, a new message waits, and the user presses `R` in the Feed
- **THEN** the daemon sends the batch, and the status line names the space and the status of the batch

#### Scenario: R in the Spaces tab without a daemon
- **WHEN** no daemon runs, the selected space is Acme with a waiting message, and the user presses `R` in the Spaces tab
- **THEN** the dashboard sends one batch of Acme, and the batches of other spaces stay pending

#### Scenario: Event of the daemon
- **WHEN** the dashboard is open and the daemon sends a batch at its schedule
- **THEN** the status line shows the space and the status of that batch within 3 seconds

### Requirement: Space of a card from its terminal
When `i2a card add` records the terminal handle of a card and a session of a space has that handle, the store SHALL record that space as the space of the card. An update of the card by its key SHALL keep the recorded space when the new handle has no session. The Feed, the `F` filter and the ask of a card SHALL use the recorded space first. A card with no recorded space SHALL get the space of its newest ref message that has one, as before.

#### Scenario: Card from the terminal of a space
- **WHEN** the channel `release` is in Review and in Team, and the agent in the terminal of the session of Team adds a card about a release message
- **THEN** the Feed shows the card in the space Team, and `t` on the card asks the agent of Team

#### Scenario: Card from an unknown terminal
- **WHEN** an agent adds a card from a terminal that no session has
- **THEN** the card gets the space of its newest ref message, as before
