## Why

Many requests to the user arrive in WhatsApp, not in Mattermost. i2a cannot read them, so the user must check WhatsApp by hand and the agent never sees these messages. WhatsApp is the next source on the plan of i2a.

## What Changes

- New source type `whatsapp`. i2a connects to the personal account of the user as a WhatsApp linked device, with the Go library whatsmeow. i2a stays one static binary (`CGO_ENABLED=0`).
- Pairing: in the Sources tab, the add form has a new field Type (Mattermost or WhatsApp). For WhatsApp, Connect shows a QR code in the detail panel. When the user types a phone number, Connect shows an 8-character pairing code instead.
- After pairing, the user selects the direct chats and groups to follow, with the channel picker of Mattermost. The switches "All direct" and "Mentions" work as for Mattermost. A group message that quotes a message of the user counts as a mention.
- New command `i2a source add whatsapp --id <id> [--phone <number>]`. `i2a source channels <id>` lists the WhatsApp chats.
- Each WhatsApp source has its own file `<data>/whatsapp-<id>.db` (mode 0600) with the device keys and the received messages. `inbox.db` does not change.
- The daemon keeps the WhatsApp connection open while it runs. `i2a run --once` connects, collects and disconnects.
- i2a never sends read receipts, typing state or online presence to WhatsApp.
- Media messages give a placeholder and the caption, for example `[image] plan for Friday`. i2a does not download media.
- When the phone unlinks i2a, the daemon shows one notification. `i2a status` and the Sources tab show "not paired".
- `i2a source remove` of a WhatsApp source logs out the linked device and deletes its file.
- `r` on a WhatsApp card says that the source cannot send replies yet. Replies come in a later change.

## Capabilities

### New Capabilities
- `whatsapp-source`: pairing, the connection, the message file, the filters, the message fields, the read state and the logout of a WhatsApp source.

### Modified Capabilities
- `configuration`: the type `whatsapp` in the validation; the Type field and the pairing in the add form; the chat picker for WhatsApp; `i2a source add whatsapp`; `i2a source remove` logs out.
- `daemon-service`: the daemon closes the source connections at configuration reload and at stop; a notification when a WhatsApp source is not paired any more.

## Impact

- Code: new `internal/source/whatsapp`; `internal/source` (`Build` gets the data directory and the logger); `internal/config`; `internal/cli` (`source`, `status`, `run`, `daemon`); `internal/tui/tab_sources.go`; `internal/daemon`.
- Dependencies: `go.mau.fi/whatsmeow` (MPL-2.0) and its modules, `rsc.io/qr` (BSD) for the QR code. The release binary grows by about 10 MB (16.6 MB to 26.4 MB).
- External systems: a persistent WebSocket from the daemon to WhatsApp. WhatsApp does not allow unofficial clients and can ban an account.
- Data: a new file `<data>/whatsapp-<id>.db` for each source, with the account keys. No change to `inbox.db`.
- Tests: unit tests with message fixtures from a real account (names and numbers replaced); CI scenarios for the not-paired state and the add form; a local scenario by hand (S35), because pairing needs a phone. CI does not connect to WhatsApp.
- No new secret in the keychain, no network port, no CI change.
