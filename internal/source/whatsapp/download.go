package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"os"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

// MaxDownload is the largest file that i2a downloads.
const MaxDownload = 100 << 20

// File is a downloaded file at a temporary path.
type File struct {
	Path string // the temporary file, mode 0600
	Name string // the file name that the sender gave; "" when none
	MIME string
	Kind string // image, video, document, audio, voice or sticker
	Size int64
}

// ErrNoFile is returned for a message without a file.
var ErrNoFile = errors.New("the message has no file")

// ErrGone is returned when i2a or WhatsApp no longer has the file.
var ErrGone = errors.New("the file is no longer available: open it on the phone")

// TooLargeError is returned for a file larger than MaxDownload.
type TooLargeError struct{ Size, Max uint64 }

func (e *TooLargeError) Error() string {
	return fmt.Sprintf("the file has %s; the maximum is %s", mb(e.Size), mb(e.Max))
}

func mb(n uint64) string {
	return fmt.Sprintf("%.0f MB", float64(n)/(1<<20))
}

// download writes the file of the stored message to a new file in dir.
func download(ctx context.Context, c client, db *DB, chat, id, dir string) (File, error) {
	row, ok, err := db.Row(ctx, chat, id)
	if err != nil {
		return File{}, err
	}
	if !ok {
		return File{}, ErrGone
	}
	msg := &waE2E.Message{}
	if err := proto.Unmarshal(row.Raw, msg); err != nil {
		return File{}, fmt.Errorf("read the stored message: %w", err)
	}
	media, ok := MediaOf(msg)
	if !ok {
		return File{}, ErrNoFile
	}
	if media.Length > MaxDownload {
		return File{}, &TooLargeError{Size: media.Length, Max: MaxDownload}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return File{}, err
	}
	f, err := os.CreateTemp(dir, "download-*")
	if err != nil {
		return File{}, err
	}
	keep := false
	defer func() {
		f.Close()
		if !keep {
			os.Remove(f.Name())
		}
	}()
	if err := f.Chmod(0o600); err != nil {
		return File{}, err
	}
	if err := c.DownloadToFile(ctx, media.Part, f); err != nil {
		var he whatsmeow.DownloadHTTPError
		if errors.As(err, &he) && (he.StatusCode == 404 || he.StatusCode == 410) {
			return File{}, fmt.Errorf("%w (WhatsApp answered %d)", ErrGone, he.StatusCode)
		}
		return File{}, fmt.Errorf("download the file: %w", err)
	}
	info, err := f.Stat()
	if err != nil {
		return File{}, err
	}
	if err := f.Close(); err != nil {
		return File{}, err
	}
	keep = true
	return File{Path: f.Name(), Name: media.Name, MIME: media.MIME, Kind: media.Kind, Size: info.Size()}, nil
}

// Download writes the file of the stored message chat/id to a new file in
// dir and returns it. It connects when necessary.
func (src *Source) Download(ctx context.Context, chat, id, dir string) (File, error) {
	src.mu.Lock()
	defer src.mu.Unlock()
	conn, err := src.connectLocked(ctx)
	if err != nil {
		return File{}, err
	}
	if !conn.cli.WaitForConnection(connectWait) {
		return File{}, whatsmeow.ErrNotConnected
	}
	return download(ctx, conn.cli, conn.db, chat, id, dir)
}
