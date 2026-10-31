package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/sondt2709/inbox2agent/internal/inbox"
)

func TestMarkdownBodyFormats(t *testing.T) {
	var md markdown
	out := ansi.Strip(md.body(1, time.Time{}, "## Status\n\n**Blocked**: QA needs `sign-off`\n\n- item one\n- item two", 60))
	for _, want := range []string{"Status", "Blocked: QA needs", "sign-off", "• item one", "• item two"} {
		if !strings.Contains(out, want) {
			t.Errorf("no %q in:\n%s", want, out)
		}
	}
	for _, bad := range []string{"**", "## ", "`"} {
		if strings.Contains(out, bad) {
			t.Errorf("raw mark %q in:\n%s", bad, out)
		}
	}
	for _, l := range strings.Split(out, "\n") {
		if strings.HasSuffix(l, " ") {
			t.Errorf("padded line %q", l)
		}
	}
}

func TestMarkdownLinksAreNotHyperlinks(t *testing.T) {
	var md markdown
	out := md.body(1, time.Time{}, "[click](https://evil.example/x), ![img](https://evil.example/i.png) and https://evil.example/bare", 60)
	if strings.Contains(out, "\x1b]") {
		t.Errorf("OSC sequence in %q", out)
	}
	if !strings.Contains(ansi.Strip(out), "https://evil.example/x") {
		t.Errorf("the URL is not visible as text: %q", ansi.Strip(out))
	}
}

func TestMarkdownHidesRawHTML(t *testing.T) {
	var md markdown
	out := ansi.Strip(md.body(1, time.Time{}, "<script>alert(1)</script>\n\nText <img src=x onerror=y> after", 60))
	for _, bad := range []string{"<script", "alert(1)", "onerror"} {
		if strings.Contains(out, bad) {
			t.Errorf("%q in %q", bad, out)
		}
	}
}

func TestMarkdownCachesByCardTimeAndWidth(t *testing.T) {
	var md markdown
	at := time.Unix(100, 0)
	first := md.body(7, at, "**x**", 40)
	md.renderers = nil // a second render would need a new renderer
	if again := md.body(7, at, "**x**", 40); again != first || md.renderers != nil {
		t.Error("the same card, time and width rendered again")
	}
	md.body(7, at, "**x**", 50)
	md.body(7, at.Add(time.Second), "**x**", 50)
	if len(md.cache) != 3 {
		t.Errorf("cache has %d entries, want 3", len(md.cache))
	}
}

func TestFeedRendersBodyAndKeepsReplyPlain(t *testing.T) {
	h := newFeedModel(t)
	if _, err := h.st.SaveCards(context.Background(), []inbox.Card{{Category: inbox.CategoryAction, Urgency: inbox.UrgencyHigh,
		Title: "Markdown card", Body: "**Blocked** now\n\n- one\n- two", Reply: "**yes**, ship it"}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	h.m.Update(h.m.load()())
	h.m.Update(keyMsg("k"))
	s := screen(h.m)
	if !strings.Contains(s, "Markdown card") {
		t.Fatalf("the new card is not first:\n%s", s)
	}
	if !strings.Contains(s, "Blocked now") || strings.Contains(s, "**Blocked**") || !strings.Contains(s, "• one") {
		t.Errorf("the body is not rendered:\n%s", s)
	}
	if !strings.Contains(s, "**yes**, ship it") {
		t.Errorf("the reply is not plain:\n%s", s)
	}
}
