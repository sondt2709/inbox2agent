## 1. Fixtures and the first failing test

- [x] 1.1 Add `TestItemsReplyTo` in `internal/render`: a message with a reply reference gives a line with `reply_to`. Run it on 13d7f62 and keep the failure output
- [x] 1.2 On the Docker Mattermost server, check whether `GET /channels/{id}/posts` holds thread roots that are not in `order`. Add a page with a reply to an older root to `capture.sh`, and capture the fixtures again

## 2. Shared type and store

- [x] 2.1 Add `inbox.ReplyRef{Ext, ID, Author, Me, Text}` and `Message.ReplyTo *ReplyRef`
- [x] 2.2 Add `migrateV9` (backup `inbox.db.v8.bak`, `messages.reply_ext`, `reply_author`, `reply_me`, `reply_text`) and set the schema version to 9
- [x] 2.3 `SavePoll` writes the reference; `messageCols` reads it and finds the row ID with the subquery on `(source_id, external_id)`
- [x] 2.4 Tests: upgrade from version 8, failure keeps version 8, new store has version 9, row ID found, row ID 0 after a prune

## 3. Render

- [x] 3.1 Add `Item.ReplyTo` (pointer, `omitempty`); sanitize `author` with `field` and `text` with `Sanitize` and 200 characters on one line
- [x] 3.2 Add the `reply_to` sentence to `Digest`
- [x] 3.3 Tests: each line still starts with `{"id":`; no `reply_to` for a message without a reference; hidden characters removed; the primer hash does not change

## 4. WhatsApp source

- [x] 4.1 Add `quoteRef(m, me, names)` in `extract.go`: `stanzaID`, `me`, the author name or `+<phone>`
- [x] 4.2 `rowFromEvent` fills `Row.QuoteID`, `QuoteAuthor`, `QuoteMe`; `db.go` adds the columns with `ALTER TABLE` and reads and writes them
- [x] 4.3 `message()` sets `Message.ReplyTo` with the quoted text from `raw` (`quoted`)
- [x] 4.4 Tests with the real rows of `messages.jsonl`: quote of a contact, quote of the user, no quote, a row from before the columns

## 5. Mattermost source

- [x] 5.1 Keep the roots of the page (`PostList.Posts`) in `newPosts`; add the root cache (at most 1,000 entries) in `Source`
- [x] 5.2 For each post with `root_id`: root from the page, the cache, else `GetPost`; HTTP 403 and 404 give the root ID only; other errors fail the poll
- [x] 5.3 Add the root authors to the `usernames` call; set `ReplyTo` and the `ForMe` rule (root author is the user)
- [x] 5.4 Tests with the captured fixtures: root in the page, root by GET, deleted root, reply under a root of the user at `mentions`, network error keeps the cursor

## 6. Docs and e2e

- [ ] 6.1 Update `CLAUDE.md` (data flow, store version 9, the Mattermost facts) and the docs that list the data line fields
- [ ] 6.2 Add an e2e scenario on Docker Mattermost: the user posts a root, another user replies in a channel at `mentions`; the dry-run digest has `reply_to` with `me: true`
- [ ] 6.3 Run `task test`, `task lint`, `task e2e`, `task leakcheck`

## 7. Evidence

- [ ] 7.1 Upgrade a copy of the real `inbox.db` in the scratchpad: backup, version 9, same row count
- [ ] 7.2 Break each new test one time and confirm that it fails, then restore the code
- [ ] 7.3 After the install, read the next real digests and record a WhatsApp line and a Mattermost line with `reply_to`
