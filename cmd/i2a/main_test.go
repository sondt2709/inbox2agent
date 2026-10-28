package main

import (
	"runtime/debug"
	"testing"
)

func TestResolveVersion(t *testing.T) {
	info := func(v string) func() (*debug.BuildInfo, bool) {
		return func() (*debug.BuildInfo, bool) { return &debug.BuildInfo{Main: debug.Module{Version: v}}, true }
	}
	none := func() (*debug.BuildInfo, bool) { return nil, false }
	cases := []struct {
		name, v string
		bi      func() (*debug.BuildInfo, bool)
		want    string
	}{
		{"release build keeps the ldflags version", "1.2.0", info("v1.1.0"), "1.2.0"},
		{"go install of a tag", "dev", info("v1.0.0"), "v1.0.0"},
		{"local build", "dev", info("(devel)"), "dev"},
		{"no build info", "dev", none, "dev"},
	}
	for _, c := range cases {
		if got := resolveVersion(c.v, c.bi); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
