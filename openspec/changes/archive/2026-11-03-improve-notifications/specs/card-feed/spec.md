## MODIFIED Requirements

### Requirement: Card notifications
On macOS, `i2a card add` SHALL show desktop notifications for the cards of the call, unless the Feed shows them (see the requirement "Feed notifications in iTerm2" of the capability `desktop-notifications`). A card SHALL count when it was added, or updated to a higher urgency, or updated from the state `done`. Cards with an urgency lower than `[notify] min_urgency` SHALL NOT count. Each counted `high` card SHALL get one notification, for at most 3 cards in one call. All other counted cards SHALL get one grouped notification with the title `i2a: <n> new cards` and the counts by category. When the group has one card, that card SHALL get its own notification instead. A single notification SHALL have the title `<CATEGORY> · <urgency>`, with ` · updated` for an updated card, the subtitle `<group> · <space>` with each part that is set, and the plain title of the card as the body (see the requirement "Plain title" of the capability `desktop-notifications`). The `osascript` notifier SHALL put `i2a · ` before the title. `[notify] enabled = false` SHALL turn the card notifications off.

#### Scenario: One high and two normal cards
- **WHEN** a call adds one `high` action and two `normal` questions, and `min_urgency` is `normal`
- **THEN** 2 notifications show: "ACTION · high" with the plain action title, and "i2a: 2 new cards" with "2 question"

#### Scenario: Low card
- **WHEN** a call adds one `low` info card, and `min_urgency` is `normal`
- **THEN** no notification shows, and the card is in the Feed

#### Scenario: Many urgent cards
- **WHEN** a call adds 10 `high` cards
- **THEN** 3 single notifications show and one grouped notification for the other 7 cards

#### Scenario: Unchanged card
- **WHEN** a call contains only cards that are unchanged
- **THEN** no notification shows

#### Scenario: Feed shows the notifications
- **WHEN** the Feed is open in iTerm2 and holds `<data>/feed-notify.lock`, and a call adds one `high` card
- **THEN** `i2a card add` shows no notification and prints "Notifications: the Feed shows them."

### Requirement: Feed tab
The dashboard SHALL have a Feed tab as its first tab. The Feed SHALL show the `new` and `read` cards in a list and the selected card at the right: the space (with its color), the category, the urgency, the group, the title, the body, the suggested reply, and the references with account, conversation, author, time and text of each message, or "message deleted". The list row and the Card panel SHALL show the plain title of the card (see the requirement "Plain title" of the capability `desktop-notifications`). Each card row SHALL start at the left edge of the list, with no bar and no mark column. Its first line SHALL be the category as plain text, the urgency as bars (`▰▰▰` for `high` in red, `▰▰▱` for `normal`, and `▰▱▱` for `low` in the faint color), `● ` for a `new` card, and the title up to the width of the list. Its second line SHALL be the space, the group in bold, and the time. The detail panel SHALL show the bars and the word of the urgency. The space of a card SHALL be the space of its newest ref message that has one. The Feed SHALL read the store again every 2 seconds. When no store exists, it SHALL show "No cards yet".

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

#### Scenario: Markdown title in the row and the panel
- **WHEN** a card has the title "**Check** the `notes`"
- **THEN** its row ends with `● Check the notes`, and the Card panel shows "Check the notes" with no `*` and no backtick
