## Context

`internal/notify` has one notifier: `osascript -e 'display notification ...'` with the title and the text as script arguments. `i2a card add` (`card.Plan`), `i2a reply` and the daemon (failed dispatch, WhatsApp not paired) call it. macOS shows these notifications as Script Editor: its icon, a click opens Script Editor, and the Persistent style is the Script Editor setting.

Facts from the explore spikes on macOS 26.6.2 (evidence in the gate, not in the repo):

- A bare binary cannot post with UserNotifications (`UNErrorDomain error 1`); the human decided not to ship an app bundle now.
- iTerm2 3.7.6 shows OSC 9 (`ESC ] 9 ; text BEL`) as a notification with the title "Alert" and the body "Session <name> #1: <text>". A click focuses the session. iTerm2 ignores OSC 777. Only a process that owns the terminal can write OSC 9: the daemon and the agent shell have no terminal (`/dev/tty`: "device not configured").
- terminal-notifier 3.1.0 (Homebrew) uses UserNotifications from its own bundle. `-title`, `-subtitle`, `-message`, `-contentImage <png>` (image at the right), `-open <url>` (works on a click), `-activate <bundle id>`, `-group <id>` (replaces the older notification of the group). Its icon cannot change.
- `open mattermost://<host>/<team>/pl/<post>` opens that post in Mattermost Desktop. `open whatsapp://send?phone=<n>` opens the chat in WhatsApp Desktop. A WhatsApp group has no link.
- Orca 1.4.240 has no OSC 9 handler and no `orca notify`. A plugin with the capability `notifications:show` can call `orca.host.call('notifications.show', {title, body})`; Orca shows `<publisher>.<id>: <title>` and also sends it to paired phones. An outside process calls a plugin command with the runtime RPC `plugins.invokeCommand` (`{pluginKey, commandId, args}`, permission `host-admin`) on the Unix socket named in `~/Library/Application Support/orca/orca-runtime.json`, with its `authToken`, one JSON line per request. The plugin system is experimental and off by default (`pluginSystemEnabled`).

## Goals / Non-Goals

**Goals:**
- Card notifications in iTerm2 while the Feed is open.
- Notifications with a useful click and the i2a icon image when the Feed is closed.
- Notifications in Orca for a user who uses Orca and turns on its plugins.
- A plain card title everywhere i2a shows it.
- Help to install terminal-notifier.

**Non-Goals:**
- An i2a app bundle, a custom app icon, or a native API call from Go.
- Terminals other than iTerm2 (WezTerm showed nothing; the others are not tested).
- A click on an iTerm2 or Orca notification that opens the source app.
- Linux notifications.

## Decisions

1. **One `Note` type for all notifiers.** `notify.Note{Title, Subtitle, Body, Open, Activate, Group}` replaces `(title, text)`. `Notifier` becomes `func(Note)`. Each notifier uses the fields that it supports. The osascript notifier passes the subtitle to `display notification ... subtitle`.

2. **Background chain: Orca, then terminal-notifier, then osascript.** `[notify] via = "auto"` (the default) tries the Orca plugin, then terminal-notifier, then `osascript`, and stops at the first one that succeeds. `via = "orca" | "terminal-notifier" | "osascript"` uses one notifier and falls back to `osascript` when it fails, so no notification is lost. Orca is first because the user installs and allows the plugin on purpose, and Orca also sends it to the phone. Alternative: terminal-notifier first, because its click opens the source. Rejected for `auto`: a user with the Orca plugin wants Orca; `via = "terminal-notifier"` keeps the click.

