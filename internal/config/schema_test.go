package config

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "write docs/config.schema.json again")

const schemaCopy = "../../docs/config.schema.json"

func parseSchema(t *testing.T) map[string]any {
	t.Helper()
	var s map[string]any
	if err := json.Unmarshal(Schema(), &s); err != nil {
		t.Fatalf("the schema is not JSON: %v", err)
	}
	return s
}

// at returns the schema at a path of keys: "spaces" is an array, so its
// objects are at "spaces", "items".
func at(t *testing.T, s map[string]any, keys ...string) map[string]any {
	t.Helper()
	cur := s
	for _, k := range keys {
		next, ok := cur[k].(map[string]any)
		if !ok {
			if p, ok := cur["properties"].(map[string]any); ok {
				next, ok = p[k].(map[string]any)
				if !ok {
					t.Fatalf("no schema at %v (%s)", keys, k)
				}
			} else {
				t.Fatalf("no schema at %v (%s)", keys, k)
			}
		}
		cur = next
	}
	return cur
}

func strs(v any) []string {
	var out []string
	for _, x := range v.([]any) {
		out = append(out, x.(string))
	}
	return out
}

func TestSchemaDescribesTheFile(t *testing.T) {
	s := parseSchema(t)
	if s["$schema"] != "https://json-schema.org/draft/2020-12/schema" || s["additionalProperties"] != false {
		t.Fatalf("root: %v %v", s["$schema"], s["additionalProperties"])
	}
	for _, tc := range []struct {
		path []string
		want []string
	}{
		// "" is the default value of a key with a default.
		{[]string{"spaces", "items", "conversations", "items", "read"}, []string{"", "all", "mentions", "off"}},
		{[]string{"unsorted", "conversations", "items", "read"}, []string{"", "all", "mentions", "off"}},
		{[]string{"unsorted", "read"}, []string{"", "all", "mentions", "off"}},
		{[]string{"accounts", "items", "platform"}, []string{"mattermost", "whatsapp"}},
		{[]string{"agents", "items", "driver"}, []string{"", "orca", "dry-run"}},
		{[]string{"agents", "items", "session"}, []string{"", "new", "reuse", "worktree"}},
		{[]string{"agents", "items", "payload"}, []string{"", "pointer", "inline"}},
		{[]string{"schedules", "items", "days", "items"}, Days},
		{[]string{"notify", "min_urgency"}, []string{"", "high", "normal", "low"}},
	} {
		if got := strs(at(t, s, tc.path...)["enum"]); !slices.Equal(got, tc.want) {
			t.Errorf("%v enum = %v, want %v", tc.path, got, tc.want)
		}
	}
	for _, tc := range []struct {
		path []string
		want []string
	}{
		{[]string{"accounts", "items"}, []string{"id", "platform"}},
		{[]string{"spaces", "items"}, []string{"id", "agent", "schedule"}},
		{[]string{"spaces", "items", "conversations", "items"}, []string{"account", "id"}},
		{[]string{"agents", "items"}, []string{"id"}},
		{[]string{"schedules", "items"}, []string{"id"}},
	} {
		if got := strs(at(t, s, tc.path...)["required"]); !slices.Equal(got, tc.want) {
			t.Errorf("%v required = %v, want %v", tc.path, got, tc.want)
		}
	}
	re := regexp.MustCompile(at(t, s, "schedules", "items", "every")["pattern"].(string))
	for _, ok := range []string{"15m", "1h30m", "7d", "30s", "", "0"} {
		if !re.MatchString(ok) {
			t.Errorf("the duration pattern rejects %q", ok)
		}
	}
	for _, bad := range []string{"ten minutes", "15", "d"} {
		if re.MatchString(bad) {
			t.Errorf("the duration pattern accepts %q", bad)
		}
	}
	if p := at(t, s, "spaces", "items", "id")["pattern"]; p != nameRe.String() {
		t.Errorf("space id pattern = %v", p)
	}
	// 0 is the default; another value is in the range.
	v := at(t, s, "agents", "items", "inline_max_bytes")["anyOf"].([]any)
	if r := v[1].(map[string]any); len(v) != 2 || v[0].(map[string]any)["const"] != float64(0) ||
		r["minimum"] != float64(MinInlineBytes) || r["maximum"] != float64(MaxInlineBytes) {
		t.Errorf("inline_max_bytes = %v", v)
	}
	hours := regexp.MustCompile(at(t, s, "schedules", "items", "hours")["pattern"].(string))
	for _, h := range []string{"", "09:00-17:00", "09:00 - 17:00", "22:00-06:00"} {
		if _, err := ParseHours(h); err != nil || !hours.MatchString(h) {
			t.Errorf("hours %q: ParseHours %v, pattern %v", h, err, hours.MatchString(h))
		}
	}
	// Each key has a description and each object refuses unknown keys.
	var walk func(path string, n map[string]any)
	walk = func(path string, n map[string]any) {
		if n["type"] == "object" && n["additionalProperties"] != false {
			t.Errorf("%s allows unknown keys", path)
		}
		if items, ok := n["items"].(map[string]any); ok {
			walk(path+"[]", items)
		}
		props, _ := n["properties"].(map[string]any)
		for k, v := range props {
			p := v.(map[string]any)
			if d, _ := p["description"].(string); d == "" {
				t.Errorf("%s.%s has no description", path, k)
			}
			walk(path+"."+k, p)
		}
	}
	walk("", s)
}

