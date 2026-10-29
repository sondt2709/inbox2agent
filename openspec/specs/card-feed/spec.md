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
The system SHALL append a fixed card section to the primer of each batch for a driver other than `dry-run`. The section SHALL name the absolute path of the `i2a` executable (with `I2A_HOME=<path>` before it when the variable is set), show the JSON shape of a card, explain each field, and tell the agent to use the `id` values of the messages in `refs` and the same `key` for the same topic. A custom primer template SHALL NOT remove the section.

#### Scenario: Custom primer
- **WHEN** a profile has a custom primer in Vietnamese
- **THEN** the primer that the agent receives is the custom primer, followed by the card section with the path of the `i2a` executable

### Requirement: Feed tab
The dashboard SHALL have a Feed tab as its first tab. The Feed SHALL show the `new` and `read` cards in a list at the left and the selected card at the right: the category, the urgency, the group, the title, the body, the suggested reply, and the references with author, time and text of each message, or "message deleted". A `new` card SHALL have an unread mark. The Feed SHALL read the store again every 2 seconds. When no store exists, it SHALL show "No cards yet".

#### Scenario: New card arrives while the Feed is open
- **WHEN** the Feed is open and an agent adds a card
- **THEN** the card shows in the list within 3 seconds, with the unread mark

#### Scenario: Card with deleted messages
- **WHEN** retention deleted the messages that a card refers to
- **THEN** the detail panel shows "message deleted" for each reference

### Requirement: Feed sort and filter
The key `s` SHALL switch the sort between `urgency` (high first, then the newest update) and `newest` (the newest update first). The key `f` SHALL change the filter in the order all, action, question, info. The header SHALL show the number of `new` cards, of actions and of questions. The key `a` SHALL show or hide the `done` cards.

#### Scenario: Sort by urgency
- **WHEN** the Feed has a `low` info card from 17:57 and a `high` action card from 17:55, and the sort is `urgency`
- **THEN** the action card is first

#### Scenario: Filter questions
- **WHEN** the user presses `f` two times
- **THEN** the list shows only `question` cards and the filter bar marks "Question"

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

