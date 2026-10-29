package source

import (
	"errors"
	"strings"
	"testing"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/secrets"
)

type memSecrets map[string]string

func (m memSecrets) Get(k string) (string, error) {
	if v, ok := m[k]; ok {
		return v, nil
	}
	return "", secrets.ErrNotFound
}
func (m memSecrets) Set(k, v string) error { m[k] = v; return nil }
func (m memSecrets) Delete(k string) error { delete(m, k); return nil }
func (m memSecrets) Backend() string       { return "memory" }

func TestBuildReplier(t *testing.T) {
	cfg := &config.Config{Sources: []config.Source{{ID: "work", Type: config.SourceMattermost, URL: "http://mm.example"}}}
	sec := memSecrets{secrets.TokenKey("work"): "token"}
	if r, err := BuildReplier(cfg, "work", sec, nil); err != nil || r == nil {
		t.Errorf("work: %v, %v, want a replier", r, err)
	}
	if _, err := BuildReplier(cfg, "gone", sec, nil); err == nil || !strings.Contains(err.Error(), `source "gone" is not in the configuration`) {
		t.Errorf("gone: %v", err)
	}
	if _, err := BuildReplier(cfg, "work", memSecrets{}, nil); err == nil {
		t.Error("no token: no error")
	}
}

func TestCanReply(t *testing.T) {
	cfg := &config.Config{Sources: []config.Source{{ID: "work", Type: config.SourceMattermost}, {ID: "chat", Type: "whatsapp"}}}
	if err := CanReply(cfg, "work"); err != nil {
		t.Errorf("work: %v", err)
	}
	if err := CanReply(cfg, "chat"); !errors.Is(err, ErrCannotReply) {
		t.Errorf("chat: %v, want ErrCannotReply", err)
	}
	if err := CanReply(cfg, "gone"); err == nil || !strings.Contains(err.Error(), "not in the configuration") {
		t.Errorf("gone: %v", err)
	}
}
