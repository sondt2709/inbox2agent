## MODIFIED Requirements

### Requirement: Prompt submission
The `orca` driver SHALL submit a prompt with `orca terminal send --text <prompt> --enter --wait-submit 30`. Before the first prompt to a new terminal, the driver SHALL wait up to 90 seconds for `tui-idle`. When the send result has `observation: "unsupported"`, no agent runs in the terminal: the driver SHALL return the error "no agent runs in <handle>", SHALL NOT send Enter again, and SHALL delete the stored session of the space. When the send result does not include the stage `turn_started` and the observation is not `unsupported`, the driver SHALL send one bare Enter to the terminal and record the detail "turn not proven, Enter sent again". The driver SHALL NOT send the prompt text again.

#### Scenario: Prompt left in the input box
- **WHEN** `orca terminal send` returns `accepted: true` and the stages are only `input_accepted`
- **THEN** the driver sends one `orca terminal send --enter` with no text, and the dispatch status is `sent`

#### Scenario: Long multi-line prompt
- **WHEN** a new session gets a primer of 15 lines with non-ASCII text and an inline batch
- **THEN** the send result includes `turn_started`, and the input box of the agent is empty

#### Scenario: Send into a shell
- **WHEN** `orca terminal send` returns `observation: "unsupported"`
- **THEN** the driver sends no Enter, the dispatch status is `failed` with "no agent runs in", the messages stay `pending`, and the store has no session for the space

### Requirement: Orca reuse-session mode
In `reuse` mode, the `orca` driver SHALL keep for each space in the store one terminal handle, the Claude session ID of the agent in that terminal, and the hash of the primer. Before a send, the driver SHALL check the session as the requirement "Claude session of a terminal" says. When the session is valid, the stored hash is the hash of the current primer, and the agent is idle, the driver SHALL send only the batch prompt with `orca terminal send --enter`. When the stored hash is `pending` (a linked session), the driver SHALL send the primer and the batch prompt to the terminal, and store the hash of the current primer. When no valid session exists, or the stored hash is another value, the driver SHALL start a new session with the primer and the batch prompt, and store the new handle, the Claude session ID and the hash for the space. The driver SHALL NOT close the old terminal. When the agent is busy, the driver SHALL return `held`, and the messages SHALL stay `pending`.

#### Scenario: Second batch in the same session
- **WHEN** two batches of one space go to an agent in `reuse` mode, and the agent is idle before the second batch
- **THEN** both batches arrive in the same terminal, and the primer arrives one time only

#### Scenario: Two spaces with one agent
- **WHEN** the spaces `acme` and `borealis` use the same agent in `reuse` mode, and each sends a batch
- **THEN** the batches arrive in two different terminals, and the store has one handle and one Claude session ID for each space

#### Scenario: Terminal closed
- **WHEN** the stored terminal of a space was closed, and a new batch of the space arrives
- **THEN** the driver starts a new session, submits the primer and the batch, and stores the new handle

#### Scenario: Claude exited in the terminal
- **WHEN** the user ran `/exit` in the stored terminal, so the terminal runs zsh, and a new batch of the space arrives
- **THEN** the driver sends nothing to that terminal, starts a new session, and submits the primer and the batch there

#### Scenario: Another Claude session in the terminal
- **WHEN** the user ran `/clear`, or quit Claude and started `claude` again, in the stored terminal
- **THEN** the driver sends nothing to that terminal, and starts a new session with the primer and the batch

#### Scenario: Linked session
- **WHEN** the user linked the terminal `term_c` to the space, and a batch of the space arrives
- **THEN** the primer and the batch arrive in `term_c`, and the next batch arrives there without the primer

#### Scenario: Primer changed
- **WHEN** the stored terminal is live, but the user changed the primer template, or the store came from schema version 2 with no hash
- **THEN** the driver starts a new session with the new primer and the batch, and stores the new handle and hash

#### Scenario: Agent busy
- **WHEN** the agent does not become idle within 5 seconds
- **THEN** the dispatch status is `held`, the messages stay `pending`, and the next send time of the space sends them

## ADDED Requirements

### Requirement: Claude session of a terminal
When the `cli` of the agent is `claude` or starts with `claude`, the driver SHALL find the Claude processes of a terminal from the files `<claude dir>/sessions/<pid>.json` (`CLAUDE_CONFIG_DIR`, else `~/.claude`) and the environment variable `ORCA_TERMINAL_HANDLE` of each running pid. The stored session SHALL be valid when the stored terminal has exactly one Claude process with the stored Claude session ID and Orca reports the terminal live. When the stored row has no Claude session ID and the terminal has exactly one Claude process, the driver SHALL store that session ID. When the stored terminal does not run the stored session and exactly one other terminal runs it, the driver SHALL use that terminal, and store its handle. When i2a cannot read the Claude processes, or the `cli` is another agent, the session SHALL be valid when Orca reports the terminal live and `agentIdentity` is not null.

#### Scenario: Session moved to another tab
- **WHEN** the stored session `S` of the space no longer runs in `term_a`, and the user resumed `S` in `term_b`
- **THEN** the batch goes to `term_b`, and the store has `term_b` for the space

#### Scenario: Session in two tabs
- **WHEN** the stored session `S` runs in `term_b` and in `term_c`, and not in the stored terminal
- **THEN** the driver starts a new session

#### Scenario: Store upgraded from version 7
- **WHEN** the stored row has the handle `term_a`, no Claude session ID, and `term_a` runs Claude session `S`
- **THEN** the batch goes to `term_a`, and the store has `S` for the space

#### Scenario: Unknown file format
- **WHEN** `~/.claude/sessions` holds files that do not decode
- **THEN** the driver uses the Orca check with `agentIdentity`, and the batch is not refused for this reason

### Requirement: Tab name of a new session
After the driver starts a terminal (new, reuse and worktree modes), it SHALL run `orca terminal rename --terminal <handle> --title "<agent name> · <space name>"`. A failed rename SHALL add a note to the detail of the dispatch and SHALL NOT change its status. The driver SHALL NOT rename a terminal that it did not start.

#### Scenario: New session of the space Unsorted
- **WHEN** the agent `triage` starts a new session for the space Unsorted
- **THEN** the Orca tab of the session shows "triage · Unsorted"
