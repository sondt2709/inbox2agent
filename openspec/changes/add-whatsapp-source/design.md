## Context

i2a has one source type, Mattermost. The Mattermost source asks the server for new posts on each poll (`GET /channels/{id}/posts`). WhatsApp works in a different way:

- WhatsApp has no public API for a personal account. A client connects as a "linked device" over one WebSocket, after the user scans a QR code or types a pairing code on the phone.
- The server pushes messages to the connected device. When the device is offline, the server keeps the messages and sends them at the next connect (`events.OfflineSyncCompleted` marks the end). There is no "get the history of a chat" call. After pairing, the phone sends a history sync once.
- One device can have one connection. A second connection with the same keys replaces the first (`events.StreamReplaced`).
- The phone unlinks a device that is offline for about 14 days.

whatsmeow (`go.mau.fi/whatsmeow`) implements this protocol in Go. Other WhatsApp bridges use it with `mattn/go-sqlite3` (CGO). A spike showed that whatsmeow builds with `CGO_ENABLED=0` and `modernc.org/sqlite`, the driver of i2a (evidence: `tmp/evidence/add-whatsapp-source/before/cgo-free-spike.txt`, not committed).

## Goals / Non-Goals

**Goals:**
- Collect WhatsApp messages from the direct chats and groups that the user selects, and group messages that mention the user, into the existing pipeline (store, dispatch, cards, ask).
- Pair from the TUI and from the CLI.
- Keep one static binary and keep `inbox.db` unchanged.
- Never change the read state on the phone.

**Non-Goals:**
- Send replies to WhatsApp (change `add-whatsapp-reply`).
- Download media, transcribe voice messages, or read WhatsApp channels (newsletters) and status updates.
- Edits, deletes and reactions.
- Use the data of another WhatsApp bridge.

## Decisions

### 1. The process that collects holds the connection
The daemon (or `i2a run --once`) opens the connection on the first `Collect` and keeps it in the source object, which the engine already caches in `Engine.sources`. The two processes already exclude each other with `daemon.Lock`. A WhatsApp source also takes a lock file `<data>/whatsapp-<id>.lock` while it is connected, so the TUI and `i2a source remove` know when the daemon holds the connection.

Alternatives: a separate helper process (more parts to install and watch); reading the database of another WhatsApp bridge (depends on a tool that i2a does not control).

### 2. One file for each source, written by the event handler
`<data>/whatsapp-<id>.db` holds the whatsmeow tables (`sqlstore`, with the same `*sql.DB`) and two i2a tables:

- `wa_messages(rowid, chat, id, sender, sender_name, phone, kind, from_me, mention, ts, text, raw)`: one row for each received message. `text` and `mention` are extracted when the message arrives; `raw` is the protobuf of `waE2E.Message`. Primary key `(chat, id)`; an insert ignores a duplicate.
- `wa_chats(jid, name, kind, last_ts)`: the chats for the picker, from the history sync, the joined groups, the contacts and the live messages.

The whatsmeow event handler writes these rows when a message arrives, also from a history sync. whatsmeow acknowledges a message to the server after the handler returns, so a crash before the next poll loses no message. `Collect` reads the rows after the cursor `{"rowid": "<n>"}` and returns `inbox.Message` values. The rule "a source never writes to `inbox.db`" stays true.

The file keeps all received messages for `lookback` (plus one hour) and then deletes them. So a chat that the user selects later gives the messages of the last `lookback`, as a Mattermost channel does. The raw protobuf lets a later version extract more fields, and it is the source of the test fixtures.

The file and its directory get the mode 0600 and 0700. The keys cannot go in the keychain: whatsmeow reads and writes them on each message.

Alternative: filter at receive time and keep only the selected chats. Rejected by the stakeholder: a chat selected later would show no messages.

### 3. Sender names at receive time, chat names in `wa_chats`
The handler resolves the sender name (contact name from the contact store, then the push name, then `+<phone>`) and the phone number of a direct chat, and stores them in the row. A sender with a `@lid` address is mapped to the phone number with `SenderAlt`, `RecipientAlt` or the LID store of whatsmeow. The chat names go to `wa_chats` (group names from `GetJoinedGroups` and `GetGroupInfo` in a goroutine, contact names, history sync names), and `Collect` joins them when it reads. So a group name that arrives after the message is still used. `Collect` does not need a connection.

### 4. Filters and fields
`Collect` returns a row when it is not from the user, its time is inside `lookback`, its text is not empty, and one of these is true:

- the chat is in `channels`;
- the chat is a direct chat (`@s.whatsapp.net` or `@lid`) and `all_direct` is on;
- the chat is a group, `mentions` is on, and the message mentions the user (`ContextInfo.MentionedJID` has the phone or LID user of the device) or quotes a message of the user (`ContextInfo.Participant`).

`status@broadcast`, broadcast lists, newsletters, reactions, protocol messages (edit, delete), poll votes and edits are skipped at receive time.

