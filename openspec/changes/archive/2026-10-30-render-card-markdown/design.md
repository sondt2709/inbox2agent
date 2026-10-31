## Context

`cardDetail` (internal/tui/tab_feed.go:455) wraps the body with `lipgloss.NewStyle().Width(w)`. `card.Prepare` already removes hidden and control characters from card text and cuts the body at `MaxBody` (4,000 characters). The Feed reads the store every 2 seconds and draws the detail panel again each time.

A probe with glamour v2.0.1 (dark style, wrap 60) showed:
- links and images become OSC 8 hyperlinks: `\x1b]8;id=…;https://evil.example/x\a…\x1b]8;;\a`, followed by the URL as text;
- raw HTML (`<script>`, `<img onerror>`) does not show; tags are removed;
- each line is padded with spaces to the wrap width, and the document has a margin of 2.

## Goals / Non-Goals

**Goals:** readable card bodies; no clickable link from card text; no slower Feed.

**Non-Goals:** Markdown in the suggested reply, the title, the digest viewer or `i2a card list`; images; a light theme.

## Decisions

### D1. glamour v2
Use `charm.land/glamour/v2` v2.0.1. It matches `charm.land/lipgloss/v2` v2.0.6 and Bubble Tea v2. Alternative: a small renderer on `github.com/yuin/goldmark` with lipgloss styles (about 1 MB, no syntax colors), but more code to keep. Alternative: `github.com/MichaelMure/go-term-markdown`, which is not maintained for lipgloss v2. glamour is the choice, unless the size (D5) is not acceptable.

### D2. Style
Start from `styles.DarkStyleConfig`; set the document margin to 0; set the heading, link and image colors to the i2a tokens (`accent #7D56F4`, `blue #58A6FF`, `mutedFg 244`); remove the `##` marks of the headings. Remove the trailing spaces of each output line, so the panel does not wrap them.

### D3. No hyperlinks from card text
After the render, remove each OSC 8 sequence (`ESC ] 8 ; params ; uri` ended by `BEL` or `ESC \`) with one function, `stripOSC`. The URL stays visible as plain text. A test renders `[x](https://evil.example)` and `![i](https://evil.example/i.png)` and checks that the output has no `ESC ] 8`.

### D4. Cache
Cache the rendered body by card ID, `updated_at` and width. The 2-second refresh does not render again. A new width or a card update renders again.

### D5. Size
The dependency adds about 7.8 MB to a probe binary. The real `bin/i2a` grows by 5.8 MB (26,373,138 to 32,201,650 bytes). If the stakeholder does not accept it, use the goldmark alternative of D1.

### D6. Primer
Add one line to the card section: "body: … You can use Markdown (headings, lists, bold, code, tables). Links show as text and cannot be opened." The primer hash changes, so each `reuse` session starts again one time.

## Risks / Trade-offs

- [A future glamour version adds other escape sequences] → Pin v2.0.1; the test checks for OSC sequences in general (`ESC ]`), not only `8`.
- [A wide table does not fit the panel] → glamour wraps cells; the plain body is the fallback on an error.
- [Binary size] → D5.
