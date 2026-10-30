package whatsapp

import (
	"strings"
	"testing"

	"rsc.io/qr"
)

// TestQRTextDecodesToTheCode reads the half blocks back into modules and
// compares them with the QR code.
func TestQRTextDecodesToTheCode(t *testing.T) {
	const text = "2@fake-ref,fake-noise-key,fake-identity-key,fake-adv-secret"
	out, err := QRText(text)
	if err != nil {
		t.Fatal(err)
	}
	code, err := qr.Encode(text, qr.L)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(out, "\n")
	size := code.Size + 2*quiet
	if len(lines) != (size+1)/2 {
		t.Fatalf("lines = %d, want %d", len(lines), (size+1)/2)
	}
	for i, line := range lines {
		if !strings.HasPrefix(line, "\x1b[30;47m") || !strings.HasSuffix(line, "\x1b[0m") {
			t.Fatalf("line %d is not black on white: %q", i, line)
		}
		cells := []rune(strings.TrimSuffix(strings.TrimPrefix(line, "\x1b[30;47m"), "\x1b[0m"))
		if len(cells) != size {
			t.Fatalf("line %d has %d cells, want %d", i, len(cells), size)
		}
		for x, c := range cells {
			top, bottom := c == '█' || c == '▀', c == '█' || c == '▄'
			mx, my := x-quiet, 2*i-quiet
			if top != code.Black(mx, my) || bottom != code.Black(mx, my+1) {
				t.Fatalf("module (%d,%d) is wrong: %q", mx, my, c)
			}
		}
	}
}
