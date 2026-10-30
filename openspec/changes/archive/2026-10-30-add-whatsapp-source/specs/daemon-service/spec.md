## ADDED Requirements

### Requirement: Source connections
The daemon SHALL close the connection of each source when it loads a new configuration and when it stops. `i2a run --once` SHALL close the connections before it exits.

#### Scenario: Configuration reload
- **WHEN** the daemon holds the connection of a WhatsApp source and the user saves the configuration
- **THEN** the daemon closes the connection and opens it again on the next poll

#### Scenario: Daemon stops
- **WHEN** the daemon holds the connection of a WhatsApp source and stops
- **THEN** `<data>/whatsapp-<id>.lock` is not locked any more

## MODIFIED Requirements

### Requirement: Desktop notification
On macOS, the daemon and `i2a run --once` SHALL show a desktop notification after each failed dispatch with a driver other than `dry-run`, with the profile, the message count and the error text. They SHALL NOT show a notification for a sent batch; the cards of the agent give the notifications (see the capability `card-feed`). The daemon SHALL show one notification when a WhatsApp source becomes not paired, and again only after a successful poll of that source. The system SHALL pass the title and the text to `osascript` as arguments, not as script code. `[notify] enabled = false` SHALL turn all notifications off. The default SHALL be `true`. `[notify] min_urgency` SHALL be `high`, `normal` or `low`; the default SHALL be `normal`. The configuration validation SHALL reject other values.

#### Scenario: Batch sent
- **WHEN** an `orca` batch of 12 messages becomes `sent`
- **THEN** no notification shows

#### Scenario: Dispatch failed
- **WHEN** an `orca` batch fails because Orca is not reachable
- **THEN** macOS shows a notification with the title "i2a: dispatch failed", the profile name and the error text

#### Scenario: Notifications off
- **WHEN** `[notify] enabled = false` and a dispatch fails
- **THEN** no notification shows

#### Scenario: WhatsApp not paired
- **WHEN** the phone unlinks i2a and the daemon polls the source 5 times
- **THEN** macOS shows one notification with the title "i2a: WhatsApp not paired" and the source ID
