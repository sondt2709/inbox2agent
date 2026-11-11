## Why

The user wants a button for each shortcut, especially "add a conversation" and "run now". Today a click on a footer hint or on `ctrl+r run now` in the header does nothing, and the panel of a space only says "Press a to add conversations". The footer also drops the hints that do not fit (`Sort s` and `Done cards a` in the Feed at 130 columns), so some actions have no visible text at all.

## What Changes

- Each footer hint that names one key becomes a button. A click sends that key through the same path as the keyboard, so the focused list, form or dialog gets it. Hints that only move the cursor (`Move ↑↓`, `Read level ←→`, `Type`) stay text. A button has a dim background (a chip) and the format of PR #25: the label in the primary color, then the key in grey.
- When some hints do not fit, the footer shows a `More…` button. It opens a pick dialog with all the actions of the current screen; a click or `enter` on a row runs that action.
- The footer at the right always has the buttons `Find /`, `Keys ?` and `Quit q`.
- The header shows `Run now ctrl+r` as a button. A click runs the same work as `ctrl+r`: all spaces, or the selected space in the Spaces area.
- The panel of a space has the row `+ Add conversations` under "Conversations · N". `enter` or a click opens the add dialog of the space, the same as `a`.
- A click on a row of a pick dialog chooses it. A click on a row of the spaces dialog of a conversation checks or unchecks it. A click on a row of the add dialog adds or removes it. A click on a row of the search moves the cursor, and a second click opens the spaces dialog of the row.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `configuration`: the requirements "Key hints and key help fit the window", "Spaces area", "Spaces dialog of a conversation" and "Search of conversations".
- `card-feed`: the requirement "Read and send now from the dashboard" (the header button).

## Impact

- `internal/tui/style.go` (`renderHints`, `fitHints`, the chip style), `internal/tui/model.go` (header and footer click zones, `press`, the `More…` dialog, the help text), `internal/tui/tab_spaces.go` (the add row), the `hints` lists of the tabs and dialogs, `internal/tui/dialog.go` and `internal/tui/dialog_spaces.go` (row clicks).
- `test/e2e/harness/tmux.go` (mouse clicks), a new e2e scenario S48, `test/e2e/SCENARIOS.md`.
- `README.md` and `CLAUDE.md` (the dashboard text).
- No change to the configuration file, the store, the daemon API or the dependencies.
- This change is on top of PR #25 (`feat/dialog-buttons-footer`), which adds OK and Cancel to the add dialog and the `Label key` footer format.
