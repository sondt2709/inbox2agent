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
The system SHALL provide `i2a account add mattermost --id <id> --url <url> --token-stdin`, `i2a account add whatsapp --id <id> [--phone <number>]`, `i2a account list`, `i2a account remove <id>` and `i2a conversations [--account <id>] [--unsorted] [--json]`. `account add mattermost` SHALL do the same connection check as the TUI. With `--json`, `i2a conversations` SHALL print one JSON array with one object for each conversation, with the keys `account`, `id`, `kind`, `name`, `label`, `space` and `read`. The system SHALL NOT provide a command that changes one conversation: the user or an agent edits `config.toml`. `i2a conversation` with any arguments SHALL print that the command is removed, name `[[spaces.conversations]]` in `config.toml` and `i2a config --help`, and exit with code 2.

#### Scenario: Add an account from a script
- **WHEN** a script pipes a valid token to `i2a account add mattermost --id work --url <url> --token-stdin`
- **THEN** the command prints "Connected as <username>" and adds the account to the configuration file

#### Scenario: List conversations
- **WHEN** the user runs `i2a conversations --account work`
- **THEN** the output lists each conversation with its account, ID, kind, name, space and read level

#### Scenario: List conversations for an agent
- **WHEN** an agent runs `i2a conversations --json`
- **THEN** the output is a JSON array, and each object has the keys `account`, `id`, `kind`, `name`, `label`, `space` and `read`

#### Scenario: Removed command
- **WHEN** a script runs `i2a conversation set work C1 --space acme`
- **THEN** the command exits with code 2, names `[[spaces.conversations]]` and does not change `config.toml`

### Requirement: Dashboard reloads a changed configuration file
While the dashboard is open, it SHALL check `config.toml` on each refresh (every 2 seconds). When the bytes of the file differ from the last bytes that the dashboard read or wrote, the dashboard SHALL load the file again and SHALL show the loaded items in all tabs. When the loaded file is invalid, the dashboard SHALL keep its current configuration and SHALL show the first error in the status line. The dashboard SHALL NOT discard the unsaved edits of an open form because of a reload.

#### Scenario: A space added by another process shows
- **WHEN** the dashboard is open on the Spaces tab
- **AND** another process adds a `[[spaces]]` item with the name `Side project` to `config.toml`
- **THEN** within 3 seconds the Spaces list shows `Side project`

#### Scenario: Invalid file from another process
- **WHEN** another process writes `config.toml` with `every = "ten minutes"` in a schedule
- **THEN** the dashboard keeps its current spaces and schedules
- **AND** the status line shows `config.toml` and the error of the `every` field

#### Scenario: Unsaved form edits stay after a reload
- **WHEN** the user types a new name in the form of the space `Company A` and does not save
- **AND** another process adds a schedule to `config.toml`
- **THEN** the form still shows the typed name and the unsaved mark
- **AND** the Schedules tab shows the new schedule

### Requirement: A save keeps the changes of other processes
Each save of the dashboard SHALL read `config.toml` again, apply only the change of the form or the action to that content, validate the result and write it. A change of another process to a different item SHALL stay in the file. When the item of the change does not exist in the file any more, the save SHALL fail with an error that names the item, and SHALL NOT write the file. When the file does not exist, the save SHALL apply the change to the default configuration.

#### Scenario: A settings save keeps a space of another process
- **WHEN** the dashboard is open
- **AND** another process adds the space `side-project` to `config.toml`
- **AND** the user changes the lookback to `2d` in the Settings tab and saves
- **THEN** `config.toml` contains `lookback = '2d'` and the space `side-project`

#### Scenario: The item was deleted by another process
- **WHEN** the user edits the agent `preview` in the dashboard
- **AND** another process removes the agent `preview` from `config.toml`
- **AND** the user saves the form
- **THEN** the form shows an error that the agent `preview` does not exist any more
- **AND** `config.toml` does not contain the agent `preview`

### Requirement: Conflict on the item of a form
When the user saves a form, and another process changed the same item in `config.toml` after the form was made, the dashboard SHALL open a dialog before it writes. The choice "Reload" SHALL be the default; it SHALL discard the edits of the form and SHALL make the form again from the file. The choice "Overwrite" SHALL save the values of the form over the item. The settings and notification fields are one item; the Unsorted space is one item; each account, agent, schedule, space and conversation is one item.

#### Scenario: Reload after a conflict
- **WHEN** the user changes the `every` field of the schedule `work-hours` to `30m` in the form
- **AND** another process changes `every` of `work-hours` to `5m` in `config.toml`
- **AND** the user saves and chooses "Reload"
- **THEN** `config.toml` contains `every = '5m'` for `work-hours`
- **AND** the form shows `5m`

#### Scenario: Overwrite after a conflict
- **WHEN** the same conflict happens and the user chooses "Overwrite"
- **THEN** `config.toml` contains `every = '30m'` for `work-hours`

