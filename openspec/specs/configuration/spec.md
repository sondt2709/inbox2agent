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
The system SHALL validate the configuration when it loads it and when the TUI or a command saves it. Durations SHALL use Go duration syntax. `hours` SHALL be empty or `HH:MM-HH:MM`. `days` SHALL contain only `mon` to `sun`. `inline_max_bytes` SHALL be between 1,024 and 65,536. IDs of accounts, agents, schedules and spaces SHALL be unique in their kind and SHALL be 1 to 32 lowercase letters, digits, `-` or `_`. The `primer` of an agent (the instruction) SHALL NOT contain three backticks. The references SHALL follow the requirement "References between items" of the `spaces` capability. The system SHALL report all problems at one time, each with its field. The system SHALL read the `batch` field of an agent, SHALL NOT use it, and SHALL NOT write it at the next save.

#### Scenario: Invalid duration
- **WHEN** a schedule has `every = "ten minutes"`
- **THEN** the system reports the field name and the invalid value and does not start

#### Scenario: Invalid day
- **WHEN** a schedule has `days = ["monday"]`
- **THEN** the system reports that `monday` is not one of `mon` to `sun`

#### Scenario: Three backticks in the instruction
- **WHEN** the agent `triage` has a `primer` with a line of three backticks
- **THEN** the system reports `agent "triage".primer: the instruction cannot contain three backticks (```)` and does not start

#### Scenario: Old batch template
- **WHEN** an agent has a `batch` template and the user saves another field in the dashboard
- **THEN** the file loads with no error, and the saved file has no `batch` field

### Requirement: Dashboard tabs
The system SHALL provide a full-screen dashboard in the alternate screen with three areas: Feed, Spaces and Setup. Setup SHALL have the sections Accounts, Agents, Schedules, Settings and Send log, in this order. The header SHALL show the areas as `Feed`, `Spaces` and `Setup` with no numbers, with the number of new cards after Feed and the number of new conversations after Spaces. The line under the header SHALL show the sections of Setup when Setup is open, and else the result of the last run. `i2a` with no arguments SHALL open the Feed; `i2a config` SHALL open the Spaces area. A mouse click on an area or a section SHALL open it. The keys `1` to `3` SHALL NOT open an area. `tab` and `shift+tab` SHALL go to the next and the previous section, in the order Feed, Spaces, Accounts, Agents, Schedules, Settings, Send log. Forms SHALL stay in the detail panel, as before. `q` SHALL quit; `?` SHALL show the model of i2a and all keys.

#### Scenario: Header
- **WHEN** the user runs `i2a config` in a window of 120 × 40, with 3 new conversations
- **THEN** the header shows `Feed`, `Spaces 3 new`, `Setup` and `ctrl+r run now`, and the Spaces area is open

#### Scenario: Setup sections
- **WHEN** the user clicks `Setup`
- **THEN** the line under the header shows Accounts, Agents, Schedules, Settings and Send log, and a click on Agents opens Agents

#### Scenario: Feed first
- **WHEN** the user runs `i2a` with no arguments
- **THEN** the dashboard opens on the Feed

#### Scenario: Terminal restored
- **WHEN** the user exits the TUI
- **THEN** the terminal shows the shell screen from before the dashboard

### Requirement: Accounts tab
The Accounts section of Setup SHALL list the accounts at the left with their name and platform. The detail panel SHALL show the name (editable), the ID, the platform, the information that identifies the account (Mattermost: server URL and username; WhatsApp: phone number and profile name), the connection state, the number of conversations found, and the number of conversations in each space. `read_every` SHALL be under "Advanced", with the help "This is not when the agent gets messages: that is the schedule of each space." "+ Add account" SHALL show the form of a new account: Mattermost (account ID, server URL, token, "Connect") or WhatsApp (account ID, phone number, "Connect", QR code or phone code). The TUI SHALL check a Mattermost URL and token with the server before it saves them. After a connection, the conversations of the new account SHALL be in Everything else.

#### Scenario: Valid token
- **WHEN** the user types a valid URL and token and selects "Connect"
- **THEN** the TUI shows "Connected as <username>", saves the account, and says that its conversations are in Everything else

#### Scenario: Invalid token
- **WHEN** the user types a token that the server rejects
- **THEN** the TUI shows the error from the server, and the system saves no secret and no account

#### Scenario: Account information
- **WHEN** the user selects a paired WhatsApp account and a Mattermost account
- **THEN** the WhatsApp detail shows the phone number in international format and the profile name, and the Mattermost detail shows the server URL and the username

### Requirement: Agents tab
The Agents tab SHALL list the agents with their driver. The detail panel SHALL show the spaces that use the agent, the sentence "Each space has its own session", one line for each of these spaces with its stored terminal handle and short Claude session ID, and the form: name, driver, CLI, workspace, session, payload, limits and instruction. The Instruction field SHALL say that i2a sends it with each batch, and SHALL show the error "the instruction cannot contain three backticks (```)" for such a text. The form SHALL NOT have a batch prompt field. Delete SHALL follow the requirement "References between items" of the `spaces` capability. For an `orca` agent, the key `s` SHALL open the sessions dialog: one row for each space that uses the agent, with the terminal handle, the Claude session ID, the Claude session name and the state (`live`, `no Claude`, `other session`, `closed`, `no agent` for an agent that is not Claude Code, `unknown`). In the dialog, `l` SHALL link a session to the selected space, `u` SHALL unlink it, and `o` SHALL show its terminal in Orca with `orca terminal switch`.

