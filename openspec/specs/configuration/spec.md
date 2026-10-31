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
The system SHALL store each Mattermost account token in the secret backend with the key `<account-id>/token`. The system SHALL NOT write a token to the configuration file or to the log. The default backend SHALL be the operating-system keychain. When `I2A_SECRET_BACKEND=file` is set, the system SHALL use `<data>/secrets.json` with file mode 0600.

#### Scenario: Token saved in the secret backend
- **WHEN** the user adds a Mattermost account with a valid token
- **THEN** the configuration file contains the account URL and no token
- **AND** the secret backend contains the token under `<account-id>/token`

#### Scenario: File backend permissions
- **WHEN** the file backend saves a secret
- **THEN** `secrets.json` has the file mode 0600

### Requirement: Orca permission warning
When the user selects the `orca` driver for an agent, the TUI SHALL show a warning that Orca can start agents with permission prompts off, and that message text can contain instructions from other people.

#### Scenario: Warning shown
- **WHEN** the user sets the driver of an agent to `orca` in the TUI
- **THEN** the screen shows the permission warning before the user saves the agent

### Requirement: Configuration validation of version 2
The system SHALL validate the configuration when it loads it and when the TUI or a command saves it. Durations SHALL use Go duration syntax. `hours` SHALL be empty or `HH:MM-HH:MM`. `days` SHALL contain only `mon` to `sun`. `inline_max_bytes` SHALL be between 1,024 and 65,536. IDs of accounts, agents, schedules and spaces SHALL be unique in their kind and SHALL be 1 to 32 lowercase letters, digits, `-` or `_`. The references SHALL follow the requirement "References between items" of the `spaces` capability. The system SHALL report all problems at one time, each with its field.

#### Scenario: Invalid duration
- **WHEN** a schedule has `every = "ten minutes"`
- **THEN** the system reports the field name and the invalid value and does not start

#### Scenario: Invalid day
- **WHEN** a schedule has `days = ["monday"]`
- **THEN** the system reports that `monday` is not one of `mon` to `sun`

### Requirement: Dashboard tabs
The system SHALL provide a full-screen dashboard in the alternate screen with the tabs Feed, Activity, Accounts, Conversations, Spaces, Agents, Schedules and Settings, in this order. The tab bar SHALL draw `→` between Accounts, Conversations, Spaces and Agents, `+` before Schedules, and `│` between the groups. A line under the tab bar SHALL define the item of the current tab in one sentence. `i2a` with no arguments SHALL open the Feed tab; `i2a config` SHALL open the Spaces tab. The keys `1` to `8`, `tab`, `shift+tab` and a mouse click SHALL change the tab. The Conversations tab SHALL show the number of unsorted conversations next to its name. Forms SHALL stay in the detail panel, with the help of the focused field, an error under each field that is not valid, and buttons, as before. `q` SHALL quit; `?` SHALL show the model of i2a and all keys.

#### Scenario: Tab bar
- **WHEN** the user runs `i2a config` in a window of 140 × 40 with 4 unsorted conversations
- **THEN** the tab bar shows `3 Accounts → 4 Conversations 4 unsorted → 5 Spaces → 6 Agents + 7 Schedules`, and the Spaces tab is open

#### Scenario: Feed first
- **WHEN** the user runs `i2a` with no arguments
- **THEN** the dashboard opens on the Feed tab

#### Scenario: Terminal restored
- **WHEN** the user exits the TUI
- **THEN** the terminal shows the shell screen from before the dashboard

### Requirement: Accounts tab
The Accounts tab SHALL list the accounts at the left with their name and platform. The detail panel SHALL show the name (editable), the ID, the platform, the information that identifies the account (Mattermost: server URL and username; WhatsApp: phone number and profile name), the connection state, the number of conversations found, and a bar with the number of conversations in each space. `read_every` SHALL be under "Advanced", with the help "This is not when the agent gets messages: that is the schedule of each space." "+ Add account" SHALL show the form of a new account: Mattermost (account ID, server URL, token, "Connect") or WhatsApp (account ID, phone number, "Connect", QR code or phone code). The TUI SHALL check a Mattermost URL and token with the server before it saves them. After a connection, the new conversations SHALL be in Unsorted.

#### Scenario: Valid token
- **WHEN** the user types a valid URL and token and selects "Connect"
- **THEN** the TUI shows "Connected as <username>", saves the account, and says that its conversations are in Unsorted

#### Scenario: Invalid token
- **WHEN** the user types a token that the server rejects
- **THEN** the TUI shows the error from the server, and the system saves no secret and no account

#### Scenario: Account information
- **WHEN** the user selects a paired WhatsApp account and a Mattermost account
- **THEN** the WhatsApp detail shows the phone number in international format and the profile name, and the Mattermost detail shows the server URL and the username