### Requirement: Write lock of the configuration file
Each writer of `config.toml` SHALL hold an exclusive `flock` on `config.toml.lock` in the same directory while it reads, changes and writes the file. The writers are the dashboard, `i2a account add`, `i2a account remove` and the conversion of a version 1 file. The lock file SHALL have the mode 0600. When another process holds the lock for more than 5 seconds, the writer SHALL fail with the error `config.toml is locked by another process` and SHALL NOT write. A writer SHALL write the file to a temporary file and rename it, so a reader never reads a partly written file.

#### Scenario: Two writers at the same time
- **WHEN** two processes each add a different space with the locked update at the same time
- **THEN** `config.toml` contains both spaces

#### Scenario: Lock held too long
- **WHEN** a process holds `config.toml.lock` for 10 seconds
- **AND** the user saves a form in the dashboard
- **THEN** the form shows `config.toml is locked by another process`
- **AND** the file does not change

### Requirement: Header of the configuration file
Each save SHALL write `#:schema ./config.schema.json` as the first line, then a header comment that says that i2a writes the file again on each save and removes other comments.

#### Scenario: Header after a save
- **WHEN** the dashboard saves `config.toml`
- **THEN** the first line is `#:schema ./config.schema.json`, and the next lines say that a save by i2a removes comments

### Requirement: Key hints and key help fit the window
The footer SHALL show only the key hints that fit the width of the window, in their order, and SHALL NOT cut a hint. The key help (`?`) SHALL be a dialog as wide as its longest line, or the window when the window is narrower. It SHALL NOT wrap a line that fits, and it SHALL scroll when the window is too short.

#### Scenario: Feed footer at 140 columns
- **WHEN** the window is 140 columns wide and the Feed tab is open
- **THEN** each hint in the footer is complete, and no hint ends in the middle of its text

#### Scenario: Key help at 140 columns
- **WHEN** the window is 140 × 34 and the user presses `?`
- **THEN** each line of the key help shows as one line, and the line of the Feed keys starts with "Feed"

### Requirement: Command help uses the words of the model
The help and the errors of `i2a run` and `i2a daemon` SHALL use the words account, conversation, space, agent and schedule. They SHALL NOT use the words source, profile, interval or active hours.

#### Scenario: Help of run
- **WHEN** the user runs `i2a run --help`
- **THEN** the output says that the command reads each account one time and sends the waiting messages of each space to its agent, and the output does not contain "profile" or "source"

### Requirement: Schema of the configuration file
The system SHALL generate a JSON Schema (draft 2020-12) of `config.toml` from the Go types of the configuration. The schema SHALL describe each key with a description, its type, the allowed values of each choice, the pattern of a duration and of hours, and SHALL NOT allow unknown keys. The empty text and 0 SHALL be valid for each key that has a default, because i2a uses the default for them. Each load and each save of `config.toml` SHALL write the schema to `config.schema.json` in the same directory, with the mode 0600, when the file is missing or different. A failed write of the schema SHALL NOT stop a load or a save. `i2a config schema` SHALL print the schema. The repository SHALL have a copy in `docs/config.schema.json`, and a test SHALL fail when the copy differs from the generated schema.

#### Scenario: Schema next to the file
- **WHEN** i2a saves `config.toml` in a new directory
- **THEN** the directory contains `config.schema.json`, and it is the output of `i2a config schema`

#### Scenario: Editor marks an unknown key
- **WHEN** a TOML language server that reads `#:schema` (Taplo) checks a file with the key `colour` in `[settings]`
- **THEN** it reports the key `colour` as not allowed

#### Scenario: Default values are valid in an editor
- **WHEN** Taplo checks a file with `read = ''` and `inline_max_bytes = 0`
- **THEN** it reports no error

#### Scenario: Schema after an upgrade
- **WHEN** a new i2a loads a `config.toml` whose `config.schema.json` is from an older i2a
- **THEN** `config.schema.json` is the schema of the new i2a

#### Scenario: Choices in the schema
- **WHEN** an agent reads the schema
- **THEN** the key `read` of a conversation allows only `all`, `mentions`, `off` and the empty text

### Requirement: Check a configuration file
`i2a config check [file]` SHALL read the file (default: the configuration file of i2a), and run the same validation as a load: the format, the unknown keys, the references between items, the duplicates and the templates. When the file is valid, the command SHALL print the number of accounts, agents, schedules, spaces and conversations, and exit with code 0. When it is invalid, the command SHALL print each problem and exit with code 1. The command SHALL NOT write a file.

#### Scenario: Valid file
- **WHEN** an agent runs `i2a config check` on a valid file with 1 account and 2 spaces
- **THEN** the output says that the file is valid and names 1 account and 2 spaces, and the exit code is 0

#### Scenario: Two problems
- **WHEN** a file has a space with the agent `nope` and a conversation with the read level `some`
- **THEN** the output has both problems, and the exit code is 1

### Requirement: Help for agents
`i2a config --help` SHALL explain the file for an agent: the path of `config.toml`, the five items and their keys, that `i2a conversations --json` gives the conversation IDs, that `i2a config check` checks a file, that a running dashboard and daemon load a changed file, and that a writer writes a temporary file and renames it.

#### Scenario: Agent reads the help
- **WHEN** an agent runs `i2a config --help`
- **THEN** the output names `config.toml`, `i2a conversations --json`, `i2a config check` and `config.schema.json`
