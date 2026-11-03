## MODIFIED Requirements

### Requirement: Desktop notification
On macOS, the daemon and `i2a run --once` SHALL show a desktop notification after each failed dispatch with a driver other than `dry-run`, with the space, the agent, the message count and the error text. They SHALL NOT show a notification for a sent batch; the cards of the agent give the notifications (see the capability `card-feed`). The daemon SHALL show one notification when a WhatsApp account becomes not paired, and again only after a successful read of that account. The system SHALL show these notifications through the notifier chain (see the requirement "Notifier chain" of the capability `desktop-notifications`). `[notify] enabled = false` SHALL turn all notifications off. The default SHALL be `true`. `[notify] min_urgency` SHALL be `high`, `normal` or `low`; the default SHALL be `normal`. The configuration validation SHALL reject other values.

#### Scenario: Batch sent
- **WHEN** an `orca` batch of 12 messages becomes `sent`
- **THEN** no notification shows

#### Scenario: Dispatch failed
- **WHEN** an `orca` batch fails because Orca is not reachable
- **THEN** a notification shows with the title "i2a: dispatch failed", the space name and the error text

#### Scenario: Notifications off
- **WHEN** `[notify] enabled = false` and a dispatch fails
- **THEN** no notification shows

#### Scenario: WhatsApp not paired
- **WHEN** the phone unlinks i2a and the daemon polls the source 5 times
- **THEN** one notification shows with the title "i2a: WhatsApp not paired" and the source ID

#### Scenario: Daemon as a LaunchAgent with terminal-notifier
- **WHEN** the daemon runs as the LaunchAgent, terminal-notifier is in `/opt/homebrew/bin`, Orca does not run, and a dispatch fails
- **THEN** terminal-notifier shows the notification
