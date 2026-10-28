// Package app holds process-wide values: the file paths and the version.
package app

import (
	"fmt"
	"os"
	"path/filepath"
)

// Version is set by main from the build flags.
var Version = "dev"

// Paths are the files and directories that i2a uses.
type Paths struct {
	ConfigFile  string
	DataDir     string
	DBFile      string
	DigestDir   string
	LogFile     string
	LockFile    string
	SecretsFile string
}

// ResolvePaths returns the paths for this process.
// I2A_HOME puts the configuration and the data in one directory.
// Otherwise the XDG variables apply, with the XDG defaults on all platforms.
func ResolvePaths() (Paths, error) {
	var configDir, dataDir string
	if home := os.Getenv("I2A_HOME"); home != "" {
		abs, err := filepath.Abs(home)
		if err != nil {
			return Paths{}, fmt.Errorf("resolve I2A_HOME: %w", err)
		}
		configDir, dataDir = abs, abs
	} else {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return Paths{}, fmt.Errorf("find home directory: %w", err)
		}
		configDir = xdg("XDG_CONFIG_HOME", filepath.Join(userHome, ".config"))
		dataDir = xdg("XDG_DATA_HOME", filepath.Join(userHome, ".local", "share"))
		configDir = filepath.Join(configDir, "i2a")
		dataDir = filepath.Join(dataDir, "i2a")
	}
	return Paths{
		ConfigFile:  filepath.Join(configDir, "config.toml"),
		DataDir:     dataDir,
		DBFile:      filepath.Join(dataDir, "inbox.db"),
		DigestDir:   filepath.Join(dataDir, "digests"),
		LogFile:     filepath.Join(dataDir, "i2a.log"),
		LockFile:    filepath.Join(dataDir, "daemon.lock"),
		SecretsFile: filepath.Join(dataDir, "secrets.json"),
	}, nil
}

// EnsureDataDir creates the data directories with private permissions.
func (p Paths) EnsureDataDir() error {
	for _, d := range []string{p.DataDir, p.DigestDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return fmt.Errorf("create %s: %w", d, err)
		}
	}
	return nil
}

func xdg(env, fallback string) string {
	if v := os.Getenv(env); v != "" && filepath.IsAbs(v) {
		return v
	}
	return fallback
}
