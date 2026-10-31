## MODIFIED Requirements

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

## ADDED Requirements

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
