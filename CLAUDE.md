# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`i2a` (inbox2agent) is a Go CLI. It polls chat and work tools for new messages and sends them in batches to an AI agent. The agent sends its results back as cards with `i2a card add`; the user reads them in the Feed tab of `i2a`, sends replies from there (`r`) and asks the agent for more about a card (`t`). The agent can send a reply with `i2a reply`. Accounts: Mattermost and WhatsApp (receive only) now; GitHub and Linear later.

The model has five items, each with one job. An **account** is one login; it only connects. A **conversation** is one DM, group or channel of an account; each is in exactly one space and has a read level (`all`, `mentions` = messages for the user, or each message of a direct conversation, `off`). A **space** is a group of conversations that the user names; it is the only item that names an agent and a schedule. An **agent** is how to send a batch (the old profile). A **schedule** is when a space sends. The built-in space **Unsorted** (`config.UnsortedID`) holds each conversation that no space lists; its agent can be empty (hold). Accounts, agents, schedules and spaces have a stable `id` and a `name` that the user can change; a conversation has the platform `name` and an optional `label`. Use these words in code, docs and errors; do not say source or profile for them (`internal/source` is the platform adapter that reads one account). Drivers: `orca` (the Orca CLI) and `dry-run`. One static binary (`CGO_ENABLED=0`), macOS first.

## Commands

```sh
task build                                   # bin/i2a
task test                                    # go test -race ./...
go test ./internal/render -run TestAttack    # one test
task lint                                    # golangci-lint v2 (.golangci.yml)
golangci-lint run --build-tags e2e ./test/...
task e2e                                     # starts Docker Mattermost, runs the CI scenarios
I2A_E2E_ORCA=1 I2A_E2E_SERVICE=1 task e2e    # also the Orca and LaunchAgent scenarios (local only)
task leakcheck                               # forbidden terms from .leakcheck.local
task dist -- 1.2.0                           # release archives and checksums.txt in dist/
internal/source/mattermost/testdata/capture.sh   # capture Mattermost fixtures again (needs a new server)
go run ./internal/source/whatsapp/testdata/capture -db <data>/whatsapp-<id>.db   # WhatsApp fixtures from a paired source
docs/demo/record.sh                          # record docs/demo/i2a-config.gif with VHS
```

The e2e Mattermost server is `test/e2e/mattermost/compose.yaml`. It runs as `linux/amd64` (no arm64 image). Its first user must be `e2e-admin`. If the harness reports that `e2e-admin` is not an admin, or user creation fails with `ERROR_SAFETY_LIMITS_EXCEEDED` (each run creates users), run `docker compose -f test/e2e/mattermost/compose.yaml down -v` and start it again.

## Architecture

Data flow: `source.Source.Collect` (read level of each conversation: `config.Level`, `Env.Levels`, `Env.Default`) → `store.SavePoll` (messages and cursor in one transaction) → `daemon.Run` (one next send time for each space: `config.Schedule.Next`) → `daemon.Engine.Dispatch(spaces)` → `config.Route` (conversation → space, computed at send time, so a moved conversation takes its waiting messages) → `render.Render` (sanitize, digest file, prompts, card section) → `agent.Driver.Deliver` (session of the space) → `store.FinishDispatch` (`messages.space`, `dispatches.space` and `agent`).

Return path: the agent runs `i2a card add` (JSON on stdin) → `card.Parse` and `card.Prepare` (validate, sanitize, cut) → `card.Handle` (`ORCA_TERMINAL_HANDLE`) → `store.SaveCards` (one transaction, upsert by key) → `card.Plan` → notifications. The Feed tab reads the cards every 2 seconds. `i2a card add` writes the store directly; it does not talk to the daemon.

Reply path: the Feed form (`r`, `ctrl+s`) or `i2a reply` → `reply.Send` (text check, duplicate rule) → `store.StartReply` (`sending`) → `source.Replier` (`mattermost.Source.Reply`: `GET /posts/{id}`, then `POST /posts?set_online=false`) → `store.FinishReply` (`sent` or `failed`) → the card becomes `done`. The daemon is not in this path.

