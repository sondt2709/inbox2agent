## ADDED Requirements

### Requirement: Notifier chain
On macOS, the system SHALL show each background notification (cards of `i2a card add` when the Feed does not show them, the reply of `i2a reply`, the failed dispatch and the WhatsApp account that is not paired) through one notifier. With `[notify] via = "auto"` the system SHALL try the Orca plugin, then terminal-notifier, then `osascript`, and SHALL stop at the first notifier that succeeds. With `via = "orca"`, `"terminal-notifier"` or `"osascript"`, the system SHALL use that notifier, and SHALL use `osascript` when it fails. The system SHALL run each notifier with arguments, never through a shell, and SHALL stop it after 5 seconds. `I2A_NOTIFY=off` and `[notify] enabled = false` SHALL turn all notifications off.

#### Scenario: Only osascript is available
- **WHEN** `via = "auto"`, Orca is not running and terminal-notifier is not installed
- **THEN** the notification shows through `osascript`

#### Scenario: Orca plugin installed
- **WHEN** `via = "auto"`, Orca runs with the i2a plugin allowed, and terminal-notifier is installed
- **THEN** Orca shows the notification, and terminal-notifier shows nothing

#### Scenario: Selected notifier fails
- **WHEN** `via = "terminal-notifier"` and terminal-notifier is not installed
- **THEN** the notification shows through `osascript`

### Requirement: Plain title
The system SHALL convert the Markdown of a card title to plain text when it shows the title: in the Cards rows, the Card panel, the status line and each notification. The plain text SHALL keep the text of emphasis, code spans, link text and image alt text, SHALL drop raw HTML, and SHALL join the lines with one space. The store SHALL keep the title as the agent wrote it.

#### Scenario: Markdown title
- **WHEN** a card has the title "**Deploy** of `api` failed on [prod](https://example.com)"
- **THEN** the Feed row and the notification show "Deploy of api failed on prod"

### Requirement: terminal-notifier notification
A terminal-notifier notification SHALL have the title, the subtitle and the body of the note, the i2a icon as `-contentImage` from `<data>/icon.png`, and the group of the note. The system SHALL write `<data>/icon.png` from the embedded icon when the file is missing or different. The system SHALL find terminal-notifier in `PATH`, then in `/opt/homebrew/bin` and `/usr/local/bin`.

#### Scenario: Card notification through terminal-notifier
- **WHEN** a `high` question card of the space Work with the group `release` and the title "Check the deploy" gets a notification through terminal-notifier
- **THEN** the notification has the title "QUESTION · high", the subtitle "release · Work", the body "Check the deploy" and the i2a image

### Requirement: Click target
A terminal-notifier notification of a card SHALL open the source of the newest ref message that has a URL. A Mattermost permalink SHALL open as `mattermost://<host>/<team>/pl/<post>` when `Mattermost.app` is in `/Applications` or `~/Applications`, else as the https URL. A WhatsApp `https://wa.me/<number>` URL SHALL open as `whatsapp://send?phone=<number>` when `WhatsApp.app` is installed, else as the https URL. A WhatsApp card with no URL SHALL activate `net.whatsapp.WhatsApp` when the app is installed. The system SHALL open only URLs with the scheme `https`, `mattermost` or `whatsapp`, and only URLs from the store, never text of a card. A grouped notification and a daemon notification SHALL have no click target.

#### Scenario: Mattermost card with Mattermost Desktop
- **WHEN** the newest ref message of a card has the URL `https://chat.example.com/team/pl/abc` and `Mattermost.app` is installed
- **THEN** a click on the notification opens `mattermost://chat.example.com/team/pl/abc`

#### Scenario: WhatsApp group card
- **WHEN** the ref messages of a card are from a WhatsApp group and `WhatsApp.app` is installed
- **THEN** a click on the notification brings WhatsApp to the front

#### Scenario: Card text with a link
- **WHEN** the body of a card has the link `file:///etc/passwd` and its ref messages have no URL
- **THEN** the notification has no click target

