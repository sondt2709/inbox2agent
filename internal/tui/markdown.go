package tui

import (
	"regexp"
	"strings"
	"time"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
)

// osc matches an operating system command: ESC ] ... ended by BEL or ESC \.
// glamour writes each link and image as an OSC 8 hyperlink. Card text comes
// from an agent that read untrusted messages, and the Feed opens only the
// stored URL of a source message, so no OSC sequence may reach the terminal.
var osc = regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`)

// stripOSC removes each OSC sequence. The visible text stays.
func stripOSC(s string) string { return osc.ReplaceAllString(s, "") }

// padding matches the spaces and SGR codes at the end of a line. glamour pads
// each line to the wrap width; the padding would make the panel wrap.
var padding = regexp.MustCompile(`(?:\x1b\[[0-9;]*m| )+$`)

// trimLine removes the padding of a styled line and resets the style.
func trimLine(l string) string {
	t := padding.ReplaceAllString(l, "")
	if t != l && strings.Contains(t, "\x1b[") {
		t += "\x1b[m"
	}
	return t
}

// mdStyle is the dark glamour style with the colors of i2a, no margin and no
// heading marks.
func mdStyle() ansi.StyleConfig {
	st := styles.DarkStyleConfig
	zero := uint(0)
	st.Document.Margin = &zero
	st.Document.BlockPrefix = ""
	st.Document.BlockSuffix = ""
	accentHex, blueHex, mutedHex := "#7D56F4", "#58A6FF", "244"
	st.Heading.Color = &accentHex
	for _, h := range []*ansi.StyleBlock{&st.H2, &st.H3, &st.H4, &st.H5, &st.H6} {
		h.Prefix = ""
	}
	st.Link.Color = &blueHex
	st.LinkText.Color = &blueHex
	st.Image.Color = &mutedHex
	st.ImageText.Color = &mutedHex
	return st
}

// mdKey identifies one rendered card body.
type mdKey struct {
	card    int64
	updated time.Time
	width   int
}

// markdown renders card bodies and keeps the results, so the refresh of the
// Feed every 2 seconds does not render again.
type markdown struct {
	renderers map[int]*glamour.TermRenderer
	cache     map[mdKey]string
}

// maxCached limits the cache; the Feed shows one body at a time.
const maxCached = 200

// body returns the body of a card as styled text for the width w. When the
// render fails, it returns the plain text.
func (md *markdown) body(card int64, updated time.Time, text string, w int) string {
	k := mdKey{card, updated, w}
	if s, ok := md.cache[k]; ok {
		return s
	}
	s, err := md.render(text, w)
	if err != nil {
		return text
	}
	if md.cache == nil || len(md.cache) >= maxCached {
		md.cache = map[mdKey]string{}
	}
	md.cache[k] = s
	return s
}

func (md *markdown) render(text string, w int) (string, error) {
	r, ok := md.renderers[w]
	if !ok {
		var err error
		r, err = glamour.NewTermRenderer(glamour.WithStyles(mdStyle()), glamour.WithWordWrap(w))
		if err != nil {
			return "", err
		}
		if md.renderers == nil {
			md.renderers = map[int]*glamour.TermRenderer{}
		}
		md.renderers[w] = r
	}
	out, err := r.Render(text)
	if err != nil {
		return "", err
	}
	lines := strings.Split(stripOSC(out), "\n")
	for i, l := range lines {
		lines[i] = trimLine(l)
	}
	return strings.Trim(strings.Join(lines, "\n"), "\n"), nil
}
