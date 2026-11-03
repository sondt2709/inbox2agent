## MODIFIED Requirements

### Requirement: Reply notification
On macOS, each successful send with `i2a reply` SHALL show one desktop notification through the notifier chain (see the capability `desktop-notifications`) with the title "i2a · the agent sent a reply" and the text `<channel>: <text>`, cut to 200 characters. A terminal-notifier notification SHALL open the sent post on a click, with the rule of the requirement "Click target" of the capability `desktop-notifications`. `[notify] enabled = false` SHALL turn it off. A send from the Feed SHALL NOT show a notification.

#### Scenario: Agent send
- **WHEN** an agent sends "Thanks Minh, noted." with `i2a reply` to a message in `pr-reviews`
- **THEN** a notification shows "i2a · the agent sent a reply" with "pr-reviews: Thanks Minh, noted."

#### Scenario: Click on the reply notification
- **WHEN** the reply notification shows through terminal-notifier and Mattermost Desktop is installed
- **THEN** a click opens the sent post in Mattermost Desktop
