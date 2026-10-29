## MODIFIED Requirements

### Requirement: Desktop notification
On macOS, the daemon and `i2a run --once` SHALL show a desktop notification after each failed dispatch with a driver other than `dry-run`, with the profile, the message count and the error text. They SHALL NOT show a notification for a sent batch; the cards of the agent give the notifications (see the capability `card-feed`). The system SHALL pass the title and the text to `osascript` as arguments, not as script code. `[notify] enabled = false` SHALL turn all notifications off. The default SHALL be `true`. `[notify] min_urgency` SHALL be `high`, `normal` or `low`; the default SHALL be `normal`. The configuration validation SHALL reject other values.

#### Scenario: Batch sent
- **WHEN** an `orca` batch of 12 messages becomes `sent`
- **THEN** no notification shows

#### Scenario: Dispatch failed
- **WHEN** an `orca` batch fails because Orca is not reachable
- **THEN** macOS shows a notification with the title "i2a: dispatch failed", the profile name and the error text

#### Scenario: Notifications off
- **WHEN** `[notify] enabled = false` and a dispatch fails
- **THEN** no notification shows

#### Scenario: Dry run
- **WHEN** a `dry-run` batch is written
- **THEN** no notification shows

#### Scenario: Wrong minimum urgency
- **WHEN** `[notify] min_urgency = "urgent"`
- **THEN** the configuration validation reports that `notify.min_urgency` is not high, normal or low
