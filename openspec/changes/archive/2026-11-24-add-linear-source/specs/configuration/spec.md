## ADDED Requirements

### Requirement: Feeds of a Linear account in the file
A Linear account SHALL have one or more `[[accounts.feeds]]` entries with `id`, `name` (optional), `kind` (`comments` or `inbox`), `from`, `prefix`, `projects`, `labels` and `teams`. Only a Linear account SHALL have feeds. `url` of a Linear account SHALL be empty or start with `https://` or `http://`; empty means `https://api.linear.app/graphql`.

#### Scenario: Feed with an unknown kind
- **WHEN** the account `lin` has a feed `agent` with `kind = "issues"`
- **THEN** the system reports `account "lin".feeds "agent".kind: "issues" is not comments or inbox`

#### Scenario: Linear account with no feed
- **WHEN** the account `lin` has `platform = "linear"` and no feed
- **THEN** the system reports `account "lin".feeds: a Linear account needs one feed or more`

#### Scenario: Feed on a Mattermost account
- **WHEN** a Mattermost account has a feed
- **THEN** the system reports that only a Linear account has feeds

## MODIFIED Requirements

### Requirement: Accounts tab
The Accounts section of Setup SHALL list the accounts at the left with their name and platform. The detail panel SHALL show the name (editable), the ID, the platform, the information that identifies the account (Mattermost: server URL and username; WhatsApp: phone number and profile name; Linear: display name, workspace and one line for each feed with the note that `[[accounts.feeds]]` in `config.toml` changes them), the connection state, the number of conversations found, and the number of conversations in each space. `read_every` SHALL be under "Advanced", with the help "This is not when the agent gets messages: that is the schedule of each space." "+ Add account" SHALL show the form of a new account: Mattermost (account ID, server URL, token, "Connect"), WhatsApp (account ID, phone number, "Connect", QR code or phone code) or Linear (account ID, name, API key, "Connect"). The TUI SHALL check a Mattermost URL and token, and a Linear API key, with the server before it saves them. A new Linear account SHALL get the feeds `agent` (`comments`, `from = ["me"]`, `prefix = "/agent"`) and `inbox` (`inbox`). After a connection, the conversations of the new account SHALL be in Everything else.

#### Scenario: Valid token
- **WHEN** the user types a valid URL and token and selects "Connect"
- **THEN** the TUI shows "Connected as <username>", saves the account, and says that its conversations are in Everything else

#### Scenario: Invalid token
- **WHEN** the user types a token that the server rejects
- **THEN** the TUI shows the error from the server, and the system saves no secret and no account

#### Scenario: Account information
- **WHEN** the user selects a paired WhatsApp account and a Mattermost account
- **THEN** the WhatsApp detail shows the phone number in international format and the profile name, and the Mattermost detail shows the server URL and the username

#### Scenario: Add a Linear account
- **WHEN** the user selects Linear, types a valid API key and selects "Connect"
- **THEN** the TUI shows "Connected as <display name>@<workspace key>", saves the account with the feeds `agent` and `inbox`, and both feeds are in Everything else

### Requirement: Account and conversation commands
The system SHALL provide `i2a account add mattermost --id <id> --url <url> --token-stdin`, `i2a account add whatsapp --id <id> [--phone <number>]`, `i2a account add linear --id <id> --token-stdin [--name <name>] [--url <endpoint>]`, `i2a account list`, `i2a account remove <id>` and `i2a conversations [--account <id>] [--unsorted] [--json]`. `account add mattermost` and `account add linear` SHALL do the same connection check as the TUI. `account add linear` SHALL add the same two feeds as the TUI. With `--json`, `i2a conversations` SHALL print one JSON array with one object for each conversation, with the keys `account`, `id`, `kind`, `name`, `label`, `space` and `read`. The conversations of a Linear account SHALL be its feeds. The system SHALL NOT provide a command that changes one conversation: the user or an agent edits `config.toml`. `i2a conversation` with any arguments SHALL print that the command is removed, name `[[spaces.conversations]]` in `config.toml` and `i2a config --help`, and exit with code 2.

#### Scenario: Add an account from a script
- **WHEN** a script pipes a valid token to `i2a account add mattermost --id work --url <url> --token-stdin`
- **THEN** the command prints "Connected as <username>" and adds the account to the configuration file

#### Scenario: Add a Linear account from a script
- **WHEN** a script pipes a valid API key to `i2a account add linear --id lin --token-stdin`
- **THEN** the command prints "Connected as <display name>@<workspace key>" and adds the account with the feeds `agent` and `inbox`

#### Scenario: Two Linear accounts
- **WHEN** the user adds the accounts `lin-a` and `lin-b` with two keys
- **THEN** each account has its own key in the secret store, its own feeds and its own cursor

#### Scenario: List conversations
- **WHEN** the user runs `i2a conversations --account work`
- **THEN** the output lists each conversation with its account, ID, kind, name, space and read level

#### Scenario: List conversations for an agent
- **WHEN** an agent runs `i2a conversations --json`
- **THEN** the output is a JSON array, and each object has the keys `account`, `id`, `kind`, `name`, `label`, `space` and `read`

#### Scenario: Removed command
- **WHEN** a script runs `i2a conversation set work C1 --space acme`
- **THEN** the command exits with code 2, names `[[spaces.conversations]]` and does not change `config.toml`
