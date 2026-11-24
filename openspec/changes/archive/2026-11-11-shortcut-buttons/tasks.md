## 1. Buttons in the footer and the header

- [x] 1.1 `keyPress(name)` builds the `tea.KeyPressMsg` of a key name; `keyName` (a hint with one key is a button); `model.press(key)` calls `update` with it
- [x] 1.2 `layoutHints` writes the chips and returns one zone for each button; `fitHints` reports a dropped hint
- [x] 1.3 The footer and the header record `model.barZones`; `update` handles a left click on a zone before the dialog gets it
- [x] 1.4 Footer right: `Find /`, `Keys ?`, `Quit q`; header: the button `Run now ctrl+r`, the spinner while a run is busy
- [x] 1.5 `More…`: the pick dialog "Actions" with each button hint; choosing a row runs `press` on the screen under it
- [x] 1.6 Complete the hint lists of the tabs, so that `More…` holds each action key
- [x] 1.7 Unit tests: a click on each zone of a rendered footer runs its key; a click on a dialog footer button; `More…` then Sort; the header button starts a run

## 2. Add row and dialog rows

- [x] 2.1 The panel row `+ Add conversations` of a user space; `enter` and a click open the add dialog; the empty text "No conversation yet. Add conversations from any account."
- [x] 2.2 Row clicks: `pickDialog` chooses, `spacesDialog` checks, `searchDialog` adds or removes (with a space) or selects then opens (without a space)
- [x] 2.3 Unit tests for the add row and each row click
- [x] 2.4 Break each new test one time and see it fail, then restore the code

## 3. End-to-end

- [x] 3.1 `Term.Click(x, y)` and `Term.ClickText(pattern)` in `test/e2e/harness/tmux.go` (SGR mouse sequences)
- [x] 3.2 Scenario S48: with the mouse only, create a space, add a channel with `+ Add conversations`, click OK, click `Run now ctrl+r`, and read the dispatch in the store
- [x] 3.3 Update the footer text in the e2e tests that changed, and `test/e2e/SCENARIOS.md`

## 4. Docs and checks

- [x] 4.1 The key help, `README.md` and `CLAUDE.md`
- [x] 4.2 `task test`, `task lint`, `golangci-lint run --build-tags e2e ./test/...`, `task leakcheck`, the e2e scenarios
