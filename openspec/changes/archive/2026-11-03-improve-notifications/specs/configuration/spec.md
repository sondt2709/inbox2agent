## ADDED Requirements

### Requirement: Notifier settings
`[notify] via` SHALL be `auto`, `orca`, `terminal-notifier` or `osascript`; the default SHALL be `auto`. `[notify] terminal` SHALL be `iterm2` or `off`; the default SHALL be `iterm2`. The configuration validation SHALL reject another value of `via` or `terminal`. The schema of the configuration file SHALL describe both fields.

#### Scenario: Invalid notifier
- **WHEN** `config.toml` has `[notify] via = "growl"`
- **THEN** `i2a config check` reports that `notify.via` is not auto, orca, terminal-notifier or osascript

#### Scenario: Old file
- **WHEN** `config.toml` has a `[notify]` table with only `enabled` and `min_urgency`
- **THEN** the system loads it with `via = "auto"` and `terminal = "iterm2"`

## MODIFIED Requirements

### Requirement: Settings tab
The Settings tab SHALL show `lookback`, `retention`, the notifications, the lowest card urgency for a notification, the notifier (`via`), the Feed notifications in iTerm2 (`terminal`), the line of `i2a notify status` for the notifier that i2a uses now with the hint "run i2a notify setup", and the background service with the keys `i` (install or restart) and `u` (uninstall).

#### Scenario: Install the service from Settings
- **WHEN** the service is not installed and the user presses `i` in the Settings tab
- **THEN** the LaunchAgent is installed, and the Settings tab shows the service as running

#### Scenario: Notifier line
- **WHEN** `via = "auto"`, Orca does not run, and terminal-notifier 3.1.0 is installed
- **THEN** the Settings tab shows that i2a uses terminal-notifier 3.1.0