#### Scenario: Used by
- **WHEN** the user selects the agent `claude-work` that `acme` and `borealis` use
- **THEN** the detail panel shows Acme and Project Borealis as users of the agent, each with its terminal handle

#### Scenario: Sessions dialog
- **WHEN** the user presses `s` on the agent `triage`, and the space Triage has the live terminal `term_a` with Claude session `S`
- **THEN** the dialog shows Triage with `term_a`, the first 8 characters of `S`, the Claude session name and `live`

#### Scenario: Instruction with three backticks
- **WHEN** the user types a line of three backticks in the Instruction field and saves
- **THEN** the form shows "the instruction cannot contain three backticks (```)" and the file does not change

### Requirement: Schedules tab
The Schedules tab SHALL list the schedules with their `every` value. The detail panel SHALL show the schedule text, the next send time, the spaces that use it, the fields name, send every, days and hours, and a line of today from 00:00 to 24:00 with a mark for each send time and a mark for the current time.

#### Scenario: Day off
- **WHEN** the user turns off Thursday in the schedule `always` on a Thursday
- **THEN** the line of today shows no send mark, and the next send time is Friday 00:00

### Requirement: Settings tab
The Settings tab SHALL show `lookback`, `retention`, the notifications, the lowest card urgency for a notification, the notifier (`via`), the Feed notifications in iTerm2 (`terminal`), the line of `i2a notify status` for the notifier that i2a uses now with the hint "run i2a notify setup", and the background service with the keys `i` (install or restart) and `u` (uninstall).

#### Scenario: Install the service from Settings
- **WHEN** the service is not installed and the user presses `i` in the Settings tab
- **THEN** the LaunchAgent is installed, and the Settings tab shows the service as running

#### Scenario: Notifier line
- **WHEN** `via = "auto"`, Orca does not run, and terminal-notifier 3.1.0 is installed
- **THEN** the Settings tab shows that i2a uses terminal-notifier 3.1.0

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
The footer SHALL show the hints of the focused list or panel with its main key first, and `? keys · q quit` at the right. It SHALL show only the hints that fit the width of the window, in their order, and SHALL NOT cut a hint. At 80 columns the footer SHALL show `ctrl+r` in the Spaces area and `enter` and `a` in the panel of a space. The key help (`?`) SHALL be a dialog as wide as its longest line, or the window when the window is narrower. It SHALL NOT wrap a line that fits, and it SHALL scroll when the window is too short.

