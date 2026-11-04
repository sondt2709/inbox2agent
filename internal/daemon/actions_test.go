package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/source"
	"github.com/sondt2709/inbox2agent/internal/source/whatsapp"
)

// actSource is a fake source that sends replies and downloads files.
type actSource struct {
	*fakeSource
	amu      sync.Mutex
	replies  []inbox.ReplyRequest
	replyErr error
	dls      []string // chat/id/dir of each download
	file     whatsapp.File
	dlErr    error
}

func (a *actSource) Reply(_ context.Context, r inbox.ReplyRequest) (inbox.Posted, error) {
	a.amu.Lock()
	defer a.amu.Unlock()
	a.replies = append(a.replies, r)
	if a.replyErr != nil {
		return inbox.Posted{}, a.replyErr
	}
	return inbox.Posted{ID: "3EB0AAAA", URL: "https://wa.me/1555"}, nil
}

func (a *actSource) Find(context.Context, inbox.ReplyRequest) (inbox.Posted, bool, error) {
	return inbox.Posted{}, false, nil
}

func (a *actSource) Download(_ context.Context, chat, id, dir string) (whatsapp.File, error) {
	a.amu.Lock()
	defer a.amu.Unlock()
	a.dls = append(a.dls, chat+"/"+id+"/"+dir)
	return a.file, a.dlErr
}

