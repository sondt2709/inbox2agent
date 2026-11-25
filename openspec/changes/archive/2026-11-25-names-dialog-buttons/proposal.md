## Why

The names list of a feed field had no save button and no close button: only ctrl+s applied it. Enter checked a row there, but enter is OK in the add dialog. The Choose button of a field was not a stop of the keyboard. These break `docs/ui-principles.md` (a UI control for each action, arrow keys to each component, the same keys in all of the dashboard).

## What Changes

- The names list gets the buttons OK and Cancel, and the keys of the add dialog: the search line first; enter, ↓ or tab go to the list; space or a click checks a row; enter or OK applies; tab goes to the buttons; esc or Cancel closes; ctrl+s is OK.
- The Choose button of a field is a stop of the keyboard (tab and the arrow keys). Enter or space on it opens the list. Enter in the field moves to the next stop, as in each text field.
- `docs/ui-principles.md` gets two rules: the common practice of a dashboard UI, and the same keys in all of the dashboard.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `configuration`: the keys and the buttons of the names list and of the Choose button.

## Impact

- `internal/tui/feeds.go`, `internal/tui/form.go`, the tests, S50, `README.md`, `docs/ui-principles.md`.