Ask path: the Feed form (`t`, `ctrl+s`) or `i2a ask` → `ask.Send` (text check, `ask.Resolve`: space of the newest ref message, else `config.Route` of it; the agent of the space; no agent or dry-run refused with `ask.NoAgentError`) → `store.StartAsk` (`queued`) → `agent.Lock` (`<data>/space-<id>.lock`) → `render.Ask` → `agent.Driver.Ask` (the live terminal of the card, else `Deliver`) → `store.FinishAsk` (`sent`, `failed`, or still `queued` when the agent is busy for `ask.Wait`). `Engine.Dispatch` sends queued asks with `ask.Retry` before the batches, and `daemon.Run` tries them at least each minute (`askEvery`). The agent answers with `i2a card add` and the key of the card.

- `internal/inbox`: shared types (`Message`, `Cursor`, `Card`, states). It prevents import cycles between the source, store, render and agent packages.
- `internal/config`: version 2 of `config.toml` (`[[accounts]]`, `[[agents]]`, `[[schedules]]`, `[unsorted]`, `[[spaces]]`, `[settings]`). `Load` converts a version 1 file (`migrate_v1.go`), keeps `config.toml.v1.bak` and returns `Config.Migration`; the CLI prints it to stderr, the daemon logs it. `Route`, `Level`, `Levels`, `SetConversation`, `SetLabel`, `SpacesUsing` and `Replace` are the only ways to read or change routes. `config.Update` (lock, read the file, change, validate, write) is the only way to change the file; `Save` also holds the lock. `Schedule.Next` gives aligned send times (the start of the hours plus a multiple of `every`); the daemon, `i2a status` and the TUI use it.
- `internal/source`: the `Source` interface and `Build`. A source reads one account and returns messages and the next cursor. It never writes to the store.
- `internal/source/whatsapp`: a linked device of a personal account, made with whatsmeow. The process that collects (the daemon or `i2a run --once`) holds the connection and locks `<data>/whatsapp-<id>.lock`. The whatsmeow event handler writes each message to `wa_messages` in `<data>/whatsapp-<id>.db` (the same file holds the device keys, mode 0600); `Collect` reads the rows after the cursor `{"rowid": n}`. Pairing runs in the TUI or in `i2a account add whatsapp`, never in the daemon. `ReadDevice` reads the phone number and the push name from `whatsmeow_device` for the screens. `source.Source` can implement `io.Closer`; `Engine.SetConfig` and `Engine.Close` close the sources.
- `internal/source/mattermost`: a small typed REST client, not the official module. Polling uses `GET /channels/{id}/posts?page&per_page` (newest first, stop at the cursor). Do not use `posts?since=`: it filters on `update_at` and has an unordered `LIMIT 1000`. Never call an endpoint that marks a channel as read. Sending uses `POST /posts` only.
- Mattermost facts that the send depends on (checked on the Docker server): `root_id` must be the thread root (a reply ID gives HTTP 400); a top-level post of the user marks the channel read and a thread reply does not, also with `set_online=false`; the same `pending_post_id` returns the first post for about 30 seconds only.
- `internal/reply`: the one send flow of the Feed and `i2a reply`. An error with `Answered() == true` means that the platform created no post (`failed`). Any other error keeps the reply `sending`; the next send of the same message, place and text calls `Replier.Find` before it posts again.
- `internal/render`: the prompt-injection boundary. Each message is one `json.Marshal` line inside a fence labelled `i2a-data-<nonce>`. Hidden characters are removed before encoding. Keep every data line starting with `{`.
- `internal/card`: the input of `i2a card add` and the notification rule (`Plan`). Card text comes from an agent that read untrusted messages, so it gets the same sanitizing as message text.
- `internal/ask`: the one send flow of the Feed, `i2a ask` and the daemon. The text of the user is the only instruction in the ask prompt; `render.Ask` puts the card and its messages in the data fence. The ask prompt is not a template and does not change the primer hash, so an ask never restarts a reuse session.
- `internal/agent`: the drivers. In `reuse` mode the store keeps the primer hash with the handle; a different hash starts a new session. Each space has its own reuse session (`sessions.space`), also when two spaces use one agent. The daemon and an ask hold `agent.Lock` of the space while they type into a terminal; a batch that waits longer than 3 minutes is `held`. Delivery is at least once: messages become `sent` only after the driver returns `sent`; `held` and errors keep them `pending`. A `dry-run` batch makes its messages `previewed`; a real driver still sends `previewed` messages.
- `internal/daemon`: `Engine` (poll, dispatch for given spaces, expire, asks, prune) and `Run` (loop, one next send time for each space, asks each minute, configuration reload, `flock` lock). A space with no agent never sends.
- `internal/store`: schema version 6 renamed the `profile` columns to `space` and added `dispatches.agent` (backup `inbox.db.v5.bak`). `PendingCounts` gives the waiting messages of each conversation.
- `internal/tui`: the dashboard. `i2a` opens it on the Feed tab (`tab_feed.go`), `i2a config` on Spaces. The tabs are `1 Feed 2 Activity │ 3 Accounts → 4 Conversations → 5 Spaces → 6 Agents + 7 Schedules │ 8 Settings` (`tabNames`, `tabBefore`); row 2 defines the item of the tab (`tabDefs`) and holds the unsaved mark. The Feed opens only the stored URL of a source message, never a link from card text; it shows and filters (`F`) the space of each card (`cardSpace`). The card body is Markdown (`markdown.go`, glamour); `stripOSC` removes the OSC 8 hyperlinks that glamour writes for links and images, so keep it on each render. `model.go` has the layout (header, tab bar, definition, content, footer), the focus (list or form), the global keys, the mouse zones and the dialog stack. Each tab is one file (`tab_*.go`). `convRows` joins the conversations that each account lists (`loadConversations`: Mattermost `ListChannels` over the network, WhatsApp `ReadChats` from its file) with the entries of the configuration. Add and edit are forms in the detail panel, made with the small form engine in `form.go` (text inputs, segmented choices and switches, text areas, buttons, errors under the fields). `buildForm` makes the form of the selected item again when the selection changes or after a save. A form saves through `ui.commit(change, ...)`: `change` edits the configuration that `config.Update` reads from the file now, under `config.toml.lock`, so a change of another process to a different item stays. A form keeps its item (`form.kind`, `form.id`, `form.base`); `commitForm` opens the conflict dialog when another process changed that item. Each refresh (2 seconds) checks the hash of `config.toml` (`readConfigFile`, `applyFile`) and loads a changed file; a dirty form stays. Dialogs (`dialog.go`) are confirmations, the pick dialog (move to a space, choose a replacement), the input dialog (a new space, a conversation name), the digest viewer and the key help.
- `internal/notify`: macOS notifications through `osascript`. `i2a card add` notifies for cards; the daemon notifies only for failed dispatches. The title and the text are script arguments, never script code. `I2A_NOTIFY=off` turns them off (the e2e harness sets it).
- `internal/service`: the macOS LaunchAgent `io.github.sondt2709.i2a`.

