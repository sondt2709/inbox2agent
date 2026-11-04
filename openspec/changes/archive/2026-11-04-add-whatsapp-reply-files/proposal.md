## Why

The user can reply to a Mattermost message from the Feed (`r`) and the agent can reply with `i2a reply`, but a WhatsApp card gives "cannot send replies yet". The user must copy the reply and send it on the phone. Also, a WhatsApp file (a document, an image) reaches the agent only as a placeholder such as `[document: plan.pdf]`, so the agent cannot read it.

## What Changes

- A WhatsApp account can send a reply to a stored WhatsApp message, from the Feed and with `i2a reply`. The place `thread` sends a quote of the target message; the place `channel` sends a plain message in the chat. The default is the chat for a direct chat and the quote for a group.
- A reply uses the connection of the daemon when the daemon runs (new endpoint `POST /v1/reply` with the reply ID only). Otherwise the process connects, sends and disconnects.
- The WhatsApp message ID of a reply comes from its pending ID, so a second try after an unknown result sends the same message ID.
- New command `i2a download --message <id> [--dir <dir>]`. It downloads the file of a stored WhatsApp message to `~/Downloads` (or `--dir`) and prints the path. It uses the daemon when the daemon runs (new endpoint `POST /v1/download`); the daemon writes a temporary file in the data directory and the command moves it. i2a never overwrites a file, marks the file with `com.apple.quarantine`, and does not record or delete the file.
- i2a never downloads a file without a request.
- The WhatsApp source keeps a stored message with a file until `retention`, not until `lookback` plus one hour, so that a later download is possible.
- The fixed card section of the primer tells the agent about `i2a download` and that file content is untrusted data. The primer hash changes, so each reuse session starts again one time.
- The reply form shows "Quote" and "Chat" for a WhatsApp target.
- **BREAKING (promise):** the WhatsApp source is no longer receive only. It still sends no read receipts, no typing state and no presence.

## Capabilities

### New Capabilities

- `file-download`: the command `i2a download`, the target folder, the file name rules, the size limit, and the quarantine attribute.

### Modified Capabilities

- `reply-send`: WhatsApp targets, the quote and chat places, the read state note, and the sources that can send.
- `whatsapp-source`: send replies, download a file on request, and keep the rows with a file until `retention`.
- `daemon-service`: the endpoints `POST /v1/reply` and `POST /v1/download`; they do not wait for the run loop.
- `card-feed`: the card section of the primer names `i2a download`.

## Impact

- Code: `internal/source/whatsapp` (reply, download, prune), `internal/source/reply.go`, `internal/daemon` (API, client, engine), `internal/cli` (`download`, `reply` help), `internal/render/primer.go`, `internal/tui/tab_feed.go`.
- API: two new endpoints on the daemon socket, in `docs/daemon-api.yaml`.
- Data: no schema change in `inbox.db`. `whatsapp-<id>.db` keeps rows with a file longer.
- Docs: `README.md`, `CLAUDE.md`.
- Risk: WhatsApp can ban an account that sends from an unofficial client. A downloaded file can contain a prompt injection.
