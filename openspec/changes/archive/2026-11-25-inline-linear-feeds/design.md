## Context

The previous change added a feed form that replaced the account form (`model.feedForm`), opened with the key `f` and a pick dialog. The user rejected it: settings must be in the panel of the item, with visible buttons.

## Goals / Non-Goals

**Goals:** the feeds and their filters are visible and editable in the account panel; each action is a visible button.

**Non-Goals:** a change of the names dialog (a multi-select list with search needs a box); a change of the feed ID of a saved feed.

## Decisions

1. **One form for the account and its feeds.** `accountForm` adds one section for each feed (`feedSection`: header note, fields with the key prefix `f<n>.`, the button "Remove feed <id>"), then the button "+ Add feed". "Save changes" builds `Account.Feeds` from the sections that are not removed, and removes the entries of the removed feeds in all spaces, in one `commitForm` with the account key, so the conflict check covers the feeds.
2. **Inline button field (`kindButton`).** It takes the focus like a field; enter, space or a click runs it. "Remove feed" hides the fields of the section and becomes "Keep feed <id>"; for a new section it removes the section. The form gets `changed`, so an added or removed section makes the form dirty.
3. **The Choose button of a list field.** A text field with `pick` draws a `Choose` button after its box. A click on it or enter in the field opens the names dialog.
4. **The header of a section** names the feed and where its conversation is: the spaces that list it, else Everything else.
5. **Remove** `model.feedForm`, the key `f`, the feed list dialog and the feed form.

## Risks / Trade-offs

- [A long form with many feeds] → the form engine scrolls to the focused item; the sections have headers.
