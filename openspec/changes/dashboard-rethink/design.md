## Context

The model has a set of links for each conversation (change `multi-space-conversations`). The dashboard still showed the one-space model, and added `L` beside `m`. The proposal page `i2a dashboard rethink` shows the target screens. This design keeps all data formats and changes only how the dashboard shows and edits them.

## Goals / Non-Goals

**Goals:** one action for the spaces of a conversation; a fixed place for "run now"; separate daily use (Feed) from routing (Spaces) and setup (Setup); show only the conversations that need a decision.

**Non-Goals:** no change to `config.toml`, the store schema, the daemon API or the CLI. No new item type. The forms of accounts, agents, schedules and settings stay as they are.

## Decisions

1. **Areas over tabs.** The internal tab IDs stay (Feed, Spaces, Accounts, Agents, Schedules, Settings, Send log). The header shows three areas: an area is a group of tab IDs. `1`, `2`, `3` open an area. `tab` and `shift+tab` go through all tab IDs in order, so the Setup sections are one key away. Row 1 shows the sections of Setup, or the last run. The content starts at row 2. Other way: a second rail in Setup. Rejected: three columns do not fit 100 columns.
2. **One function for all links.** `config.SetSpaces(account, id, name, links)` removes all entries of the conversation and writes the given links: spaces with a level, or one Unsorted entry (a level, or `off` for Ignore). An empty list removes the entries, so the conversation follows `unsorted.read`. The first label stays. The dialog, the search, `x` and `i` all use it, so the file always has one valid shape.
3. **Rows of the Spaces area.** The right panel is one list. For a space the first rows are settings (Name, Sends to, When). Enter on such a row opens an input or pick dialog that saves at once. The conversation rows follow. The keys work on the row under the cursor: `←`/`→` read level of this link, `x` remove from this space, `enter` the spaces dialog, `n` label, `space` select. With a selection, `enter` opens one dialog for all selected rows.
4. **Everything else.** It is the Unsorted space with a new name. Its rows are the conversations with no space and no `off`, newest activity first. `←`/`→` on a row writes an Unsorted entry with that level. Its settings rows are "New and other conversations read", "Sends to" (with "hold") and "When".
5. **New conversations.** The dashboard writes the IDs that it showed to `<data>/conversations-seen.json` (0600), by account. The first listing of an account marks all its conversations as seen, so an upgrade does not show 475 new rows. `k` marks a row as seen, `K` all rows, `i` ignores. The file is UI state: a lost file shows nothing new until the next listing seeds it again.
6. **Save at once and undo.** Each change in the Spaces area calls `ui.commit` as before. `ui.commit` keeps the configuration before the change and the hash after it. `u` writes that configuration back when the file still has that hash; else it says that another process changed the file. One level of undo. The Feed keeps `u` for unread.
7. **Old messages for a new link.** `Store.Requeue(account, conversation, since, forMeOnly)` sets `sent` and `previewed` messages after `since` back to `pending`. The fan-out of `Engine.Dispatch` skips each space whose delivery is `sent`, so only the new spaces get them. `settle` gives the messages their state again in the next round. The dialog shows the option only when a space is added.
8. **Footer and header.** The footer shows the hints of the focused list, main key first, and `? keys · q quit` at the right. The header shows the areas at the left, and at the right `● running · next 11:15 · R run now`. A narrow window drops "next" first, then the service text.

## Risks / Trade-offs

- [Muscle memory: the tab numbers change] → the help (`?`) and the README list the new keys; `tab` still goes through all sections.
- [Save at once can save a wrong click] → `u` undo, and the status line names each saved change.
- [Requeue sends a message a second time to an agent] → off by default, only for the new spaces, only within `lookback`.
- [`conversations-seen.json` and a second dashboard] → each write reads the file and joins the sets.

## Migration Plan

No data migration. The first start writes `conversations-seen.json` with the listed conversations. Rollback: install the older binary; it ignores the file.
