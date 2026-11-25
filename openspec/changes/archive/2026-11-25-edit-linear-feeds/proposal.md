## Why

The user filters a Linear feed by project, label, team, person and prefix. The dashboard shows the feeds but cannot change them: the user must edit `[[accounts.feeds]]` in `config.toml` by hand and type the exact names. The dashboard is the main interface of the user, so it must edit the feeds and offer the names of the workspace.

## What Changes

- The detail of a Linear account in Setup › Accounts gets the button "Edit feeds" (key `f`). It opens a list of the feeds of the account and "+ New feed".
- A feed form in the detail panel: name, kind, from, prefix, projects, labels and teams. A new feed also has an ID. "Save feed" saves it in `config.toml`; "Remove feed" removes it and its links in all spaces.
- In the fields From, Projects, Labels and Teams, enter opens a list of the names of the workspace with check marks and a search. The user can also type names separated by commas.
- The Linear source gets a read-only query for the names: teams, projects, labels and users of the workspace.
- A new feed is a new conversation in Everything else.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `configuration`: the Accounts section edits the feeds of a Linear account.
- `linear-source`: a query that reads the names of the workspace for the feed form.

## Impact

- `internal/source/linear`: new `catalog.go` (`CatalogQuery`, `Source.Catalog`), fixture `testdata/catalog.json`, the capture tool.
- `internal/tui`: `linear.go` (feed form, name list dialog), `tab_accounts.go`, `model.go`, `form.go` (a text field can open a list), `tui.go` (`Deps.Catalog`).
- `internal/cli`: `Deps.Catalog` of the dashboard.
- One more read-only request to `https://api.linear.app/graphql` when the feed form opens. No new dependency. No store or file format change.
