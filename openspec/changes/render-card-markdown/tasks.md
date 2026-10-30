## 1. Dependency

- [x] 1.1 Add `charm.land/glamour/v2` v2.0.1 with an exact version; `CGO_ENABLED=0 task build`; record the binary size before and after in the evidence

## 2. Renderer

- [x] 2.1 `internal/tui/markdown.go`: renderer with the i2a style (dark base, margin 0, i2a colors), `stripOSC` for OSC sequences, trailing space trim, plain fallback on error
- [x] 2.2 Cache by card ID, update time and width in the Feed model
- [x] 2.3 `cardDetail` uses the renderer for the body only; the title and the reply stay plain

## 3. Primer

- [x] 3.1 Add the Markdown line to `render.CardSection`; update the primer tests

## 4. Tests

- [x] 4.1 Unit tests for each scenario of the spec (bold and list, link with no `ESC ]`, raw HTML hidden, reply plain, cache hit; the fallback on a render error has no test, because glamour gave no input that fails); break each test one time and restore the code
- [x] 4.2 e2e (CI): a card with a Markdown body shows bold text and bullets in the Feed, with no `**`
- [ ] 4.3 `task test`, `task lint`, `task e2e`, `task leakcheck`

## 5. Docs

- [x] 5.1 README: the Feed section says that card bodies are Markdown and that links cannot be opened
- [x] 5.2 CLAUDE.md: the Feed rule about links names the OSC 8 strip in `internal/tui/markdown.go`
- [x] 5.3 `test/e2e/SCENARIOS.md`

## 6. Verify

- [ ] 6.1 Run the test plan again on the last commit; save the results with the SHA in `tmp/evidence/render-card-markdown/after/`; screenshot of the Feed before and after
