// Package cli defines the i2a commands.
package cli

import (
	"errors"
	"fmt"

	"github.com/sondt2709/inbox2agent/internal/app"
	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/render"
	"github.com/sondt2709/inbox2agent/internal/secrets"
	"github.com/sondt2709/inbox2agent/internal/store"
)

// env is what most commands need.
type env struct {
	paths   app.Paths
	secrets secrets.Store
}

func newEnv() (*env, error) {
	p, err := app.ResolvePaths()
	if err != nil {
		return nil, err
	}
	sec, err := secrets.Open(p.SecretsFile)
	if err != nil {
		return nil, err
	}
	return &env{paths: p, secrets: sec}, nil
}

// loadConfig loads and fully checks the configuration file.
func (e *env) loadConfig() (*config.Config, error) {
	c, err := config.Load(e.paths.ConfigFile)
	if errors.Is(err, config.ErrNotFound) {
		return nil, fmt.Errorf("no configuration at %s. Run `i2a config` to create one", e.paths.ConfigFile)
	}
	if err != nil {
		return nil, err
	}
	if err := render.CheckProfiles(c); err != nil {
		return nil, fmt.Errorf("%s: %w", e.paths.ConfigFile, err)
	}
	return c, nil
}

// loadOrDefault loads the configuration, or returns the default when the file does not exist.
func (e *env) loadOrDefault() (*config.Config, error) {
	c, err := config.Load(e.paths.ConfigFile)
	if errors.Is(err, config.ErrNotFound) {
		return config.Default(), nil
	}
	return c, err
}

func (e *env) openStore() (*store.Store, error) {
	if err := e.paths.EnsureDataDir(); err != nil {
		return nil, err
	}
	return store.Open(e.paths.DBFile)
}
