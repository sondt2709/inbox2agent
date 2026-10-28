# configuration Specification

## Purpose
This capability keeps the settings of i2a: the configuration file, the secrets, the validation, the dashboard (`i2a config`) and the non-interactive source commands.

## Requirements
### Requirement: Configuration file location
The system SHALL read the configuration from `$XDG_CONFIG_HOME/i2a/config.toml`, or from `~/.config/i2a/config.toml` when `XDG_CONFIG_HOME` is not set. The system SHALL keep data in `$XDG_DATA_HOME/i2a/`, or in `~/.local/share/i2a/`. When `I2A_HOME` is set, the system SHALL use that directory for both the configuration and the data.

#### Scenario: I2A_HOME overrides the paths
- **WHEN** the user runs `i2a status` with `I2A_HOME=/tmp/x`
- **THEN** the output shows `/tmp/x/config.toml` as the configuration path and `/tmp/x` as the data path

#### Scenario: No configuration file
- **WHEN** the user runs `i2a run --once` and no configuration file exists
- **THEN** the command exits with a non-zero code and tells the user to run `i2a config`

### Requirement: Secrets are not in the configuration file
The system SHALL store each source token in the secret backend with the key `<source-id>/token`. The system SHALL NOT write a token to the configuration file or to the log. The default backend SHALL be the operating-system keychain. When `I2A_SECRET_BACKEND=file` is set, the system SHALL use `<data>/secrets.json` with file mode 0600.

#### Scenario: Token saved in the secret backend
- **WHEN** the user adds a Mattermost source with a valid token
- **THEN** the configuration file contains the source URL and no token
- **AND** the secret backend contains the token under `<source-id>/token`

#### Scenario: File backend permissions
- **WHEN** the file backend saves a secret
- **THEN** `secrets.json` has the file mode 0600

### Requirement: Configuration validation
The system SHALL validate the configuration when it loads it and when the TUI saves it. Durations SHALL use Go duration syntax. `active_hours` SHALL be empty or `HH:MM-HH:MM`. `inline_max_bytes` SHALL be between 1,024 and 65,536. Each source and each `channel_profiles` value SHALL name a profile that exists. Source IDs and profile names SHALL be unique.

#### Scenario: Invalid duration
- **WHEN** the configuration has `interval = "ten minutes"`
- **THEN** the system reports the field name and the invalid value and does not start

#### Scenario: Unknown profile
- **WHEN** a source has `profile = "missing"` and no profile has that name
- **THEN** the system reports that the profile `missing` does not exist

### Requirement: Configuration TUI
The system SHALL provide `i2a config`, a full-screen dashboard in the alternate screen of the terminal, with the tabs Overview, Sources, Profiles, Schedule and Activity. The keys `1` to `5`, `tab` and `shift+tab`, and a mouse click on a tab SHALL change the tab. The Sources and Profiles tabs SHALL have a list at the left and a detail panel at the right. The detail panel SHALL be the form of the selected item, or the form of a new item when "+ Add" is selected; the TUI SHALL NOT use separate screens or dialogs to add or edit. A form SHALL show all its fields at one time, with the help of the focused field, an error under each field that is not valid, and buttons. Submitting a form (`ctrl+s`, the primary button, or `enter` in the last field) SHALL write the configuration file. `esc` SHALL give the keyboard back to the list and SHALL ask before it discards changes. The footer SHALL show the keys of the current focus. `q` SHALL quit; `?` SHALL show all keys.

#### Scenario: Dashboard
- **WHEN** the user runs `i2a config` in a window of 120 × 40 with one source
- **THEN** the TUI fills the window, the tab bar shows the 5 tabs, and the Overview tab shows the service state, the message counts and the source

#### Scenario: Edit in the detail panel
- **WHEN** the user selects the source `work` in the Sources tab
- **THEN** the detail panel shows the form of `work` with its values, and `enter` gives the keyboard to the form

#### Scenario: Add in the detail panel
- **WHEN** the user selects "+ Add source"
- **THEN** the detail panel shows the form of a new source with the fields Source ID, Server URL and Token and the button "Connect"

#### Scenario: Error under the field
- **WHEN** the user types `abc` as the interval and presses `ctrl+s`
- **THEN** the text "is not a duration" shows under the interval field, and the file does not change

#### Scenario: Leave a form with Esc
- **WHEN** the user types `45m` as the interval, presses `esc`, and then presses `d` (discard)
- **THEN** the form shows the interval `15m` again, and the file does not change

#### Scenario: Terminal restored
- **WHEN** the user exits the TUI
- **THEN** the terminal shows the shell screen from before `i2a config`, and the last line tells whether the file was saved

#### Scenario: Save timing
- **WHEN** the user sets the interval to `10m` and presses `ctrl+s`
- **THEN** the configuration file contains `interval = "10m"`, and the dashboard stays open

### Requirement: Source and channel picker
The system SHALL let the user add a Mattermost source in the TUI. The TUI SHALL check the URL and the token with the server before it saves them. After a successful check, the TUI SHALL show a list of the channels of all teams of the user and of the direct and group conversations of the user, with a search box that always has the focus. The list SHALL show the team channels first, then the conversations with the most recent post first. A conversation label SHALL start with `direct /` or `group /` and SHALL name the other people, not the user. The search SHALL ignore case. Space SHALL select the channel under the cursor and SHALL NOT type into the search. `Esc` SHALL clear the search and keep the selected channels. The selected channels SHALL show first when the list opens. The user SHALL be able to select channels, turn on "all direct and group messages", turn on "mentions in any channel", and select a profile.

#### Scenario: Valid token
- **WHEN** the user types a valid URL and token
- **THEN** the TUI shows "Connected as <username>" and then the channel list

#### Scenario: Invalid token
- **WHEN** the user types a token that the server rejects
- **THEN** the TUI shows the error from the server
- **AND** the system saves no secret and no source

#### Scenario: Search channels
- **WHEN** the channel list is open and the user types part of a channel name, with no other key first
- **THEN** the list shows only the channels whose names contain that text, and the search line shows the text and the number of matches

#### Scenario: Select one direct conversation
- **WHEN** the user searches the name of `bob`, selects `direct / @bob`, and answers No to "all direct and group messages" and to "mentions"
- **THEN** the source watches only that conversation, and a post by `bob` in a channel does not arrive

#### Scenario: Select during a search
- **WHEN** the user searches `sec`, presses space, presses `Esc`, searches `api` and presses space
- **THEN** both channels are selected, and the list shows all channels again after each `Esc`

### Requirement: Orca permission warning
When the user selects the `orca` driver for a profile, the TUI SHALL show a warning that Orca can start agents with permission prompts off, and that message text can contain instructions from other people.

#### Scenario: Warning shown
- **WHEN** the user sets the driver of a profile to `orca` in the TUI
- **THEN** the screen shows the permission warning before the user saves the profile

### Requirement: Non-interactive source commands
The system SHALL provide `i2a source add mattermost --id <id> --url <url> --token-stdin`, `i2a source list` and `i2a source channels <id>`. `source add` SHALL do the same connection check as the TUI.

#### Scenario: Add source from a script
- **WHEN** a script pipes a valid token to `i2a source add mattermost --id work --url <url> --token-stdin`
- **THEN** the command prints "Connected as <username>" and adds the source to the configuration file

#### Scenario: List channels
- **WHEN** the user runs `i2a source channels work`
- **THEN** the output lists each channel with its ID, team, type and name