Facts about the `orca` CLI (1.4.240) that the driver depends on:

- `orca terminal show` returns `ok: true` for a closed terminal. A terminal is live when `connected` and `writable` are true and `exitCause` is null.
- `orphaned` is true for a running agent terminal that has no visible pane. Do not use it as a liveness signal.
- `--worktree path:<dir>` fails with `selector_not_found` when Orca does not know the folder. `orca repo add` accepts git repositories only.
- `orca terminal wait --for tui-idle` returns the error code `timeout` when the agent is busy.
- Do not pass `--prompt` to `orca agent launch` or `orca worktree create`. Orca types it while the agent starts, and it can stay in the input box with no Enter (`prompt.outcome` is still `handed-to-terminal`), or not arrive (`not-delivered`). The driver launches with no prompt, waits for `tui-idle`, then runs `orca terminal send --enter --wait-submit 30`. The stage `turn_started` proves the submission.
- `orca terminal send --enter` with no `--text` sends a bare Enter.
- Prompts are command arguments (the RPC limit is 1 MiB). Keep `inline_max_bytes` at 65,536 or less. An ask prompt drops message lines above 65,536 bytes.
- Orca sets `ORCA_TERMINAL_HANDLE` (`term_<uuid>`) in an agent terminal. `orca terminal show --terminal "$ORCA_TERMINAL_HANDLE"` returns that terminal. The e2e harness clears it, so a scenario card does not record the terminal of the test run.