#### Scenario: Spaces footer at 80 columns
- **WHEN** the window is 80 columns wide and a space is selected in the Spaces area
- **THEN** the footer shows `ctrl+r run now`, and each hint is complete

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

### Requirement: Spaces area
The Spaces area SHALL list at the left each space, then Everything else (the Unsorted space), New conversations with their count, Ignored with its count, and "+ New space". A space SHALL show `●` when it has an agent and `○` when it holds, the number of its conversations and the number of waiting messages. The panel at the right SHALL be one list. For a space, its first rows SHALL be Name, Sends to and When, and a line SHALL show the last send. Its conversation rows SHALL follow, each with the kind mark, the name, the account, the read level ("All messages" or "Only for me"), the other spaces of the conversation ("also in …"), and the time of the last message. The recent sends of the space SHALL follow. `enter` or `→` in the list at the left SHALL move the keyboard to the panel, and `esc` or `←` on a settings row SHALL move it back. In the panel, `enter` on a settings row SHALL open a dialog that saves at once; `←` and `→` on a conversation row SHALL change the read level of the link; `x` SHALL remove the conversation from the space; `enter` SHALL open the spaces dialog; `space` SHALL select the row; `n` SHALL rename the conversation. `x` on a space in the list at the left SHALL delete the space, and its conversations that no other space lists SHALL go to Everything else. `R` SHALL run the selected space. Each change SHALL write the configuration file and SHALL show "Saved" with the change in the status line.

#### Scenario: Change the read level of a link
- **WHEN** `# release` is in the space Review with "All messages", and the user selects its row in Review and presses `→`
- **THEN** the file has `read = "mentions"` for `# release` in Review only, and the row shows "Only for me"

#### Scenario: Remove the last space
- **WHEN** `# release` is only in Review, and the user presses `x` on its row in Review
- **THEN** the file lists `# release` in no space, and the row shows in Everything else

#### Scenario: Agent of a space
- **WHEN** the user presses `enter` on the row "Sends to" of Review and chooses the agent `reviewer`
- **THEN** the file has `agent = "reviewer"` for Review, with no Save button

### Requirement: Everything else
The Spaces area SHALL show the Unsorted space as "Everything else". Its rows SHALL be "New and other conversations read" (the level `unsorted.read`), "Sends to" (with "hold"), "When", and the conversations that are in no space and are not ignored, newest activity first. `←` and `→` on a conversation row SHALL write an Unsorted entry with that level. `i` SHALL ignore the conversation.

#### Scenario: Ignore a conversation
- **WHEN** the user presses `i` on `# random` in Everything else
- **THEN** the file lists `# random` in `[unsorted]` with `read = "off"`, and the row shows in Ignored

### Requirement: Spaces dialog of a conversation
The dialog SHALL list each space with a check mark and the read level of the conversation in that space, then "Ignore this conversation" and "+ New space". `space` SHALL check or uncheck a row; `←` and `→` SHALL change the level of a checked space; `enter` SHALL save and close; `esc` SHALL close with no change. With no space checked and no Ignore, the conversation SHALL be in Everything else. Ignore SHALL uncheck all spaces. For a selection of many conversations, the dialog SHALL start with the spaces of the first one and SHALL save the same spaces for all of them. When the user checks a space that the conversation was not in, the dialog SHALL show the option "Also send the messages of the last <lookback> to the new spaces", off by default.

#### Scenario: A conversation in two spaces
- **WHEN** `# release` is in Review, and the user opens its dialog, checks Team, sets "Only for me" for Team and presses enter
- **THEN** the file lists `# release` in Review with `all` and in Team with `mentions`

#### Scenario: Move
- **WHEN** `# release` is in Review, and the user unchecks Review, checks Team and presses enter
- **THEN** the file lists `# release` only in Team

#### Scenario: Send the old messages
- **WHEN** a message of `# release` from today was sent to triage, and the user adds Review with the option on
- **THEN** the next send of Review contains the message, and triage does not get it again

