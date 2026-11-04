// Package download puts a downloaded file in the folder of the user. The file
// name comes from the sender, so it is untrusted: the package keeps only a
// base name, never overwrites a file, and marks the file as downloaded from
// the internet. i2a does not record or delete the file after that.
package download

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"golang.org/x/sys/unix"

	"github.com/sondt2709/inbox2agent/internal/render"
)

// maxName limits the bytes of a file name; macOS allows 255.
const maxName = 200

// QuarantineAttr is the extended attribute that macOS reads before it opens
// a downloaded file.
const QuarantineAttr = "com.apple.quarantine"

// extensions are the extensions of the common WhatsApp MIME types. The mime
// package gives several extensions for some types (image/jpeg).
var extensions = map[string]string{
	"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp", "image/gif": ".gif",
	"video/mp4": ".mp4", "video/3gpp": ".3gp", "audio/ogg": ".ogg", "audio/mpeg": ".mp3", "audio/mp4": ".m4a",
	"audio/aac": ".aac", "application/pdf": ".pdf",
}

// Name returns a safe file name: the base name that the sender gave, without
// hidden characters, path separators, ':' and dots at the start. With no
// usable name, it is "whatsapp-<message id>" and the extension of the MIME type.
func Name(given, mimeType string, messageID int64) string {
	name, _ := render.Sanitize(given, 0)
	name = strings.Map(func(r rune) rune {
		switch {
		case r == '/' || r == '\\' || r == ':':
			return -1
		case r < 0x20 || r == 0x7f:
			return -1
		}
		return r
	}, name)
	name = strings.TrimLeft(strings.TrimSpace(name), ". ")
	if name == "" {
		return "whatsapp-" + strconv.FormatInt(messageID, 10) + extension(mimeType)
	}
	if len(name) > maxName {
		ext := filepath.Ext(name)
		if len(ext) > 20 {
			ext = ""
		}
		stem := name[:maxName-len(ext)]
		for !utf8.ValidString(stem) {
			stem = stem[:len(stem)-1]
		}
		name = stem + ext
	}
	return name
}

func extension(mimeType string) string {
	base, _, err := mime.ParseMediaType(mimeType)
	if err != nil {
		return ""
	}
	if e, ok := extensions[base]; ok {
		return e
	}
	if exts, _ := mime.ExtensionsByType(base); len(exts) > 0 {
		return exts[0]
	}
	return ""
}

// candidate returns the name with " (n)" before the extension.
func candidate(name string, n int) string {
	if n == 0 {
		return name
	}
	ext := filepath.Ext(name)
	if ext == name {
		ext = ""
	}
	return fmt.Sprintf("%s (%d)%s", strings.TrimSuffix(name, ext), n, ext)
}

// ErrDenied is returned when macOS does not let this process write to the folder.
var ErrDenied = errors.New("macOS does not let i2a write to this folder: give the terminal access to it in System Settings > Privacy & Security > Files and Folders, or use --dir")

// Move moves the temporary file tmp to dir with the name, or with "name (1)",
// "name (2)" and so on when the name exists. It never overwrites a file. It
// returns the path of the file.
func Move(tmp, dir, name string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", denied(err)
	}
	for n := 0; n < 1000; n++ {
		dst := filepath.Join(dir, candidate(name, n))
		err := os.Link(tmp, dst)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if errors.Is(err, syscall.EXDEV) {
			err = copyNew(tmp, dst)
			if errors.Is(err, os.ErrExist) {
				continue
			}
		}
		if err != nil {
			return "", denied(err)
		}
		_ = os.Remove(tmp)
		return dst, nil
	}
	return "", fmt.Errorf("%s: 1000 files with this name exist", filepath.Join(dir, name))
}

func denied(err error) error {
	if errors.Is(err, os.ErrPermission) || errors.Is(err, syscall.EPERM) {
		return fmt.Errorf("%w (%v)", ErrDenied, err)
	}
	return err
}

// copyNew copies src to a new file dst with the mode 0600.
func copyNew(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	return out.Close()
}

// Quarantine marks the file as downloaded from the internet by i2a, so that
// macOS asks before it opens a program from the file.
func Quarantine(path string, now time.Time) error {
	v := fmt.Sprintf("0081;%x;i2a;", now.Unix())
	return unix.Setxattr(path, QuarantineAttr, []byte(v), 0)
}
