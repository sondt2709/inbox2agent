package render

import "testing"

func TestPlainText(t *testing.T) {
	cases := []struct{ in, want string }{
		{"plain title", "plain title"},
		{"**Deploy** of `api-gateway` failed on [prod](https://example.com)", "Deploy of api-gateway failed on prod"},
		{"_rollback_ plan and ~~old~~ text", "rollback plan and old text"},
		{"see ![the graph](https://x/y.png) now", "see the graph now"},
		{"a <b>bold</b> word", "a bold word"},
		{"<div>block</div>\n\ntext", "text"},
		{"<div>only html</div>", "<div>only html</div>"},
		{"line one\nline two", "line one line two"},
		{"# Heading title", "Heading title"},
		{"- item one", "item one"},
		{"<https://example.com>", "https://example.com"},
		{"5 \\* 3 = 15", "5 * 3 = 15"},
		{"Tom & Jerry", "Tom & Jerry"},
		{"Tom &amp; Jerry &#33;", "Tom & Jerry !"},
		{"  spaced   out  ", "spaced out"},
		{"", ""},
	}
	for _, c := range cases {
		if got := PlainText(c.in); got != c.want {
			t.Errorf("PlainText(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
