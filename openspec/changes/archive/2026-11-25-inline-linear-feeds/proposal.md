## Why

The feed editor of a Linear account is behind the key `f`, a pick dialog and a separate form. The user cannot see the feeds and their filters with the account, and a hidden key is not a button. The settings of an item must show in the panel of the item, with visible buttons.

## What Changes

- The detail panel of a Linear account shows each feed as a section under the account fields: Name, Kind, From, Prefix, Projects, Labels, Teams, and the button "Remove feed <id>".
- The button "+ Add feed" adds a new feed section (with a Feed ID) in the same panel.
- "Save changes" saves the account and all its feeds in one save. A removed feed loses its entries in all spaces.
- From, Projects, Labels and Teams show a visible "Choose" button that opens the names of the workspace. Enter in the field also opens it.
- The key `f`, the feed list dialog and the separate feed form are removed.
- The form engine gets an inline button field.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `configuration`: the feeds of a Linear account are edited in the account panel.

## Impact

- `internal/tui`: `form.go` (button field, Choose button of a list field, `form.changed`), `feeds.go`, `tab_accounts.go`, `model.go`, `live.go`.
- Tests: `internal/tui/feeds_test.go`, `test/e2e/linear_test.go` (S50).
- No change to the configuration file, the Linear source or the store.
