// Package notify shows desktop notifications.
package notify

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"time"
)

// Notifier shows one notification. It never blocks the caller for long and
// ignores errors: a missing notification must not stop a dispatch.
type Notifier func(title, text string)

// script reads the title and the text from the arguments, so no part of them
// becomes AppleScript code.
const script = `on run argv
display notification (item 2 of argv) with title (item 1 of argv)
end run`

// Args returns the osascript arguments for one notification.
func Args(title, text string) []string {
	return []string{"-e", script, title, text}
}

// Desktop returns the notifier of this platform. On macOS it runs osascript.
// On other platforms, and when I2A_NOTIFY is "off", it does nothing.
func Desktop() Notifier {
	if runtime.GOOS != "darwin" || os.Getenv("I2A_NOTIFY") == "off" {
		return func(string, string) {}
	}
	return func(title, text string) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = exec.CommandContext(ctx, "osascript", Args(title, text)...).Run()
	}
}

// Short cuts s to n runes and adds "..." when it cuts.
func Short(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
