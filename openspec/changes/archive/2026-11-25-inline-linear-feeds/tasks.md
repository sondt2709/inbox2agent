## 1. Form engine

- [x] 1.1 Inline button field (`kindButton`): focus, enter, space, click; `form.changed` for dirty
- [x] 1.2 The Choose button of a text field with `pick`: drawn after the box, a click opens the list

## 2. Account panel

- [x] 2.1 Feed sections in the Linear account form: header with the spaces, fields, "Remove feed <id>" / "Keep feed <id>", "+ Add feed"
- [x] 2.2 "Save changes" saves the feeds, removes the entries of removed feeds, refuses no feed
- [x] 2.3 Remove the key `f`, the feed list dialog, the feed form and `model.feedForm`

## 3. Tests, docs, evidence

- [x] 3.1 TUI tests for each scenario; E2E S50 with the inline sections
- [x] 3.2 Break each new test one time and confirm that it fails
- [x] 3.3 Update `README.md`, `CLAUDE.md` and the PR body (screenshots)
- [x] 3.4 `task test`, `task lint`, e2e lint, `task leakcheck`, `openspec validate inline-linear-feeds --strict`, CI
