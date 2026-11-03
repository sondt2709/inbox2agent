package notify

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// The title, the text and the subtitle are arguments of the script, never part of it.
func TestOsascriptKeepsTextOutOfTheScript(t *testing.T) {
	evil := `x" & (do shell script "touch /tmp/pwned") & "`
	args := Osascript{}.Args(Note{Title: evil, Subtitle: evil, Body: evil})
	if len(args) != 5 || args[0] != "-e" || args[1] != script || args[2] != "i2a · "+evil || args[3] != evil || args[4] != evil {
		t.Fatalf("args = %q", args)
	}
	if got := (Osascript{}).Args(Note{Title: "i2a: dispatch failed"})[2]; got != "i2a: dispatch failed" {
		t.Errorf("title = %q, want no second i2a", got)
	}
}

// osascript compiles the script and reads the arguments. The test uses "return"
// in place of "display notification", so no notification shows.
func TestScriptReadsArguments(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only")
	}
	evil := `x" & (do shell script "echo pwned") & "`
	probe := `on run argv
return (item 1 of argv) & "|" & (item 2 of argv) & "|" & (item 3 of argv)
end run`
	out, err := exec.Command("osascript", "-e", probe, evil, "text", "sub").Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := string(out); got != evil+"|text|sub\n" {
		t.Errorf("osascript returned %q", got)
	}
}

func TestTerminalNotifierArgs(t *testing.T) {
	tn := TerminalNotifier{}
	got := tn.Args(Note{Title: "QUESTION · high", Subtitle: "release · Work", Body: "[prod] is down\x07\nnow",
		Open: "mattermost://chat.example.com/t/pl/p1", Activate: WhatsAppBundle, Group: "i2a-card-4"}, "/data/icon.png")
	want := []string{"-title", "QUESTION · high", "-message", `\[prod] is down now`, "-subtitle", "release · Work",
		"-group", "i2a-card-4", "-contentImage", "/data/icon.png", "-open", "mattermost://chat.example.com/t/pl/p1"}
	if !slices.Equal(got, want) {
		t.Errorf("args =\n%q\nwant\n%q", got, want)
	}
	got = tn.Args(Note{Title: "-x", Body: "b", Open: "file:///etc/passwd", Activate: WhatsAppBundle}, "")
	want = []string{"-title", `\-x`, "-message", "b", "-activate", WhatsAppBundle}
	if !slices.Equal(got, want) {
		t.Errorf("args =\n%q\nwant\n%q", got, want)
	}
}

func TestFindTerminalNotifierInHomebrewFolders(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "terminal-notifier")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	old := tnDirs
	tnDirs = []string{filepath.Join(dir, "missing"), dir}
	t.Cleanup(func() { tnDirs = old })
	if got := FindTerminalNotifier(); got != bin {
		t.Errorf("FindTerminalNotifier() = %q, want %q", got, bin)
	}
	tnDirs = nil
	if got := FindTerminalNotifier(); got != "" {
		t.Errorf("FindTerminalNotifier() = %q, want none", got)
	}
}

func TestWriteIcon(t *testing.T) {
	dir := t.TempDir()
	p, err := WriteIcon(dir)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if !bytes.Equal(b, IconPNG()) || !bytes.HasPrefix(b, []byte("\x89PNG")) {
		t.Fatal("icon.png is not the embedded PNG")
	}
	if err := os.WriteFile(p, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteIcon(dir); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); !bytes.Equal(b, IconPNG()) {
		t.Error("WriteIcon did not replace a different file")
	}
}

