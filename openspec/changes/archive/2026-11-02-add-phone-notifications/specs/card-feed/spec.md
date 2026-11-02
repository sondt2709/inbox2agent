## MODIFIED Requirements

### Requirement: Card notifications
On macOS, `i2a card add` SHALL show desktop notifications for the cards of the call. A card SHALL count when it was added, or updated to a higher urgency, or updated from the state `done`. Cards with an urgency lower than `[notify] min_urgency` SHALL NOT count. Each counted `high` card SHALL get one notification, for at most 3 cards in one call. All other counted cards SHALL get one grouped notification with the title `i2a: <n> new cards` and the counts by category. When the group has one card, that card SHALL get its own notification instead. A single notification SHALL have the title `i2a · <CATEGORY> · <urgency>` and the text `<group>: <title>`, cut to 200 characters. `[notify] enabled = false` SHALL turn the card notifications off. After the desktop notifications, when a phone pairing exists, `i2a card add` SHALL send the added and updated cards to the phone as the requirement "Push from card add" of `phone-push` describes; the counted cards SHALL be the phone alerts.

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

#### Scenario: Paired phone
- **WHEN** a phone pairing exists and a call adds one `high` action
- **THEN** macOS shows the notification and FCM gets one sealed message with `alert` true
