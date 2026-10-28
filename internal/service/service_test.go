package service

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPlistContent(t *testing.T) {
	s := Spec{
		Program:  "/opt/i2a & co/bin/i2a",
		LogFile:  "/home/user/.local/share/i2a/i2a.log",
		ExtraDir: []string{"/Applications/Orca.app/Contents/Resources/bin"},
		Env:      map[string]string{"I2A_HOME": "/tmp/x"},
	}
	p := Plist(s)
	for _, want := range []string{
		"<string>" + Label + "</string>",
		"<string>/opt/i2a &amp; co/bin/i2a</string>\n    <string>daemon</string>",
		"<key>PATH</key>\n    <string>/Applications/Orca.app/Contents/Resources/bin:/usr/bin:",
		"<key>I2A_HOME</key>\n    <string>/tmp/x</string>",
		"<key>KeepAlive</key>\n  <true/>",
		"<key>StandardErrorPath</key>\n  <string>/home/user/.local/share/i2a/i2a.log</string>",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("plist does not contain %q:\n%s", want, p)
		}
	}
	if runtime.GOOS == "darwin" {
		f := filepath.Join(t.TempDir(), "x.plist")
		if err := writeFile(f, p); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command("plutil", "-lint", f).CombinedOutput(); err != nil {
			t.Fatalf("plutil -lint: %s", out)
		}
	}
}

func TestParsePrint(t *testing.T) {
	out := "gui/501/io.github.sondt2709.i2a = {\n\tactive count = 1\n\tpath = /x.plist\n\ttype = LaunchAgent\n\tstate = running\n\n\tprogram = /usr/local/bin/i2a\n\tpid = 4242\n}"
	st := ParsePrint(out)
	if !st.Loaded || st.State != "running" || st.PID != "4242" {
		t.Errorf("state = %+v", st)
	}
}
