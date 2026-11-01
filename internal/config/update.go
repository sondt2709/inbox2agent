package config

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// ErrLocked means that another process held the write lock of the
// configuration file for longer than the lock wait.
var ErrLocked = errors.New("config.toml is locked by another process")

// lockWait is the time that a writer waits for the write lock. Tests make it
// shorter.
var lockWait = 5 * time.Second

// LockFile returns the path of the write lock of a configuration file.
func LockFile(path string) string { return path + ".lock" }

// lock takes the exclusive write lock of a configuration file. Each writer
// holds it while it reads, changes and writes the file. A lock ends when its
// process ends.
func lock(path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(LockFile(path), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(lockWait)
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() {
				_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
				f.Close()
			}, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			f.Close()
			return nil, fmt.Errorf("lock %s: %w", LockFile(path), err)
		}
		if time.Now().After(deadline) {
			f.Close()
			return nil, ErrLocked
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// Hash returns the SHA-256 of the bytes of a configuration file, in hex.
func Hash(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Update changes the configuration file that is on disk now. It holds the
// write lock, reads the file (or uses Default when the file does not exist),
// calls change, validates and writes the result. When change returns an
// error, Update writes nothing. Update returns the saved configuration and the
// hash of the written bytes.
func Update(path string, change func(*Config) error) (*Config, string, error) {
	unlock, err := lock(path)
	if err != nil {
		return nil, "", err
	}
	defer unlock()
	b, err := os.ReadFile(path)
	var c *Config
	switch {
	case errors.Is(err, os.ErrNotExist):
		c = Default()
	case err != nil:
		return nil, "", err
	default:
		if c, err = Parse(b); err != nil {
			return nil, "", fmt.Errorf("%s: %w", path, err)
		}
		if c.Migration != nil {
			if err := backupV1(path, b, c); err != nil {
				return nil, "", err
			}
		}
	}
	if err := change(c); err != nil {
		return nil, "", err
	}
	hash, err := save(path, c)
	if err != nil {
		return nil, "", err
	}
	return c, hash, nil
}

// Kinds of the items of a configuration, for Item and MissingError.
const (
	KindSettings     = "settings" // Settings and Notify together
	KindUnsorted     = "unsorted"
	KindAccount      = "account"
	KindAgent        = "agent"
	KindSchedule     = "schedule"
	KindSpace        = "space"
	KindConversation = "conversation" // the ID is "<account>/<conversation>"
)

// MissingError means that the item of a change does not exist in the file.
// Another process removed it.
type MissingError struct {
	Kind, ID string
}

func (e MissingError) Error() string {
	return fmt.Sprintf("the %s %s does not exist any more: another process removed it", e.Kind, e.ID)
}

// Item returns a copy of one item, to compare two versions of the
// configuration. A space and Unsorted come without their conversations,
// because each conversation is its own item. ok is false when the item does
// not exist.
func Item(c *Config, kind, id string) (item any, ok bool) {
	switch kind {
	case KindSettings:
		return [2]any{c.Settings, c.Notify}, true
	case KindUnsorted:
		u := c.Unsorted
		u.Conversations = nil
		return u, true
	case KindAccount:
		if a, ok := c.Account(id); ok {
			return *a, true
		}
	case KindAgent:
		if a, ok := c.Agent(id); ok {
			return *a, true
		}
	case KindSchedule:
		if s, ok := c.Schedule(id); ok {
			x := *s
			x.Days = append([]string(nil), s.Days...)
			return x, true
		}
	case KindSpace:
		if s, ok := c.Space(id); ok {
			x := *s
			x.Conversations = nil
			return x, true
		}
	case KindConversation:
		for _, s := range c.AllSpaces() {
			for _, cv := range s.Conversations {
				if cv.Account+"/"+cv.ID == id {
					return cv, true
				}
			}
		}
	}
	return nil, false
}
