## MODIFIED Requirements

### Requirement: Prompt submission
The `orca` driver SHALL submit a prompt with `orca terminal send --text <prompt> --enter --wait-submit 30`. Before the first prompt to a new terminal, the driver SHALL check each second, for up to 90 seconds, that the agent runs in the terminal: exactly one interactive Claude process when the `cli` is Claude Code and i2a can read the Claude processes, else Orca reports `agentIdentity`. When no agent runs after 90 seconds, the driver SHALL return the error "no agent started in the terminal <handle> after 1m30s; i2a sent nothing", SHALL NOT type into the terminal, and SHALL NOT store the session. After the agent runs, the driver SHALL wait up to 90 seconds for `tui-idle`. When the send result has `observation: "unsupported"`, no agent runs in the terminal: the driver SHALL return the error "no agent runs in <handle>", SHALL NOT send Enter again, and SHALL delete the stored session of the space. When the send result does not include the stage `turn_started` and the observation is not `unsupported`, the driver SHALL send one bare Enter to the terminal and record the detail "turn not proven, Enter sent again". The driver SHALL NOT send the prompt text again.

#### Scenario: Prompt left in the input box
- **WHEN** `orca terminal send` returns `accepted: true` and the stages are only `input_accepted`
- **THEN** the driver sends one `orca terminal send --enter` with no text, and the dispatch status is `sent`

#### Scenario: Long multi-line prompt
- **WHEN** a new session gets an inline prompt with an instruction of 15 lines with non-ASCII text
- **THEN** the send result includes `turn_started`, and the input box of the agent is empty

#### Scenario: Send into a shell
- **WHEN** `orca terminal send` returns `observation: "unsupported"`
- **THEN** the driver sends no Enter, the dispatch status is `failed` with "no agent runs in", the messages stay `pending`, and the store has no session for the space

#### Scenario: Agent starts after the shell
- **WHEN** a new terminal shows a shell for 2 seconds before Claude Code starts in it
- **THEN** the driver sends the prompt only after Claude Code runs, and the dispatch status is `sent`

#### Scenario: Agent does not start
- **WHEN** no agent starts in a new terminal within 90 seconds
- **THEN** the driver types nothing into the terminal, the dispatch status is `failed` with "no agent started in the terminal", and the messages stay `pending`

### Requirement: Orca new-session mode
In `new` mode, the `orca` driver SHALL run `orca agent launch` with the `cli` of the agent, `--surface terminal` and the workspace selector, and without `--prompt`. Then the driver SHALL wait until the agent TUI is idle and submit the prompt of the batch, as the requirement "Prompt submission" says.

#### Scenario: New session for each batch
- **WHEN** two batches of a space go to an agent in `new` mode
- **THEN** Orca shows two agent terminals, and each agent started a turn with the prompt of one batch

### Requirement: Orca reuse-session mode
In `reuse` mode, the `orca` driver SHALL keep for each space in the store one terminal handle and the Claude session ID of the agent in that terminal. Before a send, the driver SHALL check the session as the requirement "Claude session of a terminal" says. When the session is valid and the agent is idle, the driver SHALL send the prompt of the batch with `orca terminal send --enter`. When no valid session exists, the driver SHALL start a new session with the prompt of the batch, and store the new handle and the Claude session ID for the space. The driver SHALL NOT start a new session because the instruction, the CLI instruction or the template changed. The driver SHALL NOT close the old terminal. When the agent is busy, the driver SHALL return `held`, and the messages SHALL stay `pending`.

#### Scenario: Second batch in the same session
- **WHEN** two batches of one space go to an agent in `reuse` mode, and the agent is idle before the second batch
- **THEN** both batches arrive in the same terminal, and each has the instruction, the content and the CLI instruction

#### Scenario: Two spaces with one agent
- **WHEN** the spaces `acme` and `borealis` use the same agent in `reuse` mode, and each sends a batch
- **THEN** the batches arrive in two different terminals, and the store has one handle and one Claude session ID for each space

#### Scenario: Terminal closed
- **WHEN** the stored terminal of a space was closed, and a new batch of the space arrives
- **THEN** the driver starts a new session, submits the batch, and stores the new handle

