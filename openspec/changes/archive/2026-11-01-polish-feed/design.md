## Context

The Feed draws each card in two rows (`viewFeed` in `internal/tui/tab_feed.go`). The space filter is an index into `feedSpaces()` (`model.feedSpace`). The footer joins all hints and cuts the result with `truncateStyled` when it is too long. The key help is a `msgDialog`, which wraps its text at 72 columns.

## Goals / Non-Goals

**Goals:**
- Show the urgency as a shape and a color that the eye finds fast.
- Select a space in one step and see the number of cards first.
- One space filter for the Feed and Activity.
- No cut text in the footer and in the key help.

**Non-Goals:**
- A filter for many spaces at one time.
- Change the card data, the store or the configuration.

## Decisions

1. **Bars, not emoji.** `▰` and `▱` (U+25B0, U+25B1) are one column wide in all usual terminal fonts. Emoji (🔴) are two columns wide in some terminals and break the columns of the list. The color stays: red for high, the text color for normal, faint for low.
2. **The filter keeps the space ID** (`model.spaceFilter`, "" for all). An index moves when another process adds or removes a space (live-config). When the space of the filter does not exist any more, the filter is all spaces.
3. **One filter for two tabs.** The Feed and Activity read the same `spaceFilter`. The user who filters the cards of one space also wants its sends. The filter bar of each tab shows the space.
4. **The pick dialog shows a count as its note.** `pickDialog` already has a `note` column. The cursor starts on the current filter.
5. **The footer drops whole hints from the end** (`fitHints`). The right part (`? keys`) stays, so the user can find each key in the help.
6. **The key help is a `viewerDialog` with a width.** A new field `width` sets the width to the longest line plus the border. The viewer scrolls when the window is short. The help text gets shorter lines (at most 88 columns).

## Risks / Trade-offs

- [A font without `▰`] → The terminal shows a box. The detail panel still shows the word. Low risk: the glyphs are in the Geometric Shapes block, which all usual monospace fonts have.
- [The `F` key now opens a dialog] → One more key press (enter) for a user who pressed `F` one time to get the first space. The dialog shows all spaces, so it is fewer presses with three spaces or more.
