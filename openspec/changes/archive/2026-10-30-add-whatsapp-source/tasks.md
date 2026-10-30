## 1. Configuration and source plumbing

- [x] 1.1 Add `config.SourceWhatsApp`, `NewWhatsAppSource` and the validation (type list, `url` only for Mattermost)
- [x] 1.2 Change `source.Build` to `Build(sc, env)` with `source.Env{Secrets, Lookback, HTTP, DataDir, Log}`; update the engine, `BuildReplier` (no build for WhatsApp) and the tests
- [x] 1.3 Close sources: `Engine.SetConfig` closes old sources that implement `io.Closer`; new `Engine.Close`; call it in `i2a daemon` and `i2a run --once`

## 2. WhatsApp source package

- [x] 2.1 Add `go.mau.fi/whatsmeow` and `rsc.io/qr` with exact versions; check `CGO_ENABLED=0 task build`
- [x] 2.2 `internal/source/whatsapp/db.go`: open `<data>/whatsapp-<id>.db` (0600, WAL), the whatsmeow container on the same `*sql.DB`, the tables `wa_messages` and `wa_chats`, prune
- [x] 2.3 `extract.go`: text, placeholders, quote line, mentions and quotes of the user, skip rules
- [x] 2.4 `conn.go`: lock file, connect, event handler (live messages, history sync, groups, contacts, names, LID mapping), offline-sync wait, close, logout; device name "i2a"
- [x] 2.5 `source.go`: `Collect` from the rowid cursor with the filters and the field mapping; `ErrNotPaired`; `State` (paired, not paired)
- [x] 2.6 `pair.go`: QR channel and phone code, wait for the chats after `PairSuccess`; `qr.go`: half-block QR rendering
- [x] 2.7 `ListChats` for the picker and `i2a source channels`
- [x] 2.8 Capture tool `internal/source/whatsapp/testdata/capture` (Go program) that exports rows of `wa_messages` as fixtures with names, numbers and text replaced

## 3. CLI

- [x] 3.1 `i2a source add whatsapp --id <id> [--phone <number>] [--channels] [--all-direct] [--mentions] [--profile]`
- [x] 3.2 `i2a source channels` and `i2a source list` for WhatsApp
- [x] 3.3 `i2a source remove`: logout and delete the file for WhatsApp
- [x] 3.4 `i2a status`: `paired` or `not paired`

## 4. Daemon

- [x] 4.1 One notification "i2a: WhatsApp not paired" for each source until a successful poll
- [x] 4.2 Route whatsmeow logs to the daemon logger (warnings and errors)

## 5. TUI

- [x] 5.1 Add form: Type field (Mattermost, WhatsApp); hide URL and Token for WhatsApp; Phone number field
- [x] 5.2 Pairing panel: QR code or pairing code, new QR code each time WhatsApp sends one, timeout, cancel with `esc`
- [x] 5.3 Step 2 for WhatsApp: "Paired", chat picker, All direct, Mentions, Profile, Add source
- [x] 5.4 Form of a saved WhatsApp source: state, chat picker from the file, no URL and no Token; `x` logs out
- [x] 5.5 Overview and Sources list show the WhatsApp state

## 6. Tests

- [x] 6.1 Unit tests: config validation; extraction and filters with captured fixtures; `Collect` cursor, lookback and prune with a fake event stream; QR rendering; engine close and notification; TUI add form keys. Break each new test one time
- [x] 6.2 e2e (CI): `type = "whatsapp"` is valid; the add form shows the WhatsApp fields; `i2a status` shows `not paired`
- [x] 6.3 Local scenario S35, by hand because it needs a phone scan: pair, receive a direct message, a mention, a group message with no mention, an image with a caption; `i2a run --once`

## 7. Docs

- [x] 7.1 README: WhatsApp setup, the ban risk, the key file, the limits
- [x] 7.2 CLAUDE.md: WhatsApp facts and the package in the architecture
- [x] 7.3 `test/e2e/SCENARIOS.md`

## 8. Evidence

- [x] 8.1 Run the test plan on the last commit; save the results in `tmp/evidence/add-whatsapp-source/after/`
- [x] 8.2 Run `task leakcheck` and a scan for phone numbers, JIDs and names before each commit
