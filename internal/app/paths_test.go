package app

import (
	"path/filepath"
	"testing"
)

func TestI2AHomeOverridesPaths(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("I2A_HOME", dir)
	p, err := ResolvePaths()
	if err != nil {
		t.Fatal(err)
	}
	if p.ConfigFile != filepath.Join(dir, "config.toml") || p.DataDir != dir {
		t.Errorf("paths = %+v", p)
	}
}

func TestXDGPaths(t *testing.T) {
	t.Setenv("I2A_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "/cfg")
	t.Setenv("XDG_DATA_HOME", "/data")
	p, err := ResolvePaths()
	if err != nil {
		t.Fatal(err)
	}
	if p.ConfigFile != "/cfg/i2a/config.toml" || p.DBFile != "/data/i2a/inbox.db" {
		t.Errorf("paths = %+v", p)
	}
}
