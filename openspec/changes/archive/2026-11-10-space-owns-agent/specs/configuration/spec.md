## ADDED Requirements

### Requirement: Configuration validation
The system SHALL validate the configuration when it loads it and when the TUI or a command saves it. Durations SHALL use Go duration syntax. `hours` SHALL be empty or `HH:MM-HH:MM`. `days` SHALL contain only `mon` to `sun`. IDs of accounts, schedules and spaces SHALL be unique in their kind and SHALL be 1 to 32 lowercase letters, digits, `-` or `_`. For the agent of each space and of Unsorted, `driver` SHALL be `orca` or `dry-run`, `session` SHALL be `new`, `reuse` or `worktree`, `payload` SHALL be `pointer` or `inline`, `inline_max_bytes` SHALL be between 1,024 and 65,536, `message_max_chars` SHALL be between 100 and 20,000, and an `orca` agent SHALL have a `folder` and a `cli`. The `instruction` of a space SHALL NOT contain three backticks. The references SHALL follow the requirement "References between items" of the `spaces` capability. The system SHALL report all problems at one time, each with its field and its space.

#### Scenario: Invalid duration
- **WHEN** a schedule has `every = "ten minutes"`
- **THEN** the system reports the field name and the invalid value and does not start

#### Scenario: Invalid day
- **WHEN** a schedule has `days = ["monday"]`
- **THEN** the system reports that `monday` is not one of `mon` to `sun`

#### Scenario: Three backticks in the instruction
- **WHEN** the space `triage` has an `instruction` with a line of three backticks
- **THEN** the system reports `space "triage".instruction: the instruction cannot contain three backticks (```)` and does not start

#### Scenario: Orca agent with no folder
- **WHEN** the space `triage` has an `orca` agent with no `folder`
- **THEN** the system reports `space "triage".agent.folder: the orca driver needs a folder`

## MODIFIED Requirements

### Requirement: Dashboard tabs
The system SHALL provide a full-screen dashboard in the alternate screen with three areas: Feed, Spaces and Setup. Setup SHALL have the sections Accounts, Schedules, Settings and Send log, in this order. The header SHALL show the areas as `Feed`, `Spaces` and `Setup` with no numbers, with the number of new cards after Feed and the number of new conversations after Spaces. The line under the header SHALL show the sections of Setup when Setup is open, and else the result of the last run. `i2a` with no arguments SHALL open the Feed; `i2a config` SHALL open the Spaces area. A mouse click on an area or a section SHALL open it. The keys `1` to `3` SHALL NOT open an area. `tab` and `shift+tab` SHALL go to the next and the previous section, in the order Feed, Spaces, Accounts, Schedules, Settings, Send log. Forms SHALL stay in the detail panel, as before. `q` SHALL quit; `?` SHALL show the model of i2a and all keys.

#### Scenario: Header
- **WHEN** the user runs `i2a config` in a window of 120 × 40, with 3 new conversations
- **THEN** the header shows `Feed`, `Spaces 3 new`, `Setup` and `ctrl+r run now`, and the Spaces area is open

#### Scenario: Setup sections
- **WHEN** the user clicks `Setup`
- **THEN** the line under the header shows Accounts, Schedules, Settings and Send log, and no Agents section

#### Scenario: Feed first
- **WHEN** the user runs `i2a` with no arguments
- **THEN** the dashboard opens on the Feed

#### Scenario: Terminal restored
- **WHEN** the user exits the TUI
- **THEN** the terminal shows the shell screen from before the dashboard

### Requirement: Spaces area
The Spaces area SHALL list at the left each space, then Everything else (the Unsorted space), New conversations with their count, Ignored with its count, and "+ New space". A space SHALL show `●` when it sends and `○` when it is paused, the number of its conversations and the number of waiting messages. The panel at the right SHALL be one list. For a space, its first rows SHALL be Name, Status, Agent, Session and When, and a line SHALL show the last send. Its conversation rows SHALL follow, each with the kind mark, the name, the account, the read level ("All messages" or "Only for me"), the other spaces of the conversation ("also in …"), and the time of the last message. The recent sends of the space SHALL follow. `enter` or `→` in the list at the left SHALL move the keyboard to the panel, and `esc` or `←` on a settings row SHALL move it back. In the panel, `enter` on Status SHALL pause or resume the space and save at once; `enter` on Agent SHALL show the agent form of the space in the panel (driver, the Orca warning, CLI, folder, session, model, payload, inline limit, message limit, instruction), which saves with "Save changes" or `ctrl+s` and closes with `esc`; `enter` or `s` on Session SHALL open the sessions dialog of the space; `enter` on When SHALL open the When dialog; `enter` on Name SHALL open the rename dialog. On a conversation row, `←` and `→` SHALL change the read level of the link; `x` SHALL remove the conversation from the space; `enter` SHALL open the spaces dialog; `space` SHALL select the row; `n` SHALL rename the conversation. `x` on a space in the list at the left SHALL delete the space, and its conversations that no other space lists SHALL go to Everything else. `R` SHALL run the selected space. Each change SHALL write the configuration file and SHALL show "Saved" with the change in the status line. A new space SHALL get a copy of the agent of the selected space, or of Unsorted, with no instruction.

#### Scenario: Change the read level of a link
- **WHEN** `# release` is in the space Review with "All messages", and the user selects its row in Review and presses `→`
- **THEN** the file has `read = "mentions"` for `# release` in Review only, and the row shows "Only for me"

