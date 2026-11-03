package notify

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Found describes one notifier on this Mac.
type Found struct {
	Name   string // a value of Vias other than ViaAuto
	OK     bool   // the notifier can show a note
	Detail string // the version or the reason, for the screens
}

var (
	versionMu sync.Mutex
	versions  = map[string]string{}
)

// tnVersion returns the version line of the terminal-notifier at path, once.
func tnVersion(path string) string {
	versionMu.Lock()
	defer versionMu.Unlock()
	if v, ok := versions[path]; ok {
		return v
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, _ := exec.CommandContext(ctx, path, "-version").Output()
	v := strings.TrimSuffix(strings.TrimSpace(string(out)), ".")
	versions[path] = v
	return v
}

// Detect returns the notifiers in the order of [notify] via = "auto".
func Detect() []Found {
	var out []Found
	if sock, _, err := orcaSocket(OrcaUserData()); err != nil {
		out = append(out, Found{Name: ViaOrca, Detail: "Orca does not run"})
	} else if _, err := os.Stat(sock); err != nil {
		out = append(out, Found{Name: ViaOrca, Detail: "Orca does not run"})
	} else {
		out = append(out, Found{Name: ViaOrca, OK: true, Detail: "Orca runs; it shows notes when the i2a plugin is installed and allowed"})
	}
	if p := FindTerminalNotifier(); p != "" {
		v := tnVersion(p)
		if v == "" {
			v = "terminal-notifier"
		}
		out = append(out, Found{Name: ViaTerminalNotifier, OK: true, Detail: v + " (" + p + ")"})
	} else {
		out = append(out, Found{Name: ViaTerminalNotifier, Detail: "not installed: run i2a notify setup"})
	}
	out = append(out, Found{Name: ViaOsascript, OK: true, Detail: "always on macOS (Script Editor icon)"})
	return out
}

// Describe returns one line with the notifiers that i2a tries for via, in
// order, for the screens: "terminal-notifier 3.1.0, then osascript".
func Describe(via string, found []Found) string {
	by := map[string]Found{}
	for _, f := range found {
		by[f.Name] = f
	}
	var parts []string
	for _, s := range NewChain(Options{Via: via}) {
		f := by[s.Name()]
		if !f.OK {
			continue
		}
		label := f.Name
		if f.Name == ViaTerminalNotifier {
			label, _, _ = strings.Cut(f.Detail, " (")
		}
		parts = append(parts, label)
	}
	if len(parts) == 0 {
		return "no notifier"
	}
	return strings.Join(parts, ", then ")
}
