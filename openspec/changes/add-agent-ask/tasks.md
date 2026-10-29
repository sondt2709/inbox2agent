## 1. Store

- [x] 1.1 Add `inbox.Ask` with the states `queued`, `sent` and `failed`, `Card.Handle` and `Message.Profile`
- [x] 1.2 Schema version 5: `cards.handle`, the `asks` table, `migrateV5` with the backup `inbox.db.v4.bak`; test the upgrade on a version 4 store
- [x] 1.3 `SaveCards` stores the handle (insert, update, handle-only change stays `unchanged`); `scanMessages` reads `profile`
- [x] 1.4 `StartAsk`, `FinishAsk`, `QueuedAsks`, `Asks(cardIDs)`; `Prune` deletes old asks

## 2. Card terminal

- [x] 2.1 `card.Handle(value)` checks `^term_[A-Za-z0-9-]{1,64}$`; `i2a card add` reads `ORCA_TERMINAL_HANDLE`, warns on a bad value

## 3. Prompt and driver

- [x] 3.1 Extract the primer code of `render.Render`; add `render.Ask` (instruction lines, card and message data lines, answer command, cut to 65,536 bytes)
- [x] 3.2 `agent.Result.Handle`; `Driver.Ask(ctx, handle, wait, r)` for `orca` (live card terminal, else `Deliver`) and `dry-run` (error)
- [x] 3.3 `agent.Lock(ctx, dir, profile)` with `flock` on `<data>/agent-<profile>.lock`

## 4. Ask flow

- [x] 4.1 New package `internal/ask`: `CheckText`, `Target` (profile, dry-run refusal, handle), `Send` (record `queued`, lock, driver, finish), `Deliver` for a queued ask
- [x] 4.2 `Engine.Dispatch` sends queued asks before the batches; expired asks and asks of deleted cards become `failed`; `Engine.deliver` holds the profile lock

## 5. Command and Feed

- [x] 5.1 `i2a ask --card <id|key>` with text on stdin; outputs for `sent` and `queued`; tests for errors
- [x] 5.2 Feed key `t`: the ask form, `ctrl+s` outside the UI loop, `esc`, the status lines, the form error
- [x] 5.3 Feed: ask lines in the detail panel, "↪ asked" in the list row, `t` in the footer and in the key help

## 6. End-to-end tests and docs

- [x] 6.1 Fake `orca` for e2e that prints the real fixtures of `internal/agent/testdata` and records its arguments
- [x] 6.2 e2e S30 (CI): dry-run refusal in the Feed
- [x] 6.3 e2e S31 (CI): Feed `t` and `i2a ask --card` with the fake `orca`; the recorded prompt has the user text, one data fence and the key
- [x] 6.4 e2e S32 (local, `I2A_E2E_ORCA=1`): real Claude updates the card after an ask from the Feed
- [x] 6.5 Update `test/e2e/SCENARIOS.md`, `README.md` and `CLAUDE.md` (ask path, `ORCA_TERMINAL_HANDLE`, profile lock)

## 7. Evidence

- [ ] 7.1 `task test`, `task lint`, `golangci-lint run --build-tags e2e ./test/...`, `task e2e`, and the local Orca scenario on the last commit
- [x] 7.2 Upgrade a copy of the real `inbox.db` (version 4) and compare the row counts
- [x] 7.3 Screen captures of the Feed after the change in `tmp/evidence/add-agent-ask/after/`
