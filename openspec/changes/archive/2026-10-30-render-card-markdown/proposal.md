## Why

Agents write card bodies in Markdown (headings, lists, `code`, tables), but the Feed shows the raw text with `**` and `|` marks. The body is hard to read in the detail panel.

## What Changes

- The Feed renders the card body as Markdown with `charm.land/glamour/v2` v2.0.1 (the Markdown renderer of the Charm v2 stack that i2a already uses). The renderer wraps to the width of the detail panel and uses the i2a colors.
- Links and images in card text show as text only. The renderer output keeps no OSC 8 hyperlink, so a link from card text cannot be opened (the rule of the Feed stays: only the stored URL of a source message opens).
- Raw HTML in a card body does not show.
- The suggested reply stays plain text, because `c` copies it and `r` sends it as it is.
- The card section of the primer tells the agent that the body can use Markdown and that links cannot be opened.
- When the render fails, the Feed shows the plain body.

Research: the Orca source code renders Markdown only in its Electron GUI (`marked`, `react-markdown`, `rehype-sanitize`); its CLI prints plain text. No Go library is in Orca. glamour is the Go equivalent for a terminal.

## Capabilities

### New Capabilities
- None.

### Modified Capabilities
- `card-feed`: new requirement for the Markdown body; the card section of the primer names Markdown.

## Impact

- New dependency `charm.land/glamour/v2` v2.0.1 with `github.com/yuin/goldmark` and `github.com/alecthomas/chroma/v2`. Pure Go; builds with `CGO_ENABLED=0`. `bin/i2a` grows by 5.8 MB (26,373,138 to 32,201,650 bytes), mostly from `chroma`.
- Code: `internal/tui/tab_feed.go` (body render and a cache by card, update time and width), a new `internal/tui/markdown.go`, `internal/render/primer.go` (card section text).
- The card section text changes, so each `reuse` session starts again one time (new primer hash).
- Independent of `introduce-spaces`; both edit `cardDetail` in `tab_feed.go`.
