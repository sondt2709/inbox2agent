package extensions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestWriteOrcaPlugin(t *testing.T) {
	out, err := Write(Orca, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"orca-plugin.json", "main.mjs", "icon.png", "README.md"} {
		if _, err := os.Stat(filepath.Join(out, f)); err != nil {
			t.Errorf("missing %s: %v", f, err)
		}
	}
	b, err := os.ReadFile(filepath.Join(out, "orca-plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		ID, Publisher, Main string
		Contributes         struct{ Commands []struct{ ID string } }
		Capabilities        []struct{ Kind string }
	}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	// i2a calls "inbox2agent.i2a" and "notify" (notify.OrcaPluginKey, notify.OrcaCommand).
	if m.Publisher+"."+m.ID != "inbox2agent.i2a" || m.Main != "main.mjs" || len(m.Contributes.Commands) != 1 || m.Contributes.Commands[0].ID != "notify" {
		t.Errorf("manifest = %+v", m)
	}
	var kinds []string
	for _, c := range m.Capabilities {
		kinds = append(kinds, c.Kind)
	}
	if !slices.Equal(kinds, []string{"notifications:show"}) {
		t.Errorf("capabilities = %v, want only notifications:show", kinds)
	}
}
