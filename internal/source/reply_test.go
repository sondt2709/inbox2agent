package source

import (
	"errors"
	"io"
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
	cfg := &config.Config{Accounts: []config.Account{{ID: "work", Platform: config.PlatformMattermost, URL: "http://mm.example"},
		{ID: "phone", Platform: config.PlatformWhatsApp}}}
	sec := memSecrets{secrets.TokenKey("work"): "token"}
	if r, err := BuildReplier(cfg, "work", Env{Secrets: sec}); err != nil || r == nil {
		t.Errorf("work: %v, %v, want a replier", r, err)
	}
	r, err := BuildReplier(cfg, "phone", OneShot(Env{Secrets: sec, DataDir: t.TempDir()}))
	if c, ok := r.(io.Closer); err != nil || !ok {
		t.Errorf("phone: %v, %v, want a WhatsApp replier", r, err)
	} else {
		c.Close()
	}
	if _, err := BuildReplier(cfg, "gone", Env{Secrets: sec}); err == nil || !strings.Contains(err.Error(), `account "gone" is not in the configuration`) {
		t.Errorf("gone: %v", err)
	}
	if _, err := BuildReplier(cfg, "work", Env{Secrets: memSecrets{}}); err == nil {
		t.Error("no token: no error")
	}
}

func TestBuildDownloader(t *testing.T) {
	cfg := &config.Config{Accounts: []config.Account{{ID: "work", Platform: config.PlatformMattermost, URL: "http://mm.example"},
		{ID: "phone", Platform: config.PlatformWhatsApp}}}
	if _, err := BuildDownloader(cfg, "work", Env{Secrets: memSecrets{secrets.TokenKey("work"): "token"}}); !errors.Is(err, ErrCannotDownload) {
		t.Errorf("work: %v, want ErrCannotDownload", err)
	}
	d, err := BuildDownloader(cfg, "phone", OneShot(Env{DataDir: t.TempDir()}))
	if err != nil || d == nil {
		t.Fatalf("phone: %v, %v, want a downloader", d, err)
	}
	d.(io.Closer).Close()
}

func TestCanReply(t *testing.T) {
	cfg := &config.Config{Accounts: []config.Account{{ID: "work", Platform: config.PlatformMattermost},
		{ID: "chat", Platform: config.PlatformWhatsApp}, {ID: "issues", Platform: "linear"}}}
	for _, id := range []string{"work", "chat"} {
		if err := CanReply(cfg, id); err != nil {
			t.Errorf("%s: %v", id, err)
		}
	}
	if err := CanReply(cfg, "issues"); !errors.Is(err, ErrCannotReply) {
		t.Errorf("issues: %v, want ErrCannotReply", err)
	}
	if err := CanReply(cfg, "gone"); err == nil || !strings.Contains(err.Error(), "not in the configuration") {
		t.Errorf("gone: %v", err)
	}
}
