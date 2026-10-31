## Why

AI agents and scripts change the configuration of i2a. Since the live-config change, the dashboard shows each change of `config.toml` and keeps it on a save. So the file can be the one interface for agents: an agent reads and writes TOML, and needs no command for each item. Now an agent has no description of the file, no check of a file before i2a reads it, and no list of conversation IDs in a format that it can read. `i2a conversation set` changes one field of one item, and it is the only item command; it duplicates the file. The hidden `i2a source` command only says that it is renamed.

## What Changes

- **BREAKING** Remove `i2a conversation set`. A hidden stub prints how to do the same change in `config.toml` and exits with code 2. Remove the hidden `i2a source` stub.
- i2a writes `config.schema.json` (JSON Schema, draft 2020-12) next to `config.toml` on each save, and the first line of `config.toml` is `#:schema ./config.schema.json`. Editors with Taplo or Tombi (for example VS Code with Even Better TOML) show the description of each key, complete the values and mark errors.
- The schema comes from the Go types of `internal/config` (the `toml`, `comment`, `doc`, `enum` and `schema` struct tags). No new dependency. `docs/config.schema.json` is a committed copy; a test fails when it is different.
- New `i2a config check [file]`: the full validation (the schema, references between items, duplicates, templates). It prints each problem and exits 1, or prints a summary and exits 0.
- New `i2a config schema`: prints the schema.
- `i2a config --help` explains the file for an agent: the path, the items, how to find IDs, the check, and how to write the file safely.
- New `i2a conversations --json`: one JSON object for each conversation (account, id, kind, name, label, space, read).
- The e2e scenarios S38 and S39 write `config.toml` by hand instead of `i2a conversation set`.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `configuration`: the CLI commands (remove `conversation set`, add `config check`, `config schema`, `conversations --json`); the schema file and the `#:schema` line; the writers of the lock.

## Impact

- New `internal/config/schema.go` and `docs/config.schema.json`.
- `internal/config/config.go`: struct tags; `save` writes the `#:schema` line and the schema file.
- `internal/cli/configcmd.go`: `check`, `schema` and the help. `internal/cli/account.go`: remove `conversation set` and `source`; `conversations --json`.
- `test/e2e/spaces_test.go`, `test/e2e/scenarios_test.go`, `test/e2e/SCENARIOS.md`.
- `README.md`: the file as the interface; the command table.
- Major release (`feat!`). A script that uses `i2a conversation set` must edit `config.toml` instead.
