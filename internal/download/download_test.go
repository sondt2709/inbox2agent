package download

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestName(t *testing.T) {
	long := strings.Repeat("ả", 150) + ".pdf"
	for _, tc := range []struct {
		given, mime string
		want        string
	}{
		{"plan.pdf", "application/pdf", "plan.pdf"},
		{"../../.zshrc", "text/plain", "zshrc"},
		{`C:\Users\x\run.log`, "text/plain", "CUsersxrun.log"},
		{"report\u202Efdp.exe", "application/pdf", "reportfdp.exe"}, // the bidi override is removed
		{"", "image/jpeg", "whatsapp-1240.jpg"},
		{"...", "audio/ogg; codecs=opus", "whatsapp-1240.ogg"},
		{"", "application/x-unknown-type", "whatsapp-1240"},
		{"  Note for APK.docx ", "", "Note for APK.docx"},
	} {
		if got := Name(tc.given, tc.mime, 1240); got != tc.want {
			t.Errorf("Name(%q, %q) = %q; want %q", tc.given, tc.mime, got, tc.want)
		}
	}
	got := Name(long, "application/pdf", 1)
	if len(got) > maxName || !strings.HasSuffix(got, ".pdf") || !strings.HasPrefix(got, "ả") {
		t.Errorf("long name = %q (%d bytes); want at most %d bytes with .pdf", got, len(got), maxName)
	}
}

func tmpFile(t *testing.T, dir, content string) string {
	t.Helper()
	f, err := os.CreateTemp(dir, "download-*")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return f.Name()
}

func TestMoveNeverOverwrites(t *testing.T) {
	root := t.TempDir()
	tmpDir, dir := filepath.Join(root, "tmp"), filepath.Join(root, "Downloads")
	if err := os.MkdirAll(tmpDir, 0o700); err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, c := range []string{"one", "two", "three"} {
		p, err := Move(tmpFile(t, tmpDir, c), dir, "plan.pdf")
		if err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	want := []string{"plan.pdf", "plan (1).pdf", "plan (2).pdf"}
	for i, c := range []string{"one", "two", "three"} {
		b, err := os.ReadFile(paths[i])
		if filepath.Base(paths[i]) != want[i] || string(b) != c || err != nil {
			t.Errorf("file %d = %s %q %v; want %s %q", i, paths[i], b, err, want[i], c)
		}
	}
	if left, _ := os.ReadDir(tmpDir); len(left) != 0 {
		t.Errorf("temporary files left: %d", len(left))
	}
	p, err := Move(tmpFile(t, tmpDir, "x"), dir, "README")
	if err != nil || filepath.Base(p) != "README" {
		t.Fatalf("README: %s %v", p, err)
	}
	if p, _ = Move(tmpFile(t, tmpDir, "y"), dir, "README"); filepath.Base(p) != "README (1)" {
		t.Errorf("second README = %s", p)
	}
}

func TestMoveDenied(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "locked")
	if err := os.Mkdir(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	_, err := Move(tmpFile(t, root, "x"), dir, "plan.pdf")
	if !errors.Is(err, ErrDenied) || !strings.Contains(err.Error(), "--dir") {
		t.Errorf("err = %v; want ErrDenied with the --dir hint", err)
	}
}

func TestCopyNewDoesNotOverwrite(t *testing.T) {
	root := t.TempDir()
	src := tmpFile(t, root, "new")
	dst := filepath.Join(root, "plan.pdf")
	if err := os.WriteFile(dst, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := copyNew(src, dst); !errors.Is(err, os.ErrExist) {
		t.Errorf("err = %v; want ErrExist", err)
	}
	if b, _ := os.ReadFile(dst); string(b) != "old" {
		t.Errorf("dst = %q; want old", b)
	}
	if err := copyNew(src, filepath.Join(root, "copy.pdf")); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(filepath.Join(root, "copy.pdf"))
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v; want 0600", info.Mode())
	}
}

func TestQuarantine(t *testing.T) {
	p := tmpFile(t, t.TempDir(), "x")
	if err := Quarantine(p, time.Unix(0x6720_0000, 0)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 64)
	n, err := unix.Getxattr(p, QuarantineAttr, buf)
	if err != nil {
		t.Fatalf("no attribute: %v", err)
	}
	if string(buf[:n]) != "0081;67200000;i2a;" {
		t.Errorf("attribute = %q", buf[:n])
	}
}
