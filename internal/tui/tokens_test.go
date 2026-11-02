package tui

import (
	"encoding/json"
	"fmt"
	"image/color"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/sondt2709/inbox2agent/internal/inbox"
)

// designTokens is the format of docs/design-tokens.json.
type designTokens struct {
	Tokens map[string]struct {
		Value  string `json:"value"`
		TUI    string `json:"tui"`
		Kotlin string `json:"kotlin"`
	} `json:"tokens"`
}

func readTokens(t *testing.T) designTokens {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "docs", "design-tokens.json"))
	if err != nil {
		t.Fatal(err)
	}
	var d designTokens
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatal(err)
	}
	if len(d.Tokens) == 0 {
		t.Fatal("no tokens")
	}
	return d
}

func hexOf(c color.Color) string {
	if c == nil {
		return "<none>"
	}
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02X%02X%02X", r>>8, g>>8, b>>8)
}

// TestDesignTokensTUI checks that the TUI colors equal docs/design-tokens.json.
func TestDesignTokensTUI(t *testing.T) {
	tui := map[string]color.Color{
		"accent": accent, "accentDim": accentDim, "fgColor": fgColor, "mutedFg": mutedFg, "borderFg": borderFg,
		"green": green, "yellow": yellow, "red": red, "blue": blue, "inputBg": inputBg, "focusBg": focusBg,
		"onAccent":    brandStyle.GetForeground(),
		"labelFocus":  labelFocus.GetForeground(),
		"header":      headerStyle.GetBackground(),
		"btnNormal":   btnNormal.GetBackground(),
		"unreadMark":  unreadMark.GetForeground(),
		"btnDangerBg": btnDanger.GetBackground(),
		"btnDangerFg": btnDanger.GetForeground(),
		"categoryFg":  catStyles[inbox.CategoryAction].GetForeground(),
	}
	for name, tok := range readTokens(t).Tokens {
		if tok.TUI == "" {
			continue
		}
		c, ok := tui[tok.TUI]
		if !ok {
			t.Errorf("%s: the test knows no TUI color %q", name, tok.TUI)
			continue
		}
		if got := hexOf(c); got != strings.ToUpper(tok.Value) {
			t.Errorf("%s: TUI %s is %s, the token is %s", name, tok.TUI, got, tok.Value)
		}
	}
}

var kotlinToken = regexp.MustCompile(`const val (\w+): Long = 0x([0-9A-Fa-f]{8})`)

// TestDesignTokensAndroid checks that DesignTokens.kt of the Android app equals
// docs/design-tokens.json, so the app and the TUI have one primary color.
func TestDesignTokensAndroid(t *testing.T) {
	root := filepath.Join("..", "..", "mobile", "shared", "src", "commonMain")
	var file string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.Name() == "DesignTokens.kt" {
			file = p
		}
		return nil
	})
	if file == "" {
		t.Fatalf("no DesignTokens.kt under %s", root)
	}
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	kt := map[string]string{}
	for _, m := range kotlinToken.FindAllStringSubmatch(string(b), -1) {
		kt[m[1]] = "#" + strings.ToUpper(m[2][2:])
		if !strings.EqualFold(m[2][:2], "FF") {
			t.Errorf("%s: alpha is %s; tokens are opaque", m[1], m[2][:2])
		}
	}
	for name, tok := range readTokens(t).Tokens {
		got, ok := kt[tok.Kotlin]
		switch {
		case !ok:
			t.Errorf("%s: DesignTokens.kt has no %s", name, tok.Kotlin)
		case got != strings.ToUpper(tok.Value):
			t.Errorf("%s: DesignTokens.kt %s is %s, the token is %s", name, tok.Kotlin, got, tok.Value)
		}
	}
}
