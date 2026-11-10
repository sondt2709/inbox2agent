## 1. First failing tests

- [x] 1.1 Add `TestReuseKeepsSessionWhenInstructionChanges` in `internal/agent`: a live stored session with another primer hash gets the batch in the same terminal. Run it on the base commit and keep the failure output in `tmp/evidence/prompt-template/`
- [x] 1.2 Add `TestPromptTemplate` in `internal/render`: the prompt has the four parts and the content is YAML. Run it on the base commit and keep the failure output

## 2. Dependency

- [x] 2.1 Add `go.yaml.in/yaml/v3` v3.0.4 to `go.mod`

## 3. Render

- [x] 3.1 Add `FenceSafe` (three backticks become `'''`) and the YAML content structs for a batch and an ask; `yaml.NewEncoder` with `SetIndent(2)`; the round-trip check with the double-quoted fallback; the error when the content still has three backticks
- [x] 3.2 Add `Prompt(instruction, content, cli)`, `DefaultInstruction` and `CLIInstruction` (the old `CardSection` text); remove `DefaultPrimer`, `DefaultPointerBatch`, `DefaultInlineBatch`, `FencePrefix`, `NewNonce`, `Block`, `primerHash`, `CheckTemplate` and `CheckAgents`
- [x] 3.3 `Render`: the digest file holds the inline prompt; the inline prompt when the content is at most `inline_max_bytes` and the prompt is at most 65,536 bytes, else the pointer line with the detail; `Rendered` has no `Primer` and no `PrimerHash`
- [x] 3.4 `Ask`: the question in the instruction part, the card and the messages in the content, the ask lines in the CLI part, the drop of messages from the end with `dropped`
- [x] 3.5 Tests: block scalar, two conversations, `reply_to` cases, text that starts with a newline, fence and four backticks, hidden characters, truncation, inline limit on the content, the 65,536 byte limit, dry-run without the CLI part, the ask prompt, braces in the instruction; a fuzz test that each content decodes to the input and has no three backticks

## 4. Configuration and ask text

- [x] 4.1 `config.Validate` refuses three backticks in `primer`; `Parse` clears `batch`; update the `doc` tags of `primer` and `batch`; write `docs/config.schema.json` again with `-update`
- [x] 4.2 `ask.CheckText` (or the ask text check) refuses three backticks
- [x] 4.3 Remove the calls of `render.CheckAgents` in `internal/cli` and `internal/tui`
- [x] 4.4 Tests: the validation error text, the save without `batch`, the ask text error

## 5. Driver and sessions

- [x] 5.1 `orca.go`: remove `joinPrompts`, the hash check and the `pending` branch; send `Rendered.Prompt` in all modes; store sessions with an empty hash
- [x] 5.2 Remove `inbox.Session.PrimerHash` and `inbox.PrimerPending`; the store writes `''` to `sessions.primer_hash`
- [x] 5.3 `dialog_sessions.go`: the link stores no hash; the message says "The next batch goes there."
- [x] 5.4 Update the driver tests that expect the primer or `primer changed`

## 6. Dashboard

- [x] 6.1 Agents form: the Instruction field (plain text, sent with each batch, the backtick error); remove the Batch prompt field
- [x] 6.2 Tests of the form: save, backtick error

## 7. End-to-end and documents

- [x] 7.1 Update the e2e parser of the digest (`scenarios_test.go`) for the YAML content; update S05, S08, S30, S31, S44 and `SCENARIOS.md`
- [x] 7.2 Update the local Orca scenarios S14, S15/S16, S21, S25, S29 for the new prompt; add the check that an instruction edit keeps the terminal in S15
- [x] 7.3 Update `CLAUDE.md` (render, agent, ask paragraphs) and `README.md` where they name the primer or the JSON lines
- [x] 7.4 Run `task lint`, `task test`, `task e2e`, `I2A_E2E_ORCA=1 task e2e`, `task leakcheck`

## 8. Agent check of a new terminal (found during apply)

- [x] 8.1 `Orca.submit` waits until the agent runs (Claude process, else `agentIdentity`) before `tui-idle`; no agent after `StartTimeout` gives `errNoAgentStarted` and no send
- [x] 8.2 Tests: the agent starts after two shell reads; no agent with the Orca check and with the Claude check; break the check one time
- [x] 8.3 Update the spec requirement "Prompt submission", the design (D7) and `CLAUDE.md`
