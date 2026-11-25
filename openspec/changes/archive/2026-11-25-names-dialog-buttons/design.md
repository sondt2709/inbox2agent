## Context

The add dialog (`searchDialog`) is the check list of the dashboard: a search line, rows with check marks, and OK and Cancel. The names list must work the same way.

## Decisions

1. `namesDialog` copies the keys, the buttons and the click zones of `searchDialog` (`btnNone`, `btnOK`, `btnCancel`, `keyButton`, `btnY`, `btnX`).
2. A form item can be the Choose button of a field (`item.choose`). `items()` adds it after the field; `render` draws `▸ Choose ▾` when it has the focus; the click zone of the button is its own item.

## Risks / Trade-offs

- [One more stop for each list field] → tab moves one more time; the arrow keys and a click reach each stop.
