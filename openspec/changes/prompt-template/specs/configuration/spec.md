## MODIFIED Requirements

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
- **THEN** the system reports `agents[0].primer: the instruction cannot contain three backticks (```)` and does not start

#### Scenario: Old batch template
- **WHEN** an agent has a `batch` template and the user saves another field in the dashboard
- **THEN** the file loads with no error, and the saved file has no `batch` field

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
