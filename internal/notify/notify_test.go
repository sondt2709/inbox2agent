package notify

import (
	"os/exec"
	"runtime"
	"testing"
)

// The title and the text are arguments of the script, never part of it.
func TestArgsKeepTextOutOfTheScript(t *testing.T) {
	evil := `x" & (do shell script "touch /tmp/pwned") & "`
	args := Args(evil, evil)
	if len(args) != 4 || args[0] != "-e" || args[1] != script || args[2] != evil || args[3] != evil {
		t.Fatalf("args = %q", args)
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
return (item 1 of argv) & "|" & (item 2 of argv)
end run`
	out, err := exec.Command("osascript", "-e", probe, evil, "text").Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := string(out); got != evil+"|text\n" {
		t.Errorf("osascript returned %q", got)
	}
}

func TestShort(t *testing.T) {
	if Short("héllo", 3) != "hél..." || Short("ok", 3) != "ok" {
		t.Error(Short("héllo", 3))
	}
}
