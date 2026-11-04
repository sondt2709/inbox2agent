package cli

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/sondt2709/inbox2agent/internal/app"
	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/download"
	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/source"
	"github.com/sondt2709/inbox2agent/internal/source/whatsapp"
	"github.com/sondt2709/inbox2agent/internal/store"
)

// downloadEnv stores one WhatsApp document message and replaces the fetch
// with a fake that writes a temporary file as the daemon does.
func downloadEnv(t *testing.T, name string, fetchErr error) (msgID int64, paths app.Paths, fetched *[]inbox.Message) {
	t.Helper()
	cardEnv(t)
	paths, _ = app.ResolvePaths()
	st, err := store.Open(paths.DBFile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SavePoll(context.Background(), "phone", []inbox.Message{{SourceID: "phone", SourceType: "whatsapp",
		ExternalID: "3EB0AA", ChannelID: "15550000001@s.whatsapp.net", ChannelKind: inbox.KindDirect, Author: "Lan",
		Text: "[document: " + name + "]", CreatedAt: time.Now()}}, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	msgs, _ := st.Pending(context.Background())
	st.Close()
	for _, m := range msgs {
		if m.SourceID == "phone" {
			msgID = m.ID
		}
	}
	var got []inbox.Message
	old := fetchFile
	fetchFile = func(_ context.Context, _ *config.Config, m inbox.Message, env source.Env) (whatsapp.File, error) {
		got = append(got, m)
		if fetchErr != nil {
			return whatsapp.File{}, fetchErr
		}
		dir := (app.Paths{DataDir: env.DataDir}).TmpDir()
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(dir, "download-1")
		if err := os.WriteFile(p, []byte("%PDF-1.7 plan"), 0o600); err != nil {
			t.Fatal(err)
		}
		return whatsapp.File{Path: p, Name: name, MIME: "application/pdf", Kind: "document", Size: 13}, nil
	}
	t.Cleanup(func() { fetchFile = old })
	return msgID, paths, &got
}

func TestDownloadSavesTheFile(t *testing.T) {
	id, paths, fetched := downloadEnv(t, "../../plan.pdf", nil)
	dir := filepath.Join(t.TempDir(), "Downloads")
	r := i2a(t, "", "download", "--message", strconv.FormatInt(id, 10), "--dir", dir)
	if r.err != nil {
		t.Fatal(r.err)
	}
	want := filepath.Join(dir, "plan.pdf")
	if strings.TrimSpace(r.out) != want {
		t.Errorf("output = %q; want %s", r.out, want)
	}
	if b, err := os.ReadFile(want); string(b) != "%PDF-1.7 plan" || err != nil {
		t.Errorf("file = %q, %v", b, err)
	}
	if (*fetched)[0].ExternalID != "3EB0AA" {
		t.Errorf("fetched %+v", (*fetched)[0])
	}
	if runtime.GOOS == "darwin" {
		buf := make([]byte, 64)
		if n, err := unix.Getxattr(want, download.QuarantineAttr, buf); err != nil {
			t.Errorf("no quarantine attribute: %v", err)
		} else if !strings.HasSuffix(string(buf[:n]), ";i2a;") {
			t.Errorf("quarantine = %q", buf[:n])
		}
	}
	if left, _ := os.ReadDir(paths.TmpDir()); len(left) != 0 {
		t.Errorf("temporary files left: %d", len(left))
	}
	r = i2a(t, "", "download", "--message", strconv.FormatInt(id, 10), "--dir", dir)
	if r.err != nil || strings.TrimSpace(r.out) != filepath.Join(dir, "plan (1).pdf") {
		t.Errorf("second download = %q, %v; want plan (1).pdf", r.out, r.err)
	}
}

func TestDownloadErrors(t *testing.T) {
	id, _, _ := downloadEnv(t, "plan.pdf", whatsapp.ErrNoFile)
	dir := t.TempDir()
	r := i2a(t, "", "download", "--message", strconv.FormatInt(id, 10), "--dir", dir)
	if r.err == nil || r.err.Error() != "message "+strconv.FormatInt(id, 10)+": the message has no file" {
		t.Errorf("no file: %v", r.err)
	}
	if left, _ := os.ReadDir(dir); len(left) != 0 {
		t.Errorf("files in --dir: %d", len(left))
	}
	r = i2a(t, "", "download", "--message", "99999", "--dir", dir)
	if r.err == nil || r.err.Error() != "the store has no message 99999" {
		t.Errorf("unknown message: %v", r.err)
	}
}

func TestDownloadOfAMattermostMessage(t *testing.T) {
	id, _ := cardEnv(t)
	if err := os.WriteFile(filepath.Join(os.Getenv("I2A_HOME"), "config.toml"), []byte(`version = 2
[[accounts]]
id = "work"
platform = "mattermost"
url = "https://chat.example.com"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	r := i2a(t, "", "download", "--message", strconv.FormatInt(id, 10), "--dir", t.TempDir())
	if r.err == nil || !strings.Contains(r.err.Error(), `account "work" (mattermost): cannot download files`) {
		t.Errorf("err = %v", r.err)
	}
}
