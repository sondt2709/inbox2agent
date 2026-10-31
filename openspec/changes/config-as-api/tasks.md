## 1. Tests that fail on the base

- [x] 1.1 Test: `Schema()` describes each key, the enums of `read`, `platform`, `driver`, `session`, `payload`, `days` and `min_urgency`, the duration pattern, and has no unknown key
- [x] 1.2 Test: `docs/config.schema.json` equals `Schema()`
- [x] 1.3 Test: a save writes `#:schema ./config.schema.json` as line 1 and writes `config.schema.json`
- [x] 1.4 Test: `i2a config check` on a valid file, on a file with two problems, on a missing file
- [x] 1.5 Test: `i2a config schema` prints the schema; `i2a config --help` names the file, the check and the JSON list
- [x] 1.6 Test: `i2a conversations --json` prints the keys; `i2a conversation set` exits 2 and does not write

## 2. Schema

- [x] 2.1 `doc`, `enum` and `schema` tags on the types of `internal/config`
- [x] 2.2 `internal/config/schema.go`: `Schema()`
- [x] 2.3 `save` writes the `#:schema` line and `config.schema.json`
- [x] 2.4 `docs/config.schema.json` and the `-update` flag

## 3. CLI

- [x] 3.1 `i2a config check [file]`, `i2a config schema`, the help of `i2a config`
- [x] 3.2 `i2a conversations --json`
- [x] 3.3 Remove `conversation set` (hidden stub with exit 2) and `source`; update the help that names them

## 4. E2E and docs

- [x] 4.1 S38 and S39 write `config.toml` by hand and run `i2a config check`; `addAccount` edits the file
- [x] 4.2 README: the file as the interface for agents, the command table; CLAUDE.md
- [x] 4.3 Check the schema with Taplo on a good and a bad file

## 5. Check

- [x] 5.1 Break each change one time and see the new test fail
- [ ] 5.2 `task test`, `task lint`, e2e lint, `task e2e`, `task leakcheck`, `openspec validate --strict`
- [ ] 5.3 Archive the change and open the PR