Field mapping: `ExternalID` the message ID; `ChannelID` the chat JID; `ChannelName` the chat name; `ChannelKind` `direct` or `group`; `ThreadID` empty; `Author` the sender name; `URL` `https://wa.me/<phone>` for a direct chat with a known phone number, else empty; `CreatedAt` the message time.

Text: `conversation`, `extendedTextMessage.text`, captions of image, video and document, with a placeholder in front: `[image]`, `[video]`, `[document: <file name>]`, `[voice message]`, `[audio]`, `[sticker]`, `[contact: <name>]`, `[location: <name>]`, `[poll] <question>`. A quoted message adds one line `> <first 200 characters of the quoted text>` before the text.

### 5. Pairing runs in the TUI or CLI process
Pairing needs a connection and a new device, so the daemon cannot do it (the source is not in the configuration yet). The TUI and `i2a source add whatsapp` open the file of the new ID, connect, and show the QR codes from `GetQRChannel` (a new code each 20 seconds) or one pairing code from `PairPhone`. After `PairSuccess`, they wait for the joined groups and the first history sync (at most 30 seconds), read `wa_chats`, disconnect, and release the lock. Then the user selects chats and saves; the daemon reloads the configuration and connects.

The device name on the phone is "i2a" (`store.DeviceProps.Os`). `PairPhone` needs a display name in the form `Browser (OS)` from a list that WhatsApp accepts, so it uses "Chrome (Mac OS)".

The QR code of WhatsApp needs about 33 character rows. The pairing panel needs 14 rows more than the QR code, so about 47 rows; a shorter window shows the number of rows that it needs and points to the pairing code.

The QR code is rendered with `rsc.io/qr` and half-block characters (`▀`, `▄`, `█`), black on white, with a quiet zone of 2 modules.

### 6. Sources can be closed
`source.Source` gets an optional interface `io.Closer`. `Engine.SetConfig` closes the old sources before it drops them, and a new `Engine.Close` closes all of them; the daemon and `i2a run --once` call it at the end. A WhatsApp source disconnects and releases its lock in `Close`.

### 7. `source.Build` gets an environment
`Build(sc, sec, lookback, hc)` becomes `Build(sc, env)` with `source.Env{Secrets, Lookback, HTTP, DataDir, Log}`. A WhatsApp source needs the data directory and a logger; it needs no token. `BuildReplier` returns `ErrCannotReply` for a WhatsApp source before it builds it, so a reply check never opens a connection.

### 8. Run once
On the first `Collect` after connect, the source waits for `OfflineSyncCompleted` (at most 30 seconds), so `i2a run --once` gets the messages that arrived while i2a was off.

### 9. Not paired
When the file has no device row (never paired, or whatsmeow deleted it after `LoggedOut`), `Build` returns an error that wraps `source.ErrNotPaired` with the text "not paired: pair again in i2a config". The engine shows one notification for each source when this error starts, and logs the error at each poll as for other source errors. `i2a status` and the Sources tab read the device row and show "paired" or "not paired".

### 10. Remove logs out
`i2a source remove` and `x` in the Sources tab save the configuration first. Then they wait up to 5 seconds for the lock of the source (the daemon closes the source at reload), connect, call `Logout`, and delete the file. When the lock or the logout fails, they delete the file and tell the user to remove "i2a" in Settings › Linked devices on the phone.

### 11. Read state
whatsmeow sends no read receipt and no presence unless the code calls `MarkRead` or `SendPresence`. i2a never calls them. whatsmeow sends delivery receipts (two grey ticks), as every linked device does.

## Risks / Trade-offs

- [WhatsApp bans the account for an unofficial client] → Receive only; no presence; no automatic sends in this change. README states the risk.
- [A WhatsApp protocol change breaks whatsmeow] → The source logs the error and the other sources continue. Update the exact whatsmeow version in `go.mod`.
- [A copy of `whatsapp-<id>.db` gives full access to the account] → Mode 0600 in a 0700 directory; `remove` logs out and deletes the file.
- [The release binary grows from 16.6 MB to 26.4 MB] → Accepted by the stakeholder (estimate: about 9 MB).
- [CI cannot test the real connection] → The logic behind `Collect` and the text extraction take a fake event stream and real fixtures. A local scenario with `I2A_E2E_WHATSAPP=1` covers pairing and receive.
- [A history sync can be large on the first pairing] → Rows older than `lookback` are skipped at receive time and are not stored.
- [The daemon is off for about 14 days and the phone unlinks i2a] → The notification and "not paired" tell the user to pair again.

## Migration Plan

No migration. A configuration without WhatsApp sources works as before. Rollback: revert the commit; remove the WhatsApp sources from `config.toml` first, because an older binary rejects the type `whatsapp`.

## Open Questions

- Does the phone still show notifications while i2a is connected? Expected yes (no presence). Check during the local test.
