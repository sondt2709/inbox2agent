## Context

The dashboard (`internal/tui`) handles mouse clicks in `model.click`: the area names, the Setup sections, the list rows and the form fields. When a dialog is open, `model.update` gives each message, mouse clicks included, to the top dialog. Since PR #25, only the add dialog (`searchDialog` with a space) reads clicks, and only on OK and Cancel. `model.dialogBox` and `model.dialogPoint` (PR #25) give the position of a dialog box and of a click in its body.

The footer text comes from `hints()` of the top dialog, the form hints, or `model.tabHints()`. `fitHints` keeps the hints that fit and drops the rest. `renderHints` writes `Label key` (PR #25). The header writes `ctrl+r run now` as text.

## Goals / Non-Goals

**Goals:**

- A button for each action that has a key, with exactly the behavior of the key.
- Buttons for "add a conversation" and "run now" in the content, not only in the footer.
- A way to reach the actions that the footer drops.
- Mouse-only use of the add flow: open, add rows, OK.

**Non-Goals:**

- An action bar in the Card panel of the Feed. The footer buttons cover these actions.
- Hover effects. The dashboard uses `MouseModeCellMotion`, which reports no motion without a pressed button.
- Clicks on the rows of the Send log or the sessions dialog beyond what exists now.

## Decisions

- **One key path.** `model.press(key)` builds the `tea.KeyPressMsg` of a key name (`keyPress`: `enter`, `esc`, `space`, `tab`, the arrows, `ctrl+<letter>`, one character) and calls `model.update` with it. So a button reaches the top dialog, the active form or the tab, exactly as a key does. The alternative, a function for each button, would copy the key logic and could differ from it.
- **Which hints are buttons.** `keyName` returns the key name when `keyPress` knows the key: one key name, or one arrow (`←` is `left`). Two keys (`↑↓`, `←→`, `pgup/pgdn`, `tab ↑↓`) or a word (`type`) stay text. The hint lists do not change shape.
- **Click zones.** `layoutHints(hs, x)` returns the text and one zone (`x0`, `x1`, key) for each button. The footer and the header record their zones in `model.barZones` at each render, with the row (`0` or `m.h-1`). `model.update` checks `barZones` for a left click before it gives the click to the dialog, so the footer of a dialog also works. A click on the header button while a dialog is open does nothing, the same as `ctrl+r` in a dialog.
- **Look.** A button is a chip: the background `#26232F`, a space at each side, the label in the accent (bold), the key in grey. Two spaces separate the chips. A text hint has no chip. This is option A of the mockup, with the colors of PR #25.
- **`More…`.** `fitHints` reports whether it dropped a hint. Then the footer keeps space for a `More…` chip at the end of the left part (it drops one more hint when it must). `More…` opens a pick dialog "Actions" with each button hint of the current screen: the label, and the key as the note. Choosing a row closes the pick and calls `press` with the key, so the action goes to the screen under the pick. The hint lists of the tabs get the keys that they did not list (for example `F` in the Feed list, `u` and `ctrl+r` on the rows of a space), so that `More…` holds all the actions. The Accounts hint `4 its conversations` is removed: the number keys went away in PR #14, and `4` does nothing. `a add an account` and `a add a schedule` take its place.
- **Global buttons.** The right part of the footer is `Find /`, `Keys ?` and `Quit q` when no dialog and no form is active.
- **Header button.** The header writes `Run now ctrl+r` as a chip. A click calls `press("ctrl+r")`. While a run is busy, the header shows the spinner and no button.
- **Add row.** `panelRows` of a user space adds the selectable row `+ Add conversations` (kind `add`) under the heading "Conversations · N". `enter` or a click on it opens `newSearchDialog(space)`. The empty text becomes "No conversation yet. Add conversations from any account." with no key, because the row is the call to action. When the space has no conversation, `enter` on the space starts the panel on this row.
- **Row clicks in dialogs.** Each dialog with rows records the body line of its first visible row and the first visible index at each `view`, as the add dialog does for its buttons. `pickDialog`: a click runs the row. `spacesDialog`: a click moves the cursor and checks or unchecks the row (`space`). `searchDialog` with a space: a click moves the cursor and adds or removes the row. `searchDialog` without a space: the first click moves the cursor, and a click on the selected row opens its spaces dialog, the same rule as the list panels (`clickList`).

- **Key help.** The line `Areas` says that a click opens an area, a section or a button. The help keeps its height, so it fits 140 × 34 with no scroll.

## Risks / Trade-offs

- [The chips take 2 more columns for each hint, so fewer hints fit.] → `More…` lists the rest, and the order of each list keeps the main key first.
- [A footer zone and the drawn text can differ.] → `layoutHints` makes both from one loop, and a unit test clicks each zone of a rendered footer.
- [A new panel row moves the row indexes of a space.] → The row is above the conversation rows; the tests that count rows get one more `down`.
- [Terminal apps send different mouse sequences.] → Bubble Tea parses the SGR mode; the e2e test drives tmux, which sends SGR. Other terminal apps are not tested.
