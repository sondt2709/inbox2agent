## 1. Names of the workspace

- [x] 1.1 `CatalogQuery` and `Source.Catalog` in `internal/source/linear/catalog.go`: teams, projects, labels, users; pages; labels one time without case
- [x] 1.2 Add `catalog.json` to the capture tool (team keys, team names, projects, labels and users replaced) and capture it with a real key; read the output
- [x] 1.3 Tests with the fixture: the names, the pages, only queries

## 2. Dashboard

- [x] 2.1 `Deps.Catalog` in `internal/tui` and its value in `internal/cli`
- [x] 2.2 A text field that opens a list (`formField.pick`), and the list dialog with check marks, search and "+ Add"
- [x] 2.3 "Edit feeds" (button and key `f`), the feed list, the feed form (new and saved feed), Save, Cancel with the question, Remove with its entries
- [x] 2.4 TUI tests: label from the list, typed project, new feed, remove feed and its entries, remove the last feed, names that cannot be read

## 3. Tests, docs, evidence

- [x] 3.1 E2E scenario: the feed form in the real binary against the fake Linear server; the saved `config.toml`
- [x] 3.2 Break each new test one time and confirm that it fails
- [x] 3.3 Update `README.md` and `CLAUDE.md`
- [x] 3.4 `task test`, `task lint`, e2e lint, `task leakcheck`, `openspec validate edit-linear-feeds --strict`
- [x] 3.5 Screenshots of the feed form and the name list (fake endpoint), and the real workspace name query
