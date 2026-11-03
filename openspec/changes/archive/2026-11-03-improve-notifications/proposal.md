## Why

The notifications of i2a come from `osascript`. macOS shows them with the Script Editor icon, a click opens Script Editor, and the text shows the raw Markdown of the card title (`**`, backticks, `[text](url)`). The text is cut after 4 lines. The Cards panel of the Feed also shows the raw Markdown title. The user wants i2a notifications in iTerm2 while the Feed is open, notifications through terminal-notifier and Orca when the Feed is closed, a click that opens the source conversation, and a plain title.

## What Changes

- The card title shows as plain text: the Markdown is converted when i2a shows the title (Cards rows, Card panel, status line, all notifications). The store keeps the text of the agent.
- The Feed sends the card notifications as OSC 9 to its terminal when it runs in iTerm2. While it does this, `i2a card add` does not show the card notifications.
- A new notifier chain for the other notifications: the Orca plugin of i2a, then terminal-notifier, then `osascript`. `[notify] via` selects one notifier or `auto`.
- terminal-notifier notifications have a title, a subtitle, the full plain title in the body, the i2a icon as an image, and a click target: the Mattermost post (Mattermost Desktop or the browser), the WhatsApp chat, or the WhatsApp app.
- A new folder `extensions/` holds plugins of external apps. `extensions/orca/` is an Orca plugin with the command `notify`. i2a calls it through the Orca runtime socket (`plugins.invokeCommand`).
- New command `i2a notify` with `setup` (find or install terminal-notifier with Homebrew, write the Orca plugin, send a test), `status` and `test`.
- New fields `[notify] via` and `[notify] terminal` in `config.toml` and in the schema.
- The Settings tab shows the notifier that i2a uses.
- The notification text changes: title `<CATEGORY> · <urgency>`, subtitle `<group> · <space>`. **BREAKING** for a user who filters notifications by the old title `i2a · <CATEGORY> · <urgency>`.

## Capabilities

### New Capabilities
- `desktop-notifications`: the notifiers (iTerm2 from the Feed, the Orca plugin, terminal-notifier, `osascript`), the selection, the click target, the plain title, `i2a notify` and the Orca plugin in `extensions/orca/`.

### Modified Capabilities
- `card-feed`: the card notifications use the new layout and move to the Feed when it sends them; the Feed rows and the Card panel show the plain title.
- `daemon-service`: the desktop notifications of the daemon use the notifier chain, not `osascript` only.
- `reply-send`: the reply notification uses the notifier chain, and a click opens the sent post.
- `configuration`: `[notify] via` and `[notify] terminal`, their validation, and the notifier in the Settings tab.

## Impact

- Code: `internal/notify`, `internal/card`, `internal/render`, `internal/config`, `internal/cli` (`card.go`, `reply.go`, new `notify.go`), `internal/daemon/engine.go`, `internal/tui` (`tab_feed.go`, Settings), new `extensions/`.
- External tools: terminal-notifier (optional, Homebrew); Orca 1.4.240 or later with the experimental plugin system on (optional).
- Files: `<data>/feed-notify.lock`, `<data>/icon.png`, `<data>/extensions/orca/`.
- No change to the store schema, the daemon API, CI or the release. No new Go module: goldmark is already in `go.mod` (indirect, through glamour).