// actHarness has one account "work" with an actSource, one stored message
// with the text "secret plan" and one reply "secret answer" in the state
// sending. The API serves without the loop, as a loop that is busy.
func actHarness(t *testing.T) (*harness, *actSource, *Client, inbox.Message, int64) {
	t.Helper()
	h := newHarness(t, cfgWith("work"))
	src := &actSource{fakeSource: h.sources["work"]}
	h.e.d.BuildSource = func(config.Account, source.Env) (source.Source, error) { return src, nil }
	ctx := context.Background()
	src.addText("m1", "Cr", "secret plan")
	h.e.Poll(ctx, true)
	msgs, err := h.st.Pending(ctx)
	if err != nil || len(msgs) != 1 {
		t.Fatalf("pending = %v, %v", msgs, err)
	}
	m := msgs[0]
	id, err := h.st.StartReply(ctx, inbox.Reply{MessageID: m.ID, SourceID: "work", ChannelName: "chan", Place: inbox.PlaceThread,
		Text: "secret answer", By: inbox.ByUser, PendingID: "1793800000000", CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	dir := shortDir(t)
	srv := NewServer("test", "config.toml", time.Now())
	if _, err := srv.Listen(dir); err != nil {
		t.Fatal(err)
	}
	h.e.SetAPI(srv)
	sctx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	go func() { _ = srv.Serve(sctx) }()
	return h, src, NewClient(dir), m, id
}

func answeredOf(err error) bool {
	var a interface{ Answered() bool }
	return errors.As(err, &a) && a.Answered()
}

func TestAPIReplyWithoutTheLoop(t *testing.T) {
	_, src, client, m, id := actHarness(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// No loop reads the runs, so a run waits; a reply does not.
	p, err := client.Reply(ctx, id)
	if err != nil || p.ID != "3EB0AAAA" {
		t.Fatalf("reply = %+v, %v", p, err)
	}
	r := src.replies[0]
	if r.Text != "secret answer" || !r.Thread || r.PendingID != "1793800000000" || r.ReplyID != id || r.Target.ID != m.ID ||
		r.Target.ExternalID != "m1" {
		t.Errorf("request = %+v", r)
	}

	var raw json.RawMessage
	if err := client.do(ctx, "POST", "/v1/reply", ReplyRequest{ReplyID: id}, &raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "secret") {
		t.Errorf("the response has text: %s", raw)
	}

	for _, tc := range []struct {
		err      error
		answered bool
	}{
		{&refusedError{errors.New("not in the group")}, true},
		{errors.New("timed out waiting for message send response"), false},
	} {
		src.replyErr = tc.err
		_, err := client.Reply(ctx, id)
		if err == nil || err.Error() != tc.err.Error() || answeredOf(err) != tc.answered || errors.Is(err, ErrNoDaemon) {
			t.Errorf("err = %v, answered %t; want %v, answered %t", err, answeredOf(err), tc.err, tc.answered)
		}
	}

	if _, err := client.Reply(ctx, 99999); err == nil || !answeredOf(err) {
		t.Errorf("unknown reply: %v, want an answered error", err)
	}
	if err := client.do(ctx, "POST", "/v1/reply", map[string]string{"text": "x"}, &raw); err == nil ||
		!strings.Contains(err.Error(), `{"reply_id": <n>}`) {
		t.Errorf("bad body: %v", err)
	}
}

func TestAPIReplyRefusesASentReply(t *testing.T) {
	h, src, client, _, id := actHarness(t)
	ctx := context.Background()
	if err := h.st.FinishReply(ctx, id, inbox.ReplySent, "3EB0AAAA", "", "", time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Reply(ctx, id); err == nil || !answeredOf(err) || len(src.replies) != 0 {
		t.Errorf("err = %v, sends %d; want an answered error and no send", err, len(src.replies))
	}
}

func TestAPIDownload(t *testing.T) {
	h, src, client, m, _ := actHarness(t)
	ctx := context.Background()
	src.file = whatsapp.File{Path: filepath.Join(h.paths.TmpDir(), "download-1"), Name: "plan.pdf", MIME: "application/pdf",
		Kind: "document", Size: 12}
	f, err := client.Download(ctx, m.ID)
	if err != nil || f != src.file {
		t.Fatalf("download = %+v, %v; want %+v", f, err, src.file)
	}
	if want := "Cr/m1/" + h.paths.TmpDir(); src.dls[0] != want {
		t.Errorf("download of %s; want %s", src.dls[0], want)
	}
	src.dlErr = whatsapp.ErrNoFile
	if _, err := client.Download(ctx, m.ID); err == nil || err.Error() != whatsapp.ErrNoFile.Error() {
		t.Errorf("err = %v; want %v", err, whatsapp.ErrNoFile)
	}
	if _, err := client.Download(ctx, 99999); err == nil || !strings.Contains(err.Error(), "the store has no message 99999") {
		t.Errorf("unknown message: %v", err)
	}
}

func TestViaDaemonUsesItsOwnConnectionOnlyWithoutADaemon(t *testing.T) {
	ctx := context.Background()
	local := &actSource{fakeSource: &fakeSource{id: "phone"}}
	builds := 0
	v := &viaDaemon{client: NewClient(shortDir(t)), local: func() (source.Replier, error) { builds++; return local, nil }}
	p, err := v.Reply(ctx, inbox.ReplyRequest{Text: "x", ReplyID: 4})
	if err != nil || p.ID != "3EB0AAAA" || builds != 1 || local.closes != 1 {
		t.Errorf("no daemon: %+v, %v, builds %d, closes %d; want a send with a closed local source", p, err, builds, local.closes)
	}

	// A daemon that answers with an error: the client does not send itself.
	dir := shortDir(t)
	srv := NewServer("test", "config.toml", time.Now()) // no engine: the reply endpoint answers 503
	if _, err := srv.Listen(dir); err != nil {
		t.Fatal(err)
	}
	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { _ = srv.Serve(sctx) }()
	v = &viaDaemon{client: NewClient(dir), local: func() (source.Replier, error) { builds++; return local, nil }}
	if _, err := v.Reply(ctx, inbox.ReplyRequest{Text: "x", ReplyID: 4}); err == nil || answeredOf(err) || builds != 1 {
		t.Errorf("daemon error: %v, answered %t, builds %d; want an unknown result and no local send", err, answeredOf(err), builds)
	}
}

func TestDownloadRefusesAnotherPlatform(t *testing.T) {
	cfg := cfgWith("work")
	_, err := Download(context.Background(), cfg, inbox.Message{ID: 812, SourceID: "work"}, source.Env{DataDir: shortDir(t)})
	if !errors.Is(err, source.ErrCannotDownload) {
		t.Errorf("err = %v; want ErrCannotDownload", err)
	}
}

func TestPruneDeletesOldTemporaryFiles(t *testing.T) {
	h := newHarness(t, cfgWith("work"))
	dir := h.paths.TmpDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	old, fresh := filepath.Join(dir, "download-old"), filepath.Join(dir, "download-new")
	for _, p := range []string{old, fresh} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	at := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(old, at, at); err != nil {
		t.Fatal(err)
	}
	if err := h.e.Prune(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("old file: %v; want deleted", err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Errorf("new file: %v; want kept", err)
	}
}
