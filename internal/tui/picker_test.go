package tui

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
)

func press(p *picker, keys ...string) tea.Cmd {
	var cmd tea.Cmd
	for _, k := range keys {
		var msg tea.KeyPressMsg
		switch k {
		case "enter":
			msg = tea.KeyPressMsg{Code: tea.KeyEnter}
		case "esc":
			msg = tea.KeyPressMsg{Code: tea.KeyEscape}
		case "space":
			msg = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
		case "down":
			msg = tea.KeyPressMsg{Code: tea.KeyDown}
		case "backspace":
			msg = tea.KeyPressMsg{Code: tea.KeyBackspace}
		case "ctrl+a":
			msg = tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl}
		default:
			for _, r := range k {
				_, cmd = p.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
			}
			continue
		}
		_, cmd = p.Update(msg)
	}
	return cmd
}

func newTestPicker(selected ...string) (*picker, *[]string) {
	items := []pickItem{
		{label: "acme / api-alerts", id: "c1"}, {label: "acme / api-team", id: "c2"},
		{label: "acme / Security-team", id: "c3"}, {label: "acme / sales", id: "c4"},
	}
	for i := range items {
		items[i].selected = slices.Contains(selected, items[i].id)
	}
	value := slices.Clone(selected)
	p := newPicker("Channels to watch", items, 10, &value)
	p.Focus()
	return p, &value
}

// The user types to search, without a key to open the search first.
func TestPickerTypeToSearchAndSelect(t *testing.T) {
	p, value := newTestPicker()
	press(p, "sec")
	if len(p.shown) != 1 || p.items[p.shown[0]].id != "c3" {
		t.Fatalf("search %q shows %v, want only Security-team (the search ignores case)", p.query, p.shown)
	}
	press(p, "space")
	if !slices.Equal(*value, []string{"c3"}) {
		t.Fatalf("value = %v, want [c3]", *value)
	}
	// Esc clears the search and keeps the selection.
	if !p.GetFiltering() {
		t.Error("GetFiltering = false with an active search")
	}
	press(p, "esc")
	if p.query != "" || len(p.shown) != 4 || p.GetFiltering() {
		t.Fatalf("after esc: query %q, %d shown", p.query, len(p.shown))
	}
	press(p, "api", "down", "space")
	if !slices.Equal(*value, []string{"c2", "c3"}) {
		t.Fatalf("value = %v, want [c2 c3]", *value)
	}
	if v := p.View(); !strings.Contains(v, "Search: api") || !strings.Contains(v, "2 of 4 · 2 selected") {
		t.Errorf("view:\n%s", v)
	}
	if cmd := press(p, "enter"); cmd == nil || cmd() != huh.NextField() {
		t.Fatal("enter does not move to the next field")
	}
}

func TestPickerSpaceDoesNotType(t *testing.T) {
	p, _ := newTestPicker()
	press(p, "api", "space", "x")
	if p.query != "apix" {
		t.Errorf("query = %q, want %q: space selects and does not type", p.query, "apix")
	}
	press(p, "backspace")
	if p.query != "api" {
		t.Errorf("after backspace query = %q", p.query)
	}
}

func TestPickerSelectAllShownAndSelectedFirst(t *testing.T) {
	p, value := newTestPicker("c4")
	if p.items[0].id != "c4" {
		t.Errorf("first item = %s, want the selected c4 first", p.items[0].id)
	}
	press(p, "api", "ctrl+a")
	if !slices.Equal(*value, []string{"c4", "c1", "c2"}) {
		t.Fatalf("value = %v, want c4 and the 2 shown api channels", *value)
	}
	press(p, "ctrl+a")
	if !slices.Equal(*value, []string{"c4"}) {
		t.Fatalf("second ctrl+a: value = %v, want [c4]", *value)
	}
	press(p, "esc", "zzz")
	if v := p.View(); !strings.Contains(v, "No channel matches") {
		t.Errorf("view with no match:\n%s", v)
	}
}

var _ huh.Field = (*picker)(nil)

// The picker must color the channel names as the Huh fields around it do.
// Huh fields use the light variant of the theme until the terminal reports
// its background color.
func TestPickerUsesTheThemeVariantOfHuhFields(t *testing.T) {
	p, _ := newTestPicker()
	p.WithTheme(huh.ThemeFunc(huh.ThemeCharm))
	ms := huh.NewMultiSelect[string]().Options(huh.NewOption("acme / api-team", "c2")).WithTheme(huh.ThemeFunc(huh.ThemeCharm))
	ms.Focus()
	want := huh.ThemeCharm(false).Focused.UnselectedOption.Render("acme / api-team")
	if !strings.Contains(ms.View(), want) {
		t.Fatalf("test setup: the Huh multi-select does not use the light variant:\n%q", ms.View())
	}
	if v := p.View(); !strings.Contains(v, want) {
		t.Errorf("the picker colors the option differently from Huh fields:\nwant %q in\n%q", want, v)
	}
}
