## MODIFIED Requirements

### Requirement: Feed tab
The dashboard SHALL have a Feed tab as its first tab. The Feed SHALL show the `new` and `read` cards in a list at the left and the selected card at the right: the space (with its color), the category, the urgency, the group, the title, the body, the suggested reply, and the references with account, conversation, author, time and text of each message, or "message deleted". The space of a card SHALL be the space of its newest ref message that has one. A `new` card SHALL have an unread mark. The Feed SHALL read the store again every 2 seconds. When no store exists, it SHALL show "No cards yet".

#### Scenario: New card arrives while the Feed is open
- **WHEN** the Feed is open and an agent adds a card
- **THEN** the card shows in the list within 3 seconds, with the unread mark and its space

#### Scenario: Card with deleted messages
- **WHEN** retention deleted the messages that a card refers to
- **THEN** the detail panel shows "message deleted" for each reference, and the space shows as "unknown"

### Requirement: Feed sort and filter
The key `s` SHALL switch the sort between `urgency` (high first, then the newest update) and `newest` (the newest update first). The key `f` SHALL change the category filter in the order all, action, question, info. The key `F` SHALL change the space filter in the order all, then each space in the order of the configuration. The header SHALL show the number of `new` cards, of actions and of questions. The key `a` SHALL show or hide the `done` cards.

#### Scenario: Sort by urgency
- **WHEN** the Feed has a `low` info card from 17:57 and a `high` action card from 17:55, and the sort is `urgency`
- **THEN** the action card is first

#### Scenario: Filter questions
- **WHEN** the user presses `f` two times
- **THEN** the list shows only `question` cards and the filter bar marks "Question"

#### Scenario: Filter a space
- **WHEN** the spaces are Acme and Personal, and the user presses `F` two times
- **THEN** the list shows only cards of Personal and the filter bar marks "Personal"