#### Scenario: Remove the last space
- **WHEN** `# release` is only in Review, and the user presses `x` on its row in Review
- **THEN** the file lists `# release` in no space, and the row shows in Everything else

#### Scenario: Pause a space
- **WHEN** the user presses `enter` on the row Status of Review
- **THEN** the file has `paused = true` for Review, the row says "Paused", and the list at the left shows `○` for Review

#### Scenario: Edit the agent of a space
- **WHEN** the user presses `enter` on the row Agent of Review, sets the folder to `~/review` and the instruction to "Review the pull requests", and presses `ctrl+s`
- **THEN** the file has `folder = "~/review"` and `instruction = "Review the pull requests"` for Review only, and the panel shows the rows again

#### Scenario: Session row
- **WHEN** the space Review has the stored terminal `term_a` and the Claude session `04ebe19c-…`
- **THEN** the row Session shows `term_a` and `04ebe19c`, and `enter` opens the sessions dialog of Review

### Requirement: Everything else
The Spaces area SHALL show the Unsorted space as "Everything else". Its rows SHALL be "New and other conversations read" (the level `unsorted.read`), Status ("Sending", or "Hold: does not send"), Agent, Session, When, and the conversations that are in no space and are not ignored, newest activity first. `←` and `→` on a conversation row SHALL write an Unsorted entry with that level. `i` SHALL ignore the conversation.

#### Scenario: Ignore a conversation
- **WHEN** the user presses `i` on `# random` in Everything else
- **THEN** the file lists `# random` in `[unsorted]` with `read = "off"`, and the row shows in Ignored

#### Scenario: Send everything else
- **WHEN** Everything else holds its messages, and the user presses `enter` on its row Status
- **THEN** the file has `paused = false` in `[unsorted]`, and the row says "Sending"

### Requirement: Link a session to a space
The sessions dialog SHALL show the space, its terminal handle, the Claude session ID, the Claude session name and the state (`live`, `no Claude`, `other session`, `closed`, `no agent` for an agent that is not Claude Code, `unknown`). `o` SHALL show the terminal in Orca with `orca terminal switch`. `l` SHALL open an input for a terminal handle (`term_…`) or a Claude session ID (a UUID). The system SHALL find the other ID from the running Claude processes and SHALL require exactly one Claude process. It SHALL ask for a confirmation when the `cwd` of the process is outside the folder of the agent of the space, or when another space has the same Claude session. The link SHALL store the handle and the Claude session ID for the space. Unlink (`u`) SHALL delete the stored session of the space; the old terminal stays open.

#### Scenario: Link by handle
- **WHEN** the user links `term_c` to the space Triage, and `term_c` runs one Claude process with session `S` in the folder of the agent
- **THEN** the store has `term_c` and `S` for Triage, and the next batch of Triage goes to `term_c`

#### Scenario: Link by Claude session ID
- **WHEN** the user links the Claude session `S`, and `S` runs in `term_c`
- **THEN** the store has `term_c` and `S` for the space

#### Scenario: No Claude process
- **WHEN** the user links `term_d`, and `term_d` runs zsh only
- **THEN** the dialog shows "no running Claude session in term_d", and the store does not change

#### Scenario: Session of another space
- **WHEN** the user links to Unsorted the Claude session that Triage has
- **THEN** a confirmation says that the data of the two spaces goes to one session, and the store changes only after `y`

### Requirement: Help for agents
`i2a config --help` SHALL explain the file for an agent: the path of `config.toml`, the four items (accounts, schedules, spaces with their agent and instruction, Unsorted) and their keys, that `i2a conversations --json` gives the conversation IDs, that `i2a config check` checks a file, that a running dashboard and daemon load a changed file, and that a writer writes a temporary file and renames it.

#### Scenario: Agent reads the help
- **WHEN** an agent runs `i2a config --help`
- **THEN** the output names `config.toml`, `[spaces.agent]`, `paused`, `i2a conversations --json`, `i2a config check` and `config.schema.json`, and does not name `[[agents]]`

### Requirement: Schedules tab
The Schedules tab SHALL list the schedules with their `every` value. The detail panel SHALL show the schedule text, the next send time, the spaces that use it, the fields name, send every, days and hours, and a line of today from 00:00 to 24:00 with a mark for each send time and a mark for the current time. The When dialog of a space SHALL list the schedules to pick one, then "Rename <schedule>…", which changes the name of the schedule (its ID stays), and "Edit the times of <schedule> in Setup", which opens the Schedules tab with that schedule and its form.

#### Scenario: Day off
- **WHEN** the user turns off Thursday in the schedule `always` on a Thursday
- **THEN** the line of today shows no send mark, and the next send time is Friday 00:00

#### Scenario: Rename from a space
- **WHEN** the user presses `enter` on the row When of Review, chooses "Rename default…" and types "Work hours"
- **THEN** the file has `name = "Work hours"` for the schedule `default`, its ID stays `default`, and each space that uses it shows "Work hours"

## REMOVED Requirements

### Requirement: Configuration validation of version 2
**Reason**: The file is version 3.
**Migration**: See the requirement "Configuration validation".

### Requirement: Agents tab
**Reason**: Each space owns its agent; the Spaces panel edits it.
**Migration**: The row Agent of a space opens the agent form; the row Session opens the sessions dialog of the space.