The fixtures in `internal/agent/testdata` are real Orca outputs with local paths replaced.

Facts about WhatsApp and whatsmeow that the source depends on:

- whatsmeow sends no read receipt and no presence unless the code calls `MarkRead` or `SendPresence`. Never call them. It sends delivery receipts (grey ticks).
- One set of device keys can have one connection. A second connection replaces the first (`events.StreamReplaced`). Never copy a `whatsapp-<id>.db` to a test.
- The QR channel ends after about 160 seconds (`QRChannelTimeout`). `PairPhone` must come after the first QR event and needs a display name `Browser (OS)` from the list of WhatsApp ("Chrome (Mac OS)").
- whatsmeow deletes the device row at `events.LoggedOut`. Event handlers run one at a time, so a network call (group name) in a handler runs in a goroutine.
- Many senders have a `@lid` address. The phone number comes from `SenderAlt`, `RecipientAlt` or the LID store.
- A disconnect always logs "Error sending close to websocket"; the source drops this warning.
- whatsmeow builds with `CGO_ENABLED=0` and `modernc.org/sqlite` (`sqlstore.NewWithDB(db, "sqlite", log)`).
- The fixtures in `internal/source/whatsapp/testdata/messages.jsonl` are real stored messages. The capture tool replaces addresses, IDs, names and texts and removes all bytes. Read its output before a commit.

## Rules

- **Writing:** use ASD-STE100 style for all text (docs, comments, errors, OpenSpec artifacts, PRs). Use short sentences, the active voice and one term for one concept. Name the specific file, command or value.
- **No personal data in committed files:** no local paths, real server names, real usernames or other project names. `task leakcheck` checks the terms in `.leakcheck.local` (not committed). `tmp/` holds local secrets and is git-ignored.
- **Fixtures must copy real responses.** Mattermost fixtures come from `capture.sh` against the Docker server. Orca fixtures come from a real `orca` run with the paths replaced. WhatsApp fixtures come from `testdata/capture` on a paired account. Do not write the shape of external data by hand.
- **A new test must be able to fail.** Break the code one time and confirm that the test fails, then restore the code.
- **E2E tests drive the real binary.** They use `test/e2e/harness` (a detached tmux session on a private socket). Wait for the screen text before you send keys. Use `Term.Press`, which sends a key again when it changed nothing. Huh can drop the first key while a form starts. Wait for text that shows only on the target screen: the dashboard draws dialogs over the tab, so the tab text stays visible under a dialog. The footer keys of a tab (for example `enter edit`) show only when no dialog is open.
- In some agent sessions, the `tmux` in `PATH` is an Orca shim. The harness skips paths that contain `/.orca/`. Set `I2A_E2E_TMUX` to choose a binary.
- **OpenSpec:** significant changes go through `openspec/changes/<name>/` (proposal, design, specs, tasks) before code.
- **PR titles** use Conventional Commits (`.github/workflows/pr-title.yml`). Merges are squash-only. The title becomes the commit subject on `main`.
- **Releases** are automatic. After all CI jobs pass on a push to `main`, the `release` job in `ci.yml` runs semantic-release (`.releaserc.json`). Each push makes a new version: `!` or a `BREAKING CHANGE` footer is major, `feat` is minor, all other types are patch. `scripts/dist.sh` builds the archives. The npm packages have exact versions in `ci.yml`; change them together. Keep `conventional-changelog-conventionalcommits` on 9.x while `@semantic-release/release-notes-generator` uses `conventional-changelog-writer` 8. Before you change a version, do a full `semantic-release --dry-run` against a local Git repository and check that `generateNotes` passes.
