## Context

The add dialog is `searchDialog` with a space. Before this change, `space` called `editLinks` for each row, and `←`/`→` called `toggleLevel`: each key was one save. Dialogs get all messages, mouse clicks included, but a dialog did not know where the model draws its box.

## Goals / Non-Goals

**Goals:**

- OK and Cancel in the add dialog, with the keyboard and the mouse.
- One save for all changes of the dialog.
- The footer format `Label key` in all areas and dialogs.

**Non-Goals:**

- Buttons in the other dialogs (the spaces dialog of a conversation, confirmations). The user accepted them.
- The key help (`?`) and the header (`ctrl+r run now`) keep their format: they are not the footer.

## Decisions

- **Pending changes in the dialog.** `searchDialog.pending` maps `convKey` to `spacePick{in, level}`. `pick` returns the pending state, else the saved state; `set` deletes an entry that is equal to the saved state, so the count shows only real changes. OK calls `editLinks` one time with all changed rows; the edit changes the link of the space in its place (the order of the links stays, so `config.Route` does not change), adds it, or deletes it, and gives `orNoSpace` for a row with no link. This is the same result as the old `toggle` and `toggleLevel`.
- **Buttons.** The dialog draws OK with `btnPrimary` and Cancel with `btnNormal`, the styles of the form buttons. A focused button has `▸` and an underline, as in a form. The dialog records the body line and the columns of each button at each render.
- **Mouse.** `model.dialogBox` returns the position and the size of the box; `View` and `dialogPoint` use it, so the click and the drawing cannot differ.
- **Footer colors.** "Primary color" is the accent `#7D56F4` (the color of the primary buttons and the active tab). "Secondary color" is the grey `244` (`mutedFg`). `renderHints` writes the first letter of the label as a capital, so the hint lists stay as they are.

## Risks / Trade-offs

- A user who pressed `space` and then `esc` now loses the change. The mark `*` and the text "N changes not saved" show the pending state, and `esc` is Cancel in each dialog of i2a.
- The purple label on a dark background has less contrast than the old white key. The label is bold.