### Requirement: Feed notifications in iTerm2
When `[notify] enabled` is `true`, `[notify] terminal` is `iterm2`, and the Feed runs in iTerm2 (`TERM_PROGRAM=iTerm.app`, or `LC_TERMINAL=iTerm2` inside tmux), the Feed SHALL hold `<data>/feed-notify.lock` and SHALL show the card notifications as OSC 9 in its terminal. Inside tmux, the Feed SHALL do this only when `allow-passthrough` is `on` or `all`, and SHALL wrap the sequence for the tmux passthrough. The Feed SHALL compare the cards of each refresh with the cards of the last refresh and SHALL use the rule of the requirement "Card notifications" of the capability `card-feed`. The first load SHALL give no notification. The OSC 9 text SHALL be the title, the subtitle and the body of the card note in one line, `<CATEGORY> · <urgency> · <group> · <space>: <plain title>` with each part that is set, with no control characters. While a process holds the lock, `i2a card add` SHALL NOT show card notifications and SHALL print "Notifications: the Feed shows them.".

#### Scenario: Card while the Feed is open in iTerm2
- **WHEN** the Feed is open in iTerm2 and an agent adds a `high` action card
- **THEN** iTerm2 shows one notification with "ACTION · high" and the plain title within 3 seconds, and terminal-notifier shows nothing

#### Scenario: Feed in another terminal
- **WHEN** the Feed is open in Terminal.app and an agent adds a `high` card
- **THEN** the Feed does not hold the lock, and the background chain shows the notification

#### Scenario: tmux without passthrough
- **WHEN** the Feed runs in tmux inside iTerm2 and `allow-passthrough` is `off`
- **THEN** the Feed does not hold the lock

### Requirement: Orca plugin
The folder `extensions/` SHALL hold the plugins of external apps, one folder for each app. `extensions/orca/` SHALL be an Orca plugin with the publisher `i2a`, the ID `notify`, the command `notify` and only the capability `notifications:show`. The command SHALL accept `title` and `body` strings, cut them to 200 and 1,000 characters, and show them with `notifications.show`. The Orca notifier SHALL read the socket and the `authToken` from `orca-runtime.json` in `$ORCA_USER_DATA_PATH` or `~/Library/Application Support/orca`, send one `plugins.invokeCommand` request for `i2a.notify` and `notify`, and wait at most 3 seconds. An error, a missing file or a refused connection SHALL be a failure. The system SHALL NOT write the `authToken` to a log or an error.

#### Scenario: Plugin allowed
- **WHEN** Orca runs, its plugin system is on, and the i2a plugin is installed and allowed
- **THEN** `i2a notify test` shows the test note in Orca and prints "orca"

#### Scenario: Plugin system off
- **WHEN** Orca runs with the plugin system off
- **THEN** the Orca notifier fails within 3 seconds, and the chain goes on to terminal-notifier

### Requirement: Notify command
`i2a notify setup` SHALL show each notifier (Orca plugin, terminal-notifier, osascript) as found or not found. When terminal-notifier is not found and `brew` is in `PATH`, it SHALL ask "Install terminal-notifier with Homebrew? [y/N]" and run `brew install terminal-notifier` only after `y`. It SHALL write the Orca plugin to `<data>/extensions/orca/`, print the steps to install it in Orca, and send a test note. `i2a notify status` SHALL print the notifiers and change nothing. `i2a notify test` SHALL send one test note and print the name of the notifier that showed it.

#### Scenario: Setup without terminal-notifier
- **WHEN** terminal-notifier is not installed, `brew` is in `PATH`, and the user answers `y`
- **THEN** `i2a notify setup` runs `brew install terminal-notifier`, and `i2a notify status` then shows terminal-notifier as found

#### Scenario: Setup answer no
- **WHEN** terminal-notifier is not installed and the user answers `n`
- **THEN** nothing is installed, and the test note shows through the next notifier
