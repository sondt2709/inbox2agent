## 1. Tests that fail on the base

- [x] 1.1 Test: two spaces list one conversation: valid; two times in one space and in Unsorted and a space: errors
- [x] 1.2 Test: `Links`, the widest `Level`, `AddLink`, `SetLevel`, the move with `SetConversation`
- [x] 1.3 Test: both sources set `ForMe`
- [x] 1.4 Test: fan-out: one message to two spaces; a `mentions` link gets only the messages for the user; one failed space does not change the other
- [x] 1.5 Test: upgrade of a real version 6 store to version 7 with the backup
- [x] 1.6 Test: a card from the terminal of a session gets its space; `ask.Resolve` uses it

## 2. Configuration and sources

- [x] 2.1 `internal/config`: links, routes, levels, validation, the schema text
- [x] 2.2 `inbox.Message.ForMe`, `Message.Delivered`, `Card.Space`; the sources set `ForMe`

## 3. Store and engine

- [x] 3.1 Store schema version 7: `messages.for_me`, `deliveries`, `cards.space`, backup `inbox.db.v6.bak`; retention
- [x] 3.2 `Pending` with the deliveries; `FinishDispatch` writes a delivery; `SetStates`
- [x] 3.3 `Engine.Dispatch`: fan-out and the filter of each link; the state after the round
- [x] 3.4 `SaveCards` gets the space from the session of the handle; `ask.Resolve` and `cardSpace` use it

## 4. Screens and commands

- [x] 4.1 Conversations tab: the spaces column, `L` adds a link, `l` sets each link; Spaces tab lists each linked conversation
- [x] 4.2 `i2a conversations` and `--json` (`spaces`), `i2a status`
- [x] 4.3 README, CLAUDE.md, the docs of the schema

## 5. Check

- [x] 5.1 E2E: one channel in two spaces with `all` and `mentions`
- [x] 5.2 Break each change one time and see the new test fail
- [x] 5.3 `task test`, `task lint`, e2e lint, `task e2e`, `task leakcheck`, `openspec validate --strict`
- [x] 5.4 Demo of the real app as evidence; archive the change and open the PR