### Requirement: Conversations tab
The Conversations tab SHALL show one table of the conversations of all accounts: the conversations in the configuration and the conversations that each account lists. Each row SHALL show the kind mark (`@` direct, `⧉` group, `#` channel), the label or the name, the kind, the account, the space and the read level, and `new` for a listed conversation that the configuration does not have. The tab SHALL have a search that ignores case, and filters for all, Unsorted, each account and each kind. `space` SHALL select a row; `a` SHALL select all shown rows, or clear them when all are selected; `m` SHALL move the selected rows (or the row under the cursor) to a space or to a new space; `l` SHALL change the read level in the order `all`, `mentions`, `off`; `n` SHALL rename the conversation (an empty label shows the platform name again). Each change SHALL write the configuration file.

#### Scenario: Move many chats to a new space
- **WHEN** the user filters Unsorted, selects two WhatsApp chats with `space`, presses `m` and creates the space "Company B"
- **THEN** the file has the space `company-b` with the two chats, and the Unsorted count is 2 less

#### Scenario: Same choices for each kind
- **WHEN** the table shows a direct conversation, a group and a channel
- **THEN** each row has the same columns and the read choices `all`, `mentions` and `off`

### Requirement: Spaces tab
The Spaces tab SHALL list Unsorted first, then the spaces, with the number of conversations and of waiting messages, and "+ New space". The detail panel of a space SHALL start with one sentence: "<name> reads <n> conversations from <accounts> → sends to <agent> · <schedule> (<schedule text>). Next send: <time> with <n> messages now waiting." It SHALL then show the name, the agent and schedule choices, and the table of its conversations with the read level and the waiting messages. `a` SHALL open the Conversations tab with the unsorted conversations, to move them to the space; `c` SHALL open the Conversations tab with the conversations of the space, to move them or change their read level. Delete SHALL move the conversations of the space to Unsorted. The detail panel of Unsorted SHALL show its read level for new conversations and its agent choice with "hold: do not send".

#### Scenario: Sentence of a space
- **WHEN** the space Acme has 5 conversations, the agent `claude-work`, the schedule `work-hours`, and 6 waiting messages at Thursday 14:37
- **THEN** the detail panel shows "Acme reads 5 conversations from acme, phone → sends to claude-work · work-hours (every 15m · Mon–Fri · 08:00–18:00). Next send: today 14:45 with 6 messages now waiting."

#### Scenario: Delete a space
- **WHEN** the user deletes the space Acme with 5 conversations
- **THEN** the 5 conversations are in Unsorted, and the cards of Acme stay in the Feed

### Requirement: Agents tab
The Agents tab SHALL list the agents with their driver. The detail panel SHALL show the spaces that use the agent, the sentence "Each space has its own session", and the form: name, driver, CLI, workspace, session, payload, limits and prompts. Delete SHALL follow the requirement "References between items" of the `spaces` capability.

#### Scenario: Used by
- **WHEN** the user selects the agent `claude-work` that `acme` and `borealis` use
- **THEN** the detail panel shows Acme and Project Borealis as users of the agent

### Requirement: Schedules tab
The Schedules tab SHALL list the schedules with their `every` value. The detail panel SHALL show the schedule text, the next send time, the spaces that use it, the fields name, send every, days and hours, and a line of today from 00:00 to 24:00 with a mark for each send time and a mark for the current time.

#### Scenario: Day off
- **WHEN** the user turns off Thursday in the schedule `always` on a Thursday
- **THEN** the line of today shows no send mark, and the next send time is Friday 00:00

### Requirement: Settings tab
The Settings tab SHALL show `lookback`, `retention`, the notifications and the lowest card urgency for a notification, and the background service with the keys `i` (install or restart) and `u` (uninstall).

#### Scenario: Install the service from Settings
- **WHEN** the service is not installed and the user presses `i` in the Settings tab
- **THEN** the LaunchAgent is installed, and the Settings tab shows the service as running

### Requirement: Account and conversation commands
The system SHALL provide `i2a account add mattermost --id <id> --url <url> --token-stdin`, `i2a account add whatsapp --id <id> [--phone <number>]`, `i2a account list`, `i2a account remove <id>`, `i2a conversations [--account <id>] [--unsorted]` and `i2a conversation set <account> <conversation-id> [--space <id>] [--read all|mentions|off] [--label <text>]`. `account add mattermost` SHALL do the same connection check as the TUI. `i2a source` with any arguments SHALL print "i2a source is renamed: use i2a account (and i2a conversations)" and exit with code 2.

#### Scenario: Add an account from a script
- **WHEN** a script pipes a valid token to `i2a account add mattermost --id work --url <url> --token-stdin`
- **THEN** the command prints "Connected as <username>" and adds the account to the configuration file

#### Scenario: List conversations
- **WHEN** the user runs `i2a conversations --account work`
- **THEN** the output lists each conversation with its account, ID, kind, name, space and read level

#### Scenario: Set the space of a conversation
- **WHEN** the user runs `i2a conversation set work C1 --space acme --read mentions`
- **THEN** the space `acme` lists `work`/`C1` with the read level `mentions`, and no other space lists it

#### Scenario: Old command
- **WHEN** a script runs `i2a source list`
- **THEN** the command exits with code 2 and names `i2a account`

