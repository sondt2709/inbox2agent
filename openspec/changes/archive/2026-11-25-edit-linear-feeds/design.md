## Context

`config.Feed` has `ID`, `Name`, `Kind`, `From`, `Prefix`, `Projects`, `Labels` and `Teams`. The match rule compares names without case (`match.go`). The Accounts section shows a feed as one line (`feedLine`). The form engine (`form.go`) has text fields, choices and buttons; the pick dialog (`dialog.go`) chooses one row.

## Goals / Non-Goals

**Goals:**
- Add, change and remove a feed of a Linear account in the dashboard.
- Choose projects, labels, teams and people from the names of the workspace.

**Non-Goals:**
- A change of the feed ID (it is the conversation ID; a change would lose the links and the cursor).
- Live search in Linear. The form reads the names one time when it opens.

## Decisions

1. **The feed form replaces the account form in the detail panel.** `model.feedForm` is `<account>/<feed>` (or `<account>/+` for a new feed), and `formTarget` returns the key `feed:<account>/<feed>`. `formItem` maps this key to the account, so the conflict check of `commitForm` compares the whole account. Esc and "Cancel" go back to the account form; a changed form asks first. Another choice was one long account form with all the feeds; it is hard to read with three feeds or more.
2. **Lists are text fields that can open a list.** A text field gets `pick`: enter opens the list dialog, and the user can still type `a, b`. When the names cannot be read (no network, a revoked key), the user types the names, or adds a typed name in the dialog.
3. **The list dialog (`namesDialog`) has check marks and a search.** Typing filters the rows; enter switches the check mark of the row; ctrl+s applies; esc cancels. When no row matches the search, the one row is `+ Add "<text>"`. The rows are the names that were checked when the dialog opened, the added names, then the other names of the workspace; the order does not change while the dialog is open, so the cursor does not jump.
4. **One query for all the names (`CatalogQuery`).** `teams`, `projects`, `issueLabels` and `users`, 250 on each page, with `@include` for the lists that have more pages, at most 10 pages. The team row shows `<key> · <name>` and saves the key. The user list starts with `me`. Label names repeat in teams, so the list keeps each name one time, without case. The TUI keeps the names of each account for the session (`model.catalogs`).
5. **Remove feed** removes the feed and each `[[spaces.conversations]]` entry of it in one save. The last feed cannot be removed, because a Linear account needs one feed or more.

## Risks / Trade-offs

- [A large workspace has more than 2,500 names in a list] → the list shows the first 10 pages; the user can type any other name.
- [The name list costs complexity points] → one request when the feed form opens, then the names stay in memory.
