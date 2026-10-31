## Context

`config.toml` has more than one writer: the dashboard (15 save sites in `internal/tui/tab_*.go`), `i2a account add`, `i2a account remove`, `i2a conversation set`, the WhatsApp pairing in `internal/cli/whatsapp.go`, and the version 1 conversion in `config.Load`. A later change (config-as-api) makes the file the interface for AI agents, so a person and an agent can change it at the same time.

Today:

- `tui.New` copies the configuration one time (`internal/tui/tui.go:129`). The refresh every 2 seconds (`refreshEvery`, `internal/tui/model.go:55`) reads the store only.
- Each save site makes `next := clone(m.u.cfg)`, changes `next`, and calls `ui.commit(next, ...)`. `commit` validates and calls `config.Save` (`internal/tui/tui.go:166`). The copy in memory replaces the file.
- `config.Save` writes a temporary file and renames it (`internal/config/config.go:402`). It has no lock.
- The daemon reloads the file when its modification time changes (`internal/daemon/loop.go:64`). It never writes the file.

Evidence of the defect: `tmp/evidence/tui-admin-ux/before/7-lost-external-edit.txt` (git-ignored).

## Goals / Non-Goals

**Goals:**

- The dashboard shows a change of another process within 3 seconds.
- A save never removes a change of another process to a different item.
- A save that would overwrite a change of another process to the same item asks the user first.
- Two writers at the same time do not lose a change.

**Non-Goals:**

- Keep the comments of the user in `config.toml`. `go-toml/v2` cannot write a file again with its comments. The header says so.
- Merge two changes to the same field. The user chooses one.
- Live reload of secrets. A token change comes with an account change in the file.
- A Unix socket or other connection to the daemon (change daemon-socket).

## Decisions

### 1. A save is a change function, applied to the file of now

`config.Update(path string, change func(*Config) error) (*Config, error)`:

1. Lock `config.toml.lock`.
2. Read and parse the file. When it does not exist, use `config.Default()`.
3. Call `change` on that configuration.
4. Validate, write the temporary file, rename.
5. Unlock. Return the saved configuration and its hash.

`ui.commit(change, tokens, removed...)` calls `config.Update` and sets `u.cfg` to the result. Each save site moves its edit into a closure. Most edits already find their item by ID (`next.Space(id)`, `SetConversation`, `SetLabel`, `Replace`), so the closure works on any version of the file. A closure that does not find its item returns `config.MissingError{Kind, ID}`; the form shows "The space Company A does not exist any more. Another process removed it."

Alternative: a three-way merge of the old copy, the form copy and the file. Rejected: it needs a merge for each field type and gives surprising results; a change function states the intent of the user.

### 2. Change detection by hash on each refresh

The dashboard keeps `cfgHash`, the SHA-256 of the bytes that it read or wrote last. On each `refreshMsg`, `load()` also reads `config.toml` (a small file, about 2 KB) and compares the hash. When the hash differs, it parses the bytes. A valid file replaces `u.cfg`; an invalid file sets the status line and keeps `u.cfg`. The hash of a save of the dashboard is set in `commit`, so the dashboard does not reload its own write.

When the configuration changes:

- When no form is open or the open form is not dirty, `buildForm` makes the form again from the new configuration.
- When the open form is dirty, the form stays. The status line says "config.toml changed in another process".
- When the account list changed (IDs, URL or platform), `loadConversations` runs again. Other changes do not call the network.

Alternative: fsnotify. Rejected: a new dependency, and the refresh tick already exists. The modification time alone is not enough, because two writes in one second can have the same time on some file systems.

### 3. Conflict check for forms

`buildForm` stores `form.ref` (kind and ID of the item) and `form.base` (a deep copy of the item from `u.cfg`). `config.Item(cfg, kind, id) (any, bool)` returns the item. In `commit`, when the change comes from a form, the closure compares `Item(file)` with `form.base` with `reflect.DeepEqual` before it changes the file. A difference returns `errConflict` and nothing is written. The dashboard opens a confirm dialog:

- "Reload" (default, `enter`): `buildForm(true)` from the file.
- "Overwrite": `commit` again with the check off.

Kinds: `settings` (Settings and Notify together), `unsorted`, `account`, `agent`, `schedule`, `space`, `conversation` (account and ID). Actions that are not forms (move to a space, read level, rename, delete, select all) have no conflict check: they change one field with the intent of the user.

### 4. File lock with a timeout

`config.Update` uses `syscall.Flock(LOCK_EX|LOCK_NB)` on `config.toml.lock` (mode 0600), and tries again every 50 ms for 5 seconds. Then it returns `ErrLocked` with the text `config.toml is locked by another process`. This is the pattern of `agent.Lock`. Readers (the daemon, `i2a status`) do not lock: the rename makes each read see a complete file.

Alternative: lock `config.toml` itself. Rejected: the rename replaces the inode, so a lock on the old inode does not protect the new file.

### 5. CLI writers

`i2a account add`, `i2a account remove`, `i2a conversation set` and `internal/cli/whatsapp.go` change their `config.Save` calls to `config.Update`. `config.Load` does the version 1 conversion inside the lock and reads the file again first, so two processes do not convert at the same time.

## Risks / Trade-offs

- [The closure of a save site uses a value from the old copy, for example the index of a list] → Each closure looks up its item by ID. A test for each tab saves after an external change to another item.
- [A user edits the file in a text editor that keeps a lock or a swap file] → i2a does not read the editor lock. The editor shows that the file changed on disk; this is the normal behavior of editors.
- [Reading the file every 2 seconds] → One read of about 2 KB. Parse only when the hash changed.
- [A stale lock file] → `flock` locks end when the process ends, so a crash does not leave a lock. The file itself stays and is harmless.
- [The dialog interrupts a save] → Only when the same item changed in another process, which is rare.

## Migration Plan

No data migration. The first save after the update writes the new header. Rollback: install the previous version. `config.toml.lock` can stay.

## Open Questions

None.
