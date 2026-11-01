## MODIFIED Requirements

### Requirement: Dashboard tabs
The system SHALL provide a full-screen dashboard in the alternate screen with three areas: Feed, Spaces and Setup. Setup SHALL have the sections Accounts, Agents, Schedules, Settings and Send log, in this order. The header SHALL show the areas as `1 Feed`, `2 Spaces` and `3 Setup`, with the number of new cards after Feed and the number of new conversations after Spaces. The line under the header SHALL show the sections of Setup when Setup is open, and else the result of the last run. `i2a` with no arguments SHALL open the Feed; `i2a config` SHALL open the Spaces area. The keys `1` to `3` and a mouse click SHALL open an area. `tab` and `shift+tab` SHALL go to the next and the previous section, in the order Feed, Spaces, Accounts, Agents, Schedules, Settings, Send log. Forms SHALL stay in the detail panel, as before. `q` SHALL quit; `?` SHALL show the model of i2a and all keys.

#### Scenario: Header
- **WHEN** the user runs `i2a config` in a window of 120 × 40, with 3 new conversations
- **THEN** the header shows `1 Feed`, `2 Spaces 3 new` and `3 Setup`, and the Spaces area is open

#### Scenario: Setup sections
- **WHEN** the user presses `3`
- **THEN** the line under the header shows Accounts, Agents, Schedules, Settings and Send log, and `tab` opens the next section

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

### Requirement: Key hints and key help fit the window
The footer SHALL show the hints of the focused list with its main key first, and `? keys · q quit` at the right. It SHALL show only the hints that fit the width of the window, in their order, and SHALL NOT cut a hint. At 80 columns the footer SHALL show `R` in the Spaces area and `enter` and `a` in the panel of a space. The key help (`?`) SHALL be a dialog as wide as its longest line, or the window when the window is narrower. It SHALL NOT wrap a line that fits, and it SHALL scroll when the window is too short.

#### Scenario: Spaces footer at 80 columns
- **WHEN** the window is 80 columns wide and a space is selected in the Spaces area
- **THEN** the footer shows `R run now`, and each hint is complete

#### Scenario: Key help at 140 columns
- **WHEN** the window is 140 × 34 and the user presses `?`
- **THEN** each line of the key help shows as one line, and the line of the Feed keys starts with "Feed"

## REMOVED Requirements

### Requirement: Conversations tab
**Reason**: The Spaces area, the spaces dialog and the conversation search replace it. Each conversation has a set of spaces, so a move is not a separate action.
**Migration**: Use `/` to find a conversation and set its spaces, or `a` in a space to add conversations.

### Requirement: Spaces tab
**Reason**: Replaced by "Spaces area".
**Migration**: `i2a config` opens the Spaces area.

## ADDED Requirements

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