#### Scenario: Claude exited in the terminal
- **WHEN** the user ran `/exit` in the stored terminal, so the terminal runs zsh, and a new batch of the space arrives
- **THEN** the driver sends nothing to that terminal, starts a new session, and submits the batch there

#### Scenario: Another Claude session in the terminal
- **WHEN** the user ran `/clear`, or quit Claude and started `claude` again, in the stored terminal
- **THEN** the driver sends nothing to that terminal, and starts a new session with the batch

#### Scenario: Linked session
- **WHEN** the user linked the terminal `term_c` to the space, and a batch of the space arrives
- **THEN** the batch arrives in `term_c`

#### Scenario: Instruction changed
- **WHEN** the stored terminal is live and runs the stored Claude session, and the user changed the instruction of the agent
- **THEN** the batch arrives in the stored terminal with the new instruction, and the dispatch detail does not say "started session"

#### Scenario: Session stored by an older version
- **WHEN** the stored row has a primer hash from an older version of i2a, and the terminal runs the stored Claude session
- **THEN** the batch arrives in the stored terminal

#### Scenario: Agent busy
- **WHEN** the agent does not become idle within 5 seconds
- **THEN** the dispatch status is `held`, the messages stay `pending`, and the next send time of the space sends them

### Requirement: Orca worktree mode
In `worktree` mode, the `orca` driver SHALL create a new Orca worktree named `i2a-<batch-id>` in the workspace repository, with the `cli` of the agent and without `--prompt`. Then the driver SHALL submit the prompt of the batch, as the requirement "Prompt submission" says.

#### Scenario: Worktree for a batch
- **WHEN** a batch of a space goes to an agent in `worktree` mode
- **THEN** Orca lists a worktree named `i2a-<batch-id>` with an agent terminal that received the prompt

### Requirement: Claude session of a terminal
When the `cli` of the agent is `claude` or starts with `claude`, the driver SHALL find the Claude processes of a terminal from the files `<claude dir>/sessions/<pid>.json` (`CLAUDE_CONFIG_DIR`, else `~/.claude`) and the environment variable `ORCA_TERMINAL_HANDLE` of each running pid. Only a process with the `kind` `interactive` (or no `kind`) SHALL count: a background process keeps the variable, but the terminal shows the shell. The stored session SHALL be valid when the stored terminal has exactly one Claude process with the stored Claude session ID and Orca reports the terminal live. When the stored row has no Claude session ID and the terminal has exactly one Claude process, the driver SHALL store that session ID. When the stored terminal does not run the stored session and exactly one other terminal runs it, the driver SHALL use that terminal, and store its handle. When i2a cannot read the Claude processes, or the `cli` is another agent, the session SHALL be valid when Orca reports the terminal live and `agentIdentity` is not null. After the agent is idle and just before the driver types, it SHALL check again that the terminal runs the session; when it does not, the driver SHALL send nothing to that terminal and SHALL start a new session (an ask goes to the agent of the space).

#### Scenario: Session moved to another tab
- **WHEN** the stored session `S` of the space no longer runs in `term_a`, and the user resumed `S` in `term_b`
- **THEN** the batch goes to `term_b`, and the store has `term_b` for the space

#### Scenario: Session in two tabs
- **WHEN** the stored session `S` runs in `term_b` and in `term_c`, and not in the stored terminal
- **THEN** the driver starts a new session

#### Scenario: Store upgraded from version 7
- **WHEN** the stored row has the handle `term_a`, no Claude session ID, and `term_a` runs Claude session `S`
- **THEN** the batch goes to `term_a`, and the store has `S` for the space

#### Scenario: Claude moved to the background
- **WHEN** the user chose "Move to background and exit" in the stored terminal, so a Claude process with the `kind` `bg` has the handle of the terminal
- **THEN** the driver sends nothing to that terminal and starts a new session

#### Scenario: Claude exits during the wait
- **WHEN** the check finds the stored session in the terminal, and the agent exits while the driver waits for `tui-idle`
- **THEN** the driver sends nothing to that terminal and starts a new session with the batch

#### Scenario: Unknown file format
- **WHEN** `~/.claude/sessions` holds files that do not decode
- **THEN** the driver uses the Orca check with `agentIdentity`, and the batch is not refused for this reason
