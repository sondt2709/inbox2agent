package cli

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sondt2709/inbox2agent/internal/app"
	"github.com/sondt2709/inbox2agent/internal/notify"
)

type recSender struct{ got *[]notify.Note }

func (recSender) Name() string { return "terminal-notifier" }
func (r recSender) Send(_ context.Context, n notify.Note) error {
	*r.got = append(*r.got, n)
	return nil
}

// notifyEnv fakes the notifiers on the Mac, Homebrew and the chain.
func notifyEnv(t *testing.T, tnInstalled bool) (installs *int, sent *[]notify.Note) {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only")
	}
	t.Setenv("I2A_HOME", t.TempDir())
	t.Setenv("I2A_SECRET_BACKEND", "file")
	t.Setenv("I2A_NOTIFY", "")
	installs, sent = new(int), &[]notify.Note{}
	oldD, oldB, oldI, oldC := notifyDetect, brewPath, brewInstall, notifyChain
	notifyDetect = func() []notify.Found {
		tn := notify.Found{Name: "terminal-notifier", Detail: "not installed: run i2a notify setup"}
		if tnInstalled || *installs > 0 {
			tn = notify.Found{Name: "terminal-notifier", OK: true, Detail: "terminal-notifier 3.1.0 (/opt/homebrew/bin/terminal-notifier)"}
		}
		return []notify.Found{{Name: "orca", Detail: "Orca does not run"}, tn, {Name: "osascript", OK: true, Detail: "always on macOS"}}
	}
	brewPath = func() string { return "/opt/homebrew/bin/brew" }
	brewInstall = func(context.Context, string, io.Writer, io.Writer) error { *installs++; return nil }
	notifyChain = func(notify.Options) notify.Chain { return notify.Chain{recSender{sent}} }
	t.Cleanup(func() { notifyDetect, brewPath, brewInstall, notifyChain = oldD, oldB, oldI, oldC })
	return installs, sent
}

func TestNotifySetupInstallsAfterYes(t *testing.T) {
	installs, sent := notifyEnv(t, false)
	r := i2a(t, "y\n", "notify", "setup")
	if r.err != nil {
		t.Fatalf("setup: %v\n%s", r.err, r.out)
	}
	if *installs != 1 {
		t.Errorf("brew install ran %d times, want 1", *installs)
	}
	for _, want := range []string{"✗ terminal-notifier", "Install terminal-notifier with Homebrew? [y/N]",
		"i2a tries terminal-notifier 3.1.0, then osascript.", "The test notification showed through terminal-notifier."} {
		if !strings.Contains(r.out, want) {
			t.Errorf("output does not contain %q:\n%s", want, r.out)
		}
	}
	if len(*sent) != 1 || (*sent)[0].Title != "i2a · test" {
		t.Errorf("sent = %+v", *sent)
	}
	paths, err := app.ResolvePaths()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(paths.DataDir, "extensions", "orca", "orca-plugin.json")); err != nil {
		t.Errorf("the Orca plugin was not written: %v", err)
	}
	if !strings.Contains(r.out, filepath.Join(paths.DataDir, "extensions", "orca")) {
		t.Errorf("the output does not name the plugin folder:\n%s", r.out)
	}
}

func TestNotifySetupAnswerNo(t *testing.T) {
	installs, _ := notifyEnv(t, false)
	r := i2a(t, "\n", "notify", "setup")
	if r.err != nil {
		t.Fatal(r.err)
	}
	if *installs != 0 || !strings.Contains(r.out, "  Not installed.") || !strings.Contains(r.out, "i2a tries osascript.") {
		t.Errorf("installs %d, output:\n%s", *installs, r.out)
	}
}

func TestNotifyStatusChangesNothing(t *testing.T) {
	installs, sent := notifyEnv(t, true)
	r := i2a(t, "", "notify", "status")
	if r.err != nil {
		t.Fatal(r.err)
	}
	if *installs != 0 || len(*sent) != 0 || !strings.Contains(r.out, "✓ terminal-notifier  terminal-notifier 3.1.0") ||
		!strings.Contains(r.out, `[notify] via = "auto": i2a tries terminal-notifier 3.1.0, then osascript.`) {
		t.Errorf("installs %d, sent %d, output:\n%s", *installs, len(*sent), r.out)
	}
}
