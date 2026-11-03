## 1. Plain title

- [x] 1.1 Test: `render.PlainText` for emphasis, code span, link, image, raw HTML, line breaks and plain text
- [x] 1.2 `render.PlainText` with goldmark; goldmark becomes a direct requirement in `go.mod`
- [x] 1.3 Test: the Feed row and the Card panel show the plain title (fails on the base)
- [x] 1.4 The Feed row, the Card panel and the "Done:" status line use the plain title

## 2. Note and notifiers

- [x] 2.1 `notify.Note` and `Notifier func(Note)`; update `i2a card add`, `i2a reply`, the daemon and their tests
- [x] 2.2 Test and code: `card.Plan` layout (title, subtitle with the space, plain body, `· updated`, grouped note)
- [x] 2.3 Test and code: the osascript notifier puts `i2a · ` before the title and passes the subtitle; text stays out of the script
- [x] 2.4 Test and code: terminal-notifier arguments (no shell, `\[` escape, `-contentImage`, `-group`, `-open`, `-activate`) and the lookup in PATH, `/opt/homebrew/bin`, `/usr/local/bin`
- [x] 2.5 Icon: `internal/notify/assets/icon.svg` (design A), `scripts/icon.sh`, `icon.png`, write `<data>/icon.png` when missing or different
- [x] 2.6 Test and code: click target (Mattermost and WhatsApp mapping, app check, scheme allow list, no target for grouped and daemon notes)
- [x] 2.7 Test and code: the chain (`auto` order, a selected notifier falls back to osascript, `I2A_NOTIFY=off`)

## 3. Orca plugin

- [x] 3.1 `extensions/orca/` (`orca-plugin.json`, `main.mjs`, `README.md`) and the package `extensions` that embeds it
- [x] 3.2 Test and code: the Orca notifier against a fake Unix socket (request line, 3 s timeout, `ok: false`, missing file; no token in errors)
- [ ] 3.3 Real run: install the plugin in Orca 1.4.240 and call it; record the real response and the error when the plugin system is off; adjust the parser to it

## 4. Configuration

- [x] 4.1 Test and code: `[notify] via` and `[notify] terminal` with defaults, validation and `doc` tags
- [x] 4.2 Write `docs/config.schema.json` again (`go test ./internal/config -run TestSchemaCopyIsCurrent -update`)

## 5. Feed notifications in iTerm2

- [x] 5.1 Test and code: iTerm2 detection (`TERM_PROGRAM`, `LC_TERMINAL`), tmux passthrough check and wrap, OSC 9 bytes with no control characters
- [x] 5.2 Test and code: the Feed diff of two refreshes gives the saved results for `card.Plan`; the first load gives none
- [x] 5.3 `<data>/feed-notify.lock`: the Feed holds it; `i2a card add` skips the card notifications and prints "Notifications: the Feed shows them." (test with a held lock)
- [x] 5.4 The Feed writes OSC 9 with `tea.Raw`

## 6. Notify command and Settings

- [x] 6.1 `i2a notify setup|status|test` (brew question, plugin files to `<data>/extensions/orca/`, test note, notifier name)
- [x] 6.2 Settings tab: `via`, `terminal`, and the notifier line with the hint
- [x] 6.3 README.md (notifications section) and CLAUDE.md (notify package, facts of terminal-notifier, iTerm2 and the Orca plugin)

## 7. Verification

- [ ] 7.1 `task test`, `task lint`, `task leakcheck`, `task e2e`
- [x] 7.2 Break each new test one time and confirm that it fails
- [ ] 7.3 Real runs on macOS with screenshots: iTerm2 Feed, terminal-notifier with click (Mattermost, WhatsApp), Orca plugin, `i2a notify setup` without terminal-notifier, daemon from the LaunchAgent, Feed rows before and after
