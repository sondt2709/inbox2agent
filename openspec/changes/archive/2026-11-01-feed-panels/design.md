## Decisions

1. **Focus of the Feed.** `model.feedFocus` is 0 (Spaces), 1 (Cards) or 2 (Card). Below 100 columns the space list is hidden, and focus 0 is skipped. The focused panel uses `detailPanel(..., true)`, the accent border that a form already uses. A reply or ask form opens in the Card panel; `enter` in that panel gives the keyboard to the form.
2. **Scroll of the card.** `cardDetail` renders all lines; `model.cardOff` is the first visible line. `moveFeed` sets it to 0. The panel title adds `↓ more` when lines are below and `↑` when lines are above.
3. **Wheel.** A `MouseWheelMsg` goes to the panel whose columns hold the pointer, and gives it the focus. In the Spaces area the left list moves its cursor, and the panel moves its row cursor.
4. **Keys.** `ctrl+r` is the run key in all areas (the selected space in Spaces). The area numbers are gone from the header and the help. `tab` and `shift+tab` stay as the keyboard way to change the area.
5. **Card rows.** Line 1: the category (8 columns, plain), the urgency bars, `● ` for a new card, the title, filling the panel width. Line 2: the space, the channel (the `group` of the card) in bold, the time, and the marks (replied, asked, state). The selected card has the selection background on both lines, with no bar.

## Risks / Trade-offs

- [`←`/`→` no longer change the space] → the space list takes the keyboard with `←` and moves with `↑`/`↓`; `F` still opens the space dialog.
- [No category filter] → the category stays on each row, and `s` sorts by urgency.
