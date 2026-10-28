// Package secrets keeps source tokens out of the configuration file.
package secrets

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/zalando/go-keyring"
)

// ErrNotFound is returned when a key has no secret.
var ErrNotFound = errors.New("secret not found")

const keychainService = "i2a"

// Store reads and writes secrets by key.
type Store interface {
	Get(key string) (string, error)
	Set(key, value string) error
	Delete(key string) error
	Backend() string
}

// TokenKey is the secret key for the token of a source.
func TokenKey(sourceID string) string { return sourceID + "/token" }

// Open returns the backend that I2A_SECRET_BACKEND selects: "keychain" (default) or "file".
func Open(secretsFile string) (Store, error) {
	switch b := os.Getenv("I2A_SECRET_BACKEND"); b {
	case "", "keychain":
		return keychainStore{}, nil
	case "file":
		return &fileStore{path: secretsFile}, nil
	default:
		return nil, fmt.Errorf("I2A_SECRET_BACKEND: unknown backend %q (use keychain or file)", b)
	}
}

type keychainStore struct{}

func (keychainStore) Backend() string { return "keychain" }

func (keychainStore) Get(key string) (string, error) {
	v, err := keyring.Get(keychainService, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	return v, err
}

func (keychainStore) Set(key, value string) error { return keyring.Set(keychainService, key, value) }

func (keychainStore) Delete(key string) error {
	err := keyring.Delete(keychainService, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}

// fileStore keeps secrets in a JSON file with mode 0600.
type fileStore struct {
	path string
	mu   sync.Mutex
}

func (*fileStore) Backend() string { return "file" }

func (f *fileStore) load() (map[string]string, error) {
	m := map[string]string{}
	b, err := os.ReadFile(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	if len(b) == 0 {
		return m, nil
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("read %s: %w", f.path, err)
	}
	return m, nil
}

func (f *fileStore) save(m map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(f.path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp := f.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, f.path)
}

func (f *fileStore) Get(key string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, err := f.load()
	if err != nil {
		return "", err
	}
	v, ok := m[key]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

func (f *fileStore) Set(key, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, err := f.load()
	if err != nil {
		return err
	}
	m[key] = value
	return f.save(m)
}

func (f *fileStore) Delete(key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, err := f.load()
	if err != nil {
		return err
	}
	delete(m, key)
	return f.save(m)
}
