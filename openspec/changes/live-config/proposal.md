## Why

The dashboard reads `config.toml` one time, when it starts. A save writes the copy in the memory of the dashboard. When a second process changes the file while the dashboard is open, the dashboard does not show the change, and its next save deletes the change. A second process can be a text editor, an `i2a account` command or an AI agent. Agents will edit the configuration file in a later change, so this data loss must stop first.

## What Changes

- The dashboard checks `config.toml` on each refresh (2 seconds). When another process changed the file, the dashboard loads it again and shows the new items in all tabs.
- An invalid file from another process does not replace the configuration of the dashboard. The status line shows the first error.
- A save of the dashboard reads the file again, applies only the change of the form or the action, validates the result and writes it. A change of another process to a different item stays in the file.
- When another process changed the item of an open form that has unsaved edits, the save opens a dialog: "Reload" (default) discards the edits of the form; "Overwrite" saves them.
- When another process deleted the item of the form, the save fails with an error that names the item.
- Each writer of `config.toml` (the dashboard, `i2a account add`, `i2a account remove`, `i2a conversation set`, the version 1 conversion) holds the lock `config.toml.lock` during the read, change and write.
- The header of `config.toml` says that a save by i2a removes comments.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `configuration`: new requirements for changes from other processes (reload in the dashboard, a save that keeps other changes, the conflict dialog, the write lock).

## Impact

- `internal/config/config.go`: new `Update` (lock, load, change, validate, save) and a `Hash` of the file bytes; `Save` writes the new header; `Load` converts a version 1 file under the lock.
- `internal/tui/tui.go`: `ui.commit` takes a change function and calls `config.Update`.
- `internal/tui/tab_*.go`: each save gives a change function instead of a changed copy (15 call sites).
- `internal/tui/model.go`: the refresh checks the file; the conflict dialog.
- `internal/cli/account.go`, `internal/cli/whatsapp.go`: use `config.Update`.
- New file at run time: `config.toml.lock` next to `config.toml` (mode 0600).
- No change to the file format, the store or the daemon. The daemon already reloads the file when its modification time changes.
