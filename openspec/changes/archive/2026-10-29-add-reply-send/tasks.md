## 1. Store

- [x] 1.1 Add `internal/inbox/reply.go`: `Reply`, the states `sending`, `sent`, `failed`, the places `thread` and `channel`, and `by` `user` and `agent`; add `ThreadID` to `inbox.Message`
- [x] 1.2 Add the `replies` table and its indexes to the schema; set `SchemaVersion` to 4
- [x] 1.3 Add `migrateV4`: `VACUUM INTO <db>.v3.bak`, then add `messages.thread_id` and set the version in one transaction; test it with a version 3 fixture that the test makes with the version 3 schema, and test 2 → 4
- [x] 1.4 Save and read `thread_id` in `SavePoll`, `MessagesByID` and the other message reads
- [x] 1.5 Add `StartReply`, `FinishReply`, `FindReply` (same message, place and text), `RepliesByCard` and `MessageByID`; extend `Prune` to delete old replies
- [x] 1.6 Unit tests for 1.3 to 1.5, each broken one time to show that it fails

## 2. Mattermost send

- [x] 2.1 Extend `capture.sh` to capture `GET /posts/{id}` of a reply, `POST /posts` (channel and thread) and the HTTP 400 for a deleted post; run it against the Docker server and commit the fixtures
- [x] 2.2 Add `Client.GetPost` and `Client.CreatePost(ctx, post, setOnline)`
- [x] 2.3 Fill `ThreadID` from `root_id` in `Collect`
- [x] 2.4 Add `Source.Reply` (root lookup, `root_id` only for thread, `set_online=false`, `pending_post_id`) and `Source.Find` (newest page of the channel; same user, text and root; after `Since`)
- [x] 2.5 Add `source.Replier`, `source.ReplyRequest`, `source.Posted`, `ErrCannotReply` and `source.BuildReplier`
- [x] 2.6 Unit tests with an `httptest` server that serves the captured fixtures: request paths and bodies, `set_online`, root of a reply in a thread, the API error, `Find`; each broken one time

## 3. Send flow

- [x] 3.1 Add `internal/reply`: `Defaults` (newest existing ref, default place), `CheckText` (sanitize, trim, empty, 4,000), and `Send` with the retry table of design D5 and the card `done` step
- [x] 3.2 Classify errors: an answer from the server makes the reply `failed`; any other error keeps it `sending`
- [x] 3.3 Unit tests with a fake `Replier`: new send, refused `sent` duplicate, `sending` with `Find` hit and miss, `failed` retry with the same pending ID, card `done` only on `sent`; each broken one time
- [x] 3.4 Check the lifetime of the Mattermost pending ID cache in the Mattermost source and write it in the evidence

## 4. Command

- [x] 4.1 Add `internal/cli/reply.go`: `i2a reply --to | --card [--thread | --channel]`, stdin when it is not a terminal, the card reply when stdin is empty, output line, exit codes, `by` = `agent`
- [x] 4.2 Show the notification "i2a · the agent sent a reply" after a successful send, when notifications are on
- [x] 4.3 Command tests: unknown message, empty text, too long, `--thread` with `--channel`, the output line, the notification text

## 5. Feed

- [x] 5.1 Add the `Store` methods that the Feed needs, and a `Replier` builder in `Deps` (default: `source.BuildReplier` with `Deps.Config` and `Deps.Secrets`)
- [x] 5.2 Add the key `r`: the reply form of design D8 in the detail panel; refuse with a status text when no ref exists or the source cannot send
- [x] 5.3 Run the send in a `tea.Cmd`; on error show it under the Text field and keep the form; on success close the form, set the card `done` and show the status line
- [x] 5.4 Show the "✓ Replied" lines in the detail panel and "✓ replied" in the list row; add `r reply` to the footer and to the key help
- [x] 5.5 TUI tests: `r` opens the form with the defaults, `esc`, `ctrl+s` success and error, empty text, the Replied line; each broken one time

## 6. End-to-end and documents

- [x] 6.1 e2e S26: real binary in tmux, Feed → `r` → edit → `ctrl+s`; read the post, its `root_id` and `last_viewed_at` from the Mattermost API; `i2a card list --all --json` shows the card `done`
- [x] 6.2 e2e S27: `i2a reply --to` with stdin, `--card` with the card reply, the same reply again (exit 1, one post), an unknown message
- [x] 6.3 Read state: S26 checks that a thread reply keeps `last_viewed_at`; S27 checks that a chat post is read as Mattermost does (Mark as read removed, design D7a)
- [x] 6.4 e2e S28: a lost answer on the first try through a local proxy, then a retry after 35 s; one post in Mattermost
- [x] 6.5 Local e2e S29 (`I2A_E2E_ORCA=1`): a real agent with a primer that allows `i2a reply` thanks a sender; the post and the `replies` row exist
- [x] 6.6 Update `test/e2e/SCENARIOS.md`, `README.md` (send replies, the read-only sentence, `i2a reply`, how to allow the agent) and `CLAUDE.md` (data flow and the Mattermost send facts)
- [x] 6.7 Run `task test`, `task lint`, the e2e lint, `task e2e`, `I2A_E2E_ORCA=1 task e2e` and `task leakcheck`
