package main

import (
	"runtime/debug"

	"github.com/sondt2709/inbox2agent/internal/app"
	"github.com/sondt2709/inbox2agent/internal/cli"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	app.Version = resolveVersion(version, debug.ReadBuildInfo)
	cli.Main()
}

// resolveVersion returns v. When v is "dev", it returns the module version that
// "go install <module>@<tag>" records in the binary, if there is one.
func resolveVersion(v string, buildInfo func() (*debug.BuildInfo, bool)) string {
	if v != "dev" {
		return v
	}
	if bi, ok := buildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return v
}