### Requirement: Search of conversations
`/` SHALL open a search of the conversations of all accounts from each area, except in a form. The rows SHALL show the kind mark, the name, the account and the spaces, with the newest activity first. Typing SHALL filter the rows by the name, the label, the account or a space; the search SHALL ignore case. `enter` SHALL open the spaces dialog of the row. `a` in the Spaces area SHALL open the same search for the selected space: each row SHALL show a check mark when the space has it, `space` SHALL add or remove it at once, and `←`/`→` SHALL change its level in that space.

#### Scenario: Find and add
- **WHEN** the user presses `a` in Review, types `rel`, moves to `# release` and presses `space`
- **THEN** the file lists `# release` in Review with `all`, and the row shows a check mark

### Requirement: New conversations
A conversation SHALL be new when an account lists it, the configuration has no entry for it, and the dashboard did not show it before. The dashboard SHALL keep the conversations that it showed in `<data>/conversations-seen.json` (mode 0600), by account. The first listing of an account SHALL mark all its conversations as shown. New conversations SHALL show in the item New conversations, with the time of their last message. `enter` SHALL open the spaces dialog, `k` SHALL keep the row in Everything else and mark it as shown, `K` SHALL do this for all rows, and `i` SHALL ignore it. Until the user decides, Everything else SHALL read a new conversation.

#### Scenario: Upgrade
- **WHEN** the dashboard starts for the first time with an account that lists 400 conversations
- **THEN** New conversations shows 0

#### Scenario: A new channel
- **WHEN** the user joins the channel `incident-42` after the first listing, and the dashboard lists the account again
- **THEN** the header shows `1 new`, and New conversations lists `# incident-42`

### Requirement: Undo of the last change
`u` in the Spaces area SHALL write back the configuration from before the last change that the dashboard saved, when the file did not change after that save. When the file changed, the dashboard SHALL show that another process changed the file, and SHALL NOT write it.

#### Scenario: Undo a remove
- **WHEN** the user removes `# release` from Review and presses `u`
- **THEN** the file lists `# release` in Review again

### Requirement: Link a session to a space
In the sessions dialog, `l` SHALL open an input for a terminal handle (`term_…`) or a Claude session ID (a UUID). The system SHALL find the other ID from the running Claude processes and SHALL require exactly one Claude process. It SHALL ask for a confirmation when the `cwd` of the process is outside the workspace of the agent, or when another space has the same Claude session. The link SHALL store the handle, the Claude session ID and the primer hash `pending` for the space. Unlink SHALL delete the stored session of the space; the old terminal stays open.

#### Scenario: Link by handle
- **WHEN** the user links `term_c` to the space Triage, and `term_c` runs one Claude process with session `S` in the workspace of the agent
- **THEN** the store has `term_c` and `S` for Triage, and the next batch of Triage sends the primer and the batch to `term_c`

#### Scenario: Link by Claude session ID
- **WHEN** the user links the Claude session `S`, and `S` runs in `term_c`
- **THEN** the store has `term_c` and `S` for the space

#### Scenario: No Claude process
- **WHEN** the user links `term_d`, and `term_d` runs zsh only
- **THEN** the dialog shows "no running Claude session in term_d", and the store does not change

#### Scenario: Session of another space
- **WHEN** the user links to Unsorted the Claude session that Triage has
- **THEN** a confirmation says that the data of the two spaces goes to one session, and the store changes only after `y`

### Requirement: Notifier settings
`[notify] via` SHALL be `auto`, `orca`, `terminal-notifier` or `osascript`; the default SHALL be `auto`. `[notify] terminal` SHALL be `iterm2` or `off`; the default SHALL be `iterm2`. The configuration validation SHALL reject another value of `via` or `terminal`. The schema of the configuration file SHALL describe both fields.

#### Scenario: Invalid notifier
- **WHEN** `config.toml` has `[notify] via = "growl"`
- **THEN** `i2a config check` reports that `notify.via` is not auto, orca, terminal-notifier or osascript

#### Scenario: Old file
- **WHEN** `config.toml` has a `[notify]` table with only `enabled` and `min_urgency`
- **THEN** the system loads it with `via = "auto"` and `terminal = "iterm2"`