func TestClick(t *testing.T) {
	all := func(string) bool { return true }
	none := func(string) bool { return false }
	for _, c := range []struct {
		src, url       string
		apps           Apps
		open, activate string
	}{
		{"mattermost", "https://chat.example.com/harbor/pl/p1", all, "mattermost://chat.example.com/harbor/pl/p1", ""},
		{"mattermost", "https://chat.example.com/harbor/pl/p1", none, "https://chat.example.com/harbor/pl/p1", ""},
		{"mattermost", "javascript:alert(1)", all, "", ""},
		{"mattermost", "http://chat.example.com/harbor/pl/p1", all, "", ""},
		{"whatsapp", "https://wa.me/15550001111", all, "whatsapp://send?phone=15550001111", ""},
		{"whatsapp", "https://wa.me/15550001111", none, "https://wa.me/15550001111", ""},
		{"whatsapp", "", all, "", WhatsAppBundle},
		{"whatsapp", "", none, "", ""},
		{"whatsapp", "https://wa.me/1555&x=1", all, "", WhatsAppBundle},
		{"github", "https://github.com/o/r/pull/1", all, "https://github.com/o/r/pull/1", ""},
		{"github", "file:///etc/passwd", all, "", ""},
	} {
		open, act := Click(c.src, c.url, c.apps)
		if open != c.open || act != c.activate {
			t.Errorf("Click(%s, %q) = %q %q, want %q %q", c.src, c.url, open, act, c.open, c.activate)
		}
	}
	for u, want := range map[string]bool{"https://x.example": true, "mattermost://h/t/pl/1": true,
		"whatsapp://send?phone=1": true, "http://x.example": false, "file:///etc/passwd": false, "javascript:alert(1)": false, "": false, "https:": false} {
		if AllowedOpen(u) != want {
			t.Errorf("AllowedOpen(%q) = %v", u, !want)
		}
	}
}

type fakeSender struct {
	name string
	err  error
	got  *[]string
}

func (f fakeSender) Name() string { return f.name }
func (f fakeSender) Send(_ context.Context, n Note) error {
	*f.got = append(*f.got, f.name)
	return f.err
}

func TestChainStopsAtTheFirstNotifierThatShows(t *testing.T) {
	var got []string
	fail := errors.New("not there")
	c := Chain{fakeSender{"orca", fail, &got}, fakeSender{"terminal-notifier", nil, &got}, fakeSender{"osascript", nil, &got}}
	name, err := c.Send(context.Background(), Note{Title: "t"})
	if err != nil || name != "terminal-notifier" || strings.Join(got, ",") != "orca,terminal-notifier" {
		t.Errorf("Send = %q %v, tried %v", name, err, got)
	}
	got = nil
	_, err = Chain{fakeSender{"orca", fail, &got}}.Send(context.Background(), Note{})
	if !errors.Is(err, ErrNone) || !errors.Is(err, fail) {
		t.Errorf("err = %v", err)
	}
}

func TestNewChainOrder(t *testing.T) {
	names := func(c Chain) string {
		var s []string
		for _, x := range c {
			s = append(s, x.Name())
		}
		return strings.Join(s, ",")
	}
	for via, want := range map[string]string{
		"": "orca,terminal-notifier,osascript", ViaAuto: "orca,terminal-notifier,osascript",
		ViaOrca: "orca,osascript", ViaTerminalNotifier: "terminal-notifier,osascript", ViaOsascript: "osascript",
	} {
		if got := names(NewChain(Options{Via: via})); got != want {
			t.Errorf("via %q: %s, want %s", via, got, want)
		}
	}
}

func TestDesktopOffDoesNothing(t *testing.T) {
	t.Setenv("I2A_NOTIFY", "off")
	if !Off() {
		t.Fatal("Off() = false with I2A_NOTIFY=off")
	}
	Desktop(Options{Via: ViaOsascript})(Note{Title: "must not show"})
}

// fakeOrca serves one Orca runtime socket that answers each request with reply(req).
func fakeOrca(t *testing.T, reply func(req map[string]any) string) (dir string, reqs chan map[string]any) {
	t.Helper()
	// A Unix socket path must be short; t.TempDir() can be too long on macOS.
	dir, err := os.MkdirTemp("/tmp", "orca")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, "o.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	rt, _ := json.Marshal(map[string]any{"authToken": "secret-token", "pid": 1,
		"transports": []map[string]string{{"kind": "websocket", "endpoint": "ws://x"}, {"kind": "unix", "endpoint": sock}}})
	if err := os.WriteFile(filepath.Join(dir, "orca-runtime.json"), rt, 0o600); err != nil {
		t.Fatal(err)
	}
	reqs = make(chan map[string]any, 4)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				line, err := bufio.NewReader(c).ReadBytes('\n')
				if err != nil {
					return
				}
				var req map[string]any
				_ = json.Unmarshal(line, &req)
				reqs <- req
				if out := reply(req); out != "" {
					_, _ = c.Write([]byte(out))
				}
				time.Sleep(4 * time.Second) // keep the connection open
			}()
		}
	}()
	return dir, reqs
}

