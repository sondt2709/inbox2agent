package config

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
)

// SchemaFile is the name of the JSON Schema file next to config.toml.
const SchemaFile = "config.schema.json"

// SchemaLine is the first line of config.toml. Taplo and Tombi read the
// schema from it; the path is relative to config.toml.
const SchemaLine = "#:schema ./" + SchemaFile + "\n"

// durationPattern matches the text of a Duration: Go duration text or a whole
// number of days.
const durationPattern = `^\s*([0-9]+d|([0-9]+(\.[0-9]+)?(ns|us|µs|ms|s|m|h))+)\s*$`

var durationType = reflect.TypeFor[Duration]()

// Schema returns the JSON Schema (draft 2020-12) of config.toml. It comes from
// the Go types: the toml tag gives the key, the comment and doc tags the
// description, the enum, pattern, min and max tags the allowed values, and
// schema:"required" or schema:"id" the required keys and the ID pattern. The
// schema cannot check references between items; Validate does.
func Schema() []byte {
	root := schemaOf(reflect.TypeFor[Config]())
	root["$schema"] = "https://json-schema.org/draft/2020-12/schema"
	root["title"] = "i2a configuration (config.toml)"
	root["description"] = "The configuration of i2a. An account connects; each conversation is in one space; " +
		"a space sends its conversations to one agent on one schedule. Check a file with `i2a config check`."
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(root); err != nil {
		panic(err) // the schema has only maps, slices, strings, numbers and booleans
	}
	return b.Bytes()
}

// schemaOf returns the schema of a Go type.
func schemaOf(t reflect.Type) map[string]any {
	if t == durationType {
		return map[string]any{"type": "string", "pattern": durationPattern}
	}
	switch t.Kind() {
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Int:
		return map[string]any{"type": "integer"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Slice:
		return map[string]any{"type": "array", "items": schemaOf(t.Elem())}
	case reflect.Struct:
		props, required := map[string]any{}, []string{}
		for f := range t.Fields() {
			key, _, _ := strings.Cut(f.Tag.Get("toml"), ",")
			if key == "" || key == "-" || !f.IsExported() {
				continue
			}
			p := schemaOf(f.Type)
			if d := strings.TrimSpace(f.Tag.Get("comment") + " " + f.Tag.Get("doc")); d != "" {
				p["description"] = d
			}
			if e := f.Tag.Get("enum"); e != "" {
				target := p
				if f.Type.Kind() == reflect.Slice {
					target = p["items"].(map[string]any)
					p["uniqueItems"] = true
				}
				target["enum"] = enumValues(f.Type, e)
			}
			if v := f.Tag.Get("pattern"); v != "" {
				p["pattern"] = v
			}
			for tag, key := range map[string]string{"min": "minimum", "max": "maximum"} {
				if v := f.Tag.Get(tag); v != "" {
					n, err := strconv.Atoi(v)
					if err != nil {
						panic("config: bad " + tag + " tag on " + f.Name)
					}
					p[key] = n
				}
			}
			for opt := range strings.SplitSeq(f.Tag.Get("schema"), ",") {
				switch opt {
				case "required":
					required = append(required, key)
				case "id":
					p["pattern"] = nameRe.String()
				}
			}
			props[key] = p
		}
		s := map[string]any{"type": "object", "additionalProperties": false, "properties": props}
		if len(required) > 0 {
			s["required"] = required
		}
		return s
	}
	panic("config: no schema for the type " + t.String())
}

// enumValues splits an enum tag. The values of an int field are numbers.
func enumValues(t reflect.Type, tag string) []any {
	var out []any
	for v := range strings.SplitSeq(tag, ",") {
		if t.Kind() == reflect.Int {
			n, err := strconv.Atoi(v)
			if err != nil {
				panic("config: bad enum value " + v)
			}
			out = append(out, n)
			continue
		}
		out = append(out, v)
	}
	return out
}

// writeSchema writes the schema next to config.toml when the file is missing
// or different. The caller holds the lock of the configuration file.
func writeSchema(configPath string) error {
	path := filepath.Join(filepath.Dir(configPath), SchemaFile)
	b := Schema()
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, b) {
		return nil
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
