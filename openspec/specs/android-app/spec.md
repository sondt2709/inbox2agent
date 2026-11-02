# android-app Specification

## Purpose
The Android app in `mobile/`: pairing, receiving sealed card messages, notifications, the read-only Feed and card screens, the brand colors and the mobile CI job.

## Requirements
### Requirement: Pair the phone
The app SHALL pair from a QR code (Google code scanner) or from a pasted link. It SHALL accept only `i2a://pair` links with `v=1`, all of `p`, `s`, `a`, `ak`, a topic that starts with `i2a-`, and a key of 32 bytes. The APK SHALL hold no Firebase project: the app SHALL store the Firebase options of the link and start Firebase with them in `Application.onCreate`. It SHALL store the key in the Android Keystore, subscribe to the topic, and show the name of the Mac. Unpair SHALL unsubscribe, delete the key and delete the cards.

#### Scenario: Valid link
- **WHEN** the user pastes a valid link of the same project
- **THEN** the app shows "Paired with <Mac name>" and the Feed

#### Scenario: Incomplete link
- **WHEN** the link has no `ak` field
- **THEN** the app shows an error that names the missing field and does not pair

### Requirement: Receive a sealed message
On each FCM data message, the app SHALL open `c` with the stored key. A message with an unknown `v` or `kind`, or one that fails to open, SHALL be dropped with no notification. For `kind` `card`, the app SHALL store the card by ID and keep the version with the highest `ts`. It SHALL keep at most 500 cards and drop cards older than 30 days.

#### Scenario: Card message
- **WHEN** a sealed card message arrives
- **THEN** the card is in the Feed

#### Scenario: Message for another key
- **WHEN** a message sealed with another key arrives
- **THEN** nothing changes and no notification shows

### Requirement: Notifications
For a message with `alert` true, the app SHALL post a notification in the channel of the urgency (`cards_high`, `cards_normal`, `cards_low`), with the card title, the group, the category and urgency as sub text, the color `#7D56F4`, the group `i2a.cards` and a summary. The notification SHALL have the action Copy reply when the card has a suggested reply, and Open thread when a source message has an `http` or `https` URL. A tap SHALL open the card. The lock screen SHALL show only "i2a · new card". A `test` message SHALL show "i2a test" with the time. On Android 13 and later, the app SHALL ask for the notification permission after pairing.

#### Scenario: Alert card
- **WHEN** a card message with `alert` true and urgency `high` arrives
- **THEN** a heads-up notification shows the card title

#### Scenario: Silent card
- **WHEN** a card message with `alert` false arrives
- **THEN** no notification shows and the card is in the Feed

### Requirement: Feed and card screens
The Feed SHALL list the cards that are not hidden, newest first, with the unread dot, the category badge, the urgency bars, the title, the group and the time. Filter chips SHALL filter by category and show counts. The card screen SHALL show the meta line, the title, the body, the source messages and the suggested reply, and the actions Copy reply, Open thread and Hide. Open thread SHALL open only a stored source URL; no text of a card SHALL become a link. Opening a card SHALL mark it read on the phone. Hide SHALL remove the card from the phone Feed only. A card with `cut` true SHALL show "The full text is on the Mac."

#### Scenario: Copy reply
- **WHEN** the user taps Copy reply on a card with a suggested reply
- **THEN** the clipboard has the reply and a snackbar shows "Copied the suggested reply."

#### Scenario: Link in the body
- **WHEN** the card body contains `https://example.com`
- **THEN** the text is not a link

### Requirement: Brand colors
The app SHALL use the tokens of `docs/design-tokens.json`, with the primary color `#7D56F4`, in a dark theme. It SHALL NOT use dynamic color. A Go test SHALL fail when `DesignTokens.kt` or the TUI colors differ from the tokens.

#### Scenario: Token drift
- **WHEN** `DesignTokens.kt` has a primary color that differs from the JSON file
- **THEN** `go test ./internal/tui -run TestDesignTokens` fails

### Requirement: Mobile CI
The CI workflow SHALL have a job `mobile` that runs the Kotlin unit tests and builds the debug APK in `mobile/` with JDK 17 and no secret, and uploads the APK as a workflow artifact. The release job SHALL need the job `mobile`.

#### Scenario: Kotlin test fails
- **WHEN** a Kotlin unit test fails on a pull request
- **THEN** the job `mobile` fails

#### Scenario: APK artifact
- **WHEN** the job `mobile` passes
- **THEN** the run has the artifact `i2a-debug-apk`

