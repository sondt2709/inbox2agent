## Context

`internal/config` decodes `config.toml` with go-toml v2 and `DisallowUnknownFields`, fills defaults (`applyDefaults`) and validates (`Validate`). Some fields have a `comment` tag; go-toml writes it as a comment above the key. Since live-config, `config.Update` and `Save` write the file under `config.toml.lock`, and the dashboard and the daemon load a changed file.

## Goals / Non-Goals

**Goals:**
- An agent can learn the file, write it and check it with no item command.
- Editors show help and errors with the schema, offline and for the version of the installed binary.
- One source of truth: the Go types.

**Non-Goals:**
- Keep comments that a person or an agent writes (a save of i2a removes them, as before).
- A lock that a shell script can take. An agent writes a temporary file and renames it; the dashboard detects a change by its hash.
- Validate references in the schema (JSON Schema cannot check that a space names an agent that exists). `i2a config check` does this.

## Decisions

1. **A small reflection generator, no library.** `Schema()` walks the `Config` type: a struct is an `object` with `additionalProperties: false`; a slice is an `array`; `string`, `int` and `bool` map to their JSON types; `Duration` is a `string` with a pattern. Tags: `toml` gives the key; `comment` or else `doc` gives the `description`; `enum:"a,b"` gives `enum`; `schema:"required"` adds the key to `required`. The output has sorted keys (`encoding/json` sorts map keys), so it is stable. A library (invopop/jsonschema) adds a dependency for 150 lines.
2. **`doc` tag for the new descriptions.** A new `comment` tag on a field in an array (for example each `[[spaces]]`) makes go-toml write the comment for each entry. `doc` does not change the TOML output.
3. **Required keys are only the keys with no default**: the `id` of each item, `platform` of an account, `account` and `id` of a conversation, `agent` and `schedule` of a space. Other keys get defaults, so a short file is valid.
4. **The schema file is next to the file, written on load and on save.** The `#:schema` line has a relative path, so the file works offline and matches the version of the binary. `Load` and `save` write `config.schema.json` only when its bytes are different; an error is ignored, because the schema only helps editors. A file of an older i2a gets the `#:schema` line on its next save.
8. **Keys with a default accept the empty value.** `applyDefaults` fills `""` and `0`, so the schema allows them (`schema:"default"` adds `""` to the enum; `min` and `max` become `anyOf` with `const 0`; the duration pattern allows `""` and `0`). The schema is looser than `Validate`, never stricter, so an editor never marks a file that i2a accepts.
5. **`docs/config.schema.json` is a committed copy** for readers of the repository. `TestSchemaCopyIsCurrent` compares it with `Schema()`; `go test ./internal/config -run TestSchemaCopyIsCurrent -update` writes it again.
6. **`i2a config check` uses `config.Parse` and `render.CheckAgents`**, the same checks as the daemon and the dashboard. It does not call `Load`, so it never converts or writes a version 1 file.
7. **A hidden `conversation` stub for one major version.** `i2a conversation set ...` exits with code 2 and gives the TOML to write. A script gets a clear error, not "unknown command". The old `source` stub goes away.

## Risks / Trade-offs

- [An agent and the dashboard write at the same time] → The dashboard detects the new hash on its next refresh or save, and keeps the change of the agent for a different item; for the same item, it opens the conflict dialog. The agent loses its change only when it writes without a rename and the dashboard reads a partly written file; the dashboard then shows the error and keeps its configuration.
- [The schema allows a file that `Validate` rejects] → Expected: `i2a config check` is the full check, and the help says so.
- [Editors without `#:schema` support] → The line is a TOML comment; it has no effect.
