## 1. Card data and store

- [x] 1.1 Add `internal/inbox/card.go`: `Card`, the categories, the urgencies, the states and an urgency rank
- [x] 1.2 Add the `cards` table, the `cards_key` partial unique index and `sessions.primer_hash` to the schema; set `SchemaVersion` to 3
- [x] 1.3 Add `migrateV3`: `VACUUM INTO <db>.v2.bak`, then add the column and set the version in one transaction; test it with a version 2 fixture that the test makes with the version 2 schema
- [x] 1.4 Add `SaveCards` (one transaction; added, updated or unchanged for each card; state `new` on update), `Cards` (open or all), `SetCardState` by ID or key, and `MessagesByID`
- [x] 1.5 Extend `Prune` to delete `done` cards older than the retention time; test that `new` and `read` cards stay
- [x] 1.6 Change `Session` and `SetSession` to read and write the primer hash

## 2. Card input and notification rule

- [x] 2.1 Add `internal/card`: decode stdin (array or one object, unknown fields rejected, 1 MiB, 50 cards), validate, sanitize and cut with warnings, drop unknown refs with warnings
- [x] 2.2 Add `card.Plan`: the notification rule of design D4 (count rule, `min_urgency`, 3 single `high` notifications, one group, single group card)
- [x] 2.3 Add `render.CardSection(command)`: the fixed primer section (in `render`, because `card` imports `render.Sanitize`)
- [x] 2.4 Unit tests for 2.1 to 2.3, each broken one time to show that it fails

## 3. Configuration

- [x] 3.1 Add `notify.min_urgency` (default `normal`, validation), and change the comment of `notify.enabled`
- [x] 3.2 Add the minimum urgency as a choice in the Schedule form

## 4. Commands

- [x] 4.1 Add `internal/cli/card.go`: `i2a card add`, `i2a card done --key|--id`, `i2a card list [--json] [--all]`; `card add` shows the planned notifications
- [x] 4.2 Make `i2a` with no arguments open the dashboard on the Feed tab (it opens or creates the store); keep `i2a config` on Overview
- [x] 4.3 Tests for the commands: exit codes, output line, stderr warnings, JSON list fields

## 5. Rendering, driver and engine

- [x] 5.1 Add `id` as the first field of `render.Item`
- [x] 5.2 Give `render.Render` the card command; append `render.CardSection` to the primer; set `Rendered.PrimerHash` from the template text and the section
- [x] 5.3 Engine: resolve the card command with `os.Executable` (and `I2A_HOME`), pass it to `Render`; remove the notification for a sent batch, keep the failure notification
- [x] 5.4 Orca `reuse`: start a new session when the stored hash is not `PrimerHash`; store the hash with the handle
- [x] 5.5 Update the render, engine and Orca tests; add tests for the `id` field, the primer section, the hash and the restart

## 6. Feed tab

- [x] 6.1 Add `tabFeed` as the first tab; move the other tabs one number up; update the help dialog and the footer
- [x] 6.2 Add `internal/tui/tab_feed.go`: list and detail panel, unread mark, header counts, sort `s`, filter `f`, done cards `a`
- [x] 6.3 Reload the cards every 2 seconds; show "No cards yet" without a store and try to open it on each tick
- [x] 6.4 Keys `c` (copy), `o` (open the first stored message URL with http or https), `d` (done), `u` (unread), `enter`; the read rule of the spec
- [x] 6.5 Add `Copy` and `Open` to `tui.Deps` (`pbcopy` when it is in `PATH`, else OSC 52; `open` on macOS, else `xdg-open`)
- [x] 6.6 Model tests for sort, filter, read rule, keys and the URL rule

## 7. End-to-end scenarios

- [x] 7.1 Change the tab keys in the existing scenarios (S02, S09, S10, S20, S22) and the expected tab bar
- [x] 7.2 S23 Card feed: `i2a card add` from stdin, then `i2a`; the Feed shows the cards by urgency; `s`, `f`; `c` and `o` reach shims of `pbcopy` and `open`/`xdg-open` in `PATH`; `d`; `i2a card list --json` shows the states
- [x] 7.3 S24 Update by key and close: a second `card add` with the same key and a higher urgency; `i2a card done --key`; a bad card exits with code 1 and saves nothing
- [x] 7.4 S25 (local, `I2A_E2E_ORCA=1`): `bob` posts; `i2a run --once` with `orca`, `reuse`, `inline`; the real agent runs `i2a card add`; a card that refers to the message is in `i2a card list --json`
- [x] 7.5 Update `test/e2e/SCENARIOS.md`

## 8. Documentation

- [x] 8.1 README: the Feed, the card commands, the notification rule, `min_urgency`, the allow rule for the agent, the upgrade and rollback steps, the new tab numbers
- [x] 8.2 CLAUDE.md: the return path in the data flow, `internal/card`, the Feed tab
- [x] 8.3 `task test`, `task lint`, the e2e lint and `task leakcheck` pass
