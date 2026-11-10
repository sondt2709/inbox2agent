package notify

import (
	"context"
	"os/exec"
	"strings"
)

// script reads the title, the text and the subtitle from the arguments, so no
// part of them becomes AppleScript code.
const script = `on run argv
display notification (item 2 of argv) with title (item 1 of argv) subtitle (item 3 of argv)
end run`

// Osascript shows a note with "display notification". macOS shows it as
// Script Editor, so the title names i2a.
type Osascript struct{}

// Name implements Sender.
func (Osascript) Name() string { return ViaOsascript }

// Args returns the osascript arguments for one note.
func (Osascript) Args(n Note) []string {
	title := n.Title
	if !strings.HasPrefix(title, "i2a") {
		title = "i2a · " + title
	}
	return []string{"-e", script, title, n.Body, n.Subtitle}
}

// Send implements Sender.
func (o Osascript) Send(ctx context.Context, n Note) error {
	return exec.CommandContext(ctx, "osascript", o.Args(n)...).Run()
}
