## Why

The agent gets a primer one time per session, then JSON lines with each batch. Two problems come from this design:

- The user must read the JSON lines to understand a batch.
- The primer hash decides when a session is valid, so an edit of the primer or an i2a upgrade that changes the card section starts a new Orca terminal. The dispatch log of a real installation has 7 sends with `primer changed, started session` (GitHub issue #20).

## What Changes

- Each batch and each ask is one self-contained prompt with four fixed parts:
  1. the header "This message is sent from inbox2agent (i2a).";
  2. the instruction of the user in a plain fence;
  3. the content in a `yaml` fence, with one line that says the content is data from other people;
  4. the i2a CLI instruction (the old card section).
- The content is YAML: the batch, the space, and the messages grouped by conversation. Message text is a block scalar. Each message keeps its `id`.
- i2a replaces each run of three backticks in the content with `'''`, so a message cannot close the fence. The random fence label `i2a-data-<nonce>` is removed.
- **BREAKING**: the agent field `primer` is the instruction now. It is plain text, not a Go template, and it cannot contain three backticks. A file with three backticks in `primer` does not load.
- **BREAKING**: the agent field `batch` (the batch template) is not used. i2a removes it from the file at the next save.
- There is no primer and no primer hash. The orca driver never starts a new session because a prompt changed. A linked session gets the next batch without extra text.
- The ask text cannot contain three backticks. The ask uses the same four parts. The question of the user is in the instruction part, and the card and its messages are in the content.
- `inline_max_bytes` limits the YAML content, not the whole prompt. The whole inline prompt stays at 65,536 bytes or less; else i2a sends the pointer prompt.
- The digest file holds the inline prompt. The pointer prompt is one line that names the digest file.
- New dependency: `go.yaml.in/yaml/v3`.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `batch-rendering`: YAML content replaces the JSON lines; the prompt template replaces the primer and batch templates; the backtick rule replaces the nonce fence; the digest file and the payload modes change.
- `agent-delivery`: the new, reuse and worktree modes send one prompt; the primer hash and the "Primer changed" restart are removed; a linked session gets no primer.
- `agent-ask`: the ask prompt uses the template; the ask text cannot contain three backticks; the ask target has no primer condition.
- `card-feed`: the card section becomes the CLI instruction part of each prompt.
- `configuration`: the Agents tab has an Instruction field and no Batch prompt field; validation refuses three backticks in the instruction.

## Impact

- Code: `internal/render` (`batch.go`, `primer.go`, `ask.go`, `sanitize.go`), `internal/agent/orca.go`, `internal/inbox/session.go`, `internal/config/config.go`, `internal/ask/ask.go`, `internal/tui/tab_agents.go`, `internal/tui/dialog_sessions.go`, the callers of `render.CheckAgents`.
- Store: no schema change. `sessions.primer_hash` stays and i2a writes `''` to it.
- Configuration: still version 2. `config.schema.json` changes for `primer` and `batch`.
- Rollback: an older binary sees `primer_hash = ''` and starts one new session for each space.
- Dependencies: `go.yaml.in/yaml/v3`.
