## 1. Real data

- [x] 1.1 Get real test comments in one Linear issue: `/agent ...`, `@<me> /agent ...`, `>agent ...`, a reply to another comment, and a comment edited to add `/agent`
- [x] 1.2 Write the capture tool `internal/source/linear/testdata/capture`: it runs the queries of the source with a real key and replaces IDs, names, texts, URLs and the workspace key
- [x] 1.3 Capture the fixtures (comments, notifications, viewer, an auth error); read the output for private data; record the stored form of the prefix, the mention and the page order

## 2. Configuration

- [x] 2.1 Add `PlatformLinear`, `Feed` and `Account.Feeds` with `doc` tags, the defaults (`url`, `from`) and `NewLinearAccount` with the feeds `agent` and `inbox`
- [x] 2.2 Add the validation of feeds and of a Linear `url`, with tests for each error text
- [x] 2.3 Write `docs/config.schema.json` again (`go test ./internal/config -run TestSchemaCopyIsCurrent -update`)

## 3. Linear source

- [x] 3.1 GraphQL client: one query document with one alias for each feed, pages with `after`, auth error, backoff on 429 and `RATELIMITED`
- [x] 3.2 Match rule (`from`, `projects`, `labels`, `teams`, `prefix` after leading mentions), tested with the captured bodies
- [x] 3.3 `Collect`: messages, `ForMe`, reply reference, cursor for each feed, first poll with `lookback`, at most 10 pages
- [x] 3.4 `Check` (viewer and workspace) and `Feeds` (the conversations of the account)
- [x] 3.5 Test that each request of a poll is a query and none is a mutation
- [x] 3.6 `source.Build` for `linear`; `canReply` stays false, with a test

## 4. CLI and dashboard

- [x] 4.1 `i2a account add linear --id <id> --token-stdin [--name <name>]`, and the feeds in `i2a conversations`
- [x] 4.2 `i2a account remove` deletes the key of a Linear account
- [x] 4.3 Accounts section: the Linear choice of the add form, the detail (user, workspace, feeds), the feeds in `loadConversations`
- [x] 4.4 Help text: remove "Linear come later" and name Linear in the command help

## 5. Tests and docs

- [x] 5.1 E2E scenario with a fake Linear server that sends the captured responses: `account add linear`, `run --once` with `dry-run`, the comment in the digest
- [x] 5.2 Break each new test one time and confirm that it fails
- [x] 5.3 Update `CLAUDE.md` (Linear source, facts of the Linear API) and `README`
- [ ] 5.4 `task test`, `task lint`, `task leakcheck`, `task e2e`

## 6. Evidence on the real workspace

- [ ] 6.1 Real run in a temporary data folder: `account add linear`, `conversations --account`, `run --once` with `dry-run`; the digest and the row count of the store
- [ ] 6.2 `readAt` of the inbox notifications before and after the run
- [ ] 6.3 Dashboard screenshots: the Linear add form and the account detail
