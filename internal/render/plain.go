package render

import (
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

var plainParser = goldmark.New(goldmark.WithExtensions(extension.Strikethrough)).Parser()

// PlainText returns the text of a Markdown string with no markup: the text of
// emphasis, code spans, link text and image alt text, with the lines joined by
// one space. It drops raw HTML (goldmark gives it no text node). When no text
// is left, it returns md with the lines joined, so a row is never empty. The
// card titles use it: an agent can write Markdown in a title, and a row or a
// notification shows no markup.
func PlainText(md string) string {
	src := []byte(md)
	doc := plainParser.Parse(text.NewReader(src))
	var b strings.Builder
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			if n.Type() == ast.TypeBlock {
				b.WriteByte(' ')
			}
			return ast.WalkContinue, nil
		}
		switch v := n.(type) {
		case *ast.Text:
			b.Write(util.ResolveNumericReferences(util.ResolveEntityNames(util.UnescapePunctuations(v.Segment.Value(src)))))
			if v.SoftLineBreak() || v.HardLineBreak() {
				b.WriteByte(' ')
			}
		case *ast.String:
			b.Write(v.Value)
		case *ast.AutoLink:
			b.Write(v.URL(src))
		case *ast.CodeSpan:
			for c := v.FirstChild(); c != nil; c = c.NextSibling() {
				if t, ok := c.(*ast.Text); ok {
					b.Write(t.Segment.Value(src))
				}
			}
			return ast.WalkSkipChildren, nil
		case *ast.FencedCodeBlock, *ast.CodeBlock:
			lines := n.Lines()
			for i := 0; i < lines.Len(); i++ {
				seg := lines.At(i)
				b.Write(seg.Value(src))
				b.WriteByte(' ')
			}
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	if out := strings.Join(strings.Fields(b.String()), " "); out != "" {
		return out
	}
	// A title of raw HTML only: keep its text, so the row is not empty.
	return strings.Join(strings.Fields(md), " ")
}
