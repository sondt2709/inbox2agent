## Why

The Feed is the screen that the user reads most. Four parts of it are hard to use:

- The urgency is a word (`high`, `normal`, `low`) in the same color as the title. The eye does not find the high cards fast.
- `F` changes the space filter one step on each press. With five spaces the user presses `F` up to five times and does not see the next space before the press. The filter bar does not show how many cards each space has.
- At 140 columns the footer of the Feed cuts the last key: "· a".
- The key help (`?`) wraps the long lines of its text, so the model text and the Feed keys break into wrong lines.

The Activity tab has no space filter. The help of `i2a run` and `i2a daemon` still uses the old words "sources", "profiles", "interval" and "active hours".

## What Changes

- The Feed shows the urgency as bars: `▰▰▰` high (red), `▰▰▱` normal, `▰▱▱` low (faint). The detail panel shows the bars and the word.
- `F` opens a pick dialog: "All spaces" and each space, with the number of cards of each. Enter selects. The filter keeps the ID of the space, so a new or removed space does not move it.
- The Activity tab uses the same space filter: `F` opens the same dialog with the number of sends of each space.
- The footer shows only the key hints that fit the width. It never cuts a hint.
- The key help is a scrolling dialog as wide as its longest line (at most the window). It does not wrap a line that fits.
- `i2a run --help`, `i2a daemon --help` and their errors use the words account, space, agent and schedule.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `card-feed`: urgency bars in the Feed; the `F` dialog with counts; the space filter in Activity.
- `configuration`: the footer and the key help fit the window; the command help uses the words of the model.

## Impact

- `internal/tui/tab_feed.go`: `urgLabel`, `F` key, the space filter by ID.
- `internal/tui/tab_activity.go`: `F` key and the filter of the dispatches.
- `internal/tui/dialog.go`: the pick dialog shows a count; the viewer can have a fixed width.
- `internal/tui/model.go`: the help dialog; the footer fits whole hints.
- `internal/tui/style.go`: `fitHints`.
- `internal/cli/run.go`: help text and errors.
- No change to the configuration file, the store or the daemon.
