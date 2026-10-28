package secrets

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFileBackend(t *testing.T) {
	t.Setenv("I2A_SECRET_BACKEND", "file")
	path := filepath.Join(t.TempDir(), "secrets.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Backend() != "file" {
		t.Fatalf("backend = %s", s.Backend())
	}
	if _, err := s.Get(TokenKey("work")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get before Set: err = %v, want ErrNotFound", err)
	}
	if err := s.Set(TokenKey("work"), "abc"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("mode = %o, want 600", mode)
	}
	if v, err := s.Get("work/token"); err != nil || v != "abc" {
		t.Fatalf("Get = %q, %v", v, err)
	}
	if err := s.Delete("work/token"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get("work/token"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after Delete: err = %v", err)
	}
}

func TestUnknownBackend(t *testing.T) {
	t.Setenv("I2A_SECRET_BACKEND", "vault")
	if _, err := Open("x"); err == nil {
		t.Fatal("Open returned no error for an unknown backend")
	}
}