func TestOrcaSendsThePluginCommand(t *testing.T) {
	dir, reqs := fakeOrca(t, func(req map[string]any) string {
		return `{"_keepalive":true}` + "\n" + `{"id":"` + req["id"].(string) + `","ok":true,"result":null,"_meta":{"runtimeId":"r"}}` + "\n"
	})
	err := Orca{UserData: dir}.Send(context.Background(), Note{Title: "ACTION · high", Subtitle: "release · Work", Body: "Fix\nit"})
	if err != nil {
		t.Fatal(err)
	}
	req := <-reqs
	params, _ := req["params"].(map[string]any)
	args, _ := params["args"].(map[string]any)
	if req["method"] != "plugins.invokeCommand" || req["authToken"] != "secret-token" ||
		params["pluginKey"] != "i2a.notify" || params["commandId"] != "notify" ||
		args["title"] != "ACTION · high" || args["body"] != "release · Work — Fix it" {
		t.Errorf("request = %v", req)
	}
}

func TestOrcaFailures(t *testing.T) {
	dir, _ := fakeOrca(t, func(req map[string]any) string {
		return `{"id":"` + req["id"].(string) + `","ok":false,"error":{"code":"plugin_not_found","message":"plugin i2a.notify is not enabled"}}` + "\n"
	})
	err := Orca{UserData: dir}.Send(context.Background(), Note{Title: "t"})
	if err == nil || !strings.Contains(err.Error(), "plugin_not_found") || strings.Contains(err.Error(), "secret-token") {
		t.Errorf("err = %v", err)
	}

	silent, _ := fakeOrca(t, func(map[string]any) string { return "" })
	start := time.Now()
	if err := (Orca{UserData: silent}).Send(context.Background(), Note{Title: "t"}); err == nil || time.Since(start) > orcaTimeout+time.Second {
		t.Errorf("silent Orca: err %v after %v", err, time.Since(start))
	}

	if err := (Orca{UserData: t.TempDir()}).Send(context.Background(), Note{Title: "t"}); err == nil || !strings.Contains(err.Error(), "orca is not running") {
		t.Errorf("no runtime file: %v", err)
	}
}

func TestITerm2AndOSC9(t *testing.T) {
	env := func(m map[string]string) Env { return func(k string) string { return m[k] } }
	if !ITerm2(env(map[string]string{"TERM_PROGRAM": "iTerm.app"})) || !ITerm2(env(map[string]string{"TERM_PROGRAM": "tmux", "LC_TERMINAL": "iTerm2"})) {
		t.Error("iTerm2 not found")
	}
	if ITerm2(env(map[string]string{"TERM_PROGRAM": "Apple_Terminal"})) || ITerm2(env(map[string]string{"TERM_PROGRAM": "WezTerm"})) {
		t.Error("another terminal is iTerm2")
	}
	text := OSC9Text(Note{Title: "ACTION · high", Subtitle: "release · Work", Body: "Fix\x07 the\x1b]0;x reply"})
	if text != "ACTION · high · release · Work: Fix the ]0;x reply" {
		t.Errorf("text = %q", text)
	}
	if got := OSC9(text, false); got != "\x1b]9;"+text+"\x07" {
		t.Errorf("OSC9 = %q", got)
	}
	if got := OSC9("hi", true); got != "\x1bPtmux;\x1b\x1b]9;hi\x07\x1b\\" {
		t.Errorf("OSC9 in tmux = %q", got)
	}
}

func TestFeedLock(t *testing.T) {
	dir := t.TempDir()
	if FeedShows(dir) {
		t.Fatal("FeedShows with no lock file")
	}
	release, ok := HoldFeedLock(dir)
	if !ok || !FeedShows(dir) {
		t.Fatalf("hold %v, FeedShows %v", ok, FeedShows(dir))
	}
	if _, ok := HoldFeedLock(dir); ok {
		t.Error("a second Feed took the lock")
	}
	release()
	if FeedShows(dir) {
		t.Error("FeedShows after release")
	}
}

func TestShort(t *testing.T) {
	if Short("héllo", 3) != "hél..." || Short("ok", 3) != "ok" {
		t.Error(Short("héllo", 3))
	}
}
