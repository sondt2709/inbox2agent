## MODIFIED Requirements

### Requirement: Accounts tab
The Accounts section of Setup SHALL list the accounts at the left with their name and platform. The detail panel SHALL show the name (editable), the ID, the platform, the information that identifies the account (Mattermost: server URL and username; WhatsApp: phone number and profile name; Linear: display name, workspace and one line for each feed), the connection state, the number of conversations found, and the number of conversations in each space. `read_every` SHALL be under "Advanced", with the help "This is not when the agent gets messages: that is the schedule of each space." "+ Add account" SHALL show the form of a new account: Mattermost (account ID, server URL, token, "Connect"), WhatsApp (account ID, phone number, "Connect", QR code or phone code) or Linear (account ID, name, API key, "Connect"). The TUI SHALL check a Mattermost URL and token, and a Linear API key, with the server before it saves them. A new Linear account SHALL get the feeds `agent` (`comments`, `from = ["me"]`, `prefix = "/agent"`) and `inbox` (`inbox`). After a connection, the conversations of the new account SHALL be in Everything else.

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

## ADDED Requirements

### Requirement: Feed form of a Linear account
The detail of a Linear account SHALL have the button "Edit feeds" (key `f` in the account list). It SHALL open a list of the feeds of the account and "+ New feed". A feed SHALL open the feed form in the detail panel with Name, Kind (Comments or Inbox), From, Prefix, Projects, Labels and Teams; a new feed SHALL also have an ID, and the ID of a saved feed SHALL NOT change. "Save feed" SHALL save the feed in `config.toml` with the checks of the configuration. "Remove feed" SHALL ask, then remove the feed and its entries in all spaces; it SHALL refuse to remove the last feed. Esc and "Cancel" SHALL go back to the account form, and SHALL ask first when the form has changes. A new feed SHALL be a conversation in Everything else.

In From, Projects, Labels and Teams, the user SHALL type names separated by commas, or press enter to open a list of the names of the workspace with check marks. In the list, typing SHALL filter the rows, enter SHALL switch the check mark of a row, ctrl+s SHALL apply the checked names to the field, and esc SHALL cancel. When no row matches the text, the list SHALL offer to add the text as a name. When the names cannot be read, the list SHALL show the error and still accept typed names. A team row SHALL show the key and the name, and SHALL save the key. The From list SHALL start with `me`.

#### Scenario: Filter a feed by label
- **WHEN** the user opens the feed `agent`, presses enter in Labels, types `inf`, presses enter on `Infra`, presses ctrl+s and selects "Save feed"
- **THEN** `config.toml` has `labels = ["Infra"]` in the feed `agent`, and the account detail shows `labels Infra` in the line of the feed

#### Scenario: Typed project
- **WHEN** the user types `Bridge, Mobile` in Projects and selects "Save feed"
- **THEN** the feed has `projects = ["Bridge", "Mobile"]`

#### Scenario: New feed
- **WHEN** the user selects "+ New feed", types the ID `bridge`, the prefix `>agent` and selects "Save feed"
- **THEN** the account has the feed `bridge` of the kind `comments`, and the conversation `bridge` is in Everything else

#### Scenario: Remove a feed
- **WHEN** the space `triage` lists the feed `inbox` and the user removes the feed `inbox`
- **THEN** the account has no feed `inbox`, and `triage` has no entry for it

#### Scenario: Remove the last feed
- **WHEN** the account has only the feed `agent` and the user selects "Remove feed"
- **THEN** the TUI shows that a Linear account needs one feed or more, and nothing changes

#### Scenario: Names cannot be read
- **WHEN** the name query fails and the user opens the Labels list
- **THEN** the list shows the error, and the user can type `infra` and add it
