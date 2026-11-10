// Package extensions holds the plugins of external apps, one folder for each
// app. The i2a binary embeds them, and "i2a notify setup" writes them to the
// data folder, so a user who installed only the binary can install them.
package extensions

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed orca
var files embed.FS

// Orca is the folder of the Orca plugin.
const Orca = "orca"

// Write copies the plugin folder of app to <dir>/extensions/<app> and returns
// that path. It replaces the files of an older version.
func Write(app, dir string) (string, error) {
	sub, err := fs.Sub(files, app)
	if err != nil {
		return "", err
	}
	out := filepath.Join(dir, "extensions", app)
	err = fs.WalkDir(sub, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(out, p)
		if d.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		b, err := fs.ReadFile(sub, p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o600)
	})
	return out, err
}