// Each key of the TOML output is in the schema: a key that the schema does
// not have is a new field without tags.
func TestSchemaHasEachKeyOfTheFile(t *testing.T) {
	keys := map[string]bool{}
	var walk func(n map[string]any)
	walk = func(n map[string]any) {
		if items, ok := n["items"].(map[string]any); ok {
			walk(items)
		}
		props, _ := n["properties"].(map[string]any)
		for k, v := range props {
			keys[k] = true
			walk(v.(map[string]any))
		}
	}
	walk(parseSchema(t))
	var fields func(rt reflect.Type)
	fields = func(rt reflect.Type) {
		for rt.Kind() == reflect.Slice {
			rt = rt.Elem()
		}
		if rt.Kind() != reflect.Struct || rt == reflect.TypeFor[Duration]() {
			return
		}
		for f := range rt.Fields() {
			k, _, _ := strings.Cut(f.Tag.Get("toml"), ",")
			if k == "-" || k == "" {
				continue
			}
			if !keys[k] {
				t.Errorf("the key %s of %s is not in the schema", k, rt.Name())
			}
			fields(f.Type)
		}
	}
	fields(reflect.TypeFor[Config]())
}

func TestSchemaCopyIsCurrent(t *testing.T) {
	if *update {
		if err := os.WriteFile(schemaCopy, Schema(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	b, err := os.ReadFile(schemaCopy)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != string(Schema()) {
		t.Fatal("docs/config.schema.json is old: run go test ./internal/config -run TestSchemaCopyIsCurrent -update")
	}
}

func TestSaveWritesTheSchemaLineAndFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new")
	path := filepath.Join(dir, "config.toml")
	if err := Save(path, Default()); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if first, _, _ := strings.Cut(string(b), "\n"); first != "#:schema ./config.schema.json" {
		t.Errorf("line 1 = %q", first)
	}
	sb, err := os.ReadFile(filepath.Join(dir, "config.schema.json"))
	if err != nil || string(sb) != string(Schema()) {
		t.Fatalf("config.schema.json: %v, equal %v", err, string(sb) == string(Schema()))
	}
	if fi, _ := os.Stat(filepath.Join(dir, "config.schema.json")); fi.Mode().Perm() != 0o600 {
		t.Errorf("schema mode %v", fi.Mode().Perm())
	}
	// The file with the line loads, and a second save gives the same bytes.
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(path, c); err != nil {
		t.Fatal(err)
	}
	if b2, _ := os.ReadFile(path); string(b2) != string(b) {
		t.Errorf("a second save changed the file:\n%s\n---\n%s", b, b2)
	}
}

func TestLoadWritesAMissingSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := Save(path, Default()); err != nil {
		t.Fatal(err)
	}
	schema := filepath.Join(filepath.Dir(path), "config.schema.json")
	if err := os.WriteFile(schema, []byte("{}"), 0o600); err != nil { // the schema of an older binary
		t.Fatal(err)
	}
	if _, err := Load(path); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(schema); string(b) != string(Schema()) {
		t.Error("the load did not write the schema of this binary")
	}
}
