## 1. Probe on the real account

- [x] 1.1 Write a probe program in `tmp/probe/` (git-ignored) that opens the source `work2` with `whatsapp.Open`, connects, and sends to the chat of A Ty
- [x] 1.2 Stop the LaunchAgent, then send one plain message and one quote; record `SendResponse` and each error in `tmp/evidence/add-whatsapp-reply-files/probe/`
- [x] 1.3 Send the same message ID two times; record what the phone of A Ty shows
- [ ] 1.4 Record whether the chat of A Ty becomes read on the phone of the user after a send
- [x] 1.5 Download the image and the document that A Ty sends; record `FileLength`, `Mimetype`, `FileName` and the byte count
- [x] 1.6 Write a file to `~/Downloads` from a LaunchAgent process and record the result (TCC)
- [ ] 1.7 Start the LaunchAgent again; update design.md decisions 3 and 5 when the probe differs

## 2. WhatsApp source

- [x] 2.1 Add the column `wa_messages.media` (`ALTER TABLE` when missing) and set it in `SaveMessage`
- [x] 2.2 Change `DB.Prune` to keep media rows until `retention`; add `Settings.Retention` and pass it from `source.Build`
- [x] 2.3 Add `DB.Row(chat, id)` that returns the stored row with `raw`
- [x] 2.4 Add `whatsapp/reply.go`: the message ID from the pending ID, the plain and quote protos, `Source.Reply`, `Source.Find`, the error mapping with `Answered()`
- [x] 2.5 Add `whatsapp/download.go`: `Source.Download(ctx, msg, dir)` with the size limit, the media check, the row check, and a 0600 temporary file
- [x] 2.6 Add `Conn.Send` and `Conn.Download`; make a closed `Source` return an error and not connect again
- [x] 2.7 Add a connect mode for one send (lock wait 10 s, no wait for offline messages)
- [x] 2.8 Unit tests with fixtures from `testdata/messages.jsonl`, `testdata/timer.jsonl` (capture `-chat`) and the probe output; break each test one time

## 3. Reply flow

- [x] 3.1 Add `ReplyID` to `inbox.ReplyRequest` and set it in `reply.Send`
- [x] 3.2 `reply.DefaultPlace`: quote for a WhatsApp group, chat for a WhatsApp direct chat
- [x] 3.3 `source.BuildReplier` and `source.CanReply` accept WhatsApp
- [x] 3.4 Unit tests for the default place and for `CanReply`

## 4. Daemon API

- [x] 4.1 Add `Engine.mu` around `sources`; add `Engine.Reply(id)` and `Engine.Download(id)` that lock the source, not the run loop
- [x] 4.2 Add `Store.ReplyByID`
- [x] 4.3 Add `POST /v1/reply` and `POST /v1/download` in `api.go`; no message, reply or file text in a response
- [x] 4.4 Add `Client.Reply` and `Client.Download` in `client.go`
- [x] 4.5 Add `daemon.Replier` (daemon first, else the own connection) and `daemon.Fetch` for the download
- [x] 4.6 Delete files older than one hour in `<data>/tmp/` at each prune
- [x] 4.7 Update `docs/daemon-api.yaml`
- [x] 4.8 Unit tests: the endpoints, a reply during a busy run, the fallback with no daemon, no text in responses

## 5. Download command

- [x] 5.1 Add `internal/cli/download.go`: `i2a download --message <id> [--dir <dir>]`
- [x] 5.2 File name rules, `os.Link` with ` (n)` names, the `EXDEV` copy, the quarantine attribute
- [x] 5.3 Errors: unknown message, no file, account that cannot download, too large, file no longer available, TCC denial with the `--dir` hint
- [x] 5.4 Unit tests for each name rule and each error; break each test one time

## 6. Primer, Feed and CLI text

- [x] 6.1 Add the file lines to `render.CardSection`; update the primer tests
- [x] 6.2 Feed reply form: "Quote" for a WhatsApp target, the WhatsApp read state note; use `daemon.Replier` through `tui.Deps`
- [x] 6.3 `i2a reply` help text: WhatsApp quote and chat; use `daemon.Replier`
- [x] 6.4 Update `README.md` (reply, download, the ban risk) and `CLAUDE.md` (WhatsApp sends replies, the download path, the new endpoints)

## 7. Checks and evidence

- [x] 7.1 `task test`, `task lint`, `golangci-lint run --build-tags e2e ./test/...`, `task leakcheck`
- [x] 7.2 `task e2e` (the Mattermost reply and the batches still work)
- [ ] 7.3 Real app: `i2a reply` to A Ty with the daemon running and stopped; read `replies` rows
- [ ] 7.4 Real app: Feed `r` with Quote and Chat; screenshots before and after
- [ ] 7.5 Real app: `i2a download` for the image and the document, with the daemon running and stopped; `xattr -l`; byte count
- [ ] 7.6 Real app: an agent batch with a document; the agent runs `i2a download` and describes the content in a card
- [ ] 7.7 Update the purpose of `openspec/specs/whatsapp-source/spec.md` at archive (it says "It only reads")
