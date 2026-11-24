## Why

Feedback after the user tried the build of PR #24:

- The add dialog of a space (`a` in the Spaces area) saves each `space` at once. It has no OK and no Cancel, so the user cannot see where the change ends, and cannot leave without a change.
- The footer writes each hint as the key, then the label (`r reply`). The user wants the label first, in the primary color, and the key after it, in the secondary color: `Reply r`.

## What Changes

- The add dialog keeps the changes of `space` and `←`/`→` in the dialog until the user selects OK. A changed row has the mark `*`, and the button line shows the number of changes that are not saved.
- The add dialog has two buttons under the list: **OK** (primary) and **Cancel**. `tab` moves from the list to OK, then to Cancel. `enter` in the list and `ctrl+s` also run OK; `esc` runs Cancel. A mouse click on a button runs it.
- OK writes all changes in one save (one `u` undoes all of them). Cancel and `esc` close the dialog with no change.
- The title of the add dialog is "Add conversations to <space>".
- Each footer hint shows the label with a capital first letter in the primary color (the accent `#7D56F4`), then the key in the secondary color (grey): `Reply r`, `Keys ?`, `Quit q`.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `configuration`: the requirements "Key hints and key help fit the window" and "Search of conversations".

## Impact

- `internal/tui/dialog_spaces.go` (`searchDialog`), `internal/tui/style.go` (`renderHints`), `internal/tui/model.go` (`dialogBox`, `dialogPoint`), `internal/tui/dialog.go` (the hints of a confirmation keep the case of the label).
- Unit tests in `internal/tui` and the footer text in three e2e tests.
- `README.md` and `CLAUDE.md`.
- No change to the configuration file, the store or the daemon.
