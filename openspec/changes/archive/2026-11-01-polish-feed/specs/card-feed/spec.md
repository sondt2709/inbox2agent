## MODIFIED Requirements

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

## ADDED Requirements

### Requirement: Space filter in Activity
The Activity tab SHALL use the space filter of the Feed. The key `F` in Activity SHALL open the same dialog, with the number of sends of each space. When the filter is a space, the Activity tab SHALL show only the sends of that space and SHALL show the name of the space in its title.

#### Scenario: Filter the sends of a space
- **WHEN** Activity has sends of Acme and of Unsorted, and the user selects Acme in the `F` dialog
- **THEN** the table shows only the sends of Acme and the title shows "space Acme"

#### Scenario: One filter for both tabs
- **WHEN** the user selects Acme with `F` in the Feed and then opens Activity
- **THEN** Activity shows only the sends of Acme
