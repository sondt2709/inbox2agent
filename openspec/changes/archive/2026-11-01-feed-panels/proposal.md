## Why

After the dashboard rethink, the user reviewed the Feed on a real screen:

- The card detail panel cannot scroll. A long card body or message list is cut at the bottom of the window.
- The category bar (All, Action, Question, Info) repeats a grouping that the space list does better.
- The number keys and labels of the areas (`1`, `2`, `3`) are not needed; a click is enough.
- `R` (upper case) is easy to press by mistake. Run now should be `ctrl` and a key.
- The card rows have empty columns at the left, a coloured category badge, and the channel in the same style as the time.

## What Changes

- The Feed has three panels: Spaces, Cards and Card. `←`/`→` move the keyboard between them, and the focused panel has the accent border. `↑`/`↓` work in the focused panel: select a space, select a card, or scroll the card. The mouse wheel and the trackpad scroll the panel under the pointer. `pgup`/`pgdn` scroll the card from each panel. The card goes back to the top when another card is selected, and its title shows `↓ more` when text is below.
- **BREAKING (dashboard keys):** the category bar and `f` are removed. The areas show no numbers, and `1`, `2`, `3` do nothing; a click opens an area (`tab` still goes to the next section). `ctrl+r` runs now; `R` does nothing. The old `ctrl+r` (reload) is removed, because the dashboard reloads every 2 seconds.
- Card rows start at the left edge, with no bar and no mark column. The category is plain text, the urgency keeps its coloured bars, and a new card has `●` before its title. The second line is the space, the channel in bold, and the time.
- The panels of the Spaces area also scroll with the wheel under the pointer.

## Capabilities

### Modified Capabilities

- `card-feed`: the panels of the Feed, the card rows, the space list, and `ctrl+r`.
- `configuration`: the header with no numbers, and the key hints.

## Impact

`internal/tui` (`model.go`, `tab_feed.go`, `tab_spaces.go`), the unit tests, the e2e scenarios that use `R`, `f`, `1`, `2`, `3`, and the README key tables. No data change.
