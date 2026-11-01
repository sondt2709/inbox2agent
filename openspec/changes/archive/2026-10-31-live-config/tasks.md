## 1. Failing test on main

- [x] 1.1 Add a TUI model test: open the model, add a space to `config.toml` from the test, save the Settings form, and expect the space in the file. Run it on b32fb12 and keep the failure output in `tmp/evidence/live-config/`.

## 2. Locked update in internal/config

- [x] 2.1 Add `ErrLocked` and the lock of `config.toml.lock` (flock, 0600, retry every 50 ms for 5 s).
- [x] 2.2 Add `Update(path, change)`: lock, read or `Default()`, change, validate, save, unlock; return the configuration and the SHA-256 of the written bytes.
- [x] 2.3 Add `Hash` and a parse of bytes that returns the configuration and its hash, for the reload check.
- [x] 2.4 Add `MissingError{Kind, ID}` and `Item(cfg, kind, id)` for the kinds settings, unsorted, account, agent, schedule, space, conversation.
- [x] 2.5 Change the header of `Save`: i2a writes the file again on each save and removes other comments.
- [x] 2.6 Do the version 1 conversion of `Load` under the lock.
- [x] 2.7 Unit tests: two goroutines update different spaces at the same time and both stay; a held lock gives `ErrLocked` after the timeout; `Item` for each kind; the header. Break each test one time.

## 3. Dashboard save path

- [x] 3.1 Change `ui.commit` to take a change function, call `config.Update`, and keep the hash.
- [x] 3.2 Move each of the 15 save sites in `tab_settings.go`, `tab_schedules.go`, `tab_agents.go`, `tab_spaces.go`, `tab_accounts.go` and `tab_conversations.go` into a change function that finds its item by ID and returns `MissingError` when it is gone.
- [x] 3.3 Store `form.ref` and `form.base` in `buildForm`; check the item in the change function of each form save and return `errConflict` when it differs.
- [x] 3.4 Add the conflict dialog: "Reload" (default) makes the form again from the file; "Overwrite" saves with the check off.
- [x] 3.5 Unit tests for each tab: an external change to another item stays after a save; a removed item gives the missing error; a changed item opens the dialog; Reload and Overwrite give the expected file.

## 4. Reload on refresh

- [x] 4.1 In `load()`, read `config.toml` and compare its hash with `cfgHash`; parse only when it differs.
- [x] 4.2 Apply a valid file: set `u.cfg`, rebuild a form that is not dirty, keep a dirty form and set the status line; run `loadConversations` again only when the accounts changed.
- [x] 4.3 Keep the configuration on an invalid file and show the first error in the status line.
- [x] 4.4 Unit tests: a new space shows after one refresh; an invalid file keeps the spaces; a dirty form stays dirty.

## 5. CLI writers

- [x] 5.1 Change `internal/cli/account.go` (add, remove, conversation set) and `internal/cli/whatsapp.go` to `config.Update`.
- [x] 5.2 Update the CLI tests that read the file after a command.

## 6. End-to-end and docs

- [x] 6.1 Add an e2e scenario in `test/e2e/spaces_test.go`: the TUI is open, the test appends a space to the file, waits for the name on tab 5, saves the Settings form, and checks that the file still has the space.
- [x] 6.2 Add the scenario to `test/e2e/SCENARIOS.md`.
- [x] 6.3 Update `README.md` (editing the file while i2a runs, the lock, comments) and `CLAUDE.md` (`config.Update` is the only write path).

## 7. Verify

- [x] 7.1 Run `task test`, `task lint`, `golangci-lint run --build-tags e2e ./test/...`, `task e2e` and `task leakcheck` on the last commit. Write the SHA in each result.
- [x] 7.2 Run the steps of `7-lost-external-edit.txt` again with the real binary and capture the screen and the file after the save.
- [x] 7.3 Open the PR with Summary, OpenSpec, Risk, Evidence, Not verified and Rollback.