3. **The Feed sends the card notifications in iTerm2.** When `[notify] enabled` is on, `[notify] terminal` is `iterm2` (the default; `off` turns it off), and the Feed runs in iTerm2 (`TERM_PROGRAM=iTerm.app`, or `LC_TERMINAL=iTerm2` inside tmux), the Feed takes `<data>/feed-notify.lock` (`flock`, not blocking) and writes OSC 9 with `tea.Raw`. Inside tmux, the Feed wraps the sequence in the tmux passthrough (`ESC P tmux; ... ESC \`, each ESC doubled) and takes the lock only when `tmux show-options -gv allow-passthrough` is `on` or `all`. Without the lock, nothing changes. The OSC 9 text is `<CATEGORY> · <urgency> · <group> · <space>: <plain title>`; i2a removes control characters and BEL from it.
   - `i2a card add` tries the lock. When another process holds it, `i2a card add` shows no card notification and prints "Notifications: the Feed shows them.". Alternative: write OSC 9 to the TTY of the Feed from `i2a card add`. Rejected: the bytes can land inside a frame of the Feed and break the screen.
   - The Feed finds the notification cards by comparing the cards of each refresh (2 seconds) with the cards of the last refresh: a new ID is `added`; a higher urgency or a change from `done` is `updated`. It then uses `card.Plan` with `[notify] min_urgency`. The first load gives no notification.
   - The failed dispatch and the reply notifications stay in the background chain: the Feed does not see them.

4. **Plain title at display time.** `render.PlainText(md string) string` parses with goldmark (already in `go.mod` through glamour) and keeps the text of text, code spans, emphasis, link text and image alt text; it drops raw HTML and joins the lines with one space. `i2a card add` does not change the stored title, so the cards in the store today also show as plain text. Alternative: strip in `card.Prepare`. Rejected: it loses the text of the agent and does not fix the stored cards.

5. **Layout.** Title `<CATEGORY> · <urgency>` (+ ` · updated`), subtitle `<group> · <space>` (each part when it is set), body the plain title, not cut (the card limit is 200 characters). The grouped note keeps `i2a: <n> new cards`. The Orca note puts the subtitle in front of the body, because Orca has no subtitle. "i2a" is not in the title of terminal-notifier: the image shows the i2a icon. It stays in the osascript title, because Script Editor is the sender there.

6. **Click target** (terminal-notifier only). The URL of the newest ref message that has one; never text from the card.
   - Mattermost `https://<host>/<team>/pl/<post>` → `mattermost://<host>/<team>/pl/<post>` when `Mattermost.app` is in `/Applications` or `~/Applications`, else the https URL.
   - WhatsApp `https://wa.me/<n>` → `whatsapp://send?phone=<n>` when `WhatsApp.app` is installed, else the https URL.
   - A WhatsApp message with no URL (a group) → `-activate net.whatsapp.WhatsApp` when the app is installed.
   - `-open` accepts only `https`, `mattermost` and `whatsapp` URLs. The reply note opens the URL of the sent post. The grouped note and the daemon notes have no target.

7. **Icon.** Design A of the explore (purple tray and diamond) as `internal/notify/assets/icon.svg`, rendered to `icon.png` (256 × 256) by `scripts/icon.sh` (`rsvg-convert`), embedded with `go:embed`. i2a writes it to `<data>/icon.png` when the file is missing or different, and passes it as `-contentImage`.

8. **terminal-notifier lookup.** `exec.LookPath`, then `/opt/homebrew/bin` and `/usr/local/bin` (the LaunchAgent PATH can be short). i2a runs it with arguments, never through a shell, with a 5 second timeout. A message that starts with `[` gets a `\` in front (terminal-notifier needs this).

9. **Orca plugin in `extensions/orca/`.** `extensions/` is the folder for plugins of external apps; each app has one folder. `extensions/orca/orca-plugin.json`: publisher `i2a`, id `notify` (Orca shows `i2a.notify: <title>`; `inbox2agent.i2a` was longer), `pluginApi: 1`, `main: main.mjs`, the command `notify`, the capability `notifications:show`. `main.mjs` checks that `title` and `body` are strings, cuts them to 200 and 1,000 characters, and calls `notifications.show`. The package `extensions` embeds the folder; `i2a notify setup` writes it to `<data>/extensions/orca/` and prints the steps to install it in Orca (Settings → Plugins → turn on → install from that folder → allow the capability).
   - `internal/notify/orca.go` reads `orca-runtime.json` (`$ORCA_USER_DATA_PATH` or `~/Library/Application Support/orca`), takes the first `unix` transport, connects with a 3 second timeout, writes `{"id","authToken","method":"plugins.invokeCommand","params":{"pluginKey":"i2a.notify","commandId":"notify","args":{"title","body"}}}` and a newline, and reads one line. An error, a missing file, a refused connection or `ok: false` is a failure, and the chain goes on. The token never goes to a log.

10. **`i2a notify`.** `setup`: shows the notifier of each kind (found or not, version), asks "Install terminal-notifier with Homebrew? [y/N]" when it is missing and `brew` is in PATH, runs `brew install terminal-notifier`, writes the Orca plugin, and sends a test note through the chain. `status`: prints the same lines without changes. `test`: sends one test note and prints the notifier that showed it. The Settings tab shows the line of `status` for the notifier in use and the hint "run i2a notify setup".

## Risks / Trade-offs

- [The Feed holds the lock but the user does not look at iTerm2] → The user selected this; `[notify] terminal = "off"` gives the background chain again.
- [tmux without `allow-passthrough`] → The Feed does not take the lock, so the background chain shows the card.
- [The Feed stops between the lock check of `i2a card add` and its next refresh] → One card can get no notification. It still shows in the Feed. Low risk.
- [Orca plugin API changes; it is experimental] → Any error falls back to terminal-notifier or osascript. The plugin and the RPC call are in two small files.
- [Orca shows `i2a.notify: <title>`] → Accepted; the plugin key is short.
- [terminal-notifier is a third-party tool] → It is optional, from Homebrew core, and i2a installs it only after a yes.
- [The click opens a URL in another app] → Only stored message URLs and three schemes.

## Migration Plan

No data migration. A config without `[notify] via` and `[notify] terminal` gets `auto` and `iterm2`. `terminal` is a string, not a bool: the decoder gives `false` for a missing bool, and the schema builder has no pointer type. Rollback: remove `via` and `terminal` from `[notify]` in `config.toml`, then install the older binary. The older binary rejects unknown keys (`DisallowUnknownFields` in `config.Parse`), and a save of the new binary writes the two keys. The files in `<data>` (`icon.png`, `feed-notify.lock`, `extensions/`) do no harm to the older binary.

## Open Questions

- The exact error shape of `plugins.invokeCommand` when the plugin system is off: confirm against the running Orca in apply.
