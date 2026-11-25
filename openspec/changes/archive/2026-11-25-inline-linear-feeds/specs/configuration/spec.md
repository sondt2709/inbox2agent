## MODIFIED Requirements

### Requirement: Feed form of a Linear account
The detail panel of a Linear account SHALL show each feed of the account as a section under the account fields, with a header that names the feed and the spaces that list it (else Everything else), the fields Name, Kind (Comments or Inbox), From, Prefix, Projects, Labels and Teams, and the button "Remove feed <id>". The button "+ Add feed" SHALL add a new section with the field Feed ID; the ID of a saved feed SHALL NOT change. "Save changes" SHALL save the account and all its feeds in one save with the checks of the configuration, and SHALL remove the entries of a removed feed in all spaces. "Remove feed <id>" SHALL hide the section and become "Keep feed <id>" until the save. The save SHALL refuse an account with no feed. A new feed SHALL be a conversation in Everything else. No key and no dialog SHALL be necessary to see or change a feed.

From, Projects, Labels and Teams SHALL show a "Choose" button. A click on it, or enter in the field, SHALL open a list of the names of the workspace with check marks. The user SHALL also type names separated by commas. In the list, typing SHALL filter the rows, enter SHALL switch the check mark of a row, ctrl+s SHALL apply the checked names to the field, and esc SHALL cancel. When no row matches the text, the list SHALL offer to add the text as a name. When the names cannot be read, the list SHALL show the error and still accept typed names. A team row SHALL show the key and the name, and SHALL save the key. The From list SHALL start with `me`. The names that were checked when the list opened SHALL come first.

#### Scenario: Feeds in the account panel
- **WHEN** the user selects a Linear account with the feeds `agent` and `inbox`
- **THEN** the panel shows the sections of `agent` and `inbox` with their fields, the buttons "Remove feed agent", "Remove feed inbox" and "+ Add feed", and no key is needed

#### Scenario: Filter a feed by label
- **WHEN** the user selects "Choose" of Labels in the section `agent`, types `inf`, presses enter on `Infra`, presses ctrl+s and selects "Save changes"
- **THEN** `config.toml` has `labels = ["Infra"]` in the feed `agent`

#### Scenario: Typed project
- **WHEN** the user types `Bridge, Mobile` in Projects of `agent` and selects "Save changes"
- **THEN** the feed has `projects = ["Bridge", "Mobile"]`

#### Scenario: New feed
- **WHEN** the user selects "+ Add feed", types the ID `bridge` and the prefix `>agent`, and selects "Save changes"
- **THEN** the account has the feed `bridge` of the kind `comments`, and the conversation `bridge` is in Everything else

#### Scenario: Remove a feed
- **WHEN** the space `triage` lists the feed `inbox`, and the user selects "Remove feed inbox" and "Save changes"
- **THEN** the account has no feed `inbox`, and `triage` has no entry for it

#### Scenario: Remove the last feed
- **WHEN** the account has only the feed `agent`, and the user selects "Remove feed agent" and "Save changes"
- **THEN** the TUI shows that a Linear account needs one feed or more, and nothing changes

#### Scenario: Names cannot be read
- **WHEN** the name query fails and the user opens the Labels list
- **THEN** the list shows the error, and the user can type `infra` and add it
